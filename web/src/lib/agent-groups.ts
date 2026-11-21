import { t } from "../i18n";
import { backgroundWaiting, byAttention, stateOf } from "./format";
import { hubWaits } from "./work";
import type { AgentState, Machine, Session } from "./types";
import { byTime } from "./times";
import { stateName } from "./state-labels";

export type AgentGrouping = "repo" | "space" | "state" | "machine" | "todo";
// "By to-do" leads and is the default for a browser that never picked one (#2812); a grouping picked before stays. The
// sidebar shows them in this order as segments, the paged list and the complete list alike (#6662), and the
// switch-grouping shortcut goes round them in it. "By machine" is herdr's own layout (machine → workspace → tab,
// lib/herdr-tree.ts). `short` is the segment's label, `label` the full name. "By pinned" (#4352) is gone (#6662): the
// sidebar's 常驻 rows unfold to the same tree, so a picked one reads as By to-do.
export const AGENT_GROUPINGS: { key: AgentGrouping; label: string; short: string }[] = [
  { key: "todo", label: t("lib-agent-groups.by_todo"), short: t("lib-agent-groups.seg_todo") },
  { key: "machine", label: t("lib-agent-groups.by_machine"), short: t("lib-agent-groups.seg_machine") },
  { key: "repo", label: t("lib-agent-groups.by_repo"), short: t("lib-agent-groups.seg_repo") },
  { key: "space", label: t("lib-agent-groups.by_project"), short: t("lib-agent-groups.seg_project") },
  { key: "state", label: t("lib-agent-groups.by_state"), short: t("lib-agent-groups.seg_state") },
];

// the grouping this browser chose last (persona.agentGrouping, #1836); by todo until it chooses (#2812, was by machine, #1990)
export function agentGrouping(value: string | null): AgentGrouping {
  return AGENT_GROUPINGS.find((g) => g.key === value)?.key ?? "todo";
}

// the host kinds a pane ref may be prefixed with (keep in step with Kinds in server/internal/host/host.go); herdr's ids go bare and may hold a colon
const HOST_KINDS = ["orca", "herdr-core", "tern", "rex", "botmux"];

/** The host tool a pane belongs to, from its ref (`botmux:…`, `orca:…`, `tern:…`; anything else is herdr's); "" for none. */
export function paneHost(pane: string | undefined): string {
  if (!pane) return "";
  const kind = pane.slice(0, pane.indexOf(":"));
  return HOST_KINDS.includes(kind) ? kind : "herdr";
}

/** The host tool a session lives in, from its pane ref; "" for none. */
export function sessionHost(s: Session): string {
  return paneHost(s.pane_id);
}

/** Where an agent was opened (#2526): "feishu" for a botmux session (a Feishu group's topic), "terminal" for a pane of a
 *  terminal engine (herdr, the Herdr App's, Orca's, Tern's and Rex's terminals), "desktop" for a desktop app's own chat (a
 *  Codex App thread, an Orca native chat, a Paseo agent); undefined for the rest (other runtimes, history no host
 *  lists), which carry no mark. */
export type SessionSource = "feishu" | "terminal" | "desktop";

export function sessionSource(s: Session): SessionSource | undefined {
  if (s.space_kind === "codex_app" || s.host === "paseo" || s.host === "cindy" || (s.source_kind === "runtime" && s.source_instance_id === "orca")) return "desktop";
  const host = sessionHost(s);
  if (host === "botmux") return "feishu";
  return host ? "terminal" : undefined;
}

/** A source's name: its mark's tooltip and what a screen reader says for it. */
export function sourceName(source: SessionSource): string {
  return t(`lib-agent-groups.source_${source}`);
}

/** The hosts a list's sessions run in, so a row names its host only when there is more than one. */
export function hostsOf(sessions: Session[]): Set<string> {
  return new Set(sessions.map(sessionHost).filter(Boolean));
}

export interface AgentMutes {
  spaces: string[];
  sessions: string[];
  allowed?: string[];
}

export interface AgentGroup {
  key: string;
  name: string;
  sessions: Session[];
  state?: AgentState;
  /** Its agents only wait on background commands (#6045): drawn with the waiting mark. */
  waiting?: boolean;
  folded?: boolean;
  todoId?: string;
  todoStatus?: string;
  /** What the group is and why its agents are in it, for a name that does not say it alone (a tooltip). */
  hint?: string;
}

export function splitAgentSessions(all: Session[], machines: Machine[], keepOffline?: (s: Session) => boolean) {
  const online = new Set(machines.filter((m) => m.online).map((m) => m.id));
  const agents = all.filter((s) => !s.assistant);
  const live = agents.filter((s) => (stateOf(s) !== "offline" && online.has(s.machine_id)) || keepOffline?.(s));
  const listed = new Set(live);
  const recent = agents
    .filter((s) => !listed.has(s))
    .sort((a, b) => byTime(b.updated_at, a.updated_at) || a.machine_id.localeCompare(b.machine_id) || a.session_id.localeCompare(b.session_id))
    .slice(0, 40);
  return { live, recent };
}

export interface Space {
  key: string;
  machineId: string;
  name: string;
  sessions: Session[];
}

/** Live agents by herdr space, in herdr's order (machine, workspace, tab); agents outside any space are `loose`. The
 * sidebar keeps that order: rows that jump around as agents change state are hard to click. */
