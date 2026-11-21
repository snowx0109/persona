import { describe, expect, it, vi } from "vitest";
import type { MenuItem } from "../ui/menu-model";
import { canChangeHost, canRenameIn, gitOutcomeText, machineNode, nodeActions, sessionNode, termPaneNode, termSpaceNode, termTabNode, type MenuNode, type NodeActionDeps } from "./node-actions";
import { buildTerminalTree, shellsOnly, type PanesResponse } from "./terminals";
import type { Session } from "./types";
import zhNode from "../i18n/locales/zh-Hans/lib-node-actions.json";
import enNode from "../i18n/locales/en/lib-node-actions.json";
import jaNode from "../i18n/locales/ja/lib-node-actions.json";
import zhWorktree from "../i18n/locales/zh-Hans/worktree-actions.json";
import enWorktree from "../i18n/locales/en/worktree-actions.json";
import jaWorktree from "../i18n/locales/ja/worktree-actions.json";
import zhAgent from "../i18n/locales/zh-Hans/agentmenus.json";
import enAgent from "../i18n/locales/en/agentmenus.json";
import jaAgent from "../i18n/locales/ja/agentmenus.json";
import zhChat from "../i18n/locales/zh-Hans/chat-chatview.json";
import enChat from "../i18n/locales/en/chat-chatview.json";
import jaChat from "../i18n/locales/ja/chat-chatview.json";

function deps(over: Partial<NodeActionDeps> = {}): NodeActionDeps {
  return {
    post: vi.fn(async () => ({ message: "已是最新" })),
    renameWorkspace: vi.fn(async () => {}),
    renameTab: vi.fn(async () => {}),
    launchers: vi.fn(async () => [{ name: "claude" }, { name: "work", model: "m1" }, { name: "cx", agent: "codex" }]),
    newAgent: vi.fn(),
    openSession: vi.fn(),
    copy: vi.fn(),
    notify: vi.fn(),
    changed: vi.fn(),
    ...over,
  };
}

const keys = (items: MenuItem[]) => items.map((i) => i.key);
const by = (items: MenuItem[], key: string) => items.find((i) => i.key === key)!;

const tab: MenuNode = { kind: "tab", mid: "m1", host: "herdr", label: "build", workspaceId: "w1", tabId: "w1:t2", paneId: "w1:p2", agent: "", sessionId: "", cwd: "/home/me/proj", branch: "main" };

describe("a tab's menu", () => {
  it("offers rename, split, run an agent, git, copies and close", () => {
    expect(keys(nodeActions(tab, deps()))).toEqual([
      "node-rename", "node-split", "node-agent", "node-git", "node-copy-path", "node-copy-id", "node-close-tab",
    ]);
  });

  it("hairlines between the groups, and the close action is the danger row that asks first", () => {
    const items = nodeActions(tab, deps());
    expect(items.filter((i) => i.divider).map((i) => i.key)).toEqual(["node-split", "node-copy-path", "node-close-tab"]);
    const close = by(items, "node-close-tab");
    expect(close).toMatchObject({ danger: true, closes: true, confirm: "确认关闭", inline: true });
  });

  it("closing the tab posts its id and the host, then tells the tree", async () => {
    const d = deps();
    await by(nodeActions(tab, d), "node-close-tab").onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "tabs/close", { tab_id: "w1:t2", kind: "herdr" });
    expect(d.notify).toHaveBeenCalledWith("已关闭标签页", "ok");
    expect(d.changed).toHaveBeenCalled();
  });

  it("a refused close is an error the menu shows, and the tree is not told", async () => {
    const d = deps({ post: vi.fn(async () => { throw new Error("tab w1:t2 is not running"); }) });
    await expect(by(nodeActions(tab, d), "node-close-tab").onSelect()).rejects.toThrow("关闭失败：tab w1:t2 is not running");
    expect(d.changed).not.toHaveBeenCalled();
  });

  it("renames inline, through the session's tab when an agent runs there", async () => {
    const d = deps();
    const rename = by(nodeActions({ ...tab, tabSessionId: "sid-1" }, d), "node-rename").rename!;
    expect(rename.value).toBe("build");
    await rename.commit("deploy");
    expect(d.renameTab).toHaveBeenCalledWith(expect.objectContaining({ tabSessionId: "sid-1", tabId: "w1:t2" }), "deploy");
    expect(d.notify).toHaveBeenCalledWith("已重命名标签页", "ok");
  });

  it("a failed rename says why", async () => {
    const d = deps({ renameTab: vi.fn(async () => { throw new Error("busy"); }) });
    await expect(by(nodeActions(tab, d), "node-rename").rename!.commit("x")).rejects.toThrow("没改成：busy");
  });
});

