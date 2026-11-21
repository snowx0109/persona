// The context-menu items of a node in a tree of machines (#2329): a machine, a workspace, a tab or a pane, in the
// sidebar's Agents tree and on the "Terminal" page. One builder for all of them, so a menu offers the same things wherever the
// node is listed (and the command palette can ask it later). What changes a computer goes through routes only a full
// device may call (server/internal/hub/panes.go); copying and folding are local.
import { t } from "../i18n";
import { api, enc, errorText, sessionPath } from "./api";
import type { TermPane, TermSpace, TermTab } from "./terminals";
import type { Launcher, Machine, Session } from "./types";
import { copyText, toast } from "../ui/atoms";
import { openNewAgent } from "./new-agent";
import { navigate } from "./router";
import { openNewWorktree } from "./worktree-request";
import type { MenuItem } from "../ui/menu-model";
import { launcherBlock, usableFirst } from "./launcher-health";
import { providerIdNote, providerTitle } from "./provider-names";
import { closeListedAgent } from "./close-agent";

export type NodeKind = "machine" | "workspace" | "tab" | "pane";

export interface MenuNode {
  kind: NodeKind;
  /** Session rows describe the conversation, never operations on its terminal pane. */
  sessionOnly?: boolean;
  mid: string;
  /** The machine is online; a node of one that is away is only copied from. */
  online?: boolean;
  /** The kind of terminal host the node is on ("herdr"); "" or unset: herdr, the one most machines have. */
  host?: string;
  /** What the node is called: a workspace's or tab's name is what "Rename" starts from. */
  label?: string;
  workspaceId?: string;
  tabId?: string;
  /** The pane the node is (kind "pane"), or one of a tab's or workspace's panes: the one a split or a git command is about. */
  paneId?: string;
  /** The agent session in the pane. */
  sessionId?: string;
  /** The agent running in the pane ("claude", "codex"…); "" a plain shell; unset when the tree does not say. */
  agent?: string;
  /** An agent session in the tab: a tab with one is renamed through it (what "Rename…" already does). */
  tabSessionId?: string;
  cwd?: string;
  branch?: string;
  /** The workspace's terminal host can make a Git worktree of its repository ("New worktree…", #2431). */
  canWorktree?: boolean;
  /** The node's fold: the row it hangs under folds and opens (the sidebar's and the terminal page's `fold:` keys). */
  fold?: { folded: boolean; toggle: () => void };
  /** "New terminal" in this workspace, or on this computer when it has no workspace yet (the terminal page). */
  onOpenShell?: () => void;
  /** Read the list again (the terminal page). */
  onRefresh?: () => void;
}

/** What the items do. The defaults call the hub; a test (or the palette) passes its own. */
export interface NodeActionDeps {
  /** POST /machines/{mid}/<path> with a JSON body. */
  post: (mid: string, path: string, body: object) => Promise<unknown>;
  renameWorkspace: (node: MenuNode, name: string) => Promise<unknown>;
  renameTab: (node: MenuNode, name: string) => Promise<unknown>;
  launchers: (mid: string) => Promise<Launcher[]>;
  /** "New agent…" in a folder of this machine (the "New agent" dialog, filled in). */
  newAgent: (mid: string, cwd: string) => void;
  /** "New worktree…" for this workspace (the dialog, #2431); a deps without it leaves the item out. */
  newWorktree?: (node: MenuNode) => void;
  /** Opens a session's chat: the agent "Run agent here" started, as "New agent" opens the one it starts (#3287). */
  openSession: (mid: string, sid: string) => void;
  copy: (text: string) => void;
  notify: (text: string, tone: "ok" | "error" | "info") => void;
  /** The tree changed on the computer: read it again. */
  changed: () => void;
  /** "Close pane" on a pane an agent runs in (#7125): the agent's "Close…" (lib/close-agent.ts), which says what
   *  closing interrupts, instead of the menu's second click. Undefined (a session this page doesn't list, or nothing
   *  to close it with): the pane closes as a shell's does. */
  closeAgent?: (mid: string, sid: string) => Pick<MenuItem, "onSelect" | "disabled" | "detail"> | undefined;
}

