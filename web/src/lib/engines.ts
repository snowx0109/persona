// "Engine" on a machine's page (docs/integrations/engines.md, iOS EngineSelectionModel / EngineSelectionView): which terminal engine the
// computer's sessions come from. The hub keeps one per machine (herdr when never chosen) and takes a new one with
// PUT /api/v1/machines/{mid}/engine, also while the machine is offline (applied when it connects).
import { t } from "../i18n";
import type { DetectedEngine, Machine } from "./types";

/** The engines this client knows (the hub's proto.EngineNames), in the order they are listed when the node reports none. */
export const KNOWN_ENGINES: { id: string; name: string; detail: string }[] = [
  { id: "herdr", name: "herdr", detail: t("lib-engines.detail_herdr") },
  { id: "herdr-core", name: "Herdr App", detail: t("lib-engines.detail_herdr_core") },
  { id: "herdr-both", name: "herdr + Herdr App", detail: t("lib-engines.detail_herdr_both") },
  { id: "orca", name: "Orca", detail: t("lib-engines.detail_orca") },
  { id: "tern", name: "Tern", detail: t("lib-engines.detail_tern") },
  { id: "rex", name: "Rex", detail: t("lib-engines.detail_rex") },
  { id: "paseo", name: "Paseo", detail: t("lib-engines.detail_paseo") },
  { id: "cindy", name: "Cindy", detail: t("lib-engines.detail_cindy") },
];

const known = (id: string) => KNOWN_ENGINES.find((e) => e.id === id);

type EngineMachine = Pick<Machine, "engine" | "engines" | "online">;

/** The machine's engine: herdr when it was never chosen (an engine is always picked; nothing is guessed from detection). */
export function currentEngine(m: Pick<Machine, "engine">): string {
  return m.engine || "herdr";
}

/** An engine's name: the node's words first, then ours, then its id (a future engine). */
export function engineName(id: string, m?: Pick<Machine, "engines">): string {
  return m?.engines?.find((e) => e.id === id)?.name || known(id)?.name || id;
}

/** One line on what the engine is; none for an engine this client does not know. */
export function engineDetail(id: string): string | undefined {
  return known(id)?.detail;
}

/** The choices: what the node reports (names, notes and engines newer than this client), else — an older hub, an
 * offline machine — every known engine, which can still be configured. */
export function engineOptions(m: Pick<Machine, "engines">): DetectedEngine[] {
  if (m.engines?.length) return m.engines;
  return KNOWN_ENGINES.map((e) => ({ id: e.id, name: e.name, available: false }));
}

/** A known engine can always be chosen (offline too, or not detected yet: it applies once there); one this client does
 * not know only while the node reports it available. */
export function canSelectEngine(id: string, m: Pick<Machine, "engines">): boolean {
  return !!known(id) || !!m.engines?.some((e) => e.id === id && e.available);
}

/** What the node found of the engine: "Available" (with its version) / "Not detected"; nothing to say while offline. */
export function detectionText(id: string, m: EngineMachine): string {
  if (!m.online) return t("lib-engines.detection_offline");
  const found = m.engines?.find((e) => e.id === id);
  if (!found) return t("lib-engines.detection_none");
  if (!found.available) return t("lib-engines.detection_not_found");
  return found.version ? t("lib-engines.detection_available_version", { version: found.version }) : t("lib-engines.detection_available");
}

/** The line under the list: what the choice decides, and when it applies. */
export function engineFooter(selected: string, online: boolean): string {
  if (selected === "herdr-both")
    return online ? t("lib-engines.footer_both") : t("lib-engines.footer_both_offline");
  return online ? t("lib-engines.footer_single") : t("lib-engines.footer_one_offline");
}

/** Whether "Save" is offered: a change the hub can take. */
export function canSaveEngine(selected: string, m: Pick<Machine, "engine" | "engines">): boolean {
  return selected !== currentEngine(m) && canSelectEngine(selected, m);
}

export function engineSaveError(message: string): string {
  return t("lib-engines.save_failed", { message });
}