describe("splitting and running an agent", () => {
  it("splits right or down in the node's pane", async () => {
    const d = deps();
    const split = by(nodeActions(tab, d), "node-split");
    const [right, down] = split.children as MenuItem[];
    expect([right.accel, down.accel]).toEqual(["r", "d"]);
    await right.onSelect();
    await down.onSelect();
    expect(d.post).toHaveBeenNthCalledWith(1, "m1", "panes/split", { pane: "w1:p2", direction: "right" });
    expect(d.post).toHaveBeenNthCalledWith(2, "m1", "panes/split", { pane: "w1:p2", direction: "down" });
  });

  it("lists the launchers that start Claude Code when the submenu opens, and starts the one picked in the pane", async () => {
    const d = deps({ post: vi.fn(async () => ({ session_id: "s9" })) });
    const agent = by(nodeActions(tab, d), "node-agent");
    expect(typeof agent.children).toBe("function");
    const list = await (agent.children as () => Promise<MenuItem[]>)();
    expect(list.map((i) => i.label)).toEqual(["claude", "work"]); // a Codex launcher needs a first message
    expect(list[1].hint).toBe("m1");
    await list[1].onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/agent", { pane: "w1:p2", launcher: "work" });
    expect(d.notify).toHaveBeenCalledWith(expect.any(String), "ok");
    expect(d.changed).toHaveBeenCalled();
    // the agent it started opens, as New agent's does (#3287)
    expect(d.openSession).toHaveBeenCalledWith("m1", "s9");
  });

  it("offers Codex launchers where the computer starts them in the user's pane (run_here, #5504), and only those", async () => {
    const d = deps({
      post: vi.fn(async () => ({ session_id: "c0de0000-0000-4000-8000-000000000009" })),
      launchers: vi.fn(async () => [
        { name: "claude" },
        { name: "codex", agent: "codex", agent_type: "codex", prompt_optional: true, run_here: true, model: "gpt-6" },
        { name: "cx_old", agent: "codex", prompt_optional: true }, // a computer that keeps no pane when a start fails
        { name: "cx_cfg", agent_type: "codex", run_here: false },
      ]),
    });
    const list = await (by(nodeActions(tab, d), "node-agent").children as () => Promise<MenuItem[]>)();
    expect(list.map((i) => i.label)).toEqual(["claude", "codex"]);
    await list[1].onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/agent", { pane: "w1:p2", launcher: "codex" });
    // its waiting row opens, where the first message goes (docs/integrations/codex.md "Starting without a first message")
    expect(d.openSession).toHaveBeenCalledWith("m1", "c0de0000-0000-4000-8000-000000000009");
  });

  it("never offers a launcher that starts the session in a desktop app (#5480): a shell would run nothing", async () => {
    const d = deps({
      launchers: vi.fn(async () => [
        { name: "claude" },
        { name: "claude-desktop", agent: "claude", source: "claude_desktop" },
        { name: "codex-app", agent: "codex", source: "codex_app", run_here: true },
      ]),
    });
    const list = await (by(nodeActions(tab, d), "node-agent").children as () => Promise<MenuItem[]>)();
    expect(list.map((i) => i.label)).toEqual(["claude"]);
  });

  it("names a provider by its display name, its ID before the model at the row's end; an rc alias by its ID (#5029)", async () => {
    const d = deps({ launchers: vi.fn(async () => [{ name: "relay-example-com", display_name: "我的中转 站", model: "glm-5" }, { name: "cc_glm" }]) });
    const list = await (by(nodeActions(tab, d), "node-agent").children as () => Promise<MenuItem[]>)();
    expect(list.map((i) => [i.label, i.hint])).toEqual([["我的中转 站", "relay-example-com · glm-5"], ["cc_glm", undefined]]);
    await list[0].onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/agent", { pane: "w1:p2", launcher: "relay-example-com" });
    expect(d.notify).toHaveBeenCalledWith("已在窗格里启动 我的中转 站", "ok");
  });

  it("stays where it is when the answer names no session", async () => {
    for (const answer of [{}, { session_id: "" }, undefined]) {
      const d = deps({ post: vi.fn(async () => answer) });
      const list = await (by(nodeActions(tab, d), "node-agent").children as () => Promise<MenuItem[]>)();
      await list[0].onSelect();
      expect(d.notify).toHaveBeenCalledWith(expect.any(String), "ok");
      expect(d.openSession).not.toHaveBeenCalled();
    }
  });

  it("opens nothing when the start fails", async () => {
    const d = deps({ post: vi.fn(async () => Promise.reject(new Error("pane busy"))) });
    const list = await (by(nodeActions(tab, d), "node-agent").children as () => Promise<MenuItem[]>)();
    await expect(list[0].onSelect()).rejects.toThrow("pane busy");
    expect(d.openSession).not.toHaveBeenCalled();
  });

  it("says why a launcher this computer's terminal can't run, after the usable ones, and still lets it run (#1643)", async () => {
    const d = deps({
      launchers: vi.fn(async () => [
        { name: "claude", health: { state: "ok" } },
        { name: "work", health: { state: "blocked", kind: "not_in_terminal", error: "bash: work: command not found" } },
        { name: "slow", health: { state: "failing", kind: "timeout" } },
      ]),
    });
    const list = await (by(nodeActions(tab, d), "node-agent").children as () => Promise<MenuItem[]>)();
    // usable first (still being retried by the hub counts: offered as before), the blocked one after a line, with why
    expect(list.map((i) => i.label)).toEqual(["claude", "slow", "work"]);
    expect(list[2].detail).toBe("这台电脑的终端里用不了");
    expect(list[2].divider).toBe(true);
    expect(list[0].detail).toBeUndefined();
    expect(list[1].detail).toBeUndefined();
    expect(list[1].divider).toBeUndefined();
    // the same as iOS's sheet and New agent's "Try anyway": once fixed, a start that works is what clears the record
    expect(list.some((i) => i.disabled)).toBe(false);
    await list[2].onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/agent", { pane: "w1:p2", launcher: "work" });
  });

  it("is for a plain shell only", () => {
    const withAgent = nodeActions({ ...tab, agent: "claude", sessionId: "sid-1" }, deps());
    expect(keys(withAgent)).not.toContain("node-agent");
    expect(keys(nodeActions({ ...tab, agent: undefined }, deps()))).not.toContain("node-agent");
  });
});