export function defaultNodeDeps(changed: () => void = () => {}): NodeActionDeps {
  return {
    post: (mid, path, body) => api(`/machines/${enc(mid)}/${path}`, { method: "POST", json: body }),
    renameWorkspace: (node, name) => api(`/machines/${enc(node.mid)}/spaces/${enc(node.workspaceId ?? "")}/rename`, { method: "POST", json: { name } }),
    renameTab: (node, name) =>
      node.tabSessionId
        ? api(`${sessionPath(node.mid, node.tabSessionId)}/rename`, { method: "POST", json: { tab_name: name } })
        : api(`/machines/${enc(node.mid)}/tabs/rename`, { method: "POST", json: { tab_id: node.tabId, label: name, ...(node.host ? { kind: node.host } : {}) } }),
    launchers: (mid) => api<{ launchers: Launcher[] }>(`/machines/${enc(mid)}/launchers`).then((r) => r.launchers ?? []),
    newAgent: (mid, cwd) => openNewAgent({ machineId: mid, cwd }),
    newWorktree: (node) => openNewWorktree(node, changed),
    openSession: (mid, sid) => navigate({ name: "session", mid, sid }),
    copy: (text) => void copyText(text),
    notify: (text, tone) => toast(text, tone),
    changed,
    closeAgent: closeListedAgent,
  };
}

export interface NodeActionOptions {
  /** Items to leave out, by key (a row whose own menu already has them: AgentRowMenu's "Copy folder path" is the path). */
  omit?: readonly string[];
}

/** Closing, splitting, git and running an agent in a shell are herdr's: another host (Orca, herdr-core, Tern, Rex, botmux) is
 *  not asked to do them. */
export const canChangeHost = (host: string | undefined) => !host || host === "herdr";

/** Renaming a workspace or a tab is herdr's, Tern's and Rex's (the capability table in docs/integrations/engines.md). */
export const canRenameIn = (host: string | undefined) => canChangeHost(host) || host === "tern" || host === "rex";

/** Claude Code, and a launcher the computer says it starts in a pane the user picked, keeping that pane when the start
 *  fails (`run_here`: its Codex launchers, #5504). An older computer's failed Codex start closes its pane, here the
 *  user's own shell (#5275): there Codex starts from "New agent" in a tab of its own. Never one that starts the session
 *  in a desktop app (#5480, lib/desktop-start.ts's sources, spelled out here so that module stays off the first screen):
 *  typed into a shell it would run nothing, and the computer refuses it. */
export const launchableHere = (l: Launcher) =>
  l.source !== "claude_desktop" && l.source !== "codex_app" && (l.run_here || (l.agent_type || l.agent || "claude") === "claude");

export const GIT_OPS = [
  { op: "pull", label: t("lib-node-actions.git_pull"), accel: "p", done: "lib-node-actions.git_pull_done" },
  { op: "push", label: t("lib-node-actions.git_push"), accel: "u", done: "lib-node-actions.git_push_done" },
  { op: "fetch", label: t("lib-node-actions.git_fetch"), accel: "f", done: "lib-node-actions.git_fetch_done" },
] as const;

/** What `panes/git` answers: what came of it (#1888) and the node's own one-line summary, all an older node sends. */
export interface PaneGitResult {
  op?: string;
  outcome?: string;
  message?: string;
}

/** What `panes/agent` answers: the session it started in the pane (`docs/protocol/protocol.md`). */
export interface PaneAgentResult {
  session_id?: string;
}

const GIT_OUTCOMES = new Set(["fetched", "up_to_date", "pulled", "nothing_to_push", "pushed"]);

/** What a node menu's Git ▸ Pull / Push / Fetch did, in the app's language; an outcome this app doesn't know (an older
 *  node sends none) shows the node's own words. */
export function gitOutcomeText(res: PaneGitResult | undefined): string {
  if (res?.outcome && GIT_OUTCOMES.has(res.outcome)) return t(`lib-node-actions.git_outcome_${res.outcome}`);
  return res?.message || t("lib-node-actions.git_done_fallback");
}

/** The menu items of a node, in groups (a hairline between them): fold and open, rename, make, run and close a pane, copy,
 *  close a tab. */