// ---- Host sets (#2576, docs/integrations/engines.md "Host sets"): several engines enabled side by side plus a default one. ----

/** The hosts a set may hold, in the hub's canonical order (proto.EngineHostIDs); herdr-both is a set, not a member. */
export const ENGINE_HOSTS = ["herdr", "herdr-core", "orca", "tern", "rex", "paseo", "cindy"];

export interface HostChoice {
  hosts: string[];
  default: string;
}

/** The set a single engine stands for (proto.EngineHostsOf). */
export function hostsOf(engine: string | undefined): HostChoice {
  if (engine === "herdr-both") return { hosts: ["herdr", "herdr-core"], default: "herdr" };
  if (engine && ENGINE_HOSTS.includes(engine)) return { hosts: [engine], default: engine };
  return { hosts: ["herdr"], default: "herdr" };
}

/** The hub takes sets: it reports engine_hosts (older hubs keep the single-engine picker). */
export function supportsHostSets(m: Pick<Machine, "engine_hosts">): boolean {
  return Array.isArray(m.engine_hosts);
}

/** The machine's set as the hub says, else the one its single engine stands for. */
export function currentHosts(m: Pick<Machine, "engine" | "engine_hosts" | "engine_default">): HostChoice {
  if (Array.isArray(m.engine_hosts)) return { hosts: m.engine_hosts, default: m.engine_default ?? "" };
  return hostsOf(m.engine);
}

/** A set in canonical order; the default stays when still enabled, else the first host ("" for none). */
export function normalizeHosts(hosts: string[], def: string): HostChoice {
  const out = ENGINE_HOSTS.filter((h) => hosts.includes(h));
  return { hosts: out, default: out.includes(def) ? def : (out[0] ?? "") };
}

/** Turns host id on or off in the set. */
export function toggleHost(c: HostChoice, id: string): HostChoice {
  const on = c.hosts.includes(id);
  return normalizeHosts(on ? c.hosts.filter((h) => h !== id) : [...c.hosts, id], c.default);
}

export function sameHosts(a: HostChoice, b: HostChoice): boolean {
  return a.default === b.default && a.hosts.length === b.hosts.length && a.hosts.every((h, i) => h === b.hosts[i]);
}

/** The host options: what the node reports of the set's members, else every one of them (offline, older nodes). */
export function hostOptions(m: Pick<Machine, "engines">): DetectedEngine[] {
  const found = (m.engines ?? []).filter((e) => ENGINE_HOSTS.includes(e.id));
  if (found.length) return ENGINE_HOSTS.map((id) => found.find((e) => e.id === id)).filter((e): e is DetectedEngine => !!e);
  return ENGINE_HOSTS.map((id) => ({ id, name: known(id)?.name ?? id, available: false }));
}

/** The enabled hosts a new session may be opened in (every host can start one, the capability table in
 * docs/integrations/engines.md): offered as a choice only when there is more than one. */
export function startHosts(m: Pick<Machine, "engine" | "engine_hosts" | "engine_default">): string[] {
  return currentHosts(m).hosts;
}

/** The line under the set picker. */
/** A set by its engines' names (herdr + Orca); "no terminal engine" for none. */
export function hostsName(c: HostChoice, m?: Pick<Machine, "engines">): string {
  return c.hosts.length ? c.hosts.map((id) => engineName(id, m)).join(" + ") : t("lib-engines.none");
}

export function hostsFooter(c: HostChoice, online: boolean): string {
  const key = c.hosts.length === 0 ? "footer_set_empty" : c.hosts.length === 1 ? "footer_set_single" : "footer_set_several";
  const text = t(`lib-engines.${key}`, { engine: engineName(c.default) });
  return online ? text : t("lib-engines.footer_set_offline", { text });
}

/** "Also open in X": the other hosts a session is live in, by name; "" when none. */
export function alsoOpenText(s: { also_open_in?: string[] }, m?: Pick<Machine, "engines">): string {
  const names = (s.also_open_in ?? []).map((id) => engineName(id, m));
  return names.length ? t("lib-engines.also_open", { engines: names.join(t("lib-engines.list_separator")) }) : "";
}