describe("git", () => {
  it("pull, push and fetch in the pane's directory", async () => {
    const d = deps();
    const git = by(nodeActions(tab, d), "node-git");
    expect(git.hint).toBe("main");
    const ops = git.children as MenuItem[];
    // the branch is copied from the Git menu, as Moshi's is
    expect(ops.map((i) => [i.label, i.accel])).toEqual([["拉取", "p"], ["推送", "u"], ["获取", "f"], ["复制分支名", "b"]]);
    await ops[1].onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/git", { op: "push", pane: "w1:p2" });
    expect(d.notify).toHaveBeenCalledWith("推送：已是最新", "ok");
  });

  it("says what came of it in the app's language, or an older node's own words (#1888)", async () => {
    const d = deps({ post: vi.fn(async () => ({ outcome: "pushed", message: "Pushed" })) });
    await (by(nodeActions(tab, d), "node-git").children as MenuItem[])[1].onSelect();
    expect(d.notify).toHaveBeenCalledWith("推送：已推送", "ok");
    expect(gitOutcomeText({ outcome: "nothing_to_push", message: "No commits to push" })).toBe("没有要推送的提交");
    expect(gitOutcomeText({ outcome: "a_new_outcome", message: "node words" })).toBe("node words");
    expect(gitOutcomeText(undefined)).toBe("完成");
  });

  it("push asks first and says where it goes and that it is never forced; pull and fetch run at once (#1883)", () => {
    const ops = by(nodeActions(tab, deps()), "node-git").children as MenuItem[];
    const [pull, push, fetch] = ops;
    expect(push.confirm).toBe("确认推送");
    expect(push.confirmDetail).toBe("main 会推到它的上游分支。不会强制推送：远端有更新的提交时会停下并说明。");
    expect(pull.confirm).toBeUndefined();
    expect(fetch.confirm).toBeUndefined();
    const noBranch = by(nodeActions({ ...tab, branch: undefined }, deps()), "node-git").children as MenuItem[];
    expect(noBranch[1].confirmDetail).toBe("当前分支会推到它的上游分支。不会强制推送：远端有更新的提交时会停下并说明。");
  });

  it("a workspace without a pane is named by its id and host", async () => {
    const d = deps();
    const ws: MenuNode = { kind: "workspace", mid: "m1", host: "herdr", workspaceId: "w1", label: "proj" };
    await (by(nodeActions(ws, d), "node-git").children as MenuItem[])[0].onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/git", { op: "pull", workspace_id: "w1", kind: "herdr" });
  });

  it("git's failure reaches the menu as it is", async () => {
    const d = deps({ post: vi.fn(async () => { throw new Error("不是 Git 仓库"); }) });
    await expect((by(nodeActions(tab, d), "node-git").children as MenuItem[])[0].onSelect()).rejects.toThrow("不是 Git 仓库");
  });
});