export function nodeActions(node: MenuNode, deps: NodeActionDeps, opts: NodeActionOptions = {}): MenuItem[] {
  const { mid } = node;
  const online = node.online !== false;
  const writes = online && canChangeHost(node.host);
  const renames = online && canRenameIn(node.host);
  const kind = node.host ? { kind: node.host } : {};
  const fail = (key: string, e: unknown) => new Error(t(key, { error: errorText(e) }));

  // where a git command runs: a pane's own directory, else the workspace's
  const gitTarget = node.paneId ? { pane: node.paneId } : node.workspaceId ? { workspace_id: node.workspaceId, ...kind } : null;
  const isShell = (node.kind === "pane" || node.kind === "tab") && node.agent === "" && !node.sessionId;

  const groups: MenuItem[][] = [];
  const group = (...items: (MenuItem | false | null | undefined)[]) => {
    const list = items.filter(Boolean) as MenuItem[];
    if (list.length) groups.push(list);
  };
  const noop = () => {};
  const copy = (key: string, label: string, accel: string, text: string | undefined): MenuItem | false =>
    Boolean(text) && { key, label, icon: "copy", accel, onSelect: () => deps.copy(text!) };

  // ── fold and open ──
  group(
    node.fold && { key: "node-fold", label: node.fold.folded ? t("lib-node-actions.expand") : t("lib-node-actions.collapse"), icon: node.fold.folded ? "arrowDown" : "chevron", accel: "z", onSelect: node.fold.toggle },
    node.kind === "workspace" && online && Boolean(node.cwd) && { key: "node-new-agent", label: t("lib-node-actions.new_agent"), icon: "agent", accel: "n", onSelect: () => deps.newAgent(mid, node.cwd!) },
    (node.kind === "workspace" || node.kind === "machine") && online && node.onOpenShell && { key: "node-shell", label: t("lib-node-actions.new_terminal"), icon: "terminal", accel: "t", onSelect: node.onOpenShell },
    node.kind === "machine" && node.onRefresh && { key: "node-refresh", label: t("lib-node-actions.refresh"), icon: "retry", accel: "r", onSelect: node.onRefresh },
  );

  // ── rename ──
  const renameValue = node.label ?? "";
  group(
    node.kind === "workspace" && renames && Boolean(node.workspaceId) && {
      key: "node-rename",
      label: t("lib-node-actions.rename"),
      icon: "pencil",
      accel: "r",
      onSelect: noop,
      rename: {
        value: renameValue,
        placeholder: t("lib-node-actions.workspace_name"),
        commit: async (name) => {
          await deps.renameWorkspace(node, name).catch((e) => Promise.reject(fail("lib-node-actions.rename_failed", e)));
          deps.notify(t("lib-node-actions.workspace_renamed"), "ok");
          deps.changed();
        },
      },
    },
    node.kind === "tab" && renames && Boolean(node.tabId) && {
      key: "node-rename",
      label: t("lib-node-actions.rename"),
      icon: "pencil",
      accel: "r",
      onSelect: noop,
      rename: {
        value: renameValue,
        placeholder: t("lib-node-actions.tab_name"),
        commit: async (name) => {
          await deps.renameTab(node, name).catch((e) => Promise.reject(fail("lib-node-actions.rename_failed", e)));
          deps.notify(t("lib-node-actions.tab_renamed"), "ok");
          deps.changed();
        },
      },
    },
  );

  // ── make, run and close: a worktree, split, close the pane, an agent here, git ──
  const split = (direction: "right" | "down", label: string, accel: string): MenuItem => ({
    key: `node-split-${direction}`,
    label,
    accel,
    inline: true,
    onSelect: async () => {
      await deps.post(mid, "panes/split", { pane: node.paneId, direction }).catch((e) => Promise.reject(fail("lib-node-actions.split_failed", e)));
      deps.notify(t("lib-node-actions.pane_created"), "ok");
      deps.changed();
    },
  });
  // a push sends the branch off the computer: it asks first, as the Git panel's Push does (#1883); pull and fetch run at once
  const pushAsk = {
    confirm: t("lib-node-actions.git_push_confirm"),
    confirmDetail: node.branch ? t("lib-node-actions.git_push_confirm_detail", { branch: node.branch }) : t("lib-node-actions.git_push_confirm_detail_current"),
  };
  const git = (op: (typeof GIT_OPS)[number]): MenuItem => ({
    key: `node-git-${op.op}`,
    label: op.label,
    accel: op.accel,
    ...(op.op === "push" ? pushAsk : {}),
    inline: true,
    onSelect: async () => {
      const res = (await deps.post(mid, "panes/git", { op: op.op, ...gitTarget }).catch((e) => Promise.reject(new Error(errorText(e))))) as PaneGitResult | undefined;
      deps.notify(t(op.done, { message: gitOutcomeText(res) }), "ok");
      deps.changed();
    },
  });
  const onHere = (l: Launcher, i: number, all: Launcher[]): MenuItem => {
    const block = launcherBlock(l);
    return {
      key: `node-agent-${l.name}`,
      // a provider by its display name, its startup ID at the row's end before the model (#5029); an rc alias by its ID
      label: providerTitle(l),
      hint: [providerIdNote(l), l.model].filter(Boolean).join(" · ") || undefined,
      // the pane's shell is a terminal's too: what "New agent" grays out is said here before it is chosen, after the
      // ones that can start (#1643); still a choice, as on iOS and New agent's "Try anyway": a start that works clears it
      ...(block ? { detail: block.reason, divider: i > 0 && !launcherBlock(all[i - 1]) } : {}),
      inline: true,
      onSelect: async () => {
        const res = (await deps.post(mid, "panes/agent", { pane: node.paneId, launcher: l.name }).catch((e) => Promise.reject(fail("lib-node-actions.start_failed", e)))) as PaneAgentResult | undefined;
        deps.notify(t("lib-node-actions.started_in_pane", { name: providerTitle(l) }), "ok");
        deps.changed();
        // into the new agent's chat, as "New agent" opens the one it starts (#3287); an answer naming none stays here
        const sid = res?.session_id?.trim();
        if (sid) deps.openSession(mid, sid);
      },
    };
  };
  const gitMenu = writes && node.kind !== "machine" && Boolean(gitTarget);
  const newWorktree = deps.newWorktree;
  // a pane closes with the pane's own actions, next to New pane (#7125); one an agent runs in asks as the agent's
  // Close… does, so a busy agent is never closed by the menu's second click alone
  const agentClose = node.kind === "pane" && node.sessionId ? deps.closeAgent?.(mid, node.sessionId) : undefined;
  const closePane: MenuItem = {
    key: "node-close-pane",
    label: t("lib-node-actions.close_pane"),
    icon: "x",
    danger: true,
    closes: true,
    ...(agentClose ?? {
      confirm: t("lib-node-actions.close_confirm"),
      inline: true,
      onSelect: async () => {
        await deps.post(mid, "panes/close", { pane: node.paneId }).catch((e) => Promise.reject(fail("lib-node-actions.close_failed", e)));
        deps.notify(t("lib-node-actions.pane_closed"), "ok");
        deps.changed();
      },
    }),
  };
  group(
    node.kind === "workspace" && writes && node.canWorktree && Boolean(node.workspaceId) && newWorktree && {
      key: "node-worktree",
      label: t("worktree.menu_new"),
      icon: "git",
      accel: "w",
      onSelect: () => newWorktree(node),
    },
    writes && Boolean(node.paneId) && node.kind !== "machine" && node.kind !== "workspace" && {
      key: "node-split",
      label: t("lib-node-actions.new_pane"),
      icon: "layers",
      accel: "n",
      onSelect: noop,
      children: [split("right", t("lib-node-actions.split_right"), "r"), split("down", t("lib-node-actions.split_down"), "d")],
    },
    node.kind === "pane" && writes && Boolean(node.paneId) && closePane,
    writes && isShell && Boolean(node.paneId) && {
      key: "node-agent",
      label: t("lib-node-actions.run_agent_here"),
      icon: "agent",
      accel: "a",
      onSelect: noop,
      children: async () => usableFirst((await deps.launchers(mid)).filter(launchableHere)).map(onHere),
    },
    gitMenu && {
      key: "node-git",
      label: "Git",
      icon: "git",
      accel: "g",
      hint: node.branch,
      onSelect: noop,
      children: [...GIT_OPS.map(git), ...(node.branch ? [copy("node-copy-branch", t("lib-node-actions.copy_branch"), "b", node.branch) as MenuItem] : [])],
    },
  );

  // ── copy ──
  // the engine's own id of the space, tab or pane, named by what it is: a menu never says the engine's name
  const [engineId, engineIdLabel] = node.kind === "pane" ? [node.paneId, t("lib-node-actions.copy_pane_id")]
    : node.kind === "tab" ? [node.tabId, t("lib-node-actions.copy_tab_id")]
    : node.kind === "workspace" ? [node.workspaceId, t("lib-node-actions.copy_workspace_id")] : [undefined, ""];
  group(
    node.kind !== "machine" && copy("node-copy-path", t("lib-node-actions.copy_path"), "c", node.cwd),
    node.kind !== "machine" && node.kind !== "workspace" && copy("node-copy-session", t("lib-node-actions.copy_session_id"), "s", node.sessionId),
    node.kind !== "machine" && copy("node-copy-id", engineIdLabel, "i", engineId),
    node.kind !== "machine" && !gitMenu && copy("node-copy-branch", t("lib-node-actions.copy_branch"), "b", node.branch),
    node.kind === "machine" && copy("node-copy-machine", t("lib-node-actions.copy_machine_id"), "i", mid),
  );

  // ── close the tab ──
  group(
    node.kind === "tab" && writes && Boolean(node.tabId) && {
      key: "node-close-tab",
      label: t("lib-node-actions.close_tab"),
      icon: "x",
      danger: true,
      closes: true,
      confirm: t("lib-node-actions.close_confirm"),
      inline: true,
      onSelect: async () => {
        await deps.post(mid, "tabs/close", { tab_id: node.tabId, ...kind }).catch((e) => Promise.reject(fail("lib-node-actions.close_failed", e)));
        deps.notify(t("lib-node-actions.tab_closed"), "ok");
        deps.changed();
      },
    },
  );

  const omit = new Set(opts.omit);
  if (node.sessionOnly) for (const key of ["node-split", "node-close-pane", "node-copy-id"]) omit.add(key);
  return groups
    .map((g) => g.filter((it) => !omit.has(it.key)))
    .filter((g) => g.length)
    .flatMap((g, i) => g.map((it, j) => (i > 0 && j === 0 ? { ...it, divider: true } : it)));
}

