// What the main conversation still needs (docs/assistant/onboarding.md 4.4, 5.1; docs/protocol/protocol.md "Main chat readiness"): the hub works
// it out (GET /api/v1/assistant/readiness) and every client draws the same thing from it: in `setup` a card above the
// composer with the first missing step, the send button saying what to do first, and example questions that open local
// help. A coded refusal of a start or a send (5.2) becomes a card too, not a toast. On a hub without the feature
// (`assistant_readiness`) nothing here shows and the page behaves as before.
import { useEffect, useSyncExternalStore } from "react";
import { t } from "../i18n";
import { api, ApiError, errorText, isAbort, pastHub } from "./api";
import { navigate } from "./router";
import { getState, hubHas, useStore } from "./store";

export const READINESS_FEATURE = "assistant_readiness";
export const READINESS_PATH = "/assistant/readiness";

export type ReadinessMode = "full" | "setup" | "chat_only";
export type StepState = "ok" | "todo" | "problem" | "checking" | "unknown" | "skipped";
export type StepId = "computer" | "claude" | "model" | "first_message";

export interface ReadinessAction {
  id: string;
}

export interface ReadinessStep {
  id: StepId | string;
  state: StepState | string;
  code?: string;
  machine?: string;
  launcher?: string;
  version?: string;
  title?: string;
  detail?: string;
  action?: ReadinessAction;
  alt_actions?: ReadinessAction[];
}

export interface ReadinessSuggestion {
  id: string;
  title: string;
  prompt?: string;
  local?: boolean;
  needs?: string[];
}

export interface Readiness {
  version: number;
  mode: ReadinessMode | string;
  ready: boolean;
  next: string | null;
  machine: { id: string; name: string } | null;
  launcher: string;
  steps: ReadinessStep[];
  suggestions?: ReadinessSuggestion[];
  probe?: { ok: boolean; agent?: string; model?: string; answer?: string; millis?: number; error?: string };
  dismissed: boolean;
  checked_at: string;
}

// ── the steps ───────────────────────────────────────────

/** The steps that decide the mode, in order; "Try it" never blocks (5.1). */
const BLOCKING: string[] = ["computer", "claude", "model"];

/**
 * The step the card shows: in `setup`, the first of computer / claude / model that is todo or a problem. A rate limit
 * passes by itself and never turns the send button (the hub keeps `full` for it too); a step in a state this page does
 * not know counts as "One more step" with its title (5, general rules).
 */
export function blockingStep(r: Readiness | null | undefined): ReadinessStep | null {
  if (!r || r.mode !== "setup") return null;
  for (const id of BLOCKING) {
    const s = r.steps.find((x) => x.id === id);
    if (!s || s.state === "ok" || s.state === "skipped" || s.state === "unknown" || s.state === "checking") continue;
    if (s.code === "rate_limited") continue;
    return s;
  }
  // `setup` with no step this page reads as missing (a newer hub's step): the one `next` names
  if (!r.next || [...BLOCKING, "first_message"].includes(r.next)) return null;
  const next = r.steps.find((x) => x.id === r.next);
  return next && next.state !== "ok" && next.state !== "skipped" ? next : null;
}

/** What the send button says while the main conversation cannot answer (4.4: "Connect a model first" / "Add a computer first"). */
export function sendLabel(step: ReadinessStep): string {
  if (step.id === "computer") {
    if (step.code === "node_unreachable") return t("lib-readiness.send_wake_computer");
    if (step.code === "machine_pending") return t("lib-readiness.send_confirm_computer");
    if (step.code === "node_outdated") return t("lib-readiness.send_update_computer");
    return t("lib-readiness.send_add_computer");
  }
  return t("lib-readiness.send_connect_model");
}

// ── actions ─────────────────────────────────────────────

/** The help a button or a local example question opens in place (no page of its own on the web yet). */
export type HelpTopic = "add_computer" | "wake_computer" | "install_claude" | "login_claude" | "what_can_you_do" | "how_to_connect_model";

const HELP_TOPICS: string[] = ["add_computer", "wake_computer", "install_claude", "login_claude", "what_can_you_do", "how_to_connect_model"];