describe("read-only items", () => {
  const pane: MenuNode = { kind: "pane", mid: "m1", host: "herdr", paneId: "w1:p3", sessionId: "sid-1", agent: "claude", cwd: "/home/me/proj", branch: "feat/x" };
  it("copies the path, the session id, the herdr id and the branch", () => {
    const d = deps();
    const items = nodeActions(pane, d);
    for (const [key, text] of [["node-copy-path", "/home/me/proj"], ["node-copy-session", "sid-1"], ["node-copy-id", "w1:p3"]] as const) {
      by(items, key).onSelect();
      expect(d.copy).toHaveBeenLastCalledWith(text);
    }
    (by(items, "node-git").children as MenuItem[]).find((i) => i.key === "node-copy-branch")!.onSelect();
    expect(d.copy).toHaveBeenLastCalledWith("feat/x");
  });
  it("says one verb and ID, and names the id by what it is, never by the engine (herdr) behind it", () => {
    const d = deps();
    const space: MenuNode = { kind: "workspace", mid: "m1", host: "herdr", label: "proj", workspaceId: "w1", cwd: "/home/me/proj" };
    const labels = (node: MenuNode) => nodeActions(node, d).filter((i) => i.icon === "copy").map((i) => i.label);
    expect(labels(pane)).toEqual(["复制目录路径", "复制会话 ID", "复制窗格 ID"]);
    expect(labels(tab)).toEqual(["复制目录路径", "复制标签页 ID"]);
    expect(labels(space)).toEqual(["复制目录路径", "复制空间 ID"]);
    expect(labels({ kind: "machine", mid: "m1" })).toEqual(["复制机器 ID"]);
    // the same values as before: only the words changed
    by(nodeActions(tab, d), "node-copy-id").onSelect();
    expect(d.copy).toHaveBeenLastCalledWith("w1:t2");
    by(nodeActions(space, d), "node-copy-id").onSelect();
    expect(d.copy).toHaveBeenLastCalledWith("w1");
    for (const node of [pane, tab, space]) for (const label of labels(node)) expect(label).not.toMatch(/herdr|拷贝|\bid\b/);
  });
  it("copies a folder's path in the same words in every menu of the Agents list and a chat, in every language", () => {
    const languages = [
      { node: zhNode, worktree: zhWorktree, agent: zhAgent, chat: zhChat },
      { node: enNode, worktree: enWorktree, agent: enAgent, chat: enChat },
      { node: jaNode, worktree: jaWorktree, agent: jaAgent, chat: jaChat },
    ];
    for (const { node, worktree, agent, chat } of languages) {
      expect(new Set([node.copy_path, worktree.copy_path, agent.copy_folder, chat.copy_folder]).size).toBe(1);
    }
    // the By repository grouping's worktree menu, beside the node menus, says their verb
    expect([zhWorktree.copy_name, zhWorktree.copy_path, zhNode.copy_path]).toEqual(["复制工作树名称", "复制目录路径", "复制目录路径"]);
  });
  it("leaves out what is not known", () => {
    const k = keys(nodeActions({ kind: "pane", mid: "m1", paneId: "w1:p3", agent: "" }, deps()));
    expect(k).not.toContain("node-copy-path");
    expect(k).not.toContain("node-copy-session");
    expect(k).not.toContain("node-copy-branch");
    expect(k).toContain("node-copy-id");
  });
  it("copies the branch from the top level when there is no Git menu to hold it", () => {
    expect(keys(nodeActions({ ...tab, host: "orca" }, deps()))).toContain("node-copy-branch");
  });
  it("leaves out what the row's own menu has", () => {
    expect(keys(nodeActions(pane, deps(), { omit: ["node-copy-path", "node-rename"] }))).not.toContain("node-copy-path");
  });
  it("a pane closes itself, not its tab", async () => {
    const d = deps();
    await by(nodeActions(pane, d), "node-close-pane").onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/close", { pane: "w1:p3" });
    expect(keys(nodeActions(pane, d))).not.toContain("node-close-tab");
  });
});