// ── the nodes of the two trees ──────────────────────────────

/** A machine's header. */
export function machineNode(m: Machine, extra: Partial<MenuNode> = {}): MenuNode {
  return { kind: "machine", mid: m.id, online: m.online, label: m.name, ...extra };
}

/** The pane an agent session runs in (the sidebar's agent rows). */
export function sessionNode(s: Session, online: boolean, extra: Partial<MenuNode> = {}): MenuNode {
  return {
    kind: "pane",
    sessionOnly: true,
    mid: s.machine_id,
    online,
    host: s.space_kind,
    workspaceId: s.workspace_id ?? s.space_id,
    tabId: s.tab_id ?? s.slot_id,
    paneId: s.pane_id,
    sessionId: s.session_id,
    agent: s.agent || "claude",
    cwd: s.cwd,
    label: s.slot_name,
    ...extra,
  };
}

/** A workspace of the "Terminal" page. */
export function termSpaceNode(mid: string, sp: TermSpace, online: boolean, extra: Partial<MenuNode> = {}): MenuNode {
  return { kind: "workspace", mid, online, host: sp.host, label: sp.label, workspaceId: sp.id, cwd: sp.cwd, branch: sp.branch, canWorktree: sp.canWorktree, ...extra };
}

/** A tab of the "Terminal" page; a tab with one pane is that pane too (its agent or shell, its session). */
export function termTabNode(mid: string, sp: TermSpace, tab: TermTab, online: boolean, extra: Partial<MenuNode> = {}): MenuNode {
  const lone = tab.panes.length === 1 ? tab.panes[0] : undefined;
  const first = tab.panes[0];
  return {
    kind: "tab",
    mid,
    online,
    host: sp.host,
    label: tab.label,
    workspaceId: sp.id,
    tabId: tab.id,
    paneId: first?.pane,
    cwd: lone?.cwd || sp.cwd,
    branch: sp.branch,
    ...(lone ? { sessionId: lone.sessionId, agent: lone.agent } : {}),
    ...extra,
  };
}

/** One pane of a tab with several, on the "Terminal" page. */
export function termPaneNode(mid: string, sp: TermSpace, tab: TermTab, pane: TermPane, online: boolean, extra: Partial<MenuNode> = {}): MenuNode {
  return {
    kind: "pane",
    mid,
    online,
    host: sp.host,
    workspaceId: sp.id,
    tabId: tab.id,
    paneId: pane.pane,
    sessionId: pane.sessionId,
    agent: pane.agent,
    cwd: pane.cwd || sp.cwd,
    branch: sp.branch,
    ...extra,
  };
}