/** The help a local example question opens: its own when this page has it, else what Persona does (never sent). */
export function helpTopic(id: string): HelpTopic {
  return (HELP_TOPICS.includes(id) ? id : "what_can_you_do") as HelpTopic;
}

/** The action ids a card can draw a button for (5.1); an id this page does not know is text only (5, general rules). */
const ACTIONS = new Set(["add_computer", "confirm_computer", "wake_computer", "install_claude", "login_claude", "connect_model", "edit_provider", "test_assistant", "enable_tls", "open_admin"]);

export function knownAction(id: string | undefined): boolean {
  return Boolean(id && ACTIONS.has(id));
}

export function actionLabel(id: string): string {
  switch (id) {
    case "add_computer":
      return t("lib-readiness.action_add_computer");
    case "confirm_computer":
      return t("lib-readiness.action_confirm_computer");
    case "wake_computer":
      return t("lib-readiness.action_wake_computer");
    case "install_claude":
      return t("lib-readiness.action_install_claude");
    case "login_claude":
      return t("lib-readiness.action_login_claude");
    case "connect_model":
      return t("lib-readiness.action_connect_model");
    case "edit_provider":
      return t("lib-readiness.action_edit_provider");
    case "test_assistant":
      return t("lib-readiness.action_test_assistant");
    case "enable_tls":
    case "open_admin":
      return t("lib-readiness.action_open_admin");
  }
  return id;
}

/** The hub's own admin page (served by the hub at /admin, its own sign-in): where adding a computer is on a hub where a
 *  signed-in device cannot add one itself (no `machines_add`, 5.3). */
export const ADMIN_MACHINES = "/admin/machines";

/** Settings › Model providers with the wizard open (`add=1`), or one provider's details (`launcher=`). */
export function providersPath(launcher?: string): string {
  return launcher ? `/settings?tab=models&launcher=${encodeURIComponent(launcher)}` : "/settings?tab=models&add=1";
}

/**
 * What a button does: a help topic to open in place (add_computer too: the page that owns `onHelp` opens the add flow
 * instead where the hub has it), or done here (a page opened, a check run). `launcher` is the step's, for "Change the key";
 * `onConfirm` opens the confirm screen of a computer waiting to be confirmed (without it: the machines page).
 */
export function runAction(id: string, o: { launcher?: string; onHelp: (topic: HelpTopic) => void; onConfirm?: () => void }): void {
  switch (id) {
    case "add_computer":
    case "wake_computer":
    case "install_claude":
    case "login_claude":
      return o.onHelp(id);
    case "confirm_computer":
      return o.onConfirm ? o.onConfirm() : navigate("/machines");
    case "connect_model":
      return navigate(providersPath());
    case "edit_provider":
      return navigate(o.launcher ? providersPath(o.launcher) : "/settings?tab=models");
    case "test_assistant":
      void refreshReadiness({ probe: true });
      return;
    case "enable_tls":
    case "open_admin":
      window.open("/admin", "_blank", "noopener,noreferrer");
      return;
  }
}

// ── coded failures (5.2) ────────────────────────────────

/** A start or a send the hub refused with a code: what the card shows. */
export interface CodedFailure {
  code: string;
  /** The hub's words, in the page's language. */
  error: string;
  /** The original text (a shell's error…): only under "Details". */
  detail?: string;
  action?: string;
  machine?: string;
  launcher?: string;
  requestId?: string;
}

/** The codes a failure card is drawn for; anything else is the hub's words with no button. */
const CODED = new Set(["no_machine", "machine_pending", "claude_missing", "launcher_missing", "node_unreachable", "assistant_offline", "node_outdated"]);

/** The card for a failed start or send: every failure is one (4.4), with the hub's words, and a button for its code. */
export function codedFailure(e: unknown): CodedFailure {
  if (!(e instanceof ApiError)) return { code: "", error: errorText(e) };
  const b = e.body;
  const str = (k: string) => (typeof b[k] === "string" && b[k] ? (b[k] as string) : undefined);
  const code = e.code && CODED.has(e.code) ? e.code : "";
  let action = str("action");
  if (!action && code === "node_unreachable") action = "wake_computer";
  if (!action && code === "no_machine") action = "add_computer";
  return { code, error: e.message, detail: str("detail"), action, machine: str("machine_name"), launcher: str("launcher"), requestId: str("request_id") ?? e.requestId };
}