describe("closing a pane (#7125)", () => {
  const shell: MenuNode = { kind: "pane", mid: "m1", host: "herdr", workspaceId: "w1", tabId: "w1:t2", paneId: "w1:p4", agent: "", cwd: "/home/me/proj", branch: "main" };
  const agent: MenuNode = { ...shell, paneId: "w1:p3", sessionId: "sid-1", agent: "claude" };
  it("sits with the pane's own actions, right after New pane, not in a danger group at the bottom", () => {
    const items = nodeActions(shell, deps());
    expect(keys(items)).toEqual(["node-split", "node-close-pane", "node-agent", "node-git", "node-copy-path", "node-copy-id"]);
    expect(by(items, "node-close-pane")).toMatchObject({ danger: true, closes: true, confirm: "确认关闭", inline: true });
    expect(by(items, "node-close-pane").divider).toBeFalsy();
  });
  it("a shell's pane closes after the menu's second click, as before", async () => {
    const d = deps({ closeAgent: vi.fn(() => undefined) });
    await by(nodeActions(shell, d), "node-close-pane").onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/close", { pane: "w1:p4" });
    expect(d.closeAgent).not.toHaveBeenCalled();
  });
  it("an agent's pane asks with the agent's Close… instead, so a busy agent is never closed by the second click alone", () => {
    const open = vi.fn();
    const d = deps({ closeAgent: vi.fn(() => ({ onSelect: open })) });
    const close = by(nodeActions(agent, d), "node-close-pane");
    expect(d.closeAgent).toHaveBeenCalledWith("m1", "sid-1");
    expect(close.confirm).toBeUndefined();
    expect(close.inline).toBeUndefined();
    close.onSelect();
    expect(open).toHaveBeenCalledOnce();
    expect(d.post).not.toHaveBeenCalled();
  });
  it("stays disabled, saying why, where the agent's Close… is", () => {
    const d = deps({ closeAgent: () => ({ onSelect: () => {}, disabled: true, detail: "正忙" }) });
    expect(by(nodeActions(agent, d), "node-close-pane")).toMatchObject({ disabled: true, detail: "正忙" });
  });
  it("closes the pane as a shell's when the page doesn't list the agent's session", async () => {
    const d = deps({ closeAgent: () => undefined });
    await by(nodeActions(agent, d), "node-close-pane").onSelect();
    expect(d.post).toHaveBeenCalledWith("m1", "panes/close", { pane: "w1:p3" });
  });
});