export function buildSpaces(live: Session[], machines: Machine[]): { spaces: Space[]; loose: Session[] } {
  const map = new Map<string, Space>();
  const loose: Session[] = [];
  for (const s of live) {
    if (!s.space_id) {
      loose.push(s);
      continue;
    }
    const key = `${s.machine_id}/${s.space_id}`;
    if (!map.has(key)) map.set(key, { key, machineId: s.machine_id, name: s.space_name || s.space_id, sessions: [] });
    map.get(key)!.sessions.push(s);
  }
  const order = (m: string) => machines.findIndex((x) => x.id === m);
  const bySlot = (a: Session, b: Session) => (a.slot_index ?? 0) - (b.slot_index ?? 0) || a.session_id.localeCompare(b.session_id);
  const spaces = [...map.values()].map((sp) => ({ ...sp, index: sp.sessions[0]?.space_index ?? 0, sessions: sp.sessions.sort(bySlot) }));
  spaces.sort((a, b) => order(a.machineId) - order(b.machineId) || a.index - b.index || a.name.localeCompare(b.name));
  return { spaces: spaces.map(({ index: _index, ...sp }) => sp), loose: loose.sort(byAttention) };
}

export function isWaiting(s: Session): boolean {
  const waiting = typeof s.waiting === "boolean" ? s.waiting
    : Boolean(s.loop || s.background?.length || s.subagents?.some((a) => a.running) || s.workflows?.some((w) => w.running) || hubWaits(s.work));
  return ["idle", "unknown"].includes(stateOf(s)) && waiting;
}

const NO_MUTES: AgentMutes = { spaces: [], sessions: [] };

/** Awaiting Reply (#5096, docs/protocol/agent-states.md): seen, but it stopped on a question to the user nobody has answered yet
 *  (`awaits_reply`), and not muted. Such an agent is not quiet: it never folds into Idle Spaces. */
export function awaitsReply(s: Session, mutes: AgentMutes = NO_MUTES): boolean {
  return stateOf(s) === "idle" && Boolean(s.awaits_reply) && !isMuted(s, mutes);
}

/** Just finished (#9751, docs/protocol/agent-states.md): seen as far as its state goes, but the hub's triage found its turn
 *  done before anybody looked, less than three hours ago (`work.just_finished`), and not muted. Such an agent is not
 *  quiet: it never folds into Idle Spaces. */
export function justFinished(s: Session, mutes: AgentMutes = NO_MUTES): boolean {
  return stateOf(s) === "idle" && Boolean(s.work?.just_finished) && !isMuted(s, mutes);
}

/** What an Awaiting Reply agent asks: what the triage says it waits for (blocked on the user), else the question its last reply
 *  put to the user; "" when neither says. */
export function replyAsk(s: Session): string {
  const w = s.work;
  if (w?.phase === "blocked" && w.on === "user" && w.line?.trim()) return w.line.trim();
  return s.asks?.trim() ?? "";
}

/** Nothing to look at: seen (or unknown), not waiting on the hub or itself, not waiting for the user's reply, and not
 *  just finished. */
export function isQuiet(s: Session, mutes: AgentMutes = NO_MUTES): boolean {
  return ["idle", "unknown"].includes(stateOf(s)) && !isWaiting(s) && !awaitsReply(s, mutes) && !justFinished(s, mutes);
}

function isMuted(s: Session, mutes: AgentMutes): boolean {
  const key = `${s.machine_id}/${s.session_id}`;
  if (mutes.sessions.includes(key)) return true;
  if (mutes.allowed?.includes(key)) return false;
  return Boolean(s.space_name && mutes.spaces.includes(`${s.machine_id}/${s.space_name}`));
}

function byDate(a: Session, b: Session): number {
  const date = (s: Session) => (stateOf(s) === "working" ? s.state_since || s.updated_at : s.updated_at) ?? "";
  return byTime(date(b), date(a)) || a.session_id.localeCompare(b.session_id) || a.machine_id.localeCompare(b.machine_id);
}

// iOS AgentListOrder (#5096): needs you → unread → awaiting reply → working → waiting in the background (#6045) → just
// finished (#9751) → waiting → muted → seen; needs you always wins,
// even when a session or its project is muted. "By status" is the only grouping built here ("By machine" is
// lib/herdr-tree.ts); the argument stays for the callers that name it.
export function buildAgentGroups(all: Session[], machines: Machine[], _grouping: "state", mutes: AgentMutes = { spaces: [], sessions: [] }, compareSessions: (a: Session, b: Session) => number = byDate): AgentGroup[] {
  const { live } = splitAgentSessions(all, machines);
  const groups: AgentGroup[] = [
    { key: "state:needs", name: stateName("needs_you"), state: "needs_you", sessions: [] },
    { key: "state:unread", name: stateName("unread"), state: "unread", sessions: [] },
    { key: "state:reply", name: stateName("awaits_reply"), sessions: [] },
    { key: "state:working", name: stateName("working"), state: "working", sessions: [] },
    { key: "state:bg_waiting", name: stateName("bg_waiting"), state: "working", waiting: true, sessions: [] },
    { key: "state:just_finished", name: stateName("just_finished"), sessions: [] },
    { key: "state:waiting", name: stateName("waiting"), sessions: [] },
    { key: "state:muted", name: t("lib-agent-groups.state_muted"), sessions: [] },
    { key: "state:idle", name: stateName("idle"), state: "idle", sessions: [], folded: true },
  ];
  for (const s of live) {
    const state = stateOf(s);
    const waiting = isWaiting(s);
    const index = state === "needs_you" ? 0 : isMuted(s, mutes) && (state === "working" || state === "unread" || waiting) ? 7
      : state === "unread" ? 1 : awaitsReply(s, mutes) ? 2 : backgroundWaiting(s) ? 4 : state === "working" ? 3 : waiting ? 6
      : justFinished(s, mutes) ? 5 : 8;
    groups[index].sessions.push(s);
  }
  return groups.filter((g) => g.sessions.length).map((g) => ({ ...g, sessions: g.sessions.sort(compareSessions) }));
}