/** The assistant restarting answers 503 assistant_offline for a few seconds: tried again quietly (4.6, principle 4). */
export function passesBySelf(e: unknown): boolean {
  return e instanceof ApiError && e.code === "assistant_offline";
}

// ── turn errors (turn_error_code) ───────────────────────

/** A turn that ended on a model error the user must fix: the card's title and its button (4.4 "Model error"). */
export function turnErrorFix(code: string | undefined): { title: string; detail: string; action: string } | null {
  switch (code) {
    case "not_logged_in":
      return { title: t("lib-readiness.turn_not_logged_in"), detail: t("lib-readiness.turn_not_logged_in_detail"), action: "connect_model" };
    case "auth_failed":
      return { title: t("lib-readiness.turn_auth_failed"), detail: t("lib-readiness.turn_auth_failed_detail"), action: "edit_provider" };
    case "model_route":
      return { title: t("lib-readiness.turn_model_route"), detail: t("lib-readiness.turn_model_route_detail"), action: "connect_model" };
  }
  return null;
}

// ── the store ───────────────────────────────────────────

interface ReadinessState {
  readiness: Readiness | null;
  /** The hub answered 404: an older hub, the page as before. */
  unsupported: boolean;
  probing: boolean;
  /** "Try it" got no answer (the hub took too long, the network dropped, an error): what to say instead of a result; "" none. */
  probeError: string;
  at: number;
  /** A request has finished (answered or failed): before it, "no readiness" means "not known yet". */
  tried: boolean;
}

let rs: ReadinessState = { readiness: null, unsupported: false, probing: false, probeError: "", at: 0, tried: false };
const listeners = new Set<() => void>();
let inflight: Promise<void> | null = null;
// Every request's number: an answer lands only if no later request has answered first (a forced ask after a change
// must not be overwritten by one sent before it).
let asked = 0;
let answered = 0;

function set(patch: Partial<ReadinessState>) {
  rs = { ...rs, ...patch };
  listeners.forEach((l) => l());
}

/** How long a fetched answer is good for before a visit asks again. */
const FRESH_MS = 5_000;

/** The hub's own limit for "Try it": the one-word check (healthCheckTimeout, 100 s) and then the node's self-check
 *  (readinessWaitGet, 12 s). The read waits past it (`pastHub`), so a slow check still ends in its result (#1892). */
export const PROBE_HUB_MS = 112_000;

/**
 * Asks the hub again (a visit, a machine coming or going, after a failure, "Try it"'s probe). `force` asks even with a
 * fresh answer; `changed` also asks with an answer in flight: that one may have been worked out before the change made
 * here. The rest share the one in flight (each can hold a node self-check for seconds).
 */