describe("what a computer that cannot be changed is offered", () => {
  it("an offline machine or another host's node only copies", () => {
    for (const node of [{ ...tab, online: false }, { ...tab, host: "orca" }, { ...tab, host: "botmux" }]) {
      const k = keys(nodeActions(node, deps()));
      expect(k).toEqual(["node-copy-path", "node-copy-id", "node-copy-branch"]);
    }
    expect(canChangeHost(undefined)).toBe(true);
    expect(canChangeHost("herdr")).toBe(true);
    expect(canChangeHost("herdr-core")).toBe(false);
  });
});

describe("a Tern node (#2154)", () => {
  const ternTab: MenuNode = { ...tab, host: "tern", workspaceId: "s1", tabId: "t2", paneId: "tern:5" };
  const ternSpace: MenuNode = { kind: "workspace", mid: "m1", host: "tern", label: "proj", workspaceId: "s1", cwd: "/home/me/proj", branch: "main", canWorktree: true };

  it("renames its tabs and workspaces, and is not asked to split, run an agent, git, close or make a worktree", async () => {
    const d = deps({ newWorktree: vi.fn() });
    expect(keys(nodeActions(ternTab, d))).toEqual(["node-rename", "node-copy-path", "node-copy-id", "node-copy-branch"]);
    expect(keys(nodeActions(ternSpace, d))).toEqual(["node-new-agent", "node-rename", "node-copy-path", "node-copy-id", "node-copy-branch"]);
    await by(nodeActions(ternTab, d), "node-rename").rename!.commit("deploy");
    expect(d.renameTab).toHaveBeenCalledWith(ternTab, "deploy");
    await by(nodeActions(ternSpace, d), "node-rename").rename!.commit("docs");
    expect(d.renameWorkspace).toHaveBeenCalledWith(ternSpace, "docs");
  });

  it("renames nothing while its machine is offline; the other hosts still rename nothing", () => {
    expect(keys(nodeActions({ ...ternTab, online: false }, deps()))).not.toContain("node-rename");
    for (const host of ["orca", "herdr-core", "botmux"]) expect(keys(nodeActions({ ...ternTab, host }, deps()))).not.toContain("node-rename");
    expect(canRenameIn("tern")).toBe(true);
    expect(canRenameIn("rex")).toBe(true);
    expect(canRenameIn(undefined)).toBe(true);
    expect(canChangeHost("tern")).toBe(false);
    expect(canChangeHost("rex")).toBe(false);
  });
});