export function refreshReadiness(o: { force?: boolean; probe?: boolean; changed?: boolean } = {}): Promise<void> {
  if (!o.probe && !o.force && !o.changed && Date.now() - rs.at < FRESH_MS) return Promise.resolve();
  if (inflight && !o.probe && !o.changed) return inflight;
  if (o.probe) set({ probing: true, probeError: "" });
  const mine = ++asked;
  // a probe always lands: it is the only answer that carries "Try it"'s result, however long it took
  const lands = () => (o.probe || mine > answered) && ((answered = Math.max(answered, mine)), true);
  const p = (o.probe ? api<Readiness>(`${READINESS_PATH}?probe=1`, { timeout: pastHub(PROBE_HUB_MS) }) : api<Readiness>(READINESS_PATH))
    .then((r) => {
      // a newer answer replaces a failed try's words, as it replaces a try's result
      if (lands()) set({ readiness: r, unsupported: false, probeError: "", at: Date.now() });
    })
    .catch((e) => {
      // 404: an older hub; 503 assistant_offline and the rest: keep what was there, ask again next time
      if (e instanceof ApiError && e.status === 404 && lands()) set({ unsupported: true, readiness: null, at: Date.now() });
      // "Try it" ends in words either way: a try that got no answer says why, and an earlier try's result is not shown for it
      else if (o.probe && !isAbort(e)) set({ probeError: errorText(e) || t("lib-api.hub_no_response") });
    })
    .finally(() => {
      if (o.probe) set({ probing: false });
      if (!rs.tried) set({ tried: true });
      if (inflight === p) inflight = null;
    });
  if (!o.probe) inflight = p;
  return p;
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

/**
 * Something the readiness depends on changed here (a computer added, confirmed or removed, a provider saved or given to
 * the main conversation): ask again now so the card follows without a reload (5.1 "When the client refetches"). Nothing on a
 * hub without readiness.
 */
export function readinessChanged(): Promise<void> {
  if (!hubHas(getState().features, READINESS_FEATURE)) return Promise.resolve();
  return refreshReadiness({ changed: true });
}

/** The first answer is still on its way (a hub with readiness that has not answered or failed yet). */
export function readinessPending(): boolean {
  return hubHas(getState().features, READINESS_FEATURE) && !rs.unsupported && !rs.readiness && !rs.tried;
}

/** Waits for the request in flight, if any (asks once if none is), so a decision can read the answer. */
export function readinessSettled(): Promise<void> {
  if (!readinessPending()) return Promise.resolve();
  return inflight ?? refreshReadiness({ force: true });
}

/** The readiness this page has now (null before the first answer or on a hub without it). */
export function currentReadiness(): Readiness | null {
  return rs.unsupported ? null : rs.readiness;
}

/** The main conversation already runs: its computer, Claude Code and model all check out (4.3 B.3). */
export function claudeWorks(r: Readiness | null | undefined): boolean {
  if (!r) return false;
  const ok = (id: string) => r.steps.find((s) => s.id === id)?.state === "ok";
  return ok("computer") && ok("claude") && ok("model");
}

/** The main conversation's computer has no Claude Code (4.3 B.4): a key alone does not make it answer. */
export function claudeMissing(r: Readiness | null | undefined): boolean {
  return !!r?.steps.some((s) => s.id === "claude" && s.code === "claude_missing" && (s.state === "problem" || s.state === "todo"));
}

/** For tests: forget everything. */
export function resetReadiness(r: Readiness | null = null) {
  rs = { readiness: r, unsupported: false, probing: false, probeError: "", at: r ? Date.now() : 0, tried: !!r };
  inflight = null;
  listeners.forEach((l) => l());
}

/**
 * The main conversation's readiness, or null on a hub without it (and until the first answer). Asked on a visit, when
 * the machines change (one added or confirmed, one going offline or coming back) and when the window comes back into view.
 */
export function useReadiness(enabled = true): { readiness: Readiness | null; probing: boolean; probeError: string; pending: boolean } {
  // a page that never shows the card (an agent session, a side panel) does not ask
  const supported = useStore((s) => hubHas(s.features, READINESS_FEATURE)) && enabled;
  // a computer confirmed (trust pending → omitted) changes the computer step as much as one coming online
  const machines = useStore((s) => s.machines.map((m) => `${m.id}:${m.online ? 1 : 0}:${m.trust ?? ""}`).join(","));
  const s = useSyncExternalStore(subscribe, () => rs, () => rs);
  useEffect(() => {
    if (!supported) return;
    void refreshReadiness({ force: true });
  }, [supported, machines]);
  useEffect(() => {
    if (!supported) return;
    const on = () => document.visibilityState === "visible" && void refreshReadiness();
    document.addEventListener("visibilitychange", on);
    window.addEventListener("focus", on);
    return () => {
      document.removeEventListener("visibilitychange", on);
      window.removeEventListener("focus", on);
    };
  }, [supported]);
  if (!supported || s.unsupported) return { readiness: null, probing: false, probeError: "", pending: false };
  return { readiness: s.readiness, probing: s.probing, probeError: s.probeError, pending: !s.readiness && !s.tried };
}