describe("machine and workspace", () => {
  it("a machine folds, refreshes and copies its id", () => {
    const toggle = vi.fn();
    const items = nodeActions(machineNode({ id: "m1", name: "桌面", online: true } as never, { fold: { folded: false, toggle }, onRefresh: vi.fn() }), deps());
    expect(keys(items)).toEqual(["node-fold", "node-refresh", "node-copy-machine"]);
    expect(by(items, "node-fold").label).toBe("折叠");
    by(items, "node-fold").onSelect();
    expect(toggle).toHaveBeenCalled();
    expect(by(nodeActions(machineNode({ id: "m1", name: "桌面", online: true } as never, { fold: { folded: true, toggle } }), deps()), "node-fold").label).toBe("展开");
  });

  it("a workspace renames inline through the space route, opens a shell, and has no close", async () => {
    const d = deps();
    const onOpenShell = vi.fn();
    const ws: MenuNode = { kind: "workspace", mid: "m1", host: "herdr", label: "proj", workspaceId: "w1", cwd: "/home/me/proj", branch: "main", onOpenShell };
    const items = nodeActions(ws, d);
    expect(keys(items)).toEqual(["node-new-agent", "node-shell", "node-rename", "node-git", "node-copy-path", "node-copy-id"]);
    await by(items, "node-rename").rename!.commit("docs");
    expect(d.renameWorkspace).toHaveBeenCalledWith(ws, "docs");
    by(items, "node-shell").onSelect();
    expect(onOpenShell).toHaveBeenCalled();
    by(items, "node-new-agent").onSelect();
    expect(d.newAgent).toHaveBeenCalledWith("m1", "/home/me/proj");
  });
});

describe("新建 worktree… (#2431)", () => {
  const ws: MenuNode = { kind: "workspace", mid: "m1", host: "herdr", label: "proj", workspaceId: "w1", cwd: "/home/me/proj", branch: "main", canWorktree: true };

  it("is on a workspace whose host can make worktrees, with the W accelerator, in front of Git", () => {
    const items = nodeActions(ws, deps({ newWorktree: vi.fn() }));
    expect(by(items, "node-worktree")).toMatchObject({ label: "新建 worktree…", accel: "w", icon: "git" });
    expect(keys(items).indexOf("node-worktree")).toBeLessThan(keys(items).indexOf("node-git"));
  });
  it("opens the dialog for that workspace", () => {
    const d = deps({ newWorktree: vi.fn() });
    by(nodeActions(ws, d), "node-worktree").onSelect();
    expect(d.newWorktree).toHaveBeenCalledWith(ws);
  });
  it("is left out where it cannot work", () => {
    const d = deps({ newWorktree: vi.fn() });
    expect(keys(nodeActions({ ...ws, canWorktree: false }, d))).not.toContain("node-worktree");
    expect(keys(nodeActions({ ...ws, canWorktree: undefined }, d))).not.toContain("node-worktree");
    expect(keys(nodeActions({ ...ws, online: false }, d))).not.toContain("node-worktree");
    expect(keys(nodeActions({ ...ws, host: "orca" }, d))).not.toContain("node-worktree");
    expect(keys(nodeActions({ ...ws, workspaceId: undefined }, d))).not.toContain("node-worktree");
    expect(keys(nodeActions({ ...ws, kind: "tab", tabId: "w1:t1", paneId: "w1:p1" }, d))).not.toContain("node-worktree");
    expect(keys(nodeActions(ws, deps()))).not.toContain("node-worktree");
  });
});

describe("the nodes of the two trees", () => {
  const s = { machine_id: "m1", session_id: "sid-1", cwd: "/p", title: "t", status: "idle", space_kind: "herdr", space_id: "w1", workspace_id: "w1", slot_id: "w1:t1", tab_id: "w1:t1", pane_id: "w1:p1", slot_name: "agent-1" } as Session;
  it("a session is its pane", () => {
    expect(sessionNode(s, true)).toMatchObject({ kind: "pane", mid: "m1", host: "herdr", workspaceId: "w1", tabId: "w1:t1", paneId: "w1:p1", sessionId: "sid-1", agent: "claude", cwd: "/p", online: true });
  });

  const res: PanesResponse = {
    hosts: [{
      kind: "herdr", stream: true, control: true, new_tab: true,
      workspaces: [{ id: "w1", label: "proj", cwd: "/home/me/proj", branch: "main" }],
      tabs: [{ id: "w1:t1", workspace_id: "w1", label: "agent-1" }, { id: "w1:t2", workspace_id: "w1", label: "shell" }, { id: "w1:t3", workspace_id: "w1", label: "split" }],
      panes: [
        { pane: "w1:p1", terminal: "pane:w1:p1", tab_id: "w1:t1", workspace_id: "w1", agent: "claude", status: "idle", cwd: "/home/me/proj", session_id: "sid-1" },
        { pane: "w1:p2", terminal: "pane:w1:p2", tab_id: "w1:t2", workspace_id: "w1", title: "zsh", cwd: "/home/me/proj/web" },
        { pane: "w1:p3", terminal: "pane:w1:p3", tab_id: "w1:t3", workspace_id: "w1", title: "zsh", cwd: "/home/me/proj" },
        { pane: "w1:p4", terminal: "pane:w1:p4", tab_id: "w1:t3", workspace_id: "w1", title: "zsh", cwd: "/home/me/proj" },
      ],
    }],
  };
  const [sp] = buildTerminalTree(res, []);

  it("a workspace carries whether its host makes worktrees", () => {
    expect(termSpaceNode("m1", sp, true).canWorktree).toBe(false);
    const [withWt] = buildTerminalTree({ hosts: [{ ...res.hosts[0], worktree: true }] }, []);
    expect(termSpaceNode("m1", withWt, true).canWorktree).toBe(true);
  });
  it("a workspace carries its directory and branch", () => {
    expect(termSpaceNode("m1", sp, true)).toMatchObject({ kind: "workspace", workspaceId: "w1", cwd: "/home/me/proj", branch: "main", host: "herdr" });
  });
  it("a tab with one pane is that pane too", () => {
    const shell = termTabNode("m1", sp, sp.tabs[1], true);
    expect(shell).toMatchObject({ kind: "tab", tabId: "w1:t2", paneId: "w1:p2", agent: "", cwd: "/home/me/proj/web" });
    expect(keys(nodeActions(shell, deps()))).toContain("node-agent");
    const agent = termTabNode("m1", sp, sp.tabs[0], true);
    expect(agent).toMatchObject({ sessionId: "sid-1", agent: "claude" });
    expect(keys(nodeActions(agent, deps()))).toContain("node-copy-session");
    expect(keys(nodeActions(agent, deps()))).not.toContain("node-agent");
  });
  it("a tab with several leaves the pane open, a pane is its own", () => {
    const split = termTabNode("m1", sp, sp.tabs[2], true);
    expect(split.agent).toBeUndefined();
    expect(keys(nodeActions(split, deps()))).not.toContain("node-agent");
    expect(termPaneNode("m1", sp, sp.tabs[2], sp.tabs[2].panes[1], true)).toMatchObject({ kind: "pane", paneId: "w1:p4", agent: "" });
  });
  it("the sidebar lists the plain shells only", () => {
    const shells = shellsOnly([sp]);
    expect(shells[0].tabs.map((t) => t.label)).toEqual(["shell", "split"]);
    expect(shells[0].tabs.flatMap((t) => t.panes.map((p) => p.pane))).toEqual(["w1:p2", "w1:p3", "w1:p4"]);
    expect(shellsOnly([{ ...sp, tabs: [sp.tabs[0]] }])).toEqual([]);
  });
});


describe("session menus keep pane operations in the terminal toolbar (#10239)", () => {
  it.each(["herdr", "orca", "tern", "botmux"])("removes pane controls for a %s session", (host) => {
    const node = sessionNode({ machine_id: "m1", session_id: "s1", pane_id: "w1:p1", space_kind: host, cwd: "/project" } as Session, true);
    const items = keys(nodeActions(node, deps()));
    for (const key of ["node-split", "node-close-pane", "node-copy-id"]) expect(items).not.toContain(key);
    expect(items).toContain("node-copy-session");
    if (host === "herdr") expect(items).toContain("node-git");
    expect(keys(nodeActions({ ...node, paneId: undefined }, deps()))).not.toContain("node-split");
  });
});
