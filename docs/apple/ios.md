# iOS / watchOS App

「连接 hub」默认提供密码登录：输入 HTTPS 地址，在系统登录窗口输入管理员设置的连接密码，通过一次性授权码和 PKCE 返回 App。密码使用 OPAQUE 验证，原文不上传；设备凭据沿用现有独立撤销机制。扫码和手动 token 入口仍可切换使用。详见 [协议](../protocol/protocol.md#客户端密码登录)。

XcodeGen source is `apple/project.yml`, generating `apple/Persona.xcodeproj` (generated files are not committed). iOS 18.6 / watchOS 10 (iOS 26-only APIs fall back before iOS 26, #6730: `apple/Persona/DesignSystem/SystemCompat.swift`, `docs/lessons/ios.md` "iOS 18.6 floor"; the watch App must install on older watches stuck on watchOS 10), Swift 6, default MainActor isolation. Depends on MarkdownUI.
Performance rules, budgets and how to measure: `docs/apple/performance.md` (read before changing `apple/` code).
Bundle ID comes from configuration (`app.bundle_id`, watch `<id>.watchkitapp`, `docs/operations/config.md`). The UI supports English, Simplified Chinese and Japanese (`en` / `zh-Hans` / `ja`).

## Side sessions (#7944)

Who it serves: people who use Agents with the Claude desktop app. A desktop app's side chat, /btw question or spawned
session (`parent_agent_id`) lives in the chat of the session it was started from, not in any list. Default: on with a
hub that lists `sessions_side_fold` in its hello, nothing to set; with any other hub everything stays as before (plain
rows).

- **The rules** are `apple/Shared/SideFold.swift` (Foundation only, nonisolated, built for watchOS 10 too). A row the
  hub folds under a listed row on its machine (`folded_under`) as a side (`folded_as: "side"`) or as a row of a side's
  chat (`folded_side`) leaves the list; one whose parent the app does not hold stays a row, so nothing is out of
  sight. The parent shows and counts in its folded state (needs you when it or a row of its sides does; working when one
  of them works, unless it is unread or awaits a reply; unread when it or one of its sides is), its sides' pending
  cards added to its own and its time the newest of theirs. The rollup comes from the side rows the app holds, so
  seeing a side updates the parent at once; only a row whose sides it does not hold goes by the hub's `children.sides*`.
- **One fold, one place**: `AppModel.sessionsByMachine`'s didSet (`sessionsChanged`) folds once per push, and the paged
  Agents window folds in `AgentListProjection.build`, off `body` (performance.md P4/P5). `allSessions` (the lists, the
  filter and header counts, Needs You, the tab badge, the app icon badge, the Persona tab's dock, widgets, the watch
  summary, machines pages, next-needs-you) is the folded list; a session's own page still finds a side by key
  (`session(_:)`). Siri and Shortcuts fold what they read (`IntentSupport.folded`), and the watch folds its own reads by
  the groups the phone's summary names (`WatchSummary.sideFold`). Each hub's last word is remembered
  (`SideFold.remembered`), so a restored list shows no sides before the hello.
- **With the child fold (#8010)**: `SideFold` runs in front of `ChildSessions`. A side is no child card in its parent's
  chat (`AppModel.foldedChildren` leaves it out), its own unread makes its parent unread, and in a paged window
  (`roots=1`, which holds no side) the sides the child index reads for an open parent fold into the lists too, so the
  chip and sheet list them and a side's page finds its row.
- **Cards**: a side's card is shown on its parent by its `folded_under`, as every folded child's
  (`Interaction.requestHome`): one Needs You row for the parent, counted once, its line "From Side Session · <title>".
  In the parent's chat it is a card among its own with that line under its title, and it is left out there while that
  side's page is shown; a row of a side's chat (`folded_side`) has its card in that side's chat too.
- **Entry point**: row two of the parent's header (`contextRow`) leads its trailing group with the Side Sessions chip
  (`SideSessionsChip`: `Theme.Icon.sideSessions`, the count, the needs-you dot while one waits), drawn like the row's
  activity pill; ⋯ has "Side Sessions (N)". Neither shows without sides, and nothing opens by itself. A hosted chat (a
  task's sheet, a side's own page) has neither.
- **The sheet** (`SideSessionsSheet`, `.medium` / `.large`): the sides flat, sides of sides too, the ones that need you
  first, then the working ones, then the newest, the stopped ones last; each row its state mark, title and state word.
  A tap closes it, then the side's own chat (`SideSessionPage`, a `ChatView` hosted as `TaskDetailSheet` hosts a task's
  agent) rises over the parent's on a phone, or is pushed into the detail column in regular width; ✕ / ‹ goes back to
  the parent, and the title's ⌄ switches to the other sides. No New Side Session: the desktop app makes them.
- **Landing**: a notification, a Needs You card or a link that names a side opens its parent with the side shown
  (`AppModel.openChat` sets `sideLanding`; a `ChatView` given a folded side opens its parent the same way).
- **Words** (`Localizable.xcstrings`): Side Sessions, Side Sessions (N), From Side Session · <title>, A side session
  needs you, No Side Sessions; zh-Hans 分支会话 (「来自分支会话 · <title>」), ja 子セッション (「子セッション・<title>から」).
- **Not yet**: a search hit inside a side (`search_hits[<id>].hit_session_id`) lands on the parent without the side.
- Screenshots: `-openSession <machine>/<session> -showSideSessions chip|sheet|side` hangs three sample sides under that
  session (one asking, one working, one stopped) and opens nothing, the sheet, or the first side;
  `-showSideSessions <machine>/<session>` hangs them under that session and opens nothing (the Agents list).

## Agents recently used view

The Agents grouping menu adds Recently used (Mac: View › Group by), still stored locally in `dev.grouping`; defaults and existing grouping choices stay unchanged.
Pinned sessions appear separately once; remaining sessions flatten across projects and machines, newest `updated_at` first, ties by full session id including machine and hub; missing timestamps last.
Working sessions also use last activity, not work-start time; seen, idle and not-running history appears directly, no folding into projects or Idle spaces.
"Used" means messages or work progress, not opening a page: reading a reply does not change its activity time or position. Machine / hub filters, search, quick state filters and row menus remain; collapse / expand actions are hidden in this view.
The existing `AgentsFeed` and scroll protection remain; structure updates only when activity timestamps change relative session order, with reordering paused while browsing to preserve reading position.
Mac's previous / next Agent follows pinned order then recent activity in this mode; other modes retain their order.

## Pinned directions (#4352)

One tree from the hub drives the sidebar, the To-dos page and the Agents page: `GET /api/v1/assistant/directions/tree`
(`DirectionTree` in `apple/Shared/DirectionTree.swift`, `APIClient.directionsTree`, 404 → nil). It is the app's only copy:
the sidebar's lead rows (#4331: their "N need you" and, #6626, their spinner from `running`), the directions' names and order come from it too. Each current leader
conversation (a direction, shown to the owner as Pinned / 常驻 / 常駐) with the open to-dos it supervises, each with its one
state (`needs_you` / `review` / `stuck` / `working`), its Agents and their sub-Agents; the secretary group (书记官, the main
conversation's id) last. The app neither groups to-dos by supervisor nor works out their state: it draws the tree.

- **Reading it**: `AppModel.refreshDirectionTree` into `AppModel.directionTree`, read on every hello of the hub and on the `{"type":"directions"}` event (`ServerEvent.directions`); an answer older than the last
  one taken is dropped (a newer read that fails does not count, so it never throws away a good answer); a 404 leaves it
  nil, another failure keeps what was read. `refreshAssistant` reads it only while none is held, in its own task after
  the settings land (it runs on every page's appear; Couldn't Load Settings does not wait for the tree). Only the default manager's page reads it (the tree is the default
  manager's); cleared when another hub group or manager is picked, and a read already under way is dropped. An answer
  also updates `directionNames` and the leader order, and a direction the conversations do not list yet reads the
  assistant again (not when read from `refreshAssistant` itself, `followLeaders: false`).
- **Words** (`StateName`, `[State Names]` keys): 在做 / 等你 / 卡住 / 待验收 (en Working / Needs you / Stuck / Awaiting
  review, ja 作業中 / 対応待ち / 停滞中 / 検収待ち, the same words as #4331's counts beside them; 待验收 reuses the
  supervision badge's key). What a direction has going reads the same everywhere (#6626): the working spinner
  (`WorkingIndicator`) while anything under it runs (`DirectionTree.Direction.running`, the hub's one rule; from an older
  hub without it, a working to-do counted), then "N need you" when to-dos wait on you (`DirectionCountsText.label`,
  `DirectionTree.Counts.label`), nothing when neither; never "N working" beside the spinner (`RowStateMark` on the
  sidebar's lead rows, `DirectionCountsLine` in the To-dos and Agents section headers). Tones: Needs You's text for 等你, Working's
  for 在做, `Theme.cautionText` for 卡住 and 待验收 (`DirectionTaskStateWord`). Agents keep their own state names.
- **Sidebar**: `docs/apple/sidebar.md` "Pinned directions".
- **To-dos page** (iOS and the Mac's `MacTodosPage`): in List, a Group By menu right of List | Calendar (`TodosModeBar`, `TodosGrouping`, stored in
  `todos.grouping`): Status (the default, the sections as before) or Pinned (`Directions`). Only a hub with the tree
  offers it. Pinned: a section per direction with a to-do, in the tree's order, headed by its name (书记官 for the
  secretary group; `AppModel.directionTitle`, the lead row's title when the hub gave no name) and its counts; its to-dos are the page's own rows (`todoRow`, swipe and checkbox included) in the
  tree's order (`DirectionTodoSections`; on the Mac its own rows, selection included); a search narrows them; Done stays
  folded at the end as by status. A stored Pinned shows Status from a hub without the tree (`TodosGrouping.resolved`).
- **Agents page**: Group By › By Pinned (`DevGrouping.direction`, not the default; offered and kept only while the hub has
  the tree, else a stored choice shows By To-do, or By Project without to-dos). A section per direction (fold id
  `direction:<id>` in `dev.folded`, open by default) headed by its name and counts; in it each to-do's row (its icon,
  title and state word; a tap opens its card), its Agents a level in as the list's own rows (`slotRow`, live state), their
  sub-Agents further in; a sub-Agent the list does not have is drawn from the tree (state dot, title, its state in words)
  and opens its session. The listed Agents the tree does not have end in By To-do's Started Manually and Unassigned
  (Unassigned folded unless one of them needs you), so no Agent disappears. In By To-do itself (#6458) both groups
  start folded below the to-dos unless one of their Agents needs you, open when no to-do is above them, stay listed while
  empty when no quick filter or search narrows the list, color their count by their most urgent Agent's state (needs you,
  finished unseen, working: `TodoAgentGroup.mostUrgent`, a to-do's order), and say what they hold behind an ⓘ beside
  their title (`InfoTip`, a popover) and as an open, empty group's only row. Under a quick filter or a search a to-do
  without a matching Agent is left out. No Sort in this grouping (the tree's order). `DirectionAgentSections` builds it,
  `AgentListRows.directions` is the Mac's ↑ / ↓ order.
- **Tests**: `DirectionTreeTests` (decoding, the 404, the event, the words, the leaders / names / counts the sidebar
  reads, the folds, the To-dos sections and their fallback, the Agents sections and their Mac order, the grouping
  offered); `AssistantLeadersTests` (#4331's names, counts and words, read from the same tree).

## iPhone Agents 分组入口与长列表（#5766）

- 快速筛选下方增加独立 List 单元中的分组菜单，始终显示当前分组名称，不与搜索输入框共用单元。菜单与原 ⋯ 菜单共用 `dev.grouping`，重启后保留选择。已有分组和默认值不变，平铺视图仍叫「最近使用」。
- In the new shell a **Personal Agents ›** pill sits right of the grouping menu (`AgentsPersonalPill`, #140) and opens the personal agents page: on a phone the shell has no toolbar, so this is the only way there. When the two don't fit on one line (large text) they stack. The iPad and Mac keep the toolbar button and show no pill. UI test `PersonalAgentsEntryTests`.
- 分组折叠继续使用 `dev.folded`，搜索时展开匹配内容。
- 长列表首次显示前 20 项并提供「显示全部 N 个」；不超过 25 项时全部显示。适用于最近使用、状态、待办、机器的活跃行，以及项目和工作树的活跃项；快速筛选或搜索时不截断。Mac 不截断，保留 ↑ / ↓ 导航行为。
- 「显示全部」的展开状态与分组折叠分开，不跨重启保存。项目内一个多 Agent tab 仍作为一项，不拆散 tab。
- 行数上限只是展示策略；是否改善帧率，以相同基底、相同数据和相同工具链的实测为准，不从行数减少推导性能收益。

## Information architecture

Expanded Idle spaces in Agents reuse `AgentListOrder.ordered` across normal cards, compact lists and capsules, newest activity first (#2443).
`SpaceGroup.sessions` keeps original tab order, preserving tab numbers and ordering on space pages; capsule-card bottom rows follow displayed order.

| Tab | Content |
| --- | --- |
| Persona | **Persona assistant** (renamed from Chat 2026-10-30): Claude **running headlessly** in `~/.persona/assistant` on the hub machine (`claude -p` + stream-json, outside herdr, token-by-token streaming; tap the centered top-bar avatar, menu Launcher: …, to change launcher / model anytime, switching with `--resume` from the next message; same menu has Session info; home model label sets new-chat model, default configured in Settings). Hub-generated CLAUDE.md explains computer access (ssh), Lark and other work data (lark-* skills), iPhone (phone_*) and mail (mail_*). Home has greeting + the token calendar (#9506) + 6 suggestions; sending immediately shows a bubble and Preparing while the session starts in background. Thinking is hidden; tool calls fold into one line "Tools N · Computer, iPhone…". Top bar and sidebar below. Knowledge (`KnowledgeView`, rebuilt 2026-10-31): top type capsules (Projects / Topics / Chats / People / Documents / Meetings / Calendar / Recordings) with no count, each as wide as its title in the selected weight, so none moves when another type is picked or a type's list arrives, in the plain style and BeautifulUI alike, and each section below says how many it holds (#7507, as the web, `docs/web/web.md` "Knowledge base"; `KnowledgeKindPillsUITests`); phone Recordings contains local recordings and controls, transcripts automatically sync to Knowledge, `docs/apple/recording.md`), top-right tag funnel; projects grouped Focus (P0 / P1), Other active, Completed, with Not projects collapsed. Rows: type tile (initials for people/chats, importance color) + name + one sentence + priority / long- or short-term / relevance / fitting tags. Full-text results merge by entry, one row with name and at most two matches, bold query; tap opens the entry. Details: tile, name, badges, sentence, all tags, Ask Persona / Start Agent; then project Agents, vertical newest-first timeline (6 by default), profile, related entries by type (4 each, expandable), code directories, last 7 days' messages, directory path; top-right Tag edits importance / relevance / tags / project type and state. Confirmed facts (#6997, `KBCorrections.swift`) follow the timeline: add, edit in a sheet, delete with Undo; a related entry held in the entry's own list has Remove Link (swipe, context menu), asked on its row first, then both entries are read back. A project row's swipe and context menu delete it (#7077): asked first, then Undo restores it from the hub's trash. Topics (#7251, `KBTopics.swift`, `KBTopicViews.swift`) are the kind after Projects: rows ranked by importance then point count with "N knowledge points", the knowledge map as the first row (and a toolbar button); a topic's page shows its points by subtopic instead of the profile, a point opens in a sheet with its sources; the map (`KnowledgeMapView`) is an outline (Topics / Projects), with a detail column on a wide iPad; Settings › Knowledge Selection starts with the curation switch (`assistant.kb_topics`). Screenshots `-tab assistant -openAssistantPage knowledge`, add `-showKnowledgeEntry <kind>/<id>`, `-showKnowledgeLower YES` to skip header and inspect timeline, `-showKnowledgeSearch <query>` for search, `-showKnowledgeFacts YES` / `-showKnowledgeUnlink YES` for the facts sheet and the Remove Link question, `-showKnowledgeDelete <slug>` for a project's delete question, `-showKnowledgeKind topics`, `-showKnowledgePoint YES` (a topic's first point), `-showKnowledgeMap YES` and `-showKnowledgeMapPoint YES` for topics and the map. First chat is **main chat** (`assistant.main`, marked Main chat in history), later new chats are branches; first launch opens main chat. Other pages' Hand to Persona (Agents long-press, Knowledge Ask Persona) sends to main chat (`AppModel.askPersona`, creates one if absent) |
| Development | On a compact phone, an empty search field starts collapsed, pulls down to show and scrolls up to hide. A nonempty query stays visible and editable while results scroll or settle and when Agents returns to the front; clearing it restores the empty-field behavior immediately (#7332; `docs/lessons/ios.md` ("Layout and components")). The regular-width Agents full search already keeps its field in a fixed bottom safe-area inset outside the scrolling results; that behavior is unchanged and has the same nonempty/empty coverage. Phone quick filters are the list's first row and scroll with it, still above the list in wide split layouts. `AgentFilter`: All / Needs you / Unread / Working with counts, immediate, combined with machine filter and search; not remembered, returns to All. Filter keeps the Agents matching when selected (`kept`), so reading does not remove them; they stay in place with gray dots, spaces and rows sorted only by time for sequential reading; reselecting or changing machine recomputes. Unread has Mark all as seen at top (confirmation). Needs you is one row per session that waits for the user (`AgentsNeedsYou`, `spec/fixtures/needs-you/`): its pending cards under it, oldest first, its place over the first only (in a card of its own a line parts them, in the list's cell room does); a session the hub says needs you with no card read, a Terminal Needs You row with the hub's reason (else its last line) that opens it, or, when it has no terminal Persona can reach (`Session.canTerminal` false: a Claude Code run in a plain terminal, #5889), a Waiting on Your Computer row with the computer's mark, the hub's reason and no chevron, which opens the conversation only to read what it asks: the note above its composer lists the open AskUserQuestion's questions and options, read-only (`AskUserAnswers.pending`), and the question's tool row shows them too once opened; a card on no session, a row of its own. The section's header, the Needs you filter and the Agents tab's badge count these rows, the badge less muted agents and quiet tasks' agents (`docs/assistant/managers.md` ("Counts")); the section shows only under All / Needs you and respects machine filtering. Top-left menu Display, the same choice as Settings › Appearance › Layout (#2530, `AgentListDisplay`, remembered in `agents.rowStyle` and `dev.compact`): three row styles for the rows with the agent's latest words, in every grouping and pinned (By Repository too since #3753, a tester's decision that replaced #3182's one-line tree, whose note on the Layout page and in the Display menu went with it; the BeautifulUI style keeps its capsules, the Mac and Apple Vision Pro their rows) — A Avatar (default: the agent's mark in a round well with its state on the corner; title and time on a line; two lines of its latest words with a Needs you chip at their end, one line and a size smaller in a regular-width sidebar, an open iPhone Duo or an iPad; then where it runs: the source tag first, the tab, space › tab, the machine only when the list mixes machines; not the agent's kind, which the mark shows), B Two lines (state dot, mark, title, time; one line of the latest words, Awaiting reply / Waiting in the background / Draft before them, the source tag at its end), C State cards (a card each on the page, the state's color down its leading edge, the state as a colored word, the one waiting for you tinted; Idle spaces a card each too, their agents' cards under them when opened) — and Compact list (`CompactSlotRow`: state dot, title, tab (Space › Tab outside project grouping), time). The Layout page draws two sample agents in each. Compact list uses one `CompactList` scale (requested 2026-10-31: harmonious sizes): three font levels (group titles = space / Needs you / Idle spaces / state / machine use `heading`, row titles `secondary`, metadata = tab / time / count / idle-tab name `caption`); every row (Agent, N more idle, Collapse, Idle spaces, Mark all as seen) shares height and padding (at least 44 pt, below), leading marks (state dot, disclosure, space icon) align, titles align left; right side only time, aligned in a column; idle tab names are small text, not capsules; idle spaces are single rows (name, tabs, time), tap lists Agents beneath, no DisclosureGroup (different height and arrow). Space-title ⋯ uses quiet gray `ellipsis` like counts; Needs you is also one row per session (`CompactInteractionRow`: blue type icon, Plan awaiting approval / Claude has a question…, Session · Machine, +N for its other cards, time, ›; open chat to answer; card mode retains quick-action cards). Single-row groups are capsules from iOS 26 grouped-list corners (custom backgrounds clipped too), like Settings, accepted 2026-10-31. Collapse all / Expand all. Tap space titles to fold (`dev.folded`, remembered; folded row keeps state counts); state / machine titles also fold. Space pages open via title ⋯ › Open space, also Mark all as seen (N). Grouping menu (`DevGrouping`, remembered): Repository (default for never-chosen grouping, #2108, below) / To-do (#1836; each incomplete to-do with Agents gets a section; unattached manually started herdr sessions (`startedByPersona` false) in expanded Manually started, #2917; remaining unattached sessions last in Unassigned, collapsed when to-dos exist unless Needs you; with herdr and other hosts together, non-herdr rows show host, e.g. an alternative host; hubs lacking to-dos use Project) / Project / State (Needs you → Unread → Awaiting reply → Working → Waiting in the background → Waiting → Muted → Seen; Seen collapsed by default, expands directly to all Agents without N more idle (2026-10-31: formerly two taps). Waiting means no new message but `/loop` not due; background commands / subagents count Working, #2228; hub triage waits (`WorkState.hubWaits`, retries / scheduled wake-ups) also Waiting, #2895; Working that only waits in the background has its own section before Waiting, #6045) / Machine (each machine section with online status and state counts, state order then last update, idle folded) / App (#3756, `AppGroups`: a section per app the agent runs in, from `AgentKind` and the node's `agent_surface`: the Claude desktop app, the Codex App, any other agent's desktop app by name, then a section per terminal engine an agent runs in (#6567, `AppGroups.engine`: the host the node placed it in, `space_kind`: herdr, Herdr App, Orca, Tern, in that order; the agents Persona and its assistant start there), then Other for every agent outside both (the command line in another terminal or tmux, a Lark group's topic, a runtime, a node that does not say); titles as the list names them elsewhere ("Claude desktop app", "Codex App"; an engine's as Settings › Machines › Engine names it, with a terminal mark); running and offline agents alike, the latest first as the apps' sidebars list them (`AgentListOrder.recent`), no Sort; within a desktop app's or an engine's section, by project as the desktop apps' sidebars group them (#4366, `AppGroups.projects`): a heading per project, the latest project first (the Codex App's in the App's own order first, the node's `space_index`: `AppGroups.inAppOrder`), its agents a level in (the first 8, then Show All), each heading folding its project (`dev.folded`) and naming its machine when the section holds more than one machine's. A project is the repository a session runs in (`RepoPlace`: the node's `repo_root`, else a working tree's main checkout from its path, else the folder), named as its folder; a Codex App chat's is the node's space (`codex-project:<root>`). An agent with no project is under Other, last, its rows naming where they run: a home directory; a Codex App chat in no project, which the App keeps in <home>/Documents/Codex/<date>/<name> (the node names it by that folder) or which has no folder (`codex-chats:`). A section whose agents are all in Other lists them without headings, and Other's own section is not by project; each section folds like a machine's, with its state counts and how many agents it holds, offline ones included; an app with no agent has no section). Subtasks are rows of no grouping (`Session.isSubtask` and `isNestedSubtask`, the hub's `IsSubtask` and Android's rule, #4767): a Codex subagent (#4313; `thread_spawn`: `is_subagent`, `parent_session_id`, `parent_thread_id`, `root_session_id`) is under the chat that spawned it, found among the Codex sessions as before, and a runtime's session that says it is a subagent or names its parent (a Paseo agent another agent created, #4494) is under the session its `parent_session_id` names on its hub and machine, else its `root_session_id`, whichever agent runs either; terminal panes never are. Each is behind its parent's Subtasks (N) row, as the Codex App's sidebar and Android (`CodexTaskForest`) show them (`CodexTasks`, `apple/Shared/Subtasks.swift`). The row is closed until tapped (per agent, not remembered) and open while searching; a subtask's own subtasks are a level further in. A search matching only a subtask keeps its parent. A subtask that needs the user is not folded: it is a row of its own, in Needs You and counted, until it no longer does; one opened under a quick filter and answered keeps its row while the filter is on (`kept`). In the groupings that list what runs (all but Recently Used and By App), a subtask the list draws (live or pinned) hangs under its parent only while a row above it is drawn too: a working agent whose creator stopped is a row of its own, where it would otherwise be drawn nowhere. No count includes a subtask, wherever it is drawn: the quick filters, the state counts on the headers (`SpaceGroup.count`), By Status's sections, and the Machines page's tallies and agents (`MachineDetailView.agents`); a paged group's header counts are the hub's (#3346). The main conversation's dock leaves out a subtask while a session above it is going too, whose pill stands for it (`DockAgent.going`); one that needs the user keeps its own. Repository (`RepoTree`, like Orca project sidebar): computers outermost, titles at page edge one font level above groups, state counts, tap folds whole computer using shared machine-fold storage; hub order. Repositories within each computer sort naturally like Finder (app2 before app10), cards contain worktrees (⌂ main first, then names; omit worktree level if only main), Agents indented below. Fully idle worktrees fold to Idle; fully idle repos into Idle repositories at that computer's end. Spaces with `parent_space_id` (Orca child workspaces, #3075, web #2073) attach under the top-level parent's worktree rather than infer from own cwd: after Agents, N children row with state counts, default folded, remembered under `<worktree>/children`; expands child spaces, each worktree row then Agents one level below; deeper levels flattened depth-first. Missing parent or cycles fall back to cwd grouping; other modes remain flat. Repository and worktree: when the node sends `worktree` (#1805, read off git for every session, with `repo_root` and `branch`), group by `repo_root` (else the worktree itself) and then by `worktree`, labelled with its `branch`; the main checkout is the worktree equal to `repo_root`, and a named branch keeps its level even when the project has only its main checkout (machine → project → branch → Agents, as Orca shows it); `RepoPlace.git`. When the host place names a linked worktree's Agents carry (`slot_name` without a `slot_id`: a Paseo workspace's title) agree on one, the worktree is headed and sorted by that name instead of its branch or folder, its fold id still its path and its branch still on it (#4768, `RepoTree.hostPlaceName`, the web's and Android's rule and cases, #4497): an Agent without one neither votes nor blocks, two different names or a `slot_name` beside a `slot_id` (a tab's label) leave the branch; the main checkout keeps its name (several workspaces can share it), and so does a worktree with an Orca Agent in it, which Orca names. From an older node, when the session carries `repo_root` (the host knows its repository, #2840: Paseo keeps its worktrees under its own folder, so the path names no repository), group by it: a cwd inside it is the main worktree, any other cwd one of its worktrees; otherwise infer from the cwd like the web (`.worktrees/`, `.claude/worktrees/`, `.herdr/worktrees/`, `docs/web/web.md`). If all repo Agents are in one herdr space (usual), that space's pin / mute / to-do / ⋯ acts on the repo; multiple spaces leave only Mark all as seen. Project grouping also starts by computer, omits machine names from space titles, each computer has its own Idle spaces. Hide computer level when only one has Agents or a machine filter is selected. Collapse all folds only spaces/repos to one row, not computers. Quick filters choose Agents before grouping; computer titles use prefilter data, so filters do not add/remove computers. Non-project row metadata includes Space › Tab (plus machine under State). Three groupings share `ChatsView.rank`: Needs you → Unread → Awaiting reply → Working → Waiting → Muted → Seen, newest update within state; muted Working / Unread / Waiting count Muted, Needs you still first. Project mode retains original sessions (not assistant chats): Needs you cards → pinned Agents (once only, status via dot like other rows, excluded from space/state/machine/app groups; Agents in spaces use state order; `SpaceView` keeps herdr tab order; watch keeps `AgentState.priority`) → pinned spaces → active → just finished → muted spaces with running Agents (dimmed) → Idle spaces (folded, includes muted idle spaces). Search, machine filters. Every row, heading and fold control a finger taps is at least 44 pt tall at the default text size, in every grouping and display (#3753): `Theme.Size.tapTarget` is the list's least row height; a row that is one plain button (a fold such as N more idle or a Codex chat's Subtasks, Show All, a tab's row, a line of By Repository's structure, a Compact List search result) has no room of its own above or below, its label carries it and is the whole row (`ButtonRowInsets`, `ButtonRowLabel`), because a plain button takes taps on its label alone; folding headings (`FoldButton`, a space's title) are `TouchHeight` tall, a fold chevron a `tapTargetFrame` square (`AgentsTouchTargetUITests`). Rows do not swipe on iOS (#1244: on the way from Agents to a chat a sideways drag kept opening a row's buttons): their actions are in the row's long-press menu (`AgentRowMenu`; the ⋯ that also held them went in #2530: Pin / Unpin, Interrupt while it works or needs you, Mark as Seen when unread, Rename…, Mute, …, Close Agent…); the Mac keeps its trackpad swipes. Close is also in the chat ⋯ › Close Agent…; any herdr Agent can close after bottom `CloseAgentSheet` confirmation: Agent, machine/tab, consequences, Stop and close / Cancel. A session without a terminal (an agent runtime's, such as Paseo's) offers Close and Rename… where its row's `capabilities.close` / `capabilities.rename` say so (#6294, `Session.canClose` / `canRename`; absent from an older node: a terminal's, as before); its sheet says only that the session ends, and its rename is the session's name alone. Its model picker shows where `capabilities.model` is true, listing its launcher's models once read (no fixed Claude aliases) and sending a pick through `POST …/modes` (`Session.switchesModel`, `SessionModes.offersModels`); the composer's "/" list offers `/model` only to Claude Code in a terminal pane, which reads the typed command (`Session.takesModelCommand`, the web's rule, #8144), since a runtime's session would get the words as a message; Rewind to Here where `capabilities.rewind` is true, with its own item ids (`<seq>#<n>`) and Rewind Code and Chat only where `capabilities.rewind_code` is true (`Rewind.offer`). The Mac app (frozen) keeps the terminal rule in its own Agent menu (`MacAgentCommands`); its rows' swipe offers Close by `Session.canClose`, as the long-press menu does (#8144). Space-title ⋯ pins space. Pins stored on hub (`/api/v1/pins`). `/loop` Agents show "⟳ Next 12:20" and do not fold into idle; turn-ended Agents with background commands / subagents show amber "⚙ Background N", Working (#2228), not folded, their spaces not idle; when the hub says all that background work only waits (`background_waiting`, #6045: `sleep`, `gh … --watch`, `tail -f`; `Session.waitsInBackground`, still `state` working so stop / queue act as before), it shows as "Waiting in the background" with one mark everywhere, a clock in Working's amber made lighter (`Theme.Icon.backgroundWaiting`, `StatusPalette.backgroundWaiting`; the user's choice, as on the web): the row's dot and BeautifulUI badge, its words on the row (the compact row only when the title leaves room), its own By Status section, its own quick filter (shown while one exists), the folded counts, the chat header's dot and its background pill / folded mark; it is not Working in the Working filter, the state counts or the Machines tally (`Session.countedState`, it counts as idle there); the watch says it too; older hubs never send it; long-press can stop loop. Rename spaces through title ⋯ / space-page Rename space (updates herdr workspace); Agents via long-press / chat ⋯ Rename… (updates both herdr tab and Claude session name, consistent with Mac herdr / claude --resume). Mute Agents by long-press / chat ⋯; spaces via title ⋯ / space-page menu (group-title long-press does not trigger): no push (Agents can override muted space with Enable notifications and vice versa, `Pins.togglingMute`); dimmed (gray state, 🔕, opacity 0.8, weaker than Working but not fainter than idle spaces), running ones stay expanded, nonrunning muted spaces are ordinary idle spaces; excluded from Agents badge (`/api/v1/mutes`). Same menu Sort (`AgentSort`, `dev.sort`, remembered; hidden under State, already state then time) offers Recently updated (#823): rows only by last update, regardless of state; repos within computers, active project spaces and machines sort by newest Agent (muted spaces stay behind); pins, Needs you, idle folding unchanged. Uses `sortDate`: Working Agents use work-start time so each step does not move them. Default State first, unchanged |
| Inbox | Lark messages and email together, one row per item (new messages in a chat, an email thread), AI triage, swipe left to mark handled; a section bar on top (Needs You · In Progress · FYI · Done · All, the Agents list's `CapsuleFilterBar` with the grouping menu at its end) shows one section at a time and opens on Needs You on every entry (#9053, `docs/assistant/inbox.md` "Section bar"; its counts are `CountBadge`s in the capsule through `CapsuleFilterItemData.countLook` the strip scrolls a choice landed on or tapped into sight, clear of its edge fade, through `revealsSelection`, and drops the grouping menu under In Progress and Done through `showsTrailing`; all unset on the Agents filters, which keep their plain numbers and centred taps); source / account / type filters (`docs/assistant/inbox.md`). Lark opens a chat like mobile Lark: others left with avatars, first consecutive group message shows sender; own right without own avatar/name, blue bubbles (#D1E3FF / dark #133063); tap avatar/name for profile, reaction toggles own reaction; long-press menu starts with 6 common emoji (hub `reactions/quick`), tap reacts and closes; ☺︎ or long-press React opens common + all emoji. Moments shares render as cards opening full post, images and comments. View original and a Received messages chain entry whose source content is a native chat open that chat directly, at the item's first new message or the linked position, without an extension's intermediate item page; report only messages actually displayed as read. Rendering below. Email: details, reply, compose, Let Claude handle, account management (top-right menu), add account (also ask Persona "Help me add an email account", `mail_account_add` with phone confirmation) |
| To-dos | List/details (sidebar To-dos with incomplete-count badge; new shell sidebar): priority Waiting for you > To review > In progress > Follow-up > Not started, completed at bottom; create title + description, tap circle or swipe right complete, swipe right reopen, swipe left delete. Details edit title/description, show current and past Agent sessions, remove failed starts; bottom Hand to Agent starts new or picks existing session with structured to-do context card |
| Settings | Assistant (no model displayed/configured here; Model configuration › Main chat, #1291; Proactive work and memory pages, `AssistantProactiveViews.swift`: Scheduled tasks row-end toggles, existing edits save on leaving; new + with Meeting preparation / Weekly report / Agent check / End-of-day review templates uses Cancel / Add sheet so trying a template does not create it; Run now / View last result switches to Chat and opens that chat; one-time follow-ups, interval and scheduled tasks share page/editor; run settings model, permission mode, effort, Allow everything confirmation. Memory parses frontmatter, title prefers Chinese MEMORY.md index title, type badges, Markdown details, Edit whole file, swipe delete, read-only index row; About me edits notes, saves on leaving). Claude Code settings (allowlisted per-machine `~/.claude/settings.json`: exit recap, effort, always think, reply language etc., apply to all; then the computer's Live Replies switch, see "An agent's live reply", and Blocked Config Folders); Machines (online, session count, version, launchers, reconnect); Connectors (2026-11-01: This iPhone = old phone-data toggles/permissions; Email = old Inbox ⋯ account management; Lark = hub gateway status and purpose, `ConnectorsViews.swift`, `GET /api/v1/lark/status`; with hub `connectors`, add rows below Lark from `GET /api/v1/connectors` (2026-11-02, GitHub first: status, account, gh login from <machine>, last sync, generic `HubConnectorView`, DEBUG `-openConnectorGitHub YES`; in-App sign-in connectors (Telegram, `signsInHere`) add enable toggle, stepwise login, sign out, delete stored data, `ConnectorSignInSections`, DEBUG `-openConnectorTelegram YES`)); Notifications; Pair again |

Inbox event detail (`InboxEventViews.swift`): the header's small line uses `InboxNow.meta` for **where · who: newest line** when `where` exists, and keeps the old source · title fallback otherwise. `InboxNow.text(for:)` supplies a state sentence for every known `now`, including done states and states with a suggestion; an absent or unknown key keeps the old status line. A state key alone opens the event page. `InboxNow.symbol(for:)` is the single row / detail mapping: waiting uses the still clock, only queued / looking / agent_working spin, action states use Needs you (failed is red), and done / FYI / folded draw no mark. The processing-state row offers **Say something** when an open item has no suggestion card, or **Try again** after processing fails. Both post `version: 0`, `action: "instruct"` through `InboxSuggestionModel`; Say something keeps the user's words on failure, Try again sends the localized "Please try again." A stale answer and a request error use the suggestion card's existing notice and error. The timeline folds to its newest three entries (all of them when there are at most three), oldest of the visible entries first, and **Show all (N)** restores the complete chain. Each icon follows `via`: a source id uses that source's brand or generic source mark; `assistant`, `agent` and `user` use the persona, Agents and person marks; without `via`, the existing kind icon stays. A `reported` entry always uses `Theme.Icon.reported`, a neutral compact **Assistant says** `Chip` and secondary text. Its words come from `detail` when present, otherwise `text`, so the server’s older-app prefix is not spoken twice. That detail is not repeated in the expansion; a reported row with non-empty detail and no link or messages has no inline expander and shows its full text in place; legacy text-fallback rows, reported rows with a link or messages, and every other kind keep the existing per-row two-line limit and expansion behavior. For each linked Agent session, only its newest Agent or Agent-stopped entry shows that session's current `AgentState.label` in its `tone`; earlier entries and unlisted sessions show nothing. `InboxTimelineFixtureTests` replays every case in `spec/fixtures/inbox/timeline.json`; `InboxDetailTimelineUITests` covers folded / expanded chains and both instruction entries with `MOCK_INBOX_EVENTS=1`. `InboxDetailNowTests` checks the header fallback, now-only event selection and every status-symbol class; `InboxDetailNowUITests` checks waiting, suggestion and done details with `MOCK_INBOX_SHOWCASE=1`.

Chat page:

- Messages: user bubbles (coral), assistant Markdown (no bubble, reading first), thinking (one folded row), grouped tool steps (>2 fold into "N steps · Run 5 · Read 3",
  always showing the last step), to-do lists (TodoWrite rendered directly), red/green Edit diffs, Bash commands and output, images.
- A long finished answer in an Agent chat (from 6,000 bytes or 80 lines) is several lazy rows, about 24 lines each (`AnswerParts`, `AssistantTextPart`, #4970):
  one row of a 30,000-character report was laid out whole, twice, on opening and redrew every table's decoration on each scrolled frame. Cuts fall only
  between plain blocks whose reply-theme margins are fixed (paragraph, ATX heading, fenced code, table), each part drawn the gap MarkdownUI would put
  there; lists, quotes, display math and HTML comments are never cut. A long press on any part lifts the whole answer and acts on all of it: Copy, Select
  Text, Quote and Forward (#5221, `AnswerPartsLift`: the parts stacked again in a lazy stack, only those near the screen laid out, placed from where the
  pressed part is; all of them laid out once its words are to be selected, under one text view); its file chips sit
  under the last part. Persona's own conversation (boxed answers) and the Mac keep one row. `AnswerPartsLookTests` checks the parts draw the same pixels as the whole.
- Expanded work is one card: each step remains a separate lazy-loaded chat row (a turn may have hundreds; one giant view would hang), with each row drawing a card slice
  (`WorkCardSlice`: top corners on first step, bottom corners on Collapse N steps, backgrounds extending halfway into spacing to join neighbors), thin separators starting at the text column; tool rows 44pt high.
  **Liquid Glass (default) has no card** (requested 2026-10-31: cards did not match chat home, `StyleSpec.workList = .timeline`): steps sit directly on the page, with a thin left vertical line from first step
  through Collapse N steps, circular tool icons on it (page background behind icons masks the line), medium-weight tool verbs; intermediate Agent remarks use smaller gray text (`AssistantText(quiet:)`),
  reading as process rather than reply; thinking, folded steps, to-dos, tool details and Collapse start at the text column right of the line. The icon column's leading edge is flush with the assistant reply bubble (`WorkCardSlice.iconLeading` = `Theme.Space.flush` inside the list margin; a card keeps `Theme.Space.m`), and the rail runs through the icon centres, including at larger text sizes (`Theme.sizeScale`). Other styles keep cards (`.card`).
  Background slices extend halfway into spacing (`WorkCardSlice.bridge = Theme.Space.l / 2`): the old hard-coded 7 for 14pt spacing left 2pt white seams after spacing changed to 16.
- One `DiffView` (Claude Edit / MultiEdit / Write supplies old/new text; Codex unified diff), sharing Git row models (`DiffText.Line` / `parse`):
  header filename, directory, +N −N (opens file); only ± and row backgrounds are colored, code uses body color; long lines scroll horizontally with right fade; beyond 40 lines show beginning + Expand all.
  Old/new text is compared in PersonaKit's `LineDiff` (#2915, a Myers line diff like the web's `compare.ts`): unchanged lines stay as plain context, two or more in a row that are over 3 lines
  from any change fold into one "… N unchanged lines" row until Show All (which opens through `toggleInPlace`, the diff's top pinned), and a removed line paired with the added one after it (first with first) marks its changed words on
  `Theme.Diff.addEmphasis` / `delEmphasis` (words as in the web's `code-review.ts`: ≤ 1,000 characters, ≤ 500 words, similar enough; Han and kana count one character per word).
  Every +N −N (header, tool chip, changed files) is that diff's. Past its budget (1,000 lines between the common start and end, 100,000 characters a side) the middle is all removed, then all added.
  A newline ending both texts (or a Write's content) ends the last line, as git counts; ending only one of an Edit's texts (pieces of a file), it is a line break put in or taken out and shows as an empty line added or removed.
  Whole-file Git diffs still use `MonoTextView` for large-diff performance, same models and colors. Tool failures put Error above output instead of covering its first line.
- Image viewer: every full-image entry in the App (chat images, sent images, images viewed this turn, tool results, Files images, notebook outputs, Markdown web images and diagrams,
  Lark messages / cards / Moments / documents, Inbox connector messages, Lark attachments) uses one `ImageViewer` (`ImageViewer.swift`, `ViewerImage` data:
  image bytes, computer file or Lark URL), styled like Photos:
  - Top: circular Back · two-line title capsule (name; "9 / 20 · source or dimensions") · circular ⋯ menu (Copy image, Annotate, Copy path, Open in Files, Info);
    bottom: thumbnail strip for multiple images (current slightly wider, tracks paging centered); circular Share · capsule Save (add-only permission) / Annotate / Info · circular Copy. Icons only, fixed 44pt,
    text scaling capped at xxxLarge, fits iPhone SE. Bars and buttons use design-system `ViewerTopBar` / `ViewerBottomBar` / `ViewerIcon`
    (`DesignSystem/ViewerChrome.swift`), also used by message Share previews, keeping both identical (#4714).
  - Background follows light/dark (`Theme.Viewer.canvas`); single tap hides bars and turns black (`immersive`); Mac single tap does not hide (Close and Esc are in the bars).
  - Gestures: pinch / double-tap zoom, pan when zoomed, swipe to switch, drag down at fitted size to dismiss: image shrinks with finger, background fades, release closes or springs back by distance / velocity (`ZoomMath.pullProgress` / `pullCloses`);
    system zoom-transition drag dismissal disabled via `.interactiveDismissDisabled()` to avoid competing gestures. Thumbnail entry points use zoom transitions.
  - Paging on iOS is a UIKit paging scroll view (`ImagePager.swift`, `ImagePagerView` / `PagerScrollView`), not a page-style `TabView`: SwiftUI updates only hand pictures to existing pages,
    `index` changes once a swipe has come to rest, and the pages move to the nearest whole page whenever they come to rest off one (`PagerMath`, #5678). A page that leaves the screen goes back to fitted.
    `-showViewerDecodeDelay 0.35` makes pictures arrive late (UI test `ImageViewerTests.testSwipesAlwaysEndOnAWholePage`).
  - Editing (`ImageEditor.swift`, the edits in `ImageEdits.swift`, tokens `Theme.ImageEdit`, #4963): on black as Photos edits, it opens on Box, as it
    always has. Box and Pen draw in 6 inks and 3 widths (thin 2, medium 3.5 (the default and the old width), thick 7 pt on screen, burned in at the same
    width relative to the picture as seen); Crop shows the whole turned picture with a frame whose corners and edges drag (inside moves it; at least
    64 pt; dimmed outside, thirds while dragged), a shape (Freeform, Original, Square, 4:3, 3:4, 16:9, 9:16), Rotate (a quarter to the left, the crop
    and its shape with it) and Reset. Undo and Redo, in the top bar, step through every change (a mark, a crop let go, a turn, a shape, a reset, Clear).
    The source is never written: Done hands back an `EditedImage` (source, edits, history, and the picture drawn from them in CoreGraphics off the main
    thread, at most 1600 px), and the composer keeps it beside the photo (`ComposerAttachments.edited`), so a photo tapped in the strip opens with its
    crop, turn and marks as they were, still undoable; the photo removed or sent, its edits go. From the viewer, Add to Message adds the result as
    next-message attachment (`AppModel.composerInbox`), ready to send with a note; only available when opened from a chat. Pending input images (a
    conversation's and the Persona tab's) can be tapped to edit before sending. Turned, the row of tools that does not fit at large text sizes shows icons,
    and scrolls (Clear staying put) where even the icons don't fit; the options row scrolls.
  - Screenshots: `-openFirstImage YES` / `-openViewedImage N`, `-showViewerChromeHidden YES` hides bars, `-showViewerInfo YES` opens info.
- Background sync (push + pull like IM; only foregrounded and connected): hub pushes which session changed; App pulls increments by cursor.
  Agents (`AgentSync`): changed `rev` in session list / `revs` triggers `after_seq` prefetch into the disk cache only for chats not retained in memory,
  so opening is already current; scope active Agents + sessions used within 24 hours (at most 40, newest first, concurrency 3).
  Only while an Agents list is in sight (#10582; `AgentsCoverWatch` through `.tabActiveTask`: the list not covered by the shell's other page,
  another tab or a pushed page, the phone's Sidebar drawer out, the iPad's column not folded, its tab in front and the app active): elsewhere
  it read 7–14 transcripts and rewrote about 0.7 MB of cache a minute on a busy hub. Leaving cancels the pulls on their way (no failure, no
  `workingGap` hold); coming back pulls what moved meanwhile at once, then the pacing below. A chat opened from elsewhere (a notification, a
  link) opens from its cache and its subscription replays what came after it. A pull that changes nothing shown leaves the file as it was.
  Retained chats without a subscription owner keep their store and covered sequence unchanged until their next visible subscription replays `since_seq`.
  Eligibility is rechecked when queued work starts; a visible catch-up cannot apply after the last owner leaves or the store is replaced.
  Currently viewed chats rely on their subscriptions without duplicate fetches. When a subscription stops (the session's `rev` moved since the
  chat's newest read was sent, or its `updated_at` is newer than the open chat's newest line, and no items came for 4 seconds), the chat fetches
  after its `covered` and merges silently: at most every 10 seconds, once per `updated_at` (`AgentSyncPacing.catchUpAt`, the rule of the
  web's `web/src/lib/stale-page.ts`). The `rev` catches the last line of a burst whose push was lost, which `updated_at` cannot (#1179); a push never settles a moved rev, only a read sent after the move (a subscription or a catch-up, `AgentSyncPacing.RevReads`, #3191). Working Agents fetch at most once per minute (`AgentSync.workingGap`; each fetch decodes and rewrites the entire disk cache),
  immediately at turn end / Needs you (most likely to open); 19-minute real-hub replay: 45 calls/minute → 9 (2026-11-02 battery investigation). Lark (`LarkSync`): hub forwards gateway live events (`lark`), refreshing Inbox Lark chats and open chats
  (polling fallback only: list 90 seconds, chat 60); while a Feishu list is in front, changed Inbox chats fetch the newest page into `lark-chat-<id>` cache, and with none in front they are only noted, the newest 20 fetched when one comes back (#4693; a chat opened before that shows its cached page until its own fetch lands); on hub connection catch up latest 20;
  unread chats open from cache if it reaches the first unread message.
- Settings › Storage (`StorageView`): space used on this phone, size and share bars by category (Agent chats, chat images, Lark images/files, lists/Lark chats, websites,
  network cache, other, App data); tapping a category asks to clear it, saying how much it frees and what comes back (#8355; a left swipe and the long press are shortcuts), or Clear all caches (settings and pairing retained; websites clear cache, not login).
  Memory copies clear too; background sync refetches. DEBUG `-tab settings -openStorage YES`.
  Without the user: the first time the app goes to the background in a process, `CacheBudget` trims `chat-images` and `lark-media`
  oldest first from over 200 MB to 150 MB and clears WebKit's disk cache over 200 MB (links `WebViewPool` preloads; logins stay), #8341.
- Lazy images: history pages and pushes omit image bytes (`lazy_images`, reducing pages from MB to hundreds of KB), dimensions only; visible rows fetch through
  `ImagePayload.ref` (`ChatImageLoader`: one download, NSCache + `Caches/chat-images`, immutable images valid until `CacheBudget` trims them),
  reserving dimensions before arrival to avoid jumps. Thumbnails and full viewer share it; locally sent images retain inline base64.
  A failed load (hub unreachable, download failed) does not spin forever (#182): tool-result and file-page images (`Base64Image`) say
  "Image didn't load. Tap to retry" on the placeholder, and the viewer's page says so with Retry, keeping its bars (and Close) and the
  page's own backdrop while it is the page shown; downloaded bytes that are not an image are not kept on disk.
  Sent photos (`SentPhotos`, the sending bubble, both `ChatThumbnail`) and the images viewed this turn (`ViewedImageThumb`) use the
  same placeholder (`ImageNotLoaded`: narrow ones fall back to "Retry" or the symbol; bytes already here that don't decode say
  "Can't show this image", without a retry); a file's thumbnail known only by path keeps its symbol (`FileThumbnails` keeps
  that failure for the session, #559). Their thumbnails sit inside the tile's button that opens the viewer, so they record a
  failure in `ImageRetries` and that button asks the hub again instead of opening the viewer, and VoiceOver reads it as the retry.
- Links (`Views/Link/LinkSheet.swift`): chat http(s) opens an in-App bottom card like Telegram: full screen immediately, at 60% pulled down the chat remains usable, further down closes;
  behind a 60% card chat still scrolls and accepts input (`presentationBackgroundInteraction`). Lark docs / Knowledge use native `FeishuDocReader`,
  other websites `WebViewPool`: preload first 3 links in each appearing reply, at most 6 resident WKWebViews (LRU), sharing default disk website data (cache, login),
  requests `returnCacheDataElseLoad`, opening takes the preloaded view. A view serves one opening (#4074): when its sheet closes the pool drops it, with the
  page the user browsed to and its back/forward history, and starts a fresh one on the link, so every tap opens the link's own page with no history from
  earlier openings (a machine's dev server loads again only when opened). Inside the sheet an edge swipe goes back and forward. Selected web text gets AI edit (enter request, immediately send to Agent) and
  Add as context (quote strip above input, sent with next message); native Lark docs use title-bar Add as context for the whole document. Message context is
  `<persona-context kind="context|edit" title="…" url="…">selected text</persona-context>`, raw tags hidden.
- This-hub links (#2696, `Shared/HubPageLink.swift`, `App/HubPageRouting.swift`; the route table every client follows: `docs/protocol/deep-links.md`): assistant links to this hub's web pages (#2194) open native App pages, not website cards:
  `/s/<machine>/<session>`, `/t/<machine>/<space>/<slot>/<session>` open Agent chat; `/persona/<id>`, `/persona/<id>` open Persona chat; `?at=<item id>` scrolls and flashes the message
  (same as quote navigation; fetch a few older pages if missing, otherwise stop at latest); `/todos?id=<id>`, `/todos/<id>` present to-do details. Session ids may be unique prefixes (web #2464).
  This hub matches host + port, ignoring http/https: paired group addresses, active socket address, mesh endpoints, hello `public_url`; reverse-proxy subpath must match its prefix.
  Other sites, `/todos` board, `/inbox`, `/machines` and unmatched App routes, other hub groups' to-dos and Persona chats with unknown machine still use website cards. Watch messages render no links.
- Quotes (#986, requested 2026-11-03, Telegram-style; `DesignSystem/QuoteViews.swift`):
  - **Swipe left to quote** (#1058, Agents chats): drag own messages or Agent text replies left, not work rows.
    - Bubble follows finger, reply icon revealed right; past `MessageSwipe.threshold` light haptic and accent-filled icon; farther motion rubber-bands.
    - Release springs back, adds quote strip, focuses input; below threshold does nothing.
    - UIKit `LeftDragRecognizer` immediately fails unless initial motion is horizontal left; outer scroll views (list, shell paging) wait for failure, preserving vertical scroll and left-edge right-swipe Back.
    - It also fails when the finger lands on something that scrolls sideways and still has room to the right (a wide table or code block, #5737, `DrawerPan.Coordinator.sidewaysScroller`): that scrolls; at its right end, on a table that fits, or on a plain reply the drag quotes (`SwipeWideContentTests`, `MOCK_WIDE=1`).
    - Not enabled for assistant chats, whose left swipe belongs to shell paging.
  - **Above input**: long-press Quote, website Add as context and @session become strips inside input above text.
    Each has accent vertical line, title ("Quote Claude", "Quote your message", "Selected text · Page", "Session …"), summary, right ×.
    One quote has two summary lines; multiple stack with one each.
  - **After sending**: same quote block embedded atop own bubble; bubble width equals its widest block. Tap:
    - Same-chat message (`kind="quote"`, `msg="<message id>"`) scrolls to original and flashes (`QuoteFlash`);
    - If not loaded, expand full text;
    - Website excerpts open the page.
  - **Styles**: Liquid Glass and others use Telegram line + text (pale bubble backing); BeautifulUI uses a small card with line on its left edge.
  - **Not quotes**: hub Lark /agent instructions/docs (kind instructions / doc) still fold into one row (`ContextGroup`).
  - **Lark chats**: reply strip also inside input; bubble quote navigates to original.
  - Screenshots `-showSampleQuotes YES` adds website excerpt and session above input; end-to-end `QuoteBubbleTests`.
- Comments (#151, #150; `Views/Chat/MessageComments.swift`, `CommentCards.swift`; iOS only, the Mac app is frozen): the `> Comment N` / `Code comment N` / `Element comment N` blocks a message leads with (docs/protocol/protocol.md, "Quote and comment blocks at the start of messages"), written and read back exactly as the web does, both checked by `spec/fixtures/comments`.
  - **Waiting**: `AppModel.composerComments` (kept across launches in the `composer-comments` StateCache file, a Draft on the Agents row like quotes) shows as a row of cards over the composer: kind and number, the review's scope, the file and lines or the element and its page, a line of the note, ×.
  - **Sending**: after the `<persona-context>` tags and before what was typed, numbered from 1; comments alone send without words.
  - **Sent bubble**: the blocks, from any client, are quote cards at the top of the bubble (kind, number, place; the note); a tap opens the whole note and the code. A plain `> ` quote before them is a quote card too; a message with no comment block is drawn as before.
  - **Edit**: the comments go back over the box as cards, the words into it.
- Forwarding a message (#3807; `Views/Chat/Forward.swift`, `ForwardSheet.swift`; iPhone and iPad, not the frozen Mac app): the
  `<persona-context kind="forward">` block and its metadata are in docs/protocol/protocol.md, "Forwarded messages".
  - **Entry**: the lifted message's bar has Forward where Share was (`MessageActionBar.forwards`: a sent message, in a chat that
    forwards), so the bar keeps its width; Share as Image is the first row of the picker. Messages not sent yet, the Mac and
    Apple Vision Pro keep Share.
  - **Forward To** (`ForwardSheet`): Share as Image, the message being forwarded, Persona (Main Chat, unless that is this
    chat) and New Agent…, then Recent (`ForwardMemory`: the last 8 targets per hub group, ids only, never synced, gone with
    the group) and each computer's live Agents in the Machines order, latest first (`ForwardTargets`: same hub, takes
    messages, computer online, never this chat or Health). Search matches titles, Agents, projects and computers, and also
    finds ended Agents (the send resumes them) and Persona's other conversations. The lists are the sessions the app holds,
    read once as the sheet opens.
  - **Picking a target** reads the whole message when the chat holds it cut (`/items/{iid}/full`) and the photos of your own
    message, then opens the target's chat (`AppModel.forward`) with the forward over its box (`composerContexts`, like a
    quote, × takes it off) and a line saying where it comes from and goes ("From MacBook Pro · to Codex on Mac Studio"), the
    photos in the box's tray (`composerInbox`). The note is typed and sent there, through the usual send. New Agent opens
    New Chat with the card over First Message, the block as its `contextBlock`. Persona with no main conversation yet: the
    new conversation's box.
  - **Sent**: the block is a card at the top of the bubble, as a quote's, with the arrow, "Forwarded from Claude · chat",
    the computer and time, and four lines; a tap opens the original chat at the message (`AppModel.openLinked`), or the
    whole text when this app does not have that chat. Nothing is written to the original chat.
  - **Several at once** (#3838, #9468, `MessagePick.swift`): the long-press bar's last row, Select Messages (under a
    hairline, `MessageActionBar.selects`: a sent message the chat can pick; as Telegram's menu ends in Select), or Forward
    To's Select Messages…, under Share as Image, starts selecting in the chat with the pressed message picked. Each
    message (your own, a reply, a relayed or another session's; not steps, notices, injected lines or `/loop` ticks) gets
    the Inbox Select's round mark on its leading edge, and a tap anywhere on it picks or unpicks it; nothing inside it
    answers. A long answer's parts (#4970) pick as the one answer, its mark on the first. The footer gives way to `PickBar`:
    Cancel (the chat as it was: the box's words, quotes and photos are the model's), "N Selected", Include Questions when it
    means something, and under them Forward · Copy · Export Image drawn as the long-press bar's actions, off with none
    picked; a pick that would send more than 30 shows a toast. Forward opens Forward To
    with the picked messages in the chat's order (`ForwardDraft.pieces`), without Share as Image or Select Messages….
    They go over the target's box as one bar (`QuoteBar` groups forwards of one chat: "2 Forwarded Messages · chat", a
    line each, one × for all) and are sent as one block each; the sent bubble shows them as one card
    (`ForwardGroupBlock`: the chat and computer, then each message's author, time and two lines, a tap opening it; past
    three, "N More"). Size: each cut at the smaller of 20,000 and 60,000 / N characters (`ForwardMessage.limit(count:)`),
    at most 30; photos share the box's limit. Selecting ends once a target is picked; cancelling Forward To comes back
    to the selection.
  - **Include Questions** (#9468, `PickQuestions`): a reply's questions are its turn's messages from the user's side (yours,
    and a relayed or another source's that opened it), in the chat's order. While a picked reply's question is not picked,
    the bar offers Include Questions as a chip; on, Forward, Copy and Export Image take those questions too, and their
    marks are filled in a lighter accent ("Goes with its answer" to VoiceOver). On by default; the last choice is kept
    (`pickIncludeQuestions`, read when selecting starts, written only when changed). What goes stays within 30.
  - **Copy** (#9468, `ExportText`): what goes as plain text, each whole (read when the chat holds it cut) as a forward
    carries it, "Me:" or the author's name over it, a blank line between; selecting ends with a toast.
  - **Export Image** (#9468, `ChatExport.swift`, `ChatExportRenderer.swift`, `ChatExportSheet.swift`): what goes drawn as the
    chat shows it, into a picture as wide as the phone (360–430 pt) at the screen's 2× or 3×, light or dark as the phone
    is, the text at the reading size (`.large`, never the phone's accessibility sizes: the picture is for others to read).
    On top the chat's title, the day (or days) and who answers, never the computer or project (Title and Date turns it
    off); then your bubbles with your photos, and each reply under its author: its Markdown in the share card's theme
    (`MarkdownUI.Theme.export`), cards as the share card draws them (`ShareCardParts`), ```math and ```mermaid drawn
    first and taken from `MarkdownRenderer`'s cache, a paragraph with inline formulas drawn as text with them in it
    (`ExportMathText`: MarkdownUI loads inline images in a task an offscreen picture never runs), pictures loaded first
    and laid out as the chat lays out a reply's (`ReplyImageLayout`, #9474: one 240 pt high, two side by side, three to
    a row past that), all of them (no "+N": a picture cannot open it), and at the foot of each page Persona's
    mark (`PersonaBrandLockup`) and "2/3" when there are several. Each block is drawn on its own and kept as a PNG; the
    pages are stacked from them (`ExportPages`) at most 16,000 px and 64 MB to decode each, broken between blocks, a
    block taller than a page cut at a row of one color (`ExportSlicer`); a code block over 60 lines is drawn in pieces.
    The preview (`ChatExportSheet`) shows the pages small, read from their files as they come into view, with Include
    Questions (when offered), Title and Date, and Share: the files go to the system's share sheet, whose Save Image saves
    them, so the app never asks for Photos. `-showExport replies|all` (DEBUG) picks every reply or message and opens it.
  - Tests: `ForwardTests` (`spec/fixtures/forward`, `several.json`), `ForwardMessageTests` and `ForwardSeveralTests` (the
    whole way on the mock hub, screenshots); `ChatExportTests` and `SelectExportTests` (`MOCK_EXPORT=1`, or `long` for a
    300-line log that takes a second page).
  - Screenshots `-showSampleComments YES` (two code comments and an element over the box); `MOCK_COMMENTS=1` gives the widget chat a review sent from the web, and Persona's main conversation a quote and a comment on its answer; end-to-end `CommentBubbleTests`.
- Files: message paths and Read / Edit / Write tool paths become tappable file tags, read from the session's computer
  (`GET /api/v1/machines/{mid}/file`, at most 2 MB, iCloud directories denied), rendered Markdown / numbered code / images / PDF / directories.
- Other Agents in the space (Agents): the left part of the header's second row is `SiblingTabs` (restored flat tabs at request 2026-10-31), one capsule each with state-color background
  (Needs you blue, Working orange, Unread green, Seen / Idle gray, no blinking dots; state-colored capsules kept at the user's request on 2026-11-06 over a neutral fill with a dot, `docs/archive/apple/plan-chat-topbar.md` Q2); current bold outlined.
  A name is one line cut at its end, at most `Theme.Size.tabName` wide (BeautifulUI's chip sizes itself, so it is cut by length, `SiblingTabs.capped`): one long name cannot push the current tab away.
  Shows current and fitting neighbors, horizontal scroll all, and an end fades while there is more to scroll that way (`.edgeFade()`); at the end the space icon and the tab count (not ⋯, the page's menu) opens the full list (dot, name, state,
  current checkmark; blue top-right dot when another Agent needs you), first All Agents in space X opens space page; chat ⋯ › All Agents in space same.
  Swipe tab rows left for Close (no full-swipe execution); list rows and capsules also long-press Stop and close Agent. Confirm if working, awaiting interaction, unknown, or holding background / queued / loop tasks; idle and completed close directly.
- Delete chat (#1749, web #801): nonrunning sessions (`Session.canDelete`: Not running / Done / Seen (idle) / Unknown, not starting, not assistant) offer Delete chat… in Agents long-press / chat ⋯ (Mac also Agent › Delete Chat…); system confirmation explains removal from client lists only, computer records retained, reappears when reused (`DeleteSessionText`). `AppModel.deleteSession` calls `DELETE /api/v1/sessions/{mid}/{sid}`, removes locally on success (matches next hub sessions push), exits every open page (split details, separate Mac windows, `AppModel.deletedSession`); failure shows hub message.
- Node menus (#3098, web #2329, `docs/integrations/terminal.md` ("Node menus")): running herdr Agent long-press adds Split pane ▸ Right / Down, Git ▸ Pull / Push / Fetch (pane cwd; Push asks first in a system confirmation, #1883), Close tab… (system confirms all panes and Agents close); space ⋯ adds Run Agent here… and Git ▸ (workspace cwd). Run Agent here… sheet loads `GET /machines/{mid}/panes`, lists workspace shell panes with no Agent/bound session and directly startable Claude Code launchers (Codex starts from New Session in a tab of its own; the footer says so only on a computer with a Codex launcher, `NodeMenu.runHereFooter`, #5275; screenshots: fake hub `MOCK_TERMINAL_PANES=1`, plus `MOCK_NO_CODEX=macbook` for a computer without Codex, and `-tab dev -showRunAgentHere macbook -showRunHereLauncher claude`), Run calls `panes/agent` and, once the list is refreshed, opens the started agent's chat (`NodeMenu.startedChat`, #3287); if no idle shell, create New terminal tab (`POST /panes`) then select. herdr only (not Orca, herdr-core or alternative hosts); rename retains Rename…. Logic `Views/Chats/NodeActions.swift` (`NodeMenu` pure + `NodeActions` execution/confirmation, `nodeActionPrompts()` on Agents list); menus stay Equatable, no changing push-data reads; the Git result is the node's `outcome` in the app's strings (`NodeMenu.GitOp.outcome`, else the node's message), a failure the hub's reason in the app's language in a red toast (#1888). `NodeMenuTests`.
- New worktree… (#3099, web #2431, `docs/integrations/terminal.md` ("New worktree…")): herdr space ⋯ sheet `NewWorktreeSheet` calls `worktree/prepare` for repo ("persona · Current branch main"), branch and checkout path (tracks branch like herdr/web until manually edited, clears to resume; immediate `git check-ref-format` validation), Create calls `worktree/create`. Git refusal shown unchanged; old hub/node says "Persona on this computer is too old; update to create worktrees". No phone terminal page to navigate to; after creation sheet becomes New chat prefilled with workspace / checkout directory (`SpacePreset`), starting inside new worktree. Logic `WorktreeForm` (`Views/Chats/NodeActions.swift`), tests `WorktreeFormTests`, `NodeMenuUITests`.
- A new agent's own worktree (#6115, web #6301, `AgentWorktree.swift`): New Agent asks `POST /machines/{mid}/worktree/status` `{cwd}` for the picked folder; when it is in a Git repository (`path` set) a Worktree section offers In a New Worktree, an optional branch (placeholder `persona/<first message or folder>`, as the node names it) and Checks out to `<checkouts>/<slug>`. Start then calls `worktree/add` and starts the agent in the `path` it returns; a refusal shows next to the start button and nothing starts. Close Agent (`CloseAgentSheet`) and Delete Chat ask the same status for the agent's folder; one Persona made (`ours`) adds Keep Worktree (default) / Remove, Delete the branch too, the counts it loses (`dirty`, and `unmerged` with the branch) and I understand these changes will be lost, which the button waits for. `worktree/remove` runs only after the close / delete succeeded, retries `worktree_in_use` for a few seconds, and sends `force` only for a confirmed loss. Delete Chat stays the system alert unless the agent is in one of ours (the computer is asked first, 4 s at most); then it is a sheet with the section. Mock: `MOCK_WORKTREES=1 MOCK_AGENT_WORKTREE=1`; UI test `AgentWorktreeUITests`.
- New Agent in one of the computer's workspaces (#7259, `WorkspaceStart.swift`, `WorkspaceStartPicker.swift`; the web's `lib/workspace-start.ts` has the same rule): when the chosen computer's catalog lists workspaces of an engine that keeps checkouts (`source` `orca` or `paseo`, `available` and `online`), a Workspace row above Folder opens Choose Workspace: Choose by Folder on top, then the workspaces grouped by project (`projectName`, else the last element of `projectPath`; none: Other Workspaces, last), main checkout first, then worktrees, then folders, each with its title (else branch, else folder name), Main checkout / Worktree / Folder with the branch when the title doesn't say it, and the folder. A pick hides the folder rows and the worktree switch, says the kind and folder under the row, and the start sends `workspace_id` and `host` = the row's source; a Git one offers New Child Worktree… (Orca) or New Worktree… (Paseo, made by Paseo through `worktree/prepare`/`create` with `kind`). Nothing is preselected (the folder stays remembered per computer); no such workspace (an offline computer, Paseo not enabled) shows no row. The groups are derived when the computer or the catalog's `revision` changes, not in `body` (P4); the catalog is read when the computer changes. Tests `WorkspaceStartTests`.
- New Agent in Claude's desktop app (#5480, `DesktopStart.swift`; the computer's side: `docs/integrations/claude-integration.md`, "New sessions in the desktop app"): with the launcher whose `source` is `claude_desktop` (`Launcher.startsInDesktopApp`), a folder not picked on the sheet (the one filled in from the last start does not count, `cwdPicked`) is a temporary session: the Folder row reads Temporary Session, Start Chat is ready, and the start carries no folder, which the computer turns into the app's scratch folder. A folder picked (a row, the browser, a card, a workspace) is used; Use a Temporary Session goes back; a space's sheet keeps its folder; a recent-setup card shows a temporary session as one. The directory browser starts in the folder that holds most of this computer's Claude desktop app sessions' folders (`DesktopStart.projectsFolder`, the computer's rule for the scratch folder, worked out when the computer or launcher changes) and offers New Folder…, a name in the folder browsed that the computer makes when the session starts. Run Agent Here does not offer the launcher. The Codex App's launcher `codex-app` (`source` `codex_app`, #5726) works the same way: no folder picked is a chat among the App's chats in no project (the computer gives it a folder where the App keeps those), still with its first message as any Codex start; browsing starts in the folder of the App's projects, its own folders for chats in no project (`<home>/Documents/Codex/<date>/<name>`, `AppGroups.isCodexChatFolder`) left out. Mock: `MOCK_DESKTOP_APPS=1` (the MacBook offers `claude-desktop` and `codex-app`); UI test `DesktopStartUITests`.
- Attachments in New Agent (#2321, web `NewAgent.tsx`, `NewAgentAttachments.swift`): under First Message, Add Images or Files offers the composer's sources (Photos, Take Photo where there is a camera, Files). Photos stay in the sheet as image attachments (at most 4, `MessagePhotos`, shared with the composer) and go with the first message; files upload to the chosen computer when picked (`FileUploads`, a row each with progress and ×) and their paths follow what was typed, one per line, as the web sends them (`FirstMessage`). Start Chat waits while an upload runs and says so. A file is on the computer it was uploaded to: picking another computer takes the uploaded files out, stops uploads still going, and a note under First Message says so; nothing is uploaded again and photos stay. A Codex launcher takes no attachments, as on the web: no add menu, and attachments picked before hold Start Chat back (Codex takes no attachments) with the web's sentence under them until they are removed; so does an omp launcher (omp takes no attachments), whose first message also goes with the start (`docs/integrations/omp.md` "Starting from the apps"). A to-do handed to a new Agent (`TodoHandoffSheet`) gets the same row, its context block still first. DEBUG `-openNewAgentAttachments photo,<file name>,…` attaches the made-up photo and uploads 3 MB made-up files once the computer's launchers are in. UI test `NewAgentAttachmentsTests`.
  Reuses `session.close`: mark local copy Not running only after remote confirmation (history/title/content retained); failure keeps confirmation with error, retry/cancel; no false success without hub.
  List directly observes active same-space sessions; closing others neither exits list nor switches current; closing current selects next active tab, otherwise previous, returns when all closed. Actions remain with one tab.
  On 10-29 changed to one-button `SiblingMenu`, hiding tabs/states; user disliked it.
  If entered from that space page, return directly, no extra layer (`spaceAt` in ChatsView tracks space-page depth).
  Space-top machine button ("Work computer ›") opens `MachineSpacesView`: online/state counts, active projects with newest sentence of most urgent Agent,
  idle projects opening spaces; Agents outside herdr spaces listed directly; top-right ⓘ machine details.
  DEBUG `-tab dev -openMachinePage <machine id>`.
- Terminals (#4754, `TerminalsView.swift`, `docs/integrations/terminal.md` ("Not implemented")): Agents menu › Terminals, and Terminals on a computer's page. Every pane of each computer: the Agents' under their titles, plain ones as Terminal with the program in front and the folder, a stopped Agent's left behind as "Finished: <title>". A tap opens the terminal cover (a plain pane by its `pane:` address); the computer's header menu has New Terminal per workspace and Close Finished Terminals; a plain pane closes after a confirmation. A plain terminal on herdr or Tern renames its tab (#6996, `TabRename`, the web's `tabs/rename`): its row's long press and its terminal page's ⋯ (in the bar's settings button's place, with Terminal Settings) open Rename Terminal, one field filled with the tab's name that takes up to 80 characters; an empty or unchanged name saves nothing (Save is never disabled: the alert would draw it enabled anyway, `docs/lessons/ios.md`). The new name shows in the list at once and goes back if the computer refuses it, with one plain sentence (offline, an engine that can't rename, a terminal already closed). An Agent's row offers the Agents list's Rename Agent… instead; a stopped Agent's leftover offers neither. The page reads the lists again every 8 seconds while it is in front (as the web's Terminal page does), keeping what is shown on a failure and redrawing nothing when nothing changed, so a rename on another device or on the computer shows up. A stopped Agent's chat shows one line where its composer was, with Terminal and Resume Session (#5521: it opens the terminal and resumes there), and its terminal page offers Resume Session. A computer never sits on a bare spinner (#6153): one whose hub isn't connected says so, and one with no answer after 15 s says "This computer hasn't answered yet." with the request's short id (`X-Client-Request-Id`; the hub reuses it as the request id but logs it only for a failed (400 or higher) or slow (over 2 s) request, so a quick 200 leaves no line: `docs/server/server.md` ("Operations")) and Retry; pull-to-refresh sends a new request for every computer and clears the old failure, keeping the list until the answer. The computer page pushes Terminals by value (`TerminalsRoute`), as the Agents menu does, so every stack that shows a computer's page declares that route. DEBUG `-tab dev -openTerminalsPage all|<machine id>`; `-showTerminalsUnlinked YES` shows every computer as not connected; the mock's `MOCK_PANES_DELAY=<seconds>` answers the panes late, and it refuses a rename to `offline`, `unsupported` or `closed` as the hub relays each. UI test `TerminalsRenameUITests`.
- Resume in Terminal (#5521, `Session.terminalResume(on:)` in `Shared/Models.swift`, `docs/integrations/engines.md` "Agents run in a terminal"): the chat's ⋯ has it under Terminal for a stopped Agent, a Codex thread nothing has open included (`Session.isStopped`); it brings the terminal up over the page (`TerminalCover` with `resumeOnOpen`), where the resume shows its progress and the Agent comes up. Where the computer enables no terminal engine (`engine_hosts: []`) it is disabled with "No terminal engine on <computer>" under it, and the terminal page says the same instead of offering Resume. Not there while the session runs, here or in a terminal of its own, while it starts, for the assistant's chats, the desktop apps' sessions or rows of older nodes. Not on the frozen Mac app. UI test `ResumeInTerminalTests` (mock `MOCK_RESUME_TERMINAL=1 MOCK_TERMINAL_STREAM=1`).
- Machines (`docs/server/machines.md` ("App")): all machines in one page, each card online/system/Agent counts (Needs you / Working / Idle), current CPU/memory/network,
  1-hour mini charts, alerts, Redeploy on version mismatch, urgent first. Details show Agents above (open chats), then charts
  (1 / 6 / 24 hours, press to scrub), system info, launchers. Below Engine: What this computer can do (#3018, node self-check by capability, expandable checks; stale generated files offer confirmed Repair, `docs/server/machines.md` ("Doctor: what this computer can do")); entries drawer Machines, Settings › Machines, Agents menu Machines (old Machine monitoring merged).
  Subscription Usage below cards (#2889, `docs/server/usage-windows.md` ("UI (iOS / Mac)")): Codex / Claude / gateway accounts, one row per window, hidden with no data.
  DEBUG `-tab dev -openMonitor all|<machine id> [-openMonitorRange 6h|24h] [-openMonitorScrub 0.55]`.
- Away recap: Claude Code away_summary becomes Away recap card, removing "(disable recaps in /config)"; disable in Settings › Claude Code Settings.
- Display logic (assistant and development unified, `chatLines` + `WorkLine`): turns split by messages, one chat segment per message:
  Your message → gray work row "Tools 5 · Thinking 1 · Replies 1 · 4 min 17 sec ›" (name then count) → Agent's last text before next message → next message…
  Active and complete look identical except active spinner, timer and terminal button ("Working 39 sec · Tools 2").
  While it runs, the line names the current step instead of its counts (#2987, `WorkNow`, every style, Agents and Persona chats): the newest call without a result in its running form, with the subject its step row shows ("Running go test ./server/... · 1 min 4 s", "Editing ChatView.swift · 2 min", "Reading 3 files" for reads running together; a command Claude described shows its description alone, already an imperative: "Run the widget snapshot tests · 1 min 16 s"), "+N" for other calls running at once; with none running, the latest step (a thought: its first line, else "Thinking"). Only the newest batch of calls counts as running, back to a thought, the agent's words or a call answered before the next began (an older call without a result never got one); notices and bookkeeping calls (to-dos, ToolSearch) are not steps; nothing known (only words, or the newest work folded by the hub) keeps the counts. The words give way in the middle or at the end on one line, "+N" and the clock stay whole (at the accessibility text sizes, where beside those the words kept a letter, the words take up to two lines, with room for both always kept so the line holds still as steps change, with "+N" and the clock on a line under them); they change at most about twice a second (`WorkLine.follow`, `WorkNow.hold`), and a step coming back after subagents or a fold is named at once, without the counts in between. Subagents it waits on keep their own words. A finished line reads as before.
  The queue strip above the composer (`QueueStrip`): each queued message's menu takes it back to edit, copies or deletes it. For a Claude in a terminal pane (`Session.steersQueue`, #6088) the strip also has Send Now (at one tap since #7169, as on the web, `docs/design/DESIGN_SYSTEM.md` 4.4; its VoiceOver hint says, as its confirmation used to, that a running command moves to the background and a reply being written starts again; `POST …/queue/send-now`, Claude Code's own send-now, which takes the whole queue; the hub's words for `nothing_queued` / `send_now_unsupported` / `send_now_unrecognized` / `send_now_in_doubt` are shown as they come), and each message Move Up / Move Down (`QueueOrder.moved`: the `/queue` `keep` in the new order). At the interface's largest text size (xxxLarge, where the accessibility sizes stop), unless the title and Send Now fit on one line, the title wraps and Send Now has the line under it, so the title keeps its words and a tap on it opens the strip (#4846, #7730; UI test `QueueStripTextSizeUITests`); smaller sizes keep the one line and cut the title short where they must. UI test `QueueSteerUITests` (`MOCK_QUEUE=1`; `MOCK_SEND_NOW=<code>` makes the mock refuse; `GET /mock/send_nows` lists the send-now requests it got, so one tap is one request). VoiceOver hears the strip's row as one element: Send Now is among its actions and the button's hint is the row's (`QueueStripHintTests`).
  A Persona chat shows the same strip (#4475): the user's own messages come first and keep those actions, with Send Now's hint in Persona's words (what it is doing stops, or goes on in the background when it can), while the last `queued_relayed` messages only show their arrival note ("Arrives when this chat is free": its turn ended and the owner not talking in it, `docs/assistant/assistant.md` "Messages from the system wait for the owner") and Copy. `AppModel` and the UI both index only `Session.ownQueuedCount`, so relayed messages cannot be changed even if a menu is wrong. UI test `PersonaQueueUITests` (`MOCK_PERSONA_QUEUE=1`, light and dark).
  Claude's queue keeps only text: photos become `[Image #N]` placeholders and an `<attached_images>` block of machine paths, videos and quotes their XML tags. `QueuedDraft` reads that back into what was written, so a row shows the words with a mark per kind of attachment, and Take Back to Edit puts the words in the box and the attachments in their bars (photos from this phone's outbox copy, else read from the machine by path; one that can't come back leaves its path in the words, #3022). A Persona hub instead returns the photos with `/queue`; those bytes go through the same off-main-actor decode and composer restoration. `QueuedDraft(photoCount:)` removes the composer's stand-in from a photo-only message. Pathless photos stay in the composer even beyond the picker's four-photo cap, but `sendNow` refuses them until enough are removed, and `sendText` clears nothing when refused; an unreadable preview keeps its original payload and says so. A Persona message sent from this device also shows its pending outbox photo count beside its queued words. UI test `QueuedAttachEditUITests` (`MOCK_QUEUE_SENDS=1` queues what is sent to the working widget Agent in Claude's shape; `/mock/queue?photos=N&text=…` queues one sent from elsewhere).
  Multiple queued messages in one turn still pair one-to-one after completion, each with its work/reply (#2512 removed merged-bubble completion); sent images are square thumbnails above message (`SentPhotos`, same as pending),
  stripping Claude "[Image #N]" placeholders from bubbles.
- Landing: Unread chats show only the last line of your last input at top, then this turn's work and answer (`landUnderInput`: jump by id, correct measured position; header collapses to slim bar and layout uses that height); other chats, Persona's conversations included (#1666), restore topmost row from last visit (`chat.read.<session>`,
  cleared when leaving at latest; in Persona's conversations only a transcript row is remembered, never a hub feed line or a day line, which may arrive after the transcript on the next launch). Landings within final screen land at bottom and follow new messages (`ChatView.landed`).
  Thus outputs always follow each input whether folded or expanded, no piled-up inputs or numbering.
  - Work row folds everything except final text: tools, thinking (new Claude Code narration uses thinking), system messages (background completion),
    intermediate replies. Tap lists steps inline in order (thin left line), tap same row to fold; Collapse N steps below last avoids scrolling hundreds back.
    Each expanded step remains independent (`ChatLine.workStep` / `.workEnd`), not a huge view. Folding/unfolding preserves viewed content
    (`toggleWork`, no animation): bottom Collapse keeps inverted list's latest end fixed, reply stationary and folded row at old Collapse position;
    title tap keeps title under finger (jump by id, correct measured offset after layout). DEBUG `-showFoldProbe YES` exercises it and writes movements
    to `Documents/fold.txt` (all should be 0).
    Expanding at latest raises title under the bar to reveal steps (#1173), per-frame increments (`ChatPin.rising`, 0.1 s time constant, at most one frame's movement per frame,
    so batched steps / a stalled frame do not jump), no ↓ while rising; collapsing at latest cannot retain title, pauses one frame then glides down (`ChatPin.glide`, about 0.35 s).
    Mac `NSClipView` clamps beyond-latest offsets on layout, so glide manually sets each frame and restores any displacement in the same layout (`ChatPin+Mac.swift`).
    Tapping thinking/tools/BeautifulUI tool-pill groups within open work keeps both title and tapped row still (no borrowing remaining title rise or replacing persistent hold with normal 1.5 s).
    Reduce Motion disables rise/glide and content fades (`Theme.Motion.reduced`, #1123).
  - Background tasks wake idle Claude (`wake`: a background command's notice, or a subagent's report as its call's
    result): the wake opens a turn of its own (`ChatBlock.build`, `Turn.wakes`, #9140). The turn before it had ended,
    so its last words stay its answer; the woken turn draws the notice where a message would be (a subagent's report
    is drawn under its call instead), then its own work row and its own last words, so the chat still ends on the
    newest words (#3783). Its work row is known by the wake (`w-<wake id>`) from before its first step, and is the
    live one while the woken agent works. Tasks completing during work (not waking) remain a work step, and a wake
    after an away recap follows the recap. Until #9140 the woken work stayed in the turn before it and its newest
    words took the answer's place, which folded a complete answer to a newer question into the work row once an older
    background command reported back. The per-reply token count follows the same exchanges (`chatUsage`).
  - Away recaps resemble background tasks: small "↺ Away recap · 21 min ago ›" strip opens `RecapCard` / `RecapDrawer` with full text. Consecutive recaps with nothing between merge into "↺ Away recap · 3 times · 3 min ago ›", newest first in drawer (`ChatLines.foldRecaps`, requested 2026-10-31); mock `MOCK_RECAPS=1`.
    On returning, if latest recap is the chat's last item and never closed, it expands at the end as a full card (`RecapStrip`, `ChatLines.openRecap`):
    same plane as Agent replies, newest inverted-list row, scrolling away with content and fully visible when following bottom, not above input
    (#1360: footer placement covered the end of replies when expanded and scrolled down). Tap or type to close, reverting to strip; closures stored per session in
    `closedRecaps`. Earlier folded recaps in that row stay as a strip above the full card.
  - Header auto-collapse only follows user drags and their inertial scroll, not programmatic landing deceleration, which otherwise collapses immediately on opening.
  - Work-row text never truncates; insufficient space uses short format with tool count and duration only.
- Can't reach hub banner appears only after 5 continuous seconds (brief network/foreground reconnects finish sooner; previously flashed), disappears immediately on reconnect.
- Layout: list above, input region (interactions / loop / background / queue / composer) below in VStack, no overlay; keyboard reduces page height, latest messages stay against input.
  An undocked iPad keyboard (floating or split, its frame ending above the screen's bottom) does not (#2565): it floats over the page, which keeps its height, and the composer stays at the bottom, as UIKit's keyboard layout guide does by default. SwiftUI's keyboard safe area took the floating keyboard's height off the bottom as if it were docked, and the composer stood that far up, half under the keyboard. `KeyboardState.isUndocked` follows the frames (Will Change Frame; a frame with no height or wholly below the screen, a keyboard going away, leaves it as it was, and so does Will Hide: undocking says Will Hide with the floating frame, and a floating keyboard comes back floating; a notification without its screen uses the app's scene's; Will Show always means docked: an undocked keyboard never says it, and turning the iPad with the docked keyboard up says it with in-between frames that end above the bottom), and `keepsHeightUnderUndockedKeyboard()` drops the keyboard's safe area on the chat page, Persona's new conversation (iPad page and shell) and Health's while it is set. An undocked keyboard is not `isUp`, so the pending cards keep their keyboard-down room; docked again it is up again, at its first docked frame (`KeyboardState.after`). `UndockedKeyboardTests`, `FloatingKeyboardTests` (iPad).
  A 1pt empty view above keeps the scroll view away from top safe area, eliminating its safe areas (inversion puts them at the wrong end) without ignoring them and manually padding.
- Messages from other sessions (`Item.from`): left gray From name card, Markdown body like replies (hub converts teammate JSON notices to Markdown).
- Inverted list (`chatScroll`: flip ScrollView and each row): latest offset 0, entry/follow/↓ jump exactly to 0,
  not estimated lazy heights (cause of occasional blank screens); older history loads at the opposite end without moving reading position. While browsing (`followsEnd` false), new rows are held outside
  the list (`frozenAt`) until returning to latest, preventing displacement. Never use `defaultScrollAnchor(…, for: .sizeChanges)`: inverted it anchors at the far end,
  and height changes move into unrendered blank regions. Inverted safe areas land wrong: ignore vertical safe areas and restore actual insets via `contentMargins`;
  iOS 26 edge blur measures the uninverted view and covers the page, so disable it (`scrollEdgeEffectHidden`).
- Local cache (`StateCache`, Caches/state): machines / sessions / pending interactions / assistant settings (`dev`), Inbox (`inbox-items`, `InboxStore`),
  mail accounts (`mail-accounts`), latest 80 messages per Lark chat (`lark-chat-<id>`). Cold launch, offline and tab switches display cache before silent refresh; no spinner when cached.
- History: turn-based paging, two complete input/answer turns (`turns=2`) on entry and at top, not item count; top spinner auto-loads,
  no Load earlier button. Latest two turns cached locally (`ChatCache`, Caches/chats; strip images beyond 6 MB), shown first on open/switch then merged with server,
  no blank. Turn start/end, own send and returning to Chat tab jump bottom (completion folds and shortens content, otherwise leaving blank).
  Following (`followsEnd`) starts at bottom; any finger drag disables it (history updates never move position); return bottom / ↓ / own send reenables.
  Only following jumps for new content: bottom edge then bottom sentinel id after row layout (`settleAtEnd`), retry until sentinel really visible
  (one id jump into unlaid-out content can land on older input; the "keeps jumping to earlier input" bug came from treating a passed sentinel as no retry).
  Entry positioning runs once, not again after full-screen terminal. LazyVStack estimated heights can land unrendered when row heights vary greatly; `atBottom` starts false,
  so a residual blank screen still shows Return to bottom. Top paging starts only after a manual list drag (LazyVStack may initially lay out top row,
  formerly cascading into loading all history on entry).
- Rendering: each message, timer, narration, count and reply is an independent LazyVStack `ChatLine`; active turns can have hundreds of steps,
  and a single VStack blocks the page with seconds of blankness.
- Cancellation is not an error: leaving/replaced requests (iOS text "Cancelled") never alert (`Error.shown` / `isCancellation`).
- Input: **assistant and Agent chats in the new shell share `ShellComposerBar`** (#1091, every style). `ChatView`’s `Composer` formerly checked only
  `shellChat` (set only by `AssistantView`), so Agents-pushed chats used `ComposerRow` / `PromptComposer` and looked different;
  now `shellChat || shell != nil` (`usesShellComposer`). Agent + menus additionally have Shortcuts (including /clear);
  BeautifulUI Agents therefore lost the Prompt Bar's @ and / menus (shortcuts in +); both came back for every style, / as `SlashMenu` (#2747), @ as `ComposerMentionMenu` (#2319).
- **Typing / opens commands** (#2747, matches web `AGENT_COMMANDS`): both non-Prompt-Bar composers (`ShellComposerBar`, `ComposerRow`, all styles),
  Agent drafts consisting of / plus a word show `SlashMenu` above input: /继续 (continue), /提交 (commit), /汇报 (report), /compact, /context, /model when selectable, /clear when allowed,
  same web ordering; command and description per row, quick-message original text right; further typing filters command prefixes/descriptions. Tap fills input, does not send
  (/clear uses existing confirmation); an exact inserted command such as /context does not reopen the list until draft changes. Beyond 5½ rows scroll internally.
  Assistant chats have none, like web, except the main chat keeps those rows and adds `/clear` when the hub has `assistant_main_clear` (#5100); that `/clear` does not ask first. In `Composer` VStack, not overlay, because the composer's own hosting controller makes out-of-bounds overlays untappable (#518).
  + Shortcuts unchanged; `-openSlashMenu YES` screenshots (`-openSlashMenu /re` opens it on that draft), `PersonaUITests/SlashMenuTests` both layouts.
- **Typing @ offers files, folders, workspaces and sessions** (#2319, workspaces #2599; matches web `ComposerPanel` / `lib/mentions.ts`): in `ShellComposerBar` (Agent and assistant chats, and the shell's
  new conversation, `StarterComposer`), an `@word` before the caret, at the start or after whitespace (an e-mail address opens nothing) and not while the keyboard still
  has marked text (Pinyin; `TypedText.composingFocused`), shows `ComposerMentionMenu` above the input where `SlashMenu` sits: the files this chat's tools read or wrote
  (at most 12, newest first, read once per opening), the session folder's entries (at most 12, folders first; `listDir`, the web's `/fs`, cached while the list is open),
  then Persona's workspaces (at most 8), then other agents' sessions (at most 8, most recently active first), 40 rows at most, scrolling past 5½. Typing filters: names that start with it first, hidden
  entries only for a query starting with ".". A query with a slash browses that folder on the session's computer (`@src/`; absolute and `~/` paths too), with no workspace or session rows.
  A file types `@path ` (relative to the session's folder) with the caret after it; a folder types `@dir/` and keeps browsing; a workspace or session takes the token out and adds its
  context over the box. A workspace goes as `<persona-context kind="workspace" machine="…" cwd="…" branch="…">` (`branch` when known), a session as a `<persona-context>` quote like any quote.
  On iPhone and iPad, Persona's conversations and the ordinary new conversation read `GET /api/v1/workspaces` once each time this list opens; Agent chats, Health, Mac and visionOS don't offer them.
  Each `HubLink` keeps its last good workspace snapshot in memory: a later opening projects it immediately, a different answer replaces it, and a 404, 503 or another failed read keeps it.
  Only the first opening after launch has no snapshot and can add rows when the answer arrives; there is no prefetch. Removing a hub group drops its link and cached snapshot. Workspaces match by title, folder, computer or branch,
  one per computer and folder (trailing `/` ignored, folderless rows omitted),
  title prefixes first and available before unavailable, stable in the hub's order. Their row's second line is computer · shortened folder (· branch). A new conversation has no files or folder: it offers
  workspaces and sessions, and what a typed absolute or `~/` path lists on the assistant's machine, as the web's does. The + menu's Mention a File or Session types `@` at the caret (the web's @
  button beside +; the iOS row has no room for another button). Logic in `Views/Chat/MentionMenu.swift`
  (`MentionToken`, `MentionRows`, `MentionWorkspaces`, `PersonaTests/MentionMenuTests`); `-openMentionMenu YES` (or a draft such as `@Sources/`) screenshots;
  `PersonaUITests/MentionMenuTests` against the mock hub (the weather agent's files and folder, and the mock workspace catalog). BeautifulUI's separate legacy `PromptComposer` @ menu is not this list.
- **iPad regular width (also open Duo) shares `ShellComposerBar`** (#2779): outside new shell (`RootView.usesShell` compact-only),
  no `shell` environment previously fell back to `ComposerRow`, differing from iPhone. `ChatComposerKind.padCard` (regular-width iOS) now makes Agent, assistant chats
  (`shellChat`) and assistant new chats (`AssistantHome` → `StarterComposer(shell:)` on the iPad, mode chips in model menu; the open Duo's is `ShellHome`, #3680) use it, centered within `ChatView.readableWidth`.
- **Drafts expand into a full-page editor and support one-tap keyboard dismissal** (#3167, #5685, like ChatGPT): when `ShellComposerBar` is focused for typing, holds a multiline draft, or reaches its viewport-dependent line limit (`ComposerGrowth.showsExpandButton`), an Expand button (`Theme.Icon.expandMessage`, `shell-composer-expand`, accessibility label `Expand the message box`) appears in the upper-right corner of the text area. It opens a full-page sheet (`ComposerPageEditor`, `.page` sizing on iPad, text limited to `ChatView.readableWidth`), with Hide Keyboard (`composer-page-dismiss-keyboard`, while focused) and Collapse (`composer-page-collapse`) in the top-trailing toolbar and Send (`composer-page-send`, or Interrupt when the Agent is working and the draft is empty) at the bottom,
  and quotes above the text. While typing in `ShellComposerBar`, a Hide Keyboard button (`Theme.Icon.dismissKeyboard`, `shell-composer-dismiss-keyboard`, accessibility label `Hide Keyboard`) appears beside `+` in the card's bottom row (`ComposerRow` also offers `composer-row-dismiss-keyboard` on iOS when focused), committing any active IME marked text and dismissing the software keyboard while keeping the draft intact. It edits the same draft (`text` binding, `model.drafts` still the sole source); pending images and files remain in the composer below and send with the message as usual.
  The page owns its cursor (takes the caller's `selection` on opening, returns it on closing; directly binding to the caller sent every cursor update through it with delayed returns, occasionally losing characters on iPad).
  While open, the composer's own field binds to an idle `parkedSelection`, so it cannot move the caller's cursor underneath the sheet. Focus bindings cannot cross into sheets, so the page has its own `@FocusState`:
  requests to focus the composer while open (⌘I) forward to the page, without showing a keyboard underneath. The composer is not treated as editing then;
  the page cursor follows the caller's only when that changed (⌘I alone does not move it).
  Collapsing, swiping down or replacing the page (chat switch, tree replacement) first commits marked input (`TypedText.commitFocused`, #2203); returning to the composer preserves keyboard and cursor positions.
  Sending from the page (toolbar ⌘⏎ shortcut) is identical to composer Send and happens once; the composer's ⌘⏎ is disabled while the page is open, preventing another send underneath. Hidden line measurement lays out at most 4 lines from the first 600 draft characters, counts trailing newlines, and stops while the page is open.
  The button belongs in the button row, not over the input corner: a corner overlay either covers text or rewraps it when appearing. iOS only; Mac unchanged.
  `ComposerExpandTests` covers assistant / Agent, keyboard opening/closing, paragraph wrapping, cursor transfer both ways, swipe dismissal preserving edits, one hub send with quotes; runs on iPhone / iPad / Duo, landscape with `TEST_RUNNER_PERSONA_ORIENTATION=landscape`. On an unfolded portrait Duo, XCUITest treats the screen as upside-down portrait and synthetic drags miss; `TEST_RUNNER_PERSONA_NO_DRAG=1` skips keyboard dismissal and swipe-down. ⌘⏎ tests require `TEST_RUNNER_PERSONA_HARDWARE_KEYS=1`: Xcode 27.2 + iOS 26.5 simulator delivers synthetic ordinary characters but does not trigger shortcuts (including the composer's existing ⌘⏎).
- Mac and Vision retain `ComposerRow`: Agent and new assistant chats share it (+, glass capsule input, Send, red Interrupt while working and empty,
  no dictation), with permission/effort capsules above (`SessionModeBar`).
- **One dictation and stop button** (#1079): `DesignSystem/ComposerButtons.swift` `ComposerMicButton` (outline mic, solid in the accent and pulsing while dictating),
  `ComposerRoundButton.stop` (stop in solid ink circle), 44pt tap target, one shell scaling step (`shellScaled`), used by `ShellComposerBar` and iOS `PromptComposer`.
  A text field outside the composer (a to-do's notes, #2188) uses `FieldMicButton`: the same tap (`Dictation.toggle(into:)`: what is heard goes
  after what was typed, a second tap ends it, why it could not start is a toast), drawn in the font and color of the text buttons beside it.
  Every mic takes a touch the same way (#9619, `DesignSystem/MicPress.swift`): let go before `Theme.Hold.delay` (0.3 s) it is a tap;
  held that long it dictates until the finger lifts (a light haptic as it starts), and a slide left drops what it heard (`DictationHold`).
  These two write into the text as they hear, so a slide left puts back what the text held when the hold began; the shell composer's mic
  keeps the words aside while held ("Input motion"). VoiceOver: "Listening…" as the value while it listens, a hint for the tap and the hold.
- BeautifulUI style (Settings › Appearance › Style, `AppStyle.beautiful`, 2026-10-30) uses `PromptComposer` (new assistant `BeautifulStarterComposer`),
  BeautifulUI Prompt Bar (`layout: .stacked`); other styles unchanged above (CLAUDE.md: new appearance as new theme):
  rounded multiline input, toolbar below: + (Photos / Camera / Files), @, /, model, dictation, Send right; working Agent with empty text gets Interrupt.
  Background current style `.surface` (library draws none), buttons 34pt. iOS uses shared dictation/stop: library microphone off (`showsDictation = false`),
  no `onStop`; overlay shared button at library Send. Stop uses even-odd `SendCutout` masking library Send, extending far out
  so menus above input are not clipped (library unchanged, `apple/Packages/BeautifulUI/VENDORED.md`). Pending images `ComposerImages`; drafts, send safety and uploads remain in each composer.
  - Drafts: directly edit `model.drafts` (`Options.text`), no reciprocal bar/host writes that prematurely commit Pinyin marked text.
    Before sending, commit marked input and update draft from text view (`TypedText.commitFocused`, all three composers, #2203: WeChat keyboard sent only half).
    Quotes (`composerContexts`) persist like text in StateCache (`composer-contexts`), surviving App termination; Agents row metadata gets Draft for text or quotes (#4578,
    `AppModel.draftMarks`, changes only when a session gains/loses draft, no row redraw while typing).
  - Paste images (#4573): clipboard image + focused input shows system Paste above (`PasteImageChip`, SwiftUI `PasteButton`); tap adds pending image,
    itself authorizes paste, no Allow paste prompt. Only `hasImages` checks presence, no content reads; after tap or × same copy does not show again. Chats and both assistant starter composers supported.
  - @: recently accessed tool files (12, `@relative/path`, understood by Claude Code and Codex), then recent 8 sessions, selected as
    `<persona-context>` before message, quote capsule above input. Only loaded-step files; folded work never expanded in long chats yields none.
  - /: `SlashCommand.all`, quick words /继续 (continue), /提交 (commit), /汇报 (report), fills request text for explicit Send (formerly menu sent immediately);
    /compact and /context unchanged; /clear when supported, confirmation first. The user's saved prompts follow the chat's own commands ("Saved prompts (#9436)").
  - /loop (#2601, web #1600): Claude Agents only, not Codex, internal Codex fork or assistant (`LoopCommand.offered`), in / and + Shortcuts;
    prefixes existing input with `/loop ` (existing text becomes task, cursor after command to enter interval), never sends directly (other + shortcuts still send on tap). A `/loop` draft shows shared `LoopDraftNote`
    explaining "Repeat every 10 minutes: Check CI" / "Repeat at Claude's chosen pace: …", gray usage when task absent;
    PersonaKit `LoopCommand` matches `web/src/lib/loop-command.ts`. Screenshot `-showLoopDraft "/loop 10m Check CI"`.
  - Model: the session's launcher list, the rows New Agent offers for it (Default, the agent's models with their versions, the IDs typed before; `RunningModelLists`, #3754), an alias named by the version it stands for (Opus → Opus 5.5) so it reads apart from the versions beside it; the fixed `/model` aliases (Default / Opus / Sonnet / Haiku) only until the list is read or when it can't be. Selection sends `/model <id>`; absent for Codex, internal Codex fork and headless assistant.
    Assistant home uses launcher list (former model label moved here).
  - Dictation: `Dictation` (`SFSpeechRecognizer` in the language the owner speaks, "Dictation language" below, + `AVAudioEngine`), live recognized text appended to draft; tap again or send stops, and held it dictates while held (#9619). The shell composer's tapped mic opens the dictation bar instead ("Input motion" in the shell section). **Dictate to Persona** opens a new assistant conversation whose visible starter takes a one-shot request into this same mic path; failure keeps the new conversation, focuses the field and shows the usual message, and stopping or a root-layout replacement keeps the words as an unsent draft.
  - Permission/effort above input (`SessionModeBar` / `NewSessionModePicker`) remain, not merged into toolbar; rationale PLAN PromptBar v2.
  - DEBUG `-showPromptMenu mention|command|attach|model` opens after a few seconds for screenshots.
- Model API errors (`APIErrorCard`): Claude Code's final "API Error: 502 …" after retries and hub "API 错误：…" ("API error") notices become error cards,
  not raw body text. First line plain-language cause (`APIErrorInfo.explain`, e.g. "Inference service has no available GPU" / "Login expired; run /login on the computer"),
  then BeautifulUI chips: error-styled status, Temporary failure / Needs attention, consecutive count. Open request ID, session, time and original, all copyable.
  Repeated identical errors separated only by Continue-like retries merge with count (`ChatView.foldAPIErrors`). Retry (sends "继续", "Continue") only when last item, idle Agent and transient.
  Currently text parsing; Claude Code also provides `apiErrorStatus`, `error`, `requestId` fields, preferred once hub reads them. DEBUG `-showErrorDetails YES` opens by default.
  Screenshots mock `MOCK_API_ERRORS=1`, assistant main and review chats.
- Refusal fallback (#4489, the iOS half of #2528; web #4487, Android #4488): a notice with `model_refusal` is a line of its own after the exchange (`RefusalNotice`; `ChatStore` puts it with the turn's trailing rows, never in the work row): the upstream refused, which model Claude Code switched from and to (or that no switch is recorded), "later turns may keep using it" for scope `session`, the category, the explanation and the refusal's own words. Models go by their short names (`SessionModes.modelName`), where the web shows ids. While the session has `model_fallback`, a card above the composer (`ModelFallbackCard`, a `ReadinessCard`) names both models and the error since: a terminal session gets only the "type /model in Claude Code" step; a Persona conversation gets **Switch Back** (zh-Hans 回到配置的模型, ja 元に戻す; `POST …/restore-model`, refusals said in words by `ModelFallbackText.restoreError`), and the main conversation also **New Main Chat** (the card's own key, ja 新しいチャット: at the largest text size, stacked, the longer Japanese and English words still ended in "…", `docs/design/DESIGN_SYSTEM.md` 1.2 rule 3 (a), #5202). While `model_restore` is pending the card says only that the next start uses the configured model, and names it. The API error card offers no Retry while `turn_error_code` is `model_fallback_unavailable`. Items keep the refusal's words as their text, never an older hub's whole record line (`Item.publicRefusal`); `ChatCache` went to `chats-v17` so notices cached without `model_refusal` are dropped. In a Persona conversation (`plainRefusals`, set by `ChatView` from `session.assistant`; #5202) the line is one sentence with the model's short name and the record under a closed Details, and the card's title says which model answers now, with the configured model while pending; the rest goes under `ReadinessCard`'s Details. Tests: `ModelFallbackTests`, `ModelFallbackUITests` (mock `MOCK_MODEL_FALLBACK=1 MOCK_MAIN_ACTIONS=1`).
- Suggested next message (`SuggestionPill`): with session `suggestion`, an idle chat and an empty input, the shared Agent / Persona
  composer shows "✦ Suggestion … Use" above it (AI purple, not accent). This includes the main Persona conversation and other
  assistant chats; there is no assistant-specific rendering path. Tap or Fill In puts the text in the editable composer without
  sending. A non-risky suggestion also keeps long-press Send Now and its VoiceOver custom action / hint. With
  `suggestion_risky: true`, it is fill-in only: no Send Now menu item or direct-send accessibility action, and the hint does not
  advertise one; the ordinary composer Send still works after review. Missing, null or malformed risk values decode as false.
  Typing hides the pill. Mock `MOCK_SUGGESTION=1` covers the review Agent; `MOCK_NEXT_SUGGESTIONS=1` covers a safe main-Persona
  pill, a risky assistant pill, safe Agents and the native switch. Tests: `SuggestionRiskTests`, `SuggestionTests`.
- Settings › Assistant › **Suggest the Next Message** (#4696) reads and sparsely writes `next_suggestions` through
  `GET/PUT /api/v1/assistant`. It is off by default on a supporting hub; an older hub that omits the field keeps it nil and hides
  the row. `NextSuggestionsModel` (an `@Observable` model with `AssistantDeps`, tested against `FakeHub`) loads the current
  assistant's value, shows a change optimistically, accepts the response and rolls back on failure, fencing late work across an
  assistant or hub switch. The English title uses Apple Title Case while the web keeps **Suggest the next message** in sentence
  case; the zh-Hans and ja titles and the footer in all three languages match the web. Tests: `NextSuggestionsModelTests`,
  `AssistantSettingsModelTests.testNextSuggestionsSwitch`; a separate untouched-mock run proves the default is off, and the
  legacy-mock case waits for the app's successful-read probe before proving the row is absent.
- Sending clears input immediately and displays Sending bubble (`ChatStore.pending`), request background; same text in transcript
  or queue replaces it, failure red with retry/delete. No waiting on network anywhere.
- Sent images: hub appends paths in `<attached_images>`, App parses square thumbnails above text (file API from session computer,
  memory cache), tap full viewer; tag hidden (legacy "相关图片：…", "Related images", / "（我从手机发来了图片…）", "I sent images from my phone", supported).
- Monospace (files, diffs, terminal, `MonoTextView`): non-scrolling UITextView measured to longest line inside UIScrollView, no wrapping, both-axis scroll and selection.
- Long-press messages (`MessageMenu.swift`, `MessageLift.swift`, #603): as in Telegram, the chat dims and blurs a little and the pressed message
  is drawn again above it (an answer on a card of its own), with the action bar right next to it (Copy / Select text / Quote / Share, own also Edit);
  a message taller than the screen shows the part around the press and scrolls inside, the bar always on screen; a long answer drawn in parts shows
  from the pressed part's start (or from what was at the screen's top, when that part starts above it) to the screen's edge (#5221). A tap or a pull
  down puts it back.
  In Persona's conversation the page draws it (`MessageLiftHost`, #3188), over the floating top bar, which dims and blurs with
  the chat: drawn inside the chat it ran under it. While its words are selected, a handle held at a long message's top or bottom edge
  scrolls it on, the selection following.
  Copy whole message. Selecting words happens in place (#603, `LiftSelection.swift`): a second long press on the lifted message selects the word under the finger, Select text (#3189, formerly Select misleading beside Copy like multi-selection) selects all of it, with the system handles and edit menu (Copy, Quote, Look Up); the bar steps aside, a tap beside the words brings it back, Copy and Quote close the lift. The words never move (#3188): lifted, the copy gets the size the chat offered it and the environment it had there; selecting, the chat's own words stay on screen. MarkdownUI cannot select a range on iOS, so a TextKit 1 text view laid out like the chat lies over them, clear except for its highlight and handles (`ReplyText`, spacing shared through `ReplyMetrics`, wrapping at the width the words were offered, in the style's font design): bold/italic/inline code/links keep their look without markers, headings their size, list markers and hanging indents, fenced and indented code monospace on the code background without the fence (closing plain fence same character and at least opener length; ``` inside ````md does not close early), quotes behind their bar, rules as a line, tables as the chat's own table drawn as a picture that copies as its Markdown; math `$…$`, `\(…\)`, `$$`, diagrams and cards keep their source. The bubble or card around the words, quotes and pictures stay as they were. The highlight is the system's, made even: a line selected to its end reaches across the words' whole width (a table's and the last line's too), drawn over the chat's own code backgrounds, quote bars and rules, so a selected block reads as selected. Own messages stay plain like bubbles, no Markdown (preserves `__init__.py`, `2*3*4`). Follows Dynamic Type live. The Mac selects in the chat itself and keeps a Select Text window. `SelectTextTests`, `ReplyTextTests`; Quote adds the message to the composer as a quote; in a selection, Quote (first in the edit menu, #4549) quotes only the picked words as `part="selection"`, still linked to the message; Edit restores old own message.
  Action icons fixed 22pt high (SF Symbols differ, otherwise text misaligns).
  A message of yours not in the transcript yet (sending, waiting for the network, failed: `PendingBubble`) has the strip too, with Copy / Select text / Share (#6427), so one the computer refused can be copied rather than typed again;
  no Quote (also not by swiping or from Select text: it may never reach the agent), no Edit (a waiting one would still go out besides the edited one), no Rewind. `PendingBubbleMenuTests` (the `SendSafetyTests` mock: refused, Not Now, long-press, Copy, paste into the composer).
  One only on this phone (`PendingMessage.onlyHere`: failed, or waiting for the network; #4626) ends the strip in red with what its mark's menu says, **Delete** or **Don't Send**, and the plain context menu where there is no strip (a new conversation's failed first message) has it too. It removes the bubble and its outbox entry and sends nothing; a toast offers Undo, which puts it back where it was (`AppModel.discard` / `undoDiscard`; a new conversation's first message through `FailedStart`). A message the hub may have (sending, sent, in the transcript) has no Delete. `PendingDeleteTests`, `PendingBubbleMenuTests.testDeleteAMessageThatWasNotSent` (Delete, Undo, deleted again, gone after a relaunch, no Delete on a delivered message).
  - Rewind here (#2915, web #2436, `Rewind.swift`): Claude Agent own transcript message action adds Rewind,
    chooses Chat only / Code and chat, latter confirms files too. Idle only; working/waiting/not running/non-Claude explains why;
    absent for assistant/sub-Agents. App sends original text (fetch full if truncated), reverse occurrence index of identical text and count after it (`POST …/rewind`),
    node checks screen then /rewind; success notice and restores message into App input if Claude input cleared, failures explain (old hub, full-permission credential required, node refusal).
    Rewound turns refetch on `history` change. Six buttons divide chat width when needed. Mock `MOCK_REWIND=1`,
    review session, `-showRewind bar|ask|run` (bar / picker / directly rewind chat only).
  - Why Select text opens a page: MarkdownUI messages are groups of SwiftUI Text blocks (paragraphs/lists/tables); on iOS Text supports
    whole-block copying, not draggable partial or cross-block selection. Inline selection would require UITextView relayout and lose table/code styles. Technical limitation: keep the separate text page.
  - Share (`ShareCard.swift`): render a card (gradient background + white/dark card: Claude or Me, source "app › leader", time, body,
    Persona mark); same chrome as image viewer (#4714): circular Close · Light / Dark capsule above, circular Share image (system share) ·
    Save (Photos, add-only permission) / Copy image capsule · circular Share text below; card scrolls behind both bars.
    `ImageRenderer` cannot draw ScrollView: separate `.card` Markdown theme wraps code and prevents horizontal table scrolling, otherwise tables and everything after turn blank;
    set `proposedSize` width or height is measured at another width, leaving a large blank bottom. Truncate beyond 3000 characters, never inside a card.
    A reply's cards are drawn as the chat draws them, never as their JSON (#7261, `ShareCardParts`, docs/assistant/assistant-views.md "Sharing a reply as a picture"): the extensions' renderer in picture mode (`extensionPicture`: buttons, fields, segmented choices and row controls drawn as they look; progress bars and switches as shapes; tables wrap), a card that only draws live (HTML, pictures loading later) as its fallback.
    Width follows the content (#2216, `ShareCardWidth`): plain text stays 390 pt; a table or fenced code block (`ShareCardBlocks`) widens the card to its own width, up to 520 pt, past which a table wraps its cells; a card taller than 2.6 times its width also widens, up to the same cap. The gradient covers the whole image (`ShareCardWidthTests`).
- Agents chat top bar: ① Back / title + subtitle / ⋯; subtitle "● State (working duration in minutes) · Machine · Space › Tab" (since 2026-10-31 calm Seen/Not running states show only dot; tap title opens bottom Session info sheet with Run / Usage / Location, copy id/resume command, swipe down or tap above closes, former under-title pullout removed; Usage below context tokens has "Context remaining · N% left" and thin bar (#3289, web #2430, `ContextUsage.swift`: `context_tokens` / `context_window`, live session pushes, warning at ≥80% used, hidden for unknown window)); state text state-colored,
  omit machine first then location if crowded; the title is one line cut at its end and uses the full width between the buttons (at most `Theme.Size.headerTitle` on wide screens);
  ② second row for other same-space Agents, loop/background or the permission mode: the tabs take the remaining width on the left; at the trailing end the activity pill (`ActivityChips`, trailing also without tabs): neutral fill, as tall as the tabs,
  "⟳ 13:24 │ ⚙ 1" with the next time and count only (VoiceOver reads it in full; the loop's arrows in `Theme.loop`, the background count by a neutral gear: the state is the subtitle's, #3719); at the largest text sizes with both present the loop is only its icon; tap for details;
  then, last, the agent's permission mode (`HeaderModeButton`, #3719; see "Effort and permission modes"), which stays put as the work comes and goes.
  Goal, workflow and port rows are unchanged (phase 1, `docs/archive/apple/plan-chat-topbar.md` Q1); ③ at most one status line (`HeaderStatusLine`, `chat.status-line`): while reading a sub-Agent "👁 Viewing sub-Agent · title (read-only)",
  the way back stays the footer's Return to main Agent (`SubagentFooter`); otherwise one running "spinner Sub-Agent title ›" opens it, several "spinner N sub-Agents running · newest title ›" open All sub-Agents;
  no line when none runs (ended / nested ones from the work row's View sub-Agent and ⋯ › All sub-Agents (N)). The Main Agent / sub-Agent strip remains only on the Mac and in hosted headers (#3255).
  Formerly title, companion bar, tabs and activity each had a row. Files/Terminal/Git in top-right ⋯; Files ⋯ › Return to chat; collapsed slim bar has Back, the session's title (the open header's, as the web and Android show it, #3631), centered, and on its right the open header's ⋯ (#7962, the user's call; #3719 had left that spot empty): one menu for both (`ChatView.moreMenu` over `moreMenuItems`), drawn bare like the slim bar's Back (the bar is 40 pt, no room for the 44 pt glass) in the same 44 pt square, as wide as Back so the title stays centered and no narrower. Its loop/background and goal marks went (#3719); they are a tap away in the open header.
  Former left-swipe Files became left-swipe Quote 2026-11-04 (#1058); that day Files first had a standalone folder left of ⋯, then moved into menu at user request (#1205).
- Header auto-collapse (Agents, like Safari toolbar): custom
  `ChromeHeader` (system navigation hidden), floating above list; **finger tracking**: drag toward newer messages slides bar upward by same distance and fades,
  slim Back + "●" and the session's title fade in during latter half; dragging toward history restores it. Release never stays halfway: collapse at latest,
  otherwise snap nearest end (`.smooth`, 0.3 s). Following new content also collapses after user has touched list; tap slim bar / switch tabs expands.
  After 3 idle seconds (`ChatView.chromeIdleSeconds`: no list/header touch, info card closed, no sheet/menu), auto-collapse; header interaction (swiping Agents), expand and tab switch reset timer.
  Invisible layer excluded from accessibility: expanded has no slim bar, collapsed swaps the full layer for a clear placeholder of its measured height (a `.hidden()` copy rebuilt the whole header at every fold and unfold, #9103), VoiceOver/tests find one Back and one ⋯ (`chat.more`; `ChatHeaderTests`, which also checks that both ⋯ list the same items).
  - Smooth because collapse lives in separate `@Observable` `ChatChrome`, redrawing only header per frame; floating above fixed-frame list (header-height margins only at oldest end),
    so messages never move a pixel. Former toggle animated system navigation plus two labels independently and changed list height, abrupt.
  - User tracking/interacting and immediately subsequent decelerating only (`onScrollPhaseChange`), not keyboard-driven offset changes.
  - Hidden system navigation disables UIKit edge Back; `SwipeBack.swift` takes `interactivePopGestureRecognizer` delegate to reenable.
- Queued messages: messages sent while Claude works are outside transcript, in Queued above input; each Recall and edit / Copy / Delete.
  Like TUI: ↑ pulls all back to input (transcript popAll), after clearing input requeue kept messages (`POST …/queue {"keep":[…]}`).
- Development list: busy spaces show only Needs you / Working / Unread Agents, idle folded into N more idle, tap expands.
- Return-to-bottom arrow: bottom sentinel determines visibility.
- Terminal/Files/Git in development chats (`Views/Session/`): **no tabs** (requested 2026-10-30: remove Chat · Terminal · Git · Files).
  Terminal in top-right ⋯ (requested 2026-10-31: remove standalone button, also collapsed bar); active work row still has terminal. Files/Git by **left swipe** (Chat → Files → Git, right back),
  or ⋯ Files / Git (change count); Back in Files/Git first returns to chat. Git only in repos, Files when cwd known; absent for assistant.
  - Terminal (`TerminalPanel`, `Views/Session/Terminal/`, `docs/integrations/terminal.md`): view in any state. Button opens **full-page cover from bottom** (`TerminalCover`, requested 2026-10-30), no title/tabs, slim top bar (size left, handle center, settings and Close right),
    drag bar down follows finger, closes past 140pt or fast swipe (handle tap, Close and VoiceOver's escape too); terminal size changes only with keyboard, not header (resize changes controlled pane, requiring Claude relayout).
    The bar is above the screen in every state (connecting, error, no terminal, live). No terminal (the session isn't running, e.g. a finished task's Agent whose tab the hub closed): an `EmptyStateView` with **Resume Session** for a Claude Code session (`POST /sessions` with `resume_session_id`; the node keeps what it ran with), no key bar, and the whole area drags down like the bar (#4328).
    Moshi-style bottom key bar: low dark capsule, flat 30pt keys, small monospace Ctrl/Esc/Tab, others icons, preserving terminal height. With `terminal_stream`, subscribe live
    (`TerminalFeed`: herdr observe frames applied to pane-sized `TerminalGrid`, dropped frame/new stream restarts from full frame, reconnect with epoch/seq);
    without hub/node/herdr streaming, fetch colored screen each second (`format=ansi`).
    Default Phone size (Settings › Size): with `terminal_control`, control at phone columns×rows, Claude lays out to phone width and keys write directly;
    header Phone size 45×29, occupied asks Take over / View only, lost control shows View only · Take over. Leaving tab/background/disconnect restores computer size (`docs/integrations/terminal.md` section 11). `TerminalView` draws each character in its grid
    (wide characters two cells), box/block characters as lines/rectangles, keeping Chinese/boxes aligned; draw only through last text/cursor row. Wrap to phone width by default,
    top-right Wrap / No wrap · Swipe horizontally (`@AppStorage("terminalWrap")`), light/dark `TerminalTheme` Persona / Solarized / Midnight,
    `terminalFontSize`. Long-press word, drag selection, Copy / Select all; OSC 8 and plain URLs open in App.
    Near top, load older 200-line colored chunks incrementally,
    to herdr's earliest retained point. Locate each chunk by current top 20 lines + loaded count in server buffer (`POST …/screen/before`): Claude redraw duplicates screens in history, choose match nearest loaded-count-from-end; continuing output does not misalign.
    History freezes screen until Return to latest (no history, e.g. fullscreen Claude, does not freeze). herdr retains about 1000 lines/pane (`pane read` caps regardless of --lines, no offsets/scroll API),
    at limit says "herdr keeps only about 1000 recent lines; earlier work is in Chat", whose transcript is complete. Card layout shows this top-left; `TerminalCover` has no space, places above Return to latest (#1133). New Mac Claude fullscreen has no pane scrollback.
    Unreachable hub, stopped stream, failed screen read or over 4 seconds without fresh screen shows `TerminalLinkStatus` atop: Last update N sec ago + Retry in N sec · Reason + Retry;
    backoff 1/2/4/8 seconds then every 15, disappears on recovery. Old content must not look live.
    `TerminalKeyBar` ctrl/alt/⇧ like Moshi: single sticky for next key, fast double locks (tap unlocks, filled key with horizontal line),
    held while pressed (release without another key makes sticky); modifiers affect input typing too, next character immediately sent as chord
    (ctrl,c → `ctrl+c`; shift-only uppercase), input outline/placeholder changes; Send's Enter includes modifiers (shift+enter newline in Claude).
    Then esc/tab/direct ⌃C/gamepad opening arrow+Enter cross; arrows use modifiers too.
    Controls: pinch font 7–20 stored `terminalFontSize`; two-finger horizontal swipe previous/next same-space Agent (herdr tab), vertical adjacent same-computer space;
    double-tap terminal pastes clipboard without Enter, multiline confirmation; while controlling, hold key-bar Paste for photo/file, upload to pane computer then type path (`docs/integrations/terminal.md` section 18 "iOS / Mac"); left input lightning shortcuts (tap fills, double sends, `terminalShortcuts`);
    top-right controls opens Terminal settings: shortcuts, visible keys (ctrl always), Hide key bar when not typing, colors/font/wrap.
    Typing completion from commands previously typed after prompts (`$ `, `❯ `, `➜ `…), visible paths/names, **pane content only, no computer shell history**.
    Dictation uses system keyboard microphone, no App voice input. Key names follow herdr `send-keys`,
    order ctrl+alt+shift+key, herdr encodes (ctrl+shift+a CSI u), unknown names reject entire chord (`invalid_key`); serial sends preserve press order.
    Live-status terminal icon opens full-screen version of same panel.
  - Git (`GitPane`): branch, upstream ↑↓ (an upstream deleted on the remote, `gone`, says so and has no Pull), staged count; changes (status letter, +/− lines, staged/partially staged), diff with old/new numbers and red/green, no wrap, horizontal scroll,
    staged/unstaged switch when both; paged history, commit message and per-file diffs. A refresh the user asked for (pull to refresh, the Mac's
    Refresh / ⌘R) that fails over a status already shown says so in a notice above it with Retry, and a failed Load Earlier Commits turns the button
    into "Couldn't load. Try again." (#1889, `GitReadFailure`); only the automatic live refresh stays quiet. Screenshots: mock hub `MOCK_GIT_READ_FAIL=1`
    with `-showGitReload YES`.
    Branch-card Pull N fetches latest upstream in one tap, retained as Pull when synced for active remote checks; disable repeat taps while running, refresh status/files/history and show result.
    Clean-worktree fast-forward only, no auto stash/merge/rebase; existing stashes unchanged; explain no upstream/detached HEAD/uncommitted changes/divergence. Other Git views stay read-only.
  - **Stage, commit, push** (#7013, the web's Git panel, `docs/protocol/protocol.md` `git/stage` · `git/commit` · `git/push`; `GitWriteModel.swift`, `GitWriteViews.swift`): only when the hub has `git_write`, on the iPhone and iPad (the Mac app is frozen); without it the tab is as before.
    Each changed file has a tick box in front (`TickBox`: ticked all staged, a dash partly, empty none; `GitStageMark`, the web's `stageState`): a tap stages or unstages it at once, a swipe from the leading edge and the long-press menu do the same, a tap on the row still opens the diff.
    While there are changes a bar floats over the bottom: every change's tick box (Stage All / Unstage All), how many are staged, and **Commit…**, which opens a sheet with the message and the staged files (off with nothing staged or a detached HEAD).
    The message is the tab's, so it stays when the sheet closes or the commit is refused; the sheet can't be swiped away while hooks run (the hub waits up to five minutes).
    **Push N** sits beside Pull when there are commits to send, no upstream yet, or an upstream deleted on the remote (`GitPushKind`, the web's `PushButton`; it takes the place of the ↑N count) and asks first in a system confirmation with the web's words (never forced; a new branch's upstream is made on origin).
    One write at a time; the tapped box or button shows progress, the status comes from the write's answer, success buzzes and says so in a toast (Committed …, Pushed …), a refusal buzzes, says what happened in the web's words with the files and git's own words in an alert (`GitRefusal`), and reads the status again.
    `GitWriteTests`, `GitWriteUITests` (mock hub `MOCK_FILES=1 MOCK_GIT_WRITE=1`; `MOCK_GIT_WRITE_FEATURE=0` keeps the repository without `git_write`, the tab as before).
  - **Branches, Stashes** (#7015, the web's `BranchesView` / `StashesView`, `docs/protocol/protocol.md` `git/branches` · `git/switch` · `git/stash`; `GitBranchViews.swift`): with `git_write`, two rows under the branch card open sheets.
    **Branches**: a search field that also names a new branch (Create Branch “…” from Here once `WorktreeForm.branchProblem` takes the name and no local branch has it), local branches then remote ones without a local one, the current one marked, each with its last commit's subject and age.
    A tap switches (a remote one by its name without the remote: the node makes a local branch tracking it): at once in a clean folder, after a system question when uncommitted changes would go along. Nothing is forced: when the other branch has any of those files differently the node switches nothing and the alert lists them, with **Stashes** to put them away (it only opens the sheet); git's own words go to the log there, not the alert (`GitRefusal.shownDetail`).
    **Stashes**: an optional message, Include New Files (on, as on the web) and Stash Changes; then the stashes, each a menu: Apply (keeps it), Pop (removes it), Drop… (a destructive confirmation). Every write names the stash by index and oid; a list that moved is refused (`git_stash_stale`) and read again, and a node without oids gets the actions off with a line saying to update it. Pull to refresh reads the list.
    The stash count in the card's second line says 贮藏 in zh-Hans, as the web does (暂存 is staging).
  - **The pull request** (#7014, the web's `PRCard` / `PRView` / `PRCreateView`, `docs/protocol/protocol.md` `git/pr` · `git/pr/merge`, through gh on that computer; `GitPRModel.swift`, `GitPRViews.swift`): with `git_write`, a Pull Request section under the branch card.
    One row: number and title, a state chip (GitHub's colors, as the Inbox's GitHub thread page), the checks in a few words, the open review comments; it opens the page. Without one, "This branch has no pull request yet" with **Open Pull Request** (off until the branch is pushed with nothing left to push). gh missing or not signed in: a line and Retry. No GitHub remote or a detached HEAD: no section.
    It is read when the tab opens and again only when the branch, its upstream, what is left to push or HEAD moved (`GitPRModel.key`, the web's rule; gh asks GitHub each time), and on pull to refresh.
    The page (a sheet): title, state, review decision and Merges When Ready; the checks (each opens its run in the link card, `LinkSheet` over the sheet); the open review threads, folded past six lines, each with **Hand to Agent**, which puts it over the message box as a comment for the next message (`GitPRText.comment`, the web's block; not shown when the chat takes no messages), and Hand All; Merge with How (Squash by default, Merge Commit, Rebase) and **Merge** when it can merge now, **Merge When Ready** when it can't yet (a conflict says so), **Don't Merge by Itself** once set — each asks first; the branch is never deleted. Open on GitHub (top left) opens in the link card.
    Opening one: a sheet with the title (the last commit's subject to start), a description, Into (the default branch or another branch's name) and Open as Draft; Create asks first (it is public), and the page then takes the sheet's place.
  - Files (`FilesPane`): descend from session cwd, breadcrumbs jump levels, directories first, size/modification/Git flags (dot for changes within directory),
    hidden files off by default, menu toggle; tap file viewer.
    The machine page's Files row (below Agents, disabled when offline) opens that computer's home in the same pane (`cwd` is `~`).
  - **Upload, Save to Files** (#4858, `Views/Session/FolderTransfers.swift`, interface in `docs/protocol/protocol.md` "Changing files"): the upload button right of the breadcrumbs offers Choose from Files or Choose from Photos (several at once; photos are saved as files in their original format and name),
    into the current folder under their own names, up to 2 GB; one after another, a row with progress and ✕ per file under the list. A taken name first asks Replace / Keep Both (Keep Both saves "name 2") before anything is sent;
    a folder that cannot be written (outside the home, `~/.persona`, `~/.persona`, `~/Library`), an outdated personad on the computer, and files over the cap each get a one-line reason. Leaving the page lets uploads finish; a pending name question counts as cancel.
    A file's long-press menu (also a VoiceOver action) has Save to Files: downloads to a temporary folder with progress, then the system's location picker; the temporary file is removed either way.
  - **Copy To, Duplicate, Compress** (#5830; progress #6113, `FolderTransfers`, contract in `docs/protocol/protocol.md` "Changing files"): the row menu and selection bar copy into another folder, duplicate beside the original, or make a zip there. A remote operation that takes over one second joins the transfers below the list in an `AttachmentRow` with file / byte counts and Cancel after the first successful operation read (an older chain without operation lookup keeps the previous no-row behavior); a quick one never flashes a row. Remote operations start independently of the serial upload / download queue, so a long transfer cannot delay their progress or Cancel. Leaving or hiding the file page pauses only polling; the POST continues and `onTabVisibility` resumes polling when the page returns to the front, even before the transfers bar first appears. MachineDetail and Chat retain the same `FolderTransfers` across file-page reconstruction so returning does not lose the operation; Chat items and polling are scoped by `SessionKey`, and file polling uses `onTabVisibility(sceneAware: true)` so both the selected tab and an active scene are required. A scene change (Control Center, a call, the app switcher) only pauses that polling: downloads, the file waiting in Save to Files and a name question stay; only leaving the page (the tab-only `onTabVisibility`) answers questions Cancel, stops downloads and removes the pending file. A remote copy and an upload can both meet a taken name at once: each question has its own continuation and is asked in order (the alert closes, then opens for the next), and a row's Cancel answers only its own. The original POST still decides success, a name conflict or failure, and a conflict retry uses a fresh operation id. Cancel stops the POST and the node's partial output is removed.
  - **Rename, move, move to the trash** (#315, `Views/Session/FolderEdits.swift`): a file's or folder's long-press menu (and VoiceOver actions) adds Rename (an alert with the name, refused names said before asking),
    Move (a sheet with the browser's folders from its top, opened on the current one; Move Here is off for the folder they are already in, and the folders being moved can't be opened), and Move to Trash (asks first).
    ⋯ › Select turns the list into checkmarks with a bar under it: the count, Move, and the trash button; Done ends it. A trash toast has Undo (puts it back through `files/restore`); there is no permanent delete.
    A move to a taken name asks Replace (the one there goes to the trash) / Keep Both. These show only when hello's `features` has `file_edit` (or the hub sends none).
    On an iPad, files dragged in from another app (Files, Photos) upload into the folder shown, with an accent outline while they are over it.
  - **Auto-refresh** (#3089, web #2439, `App/FsLive.swift`, `Views/Session/FsLiveModifier.swift`, `docs/protocol/protocol.md` ("File auto-refresh")):
    foreground Files/Git sends WebSocket `fs_watch` (current directory/Git marks, Git state; same cwd combined),
    `fs_changed` debounced 200 ms then reread in place without loading animation (retain older loaded commits if latest unchanged); unsubscribe off foreground (`.onTabVisibility`),
    reread on return and re-send/reread on socket reconnect. Old hubs/nodes lacking `fs_watch` keep pull-to-refresh / ⌘R. Open diffs/file viewers do not refresh yet. Mac shares panels.
- Loop (`/loop`): above input "Looping · Every 30 minutes · Next wake 11:56 (in 4 minutes)", tap task, confirmed Stop loop sends stop;
  each wake is "⟳ Loop wake 21:11 …" row, not user bubble.
- Sub-Agents: Agent-tool children no longer count background tasks; on iPhone / iPad the header's third row is a status line (see the top bar's ③ above); on the Mac and in hosted headers, below chat tabs Main Agent / running sub-Agents (spinner, row if children within a day; completed not chips, View sub-Agent in work), tap same
  message/tool UI read-only, footer "Sub-Agent · Explore · Running, read-only · Return to main Agent"; View sub-Agent below each Agent call.
  Ended/nested children outside row add All sub-Agents N (#1559, web; in ⋯ on iPhone / iPad) at end, bottom tree sheet (`Subagent.tree`, same `subagentTree` sort:
  `parent_id` indentation, missing parents top-level; running first then newest start within level); state dot/description/Running or Finished/type (forks labeled Fork)/start age, tap read-only.
  Mock `MOCK_CHAT_LINES=1`, `-openSession <machine>/a1f3c2d4-0000-4000-8000-0000000c1e01` for screenshots.
  Agents with running children show "👥 Sub-Agents N", not idle. Id `<main session>@<agent id>`, history/live/paging identical to normal.
  Running or finished goes by the chat's own lines (`Subagent.running(in:)`, #5602, as Claude Desktop): done once its Agent call's result is in (whatever the hub's list says; one dropped from the list kept Running for ever), one sent to the background once its notice came; the list's word only until then or when the call is not loaded. A running child's write time and last line are no change for its session's page (`Session.sameForItsPage`), and a tool run whose first call goes to the background keeps its id (an opened run stays open). Task-notice and long-notice rows keep their open state in the chat's `OpenTools`.
  Every Apple conversation and visual style follows `docs/SUPER_RULES.md` rule 2 (#2705): running and completed tool execution defaults to one folded summary. Only the latest assistant text segment appears outside it during execution; streaming replaces the preceding segment, and completion leaves the final result outside. Manual expansion survives live updates and completion; reloaded history starts folded and can still be opened to fetch hub folds. Compaction notices, failed tools and supervision visibility tools remain inside the same work line. Promoting a supervision round makes its input and reply visible, not its tool calls. The Agents chat and wake drawer use the same default-folded policy.
  In a work line each Agent / Task call is its subagent's card (`SubagentCard`, #5602, as the web's and Claude Desktop's): its description; under it, while it runs, what it is doing (`last_tool.summary`, else its name, "Starting…" before its first call) and its steps, once over Done / Failed / Stopped · N steps · time (a background one's time runs to its last line, not its receipt). Open: what it was asked, its report (a background one's: its notice's detail), its latest three tool calls as they come (`SubagentRecentSteps`: the card subscribes to the subagent's own conversation while open, as the web's `RecentSteps`; the hub replays a run folded, so the card opens its newest fold when it holds at most 40 calls and fewer than three follow it, a longer one is not fetched for three rows), View Subagent. `-showSubagentDetails YES` opens every card for screenshots; on the mock, `GET /mock/subagent-step` (`MOCK_SUBAGENT_CARDS=1`) moves the background one a step on. A card never folds into a run of chips, but stays inside its work summary until that summary is opened. Subagents launched one after another share one line (`SubagentGrid`): "Running 3 agents" while any runs, "Ran 3 agents · 12 steps" after, their cards in a grid two side by side from about 540 pt (iPad, Mac; `Theme.Size.subagentColumn`), one column on a phone; the grid is visible only inside an opened work summary. While the agent waits on subagents it launched in the foreground (running, call unanswered), the running work line says so instead of its counts: "Working 6m · 2 subagents working — <what the newest does>… · +1 more" (`AgentsWorking`), the count alone where the line is narrow. The plan (TodoWrite) is one line in its work: "Update To-dos  Step 3 of 4 · <the step it is on>" over a thin bar of how far along it is (`PlanBar`); a tap opens the plan sheet (`PlanSheet`), which shows the chat's newest plan whole and follows it. While that plan's line is not on screen (scrolled away, its work line folded, or folded by the hub, whose `fold.tasks` keeps the list), a pill over the composer says "Step 3 of 4" (`PlanPill`) and opens the sheet. A plan's call never joins a run of chips. A finished subagent's report ending in its `agentId` is not a background receipt (it read as Background · Running).
  A child session the hub folds under its parent (`folded_under`, docs/protocol/protocol.md "Session hierarchy", #5140) is not its own row, dock pill or count. The parent row shows the folded state, what a waiting child asks, and how many subagents are running. In the parent's chat each child is a card in the work summary of the exchange that was going on when it started (`ChildSessionCard`); the open chat reads that parent's child cell, not every agent. Opening the card reads that child's transcript. "Running for" follows the same elapsed clock as the work headers (`VisibleTimelineView`, 30s). The child's requests are shown on the parent page and still answered as the child's own interaction. A link to a child opens the parent with that card open, and only when the parent row is already in hand.
  A subagent's report reads as one line until opened (`FoldedReport`): its first line when that is a heading, else its first 80 characters, and Expand; open, the whole of it and Collapse. In the card, in a background subagent's notice, and in a message from another session that is a subagent of this one handing its report back (its name is the subagent's id, type or description; an older node shows a handback so). Open or not is kept by the card's id on disk (`OpenReports`, StateCache, the latest 400); `-openChatsFresh` folds them all again. A report of one line of 80 characters or fewer is not folded.
  Any other message from another session (`PeerMessage`) that runs longer than three lines, as many as an exchange shows (in every chat and on every platform), shows those lines, the last one fading out, with Expand / Collapse under it (`LineFold`, cut at the height of that many body lines because `lineLimit` does not reach into Markdown's blocks; open or not kept like a report). A short one has no button (#2010, `PeerFoldUITests`).
  Your own message (`UserBubble`, the pending bubble too) folds by the web's rule (#5676, `BubbleFold`, `BubbleFoldRule`; web `BubbleText`): past 14 lines as drawn it shows the first 10, the last one fading, with **Show More** (展开) at the bubble's bottom in the bubble's link color, and open it ends in **Show Less** (收起); at the accessibility text sizes 10 lines are taller than the screen, so there it folds past 9 lines to 5. A long press on a folded bubble opens it, with no action bar (`messageLongPress(opens:)`, the owner's choice: "a long press opens it"); open, the long press lifts it with the bar as before, and the lifted copy is always whole. Opening and folding go through `toggleInPlace(rises: false)`, the bubble's top held where it is, at the newest end too, with no rise to the top bar (`chatPinned`, only on bubbles that can fold); open or not is kept by message like a report (`OpenReports`). Cost: a message that cannot pass the rule's lines even at 9 wide characters a line (scaled with the text size) is drawn as before, with nothing measured; one that can is measured by `onGeometryChange` on its size, which scrolling does not change, and drawn again only when that changes what its characters said; folded it lays out only its start, its first 16 lines as they wrap at the bubble's width (the last long bubble's, measured from Show More's row; an em per CJK character, less per letter, so the start errs long; 3,000 characters at most; kept once per text and width), under a fade mask. A pasted report has few line breaks: cut by line breaks alone it was laid out whole, and scrolling folded reports cost more than scrolling them whole, and open it has no mask. Whether it is folded is kept for the long press outside SwiftUI state (`BubbleFoldKey.folded`): a `@State` in the row drew every long message's row twice as it scrolled in. Opening goes through `toggleInPlace(unfold:)` and shows what opens with the same `reveal` fade as a tool's details (`FoldFade`, #6481): the lines already shown stay as they were; the lifted copy is `fixedSize` vertically, so a bubble opened by Show More lifts whole, not at its folded height. DEBUG `-showPlainBubbleOpen YES` opens without the fade, for before / after recordings from one build. The Mac app (frozen) keeps its bubbles whole. DEBUG `-showWholeBubbles YES` draws every bubble whole, for before / after shots and measurements from one build (`BubbleFoldUITests`, `PerfLongInputTests`, mock `MOCK_LONG_INPUT=1|perf`; the perf chat has a twin whose reports are cut to a folded one's lines and drawn whole, as many rows to the screen with nothing folded).
- Background workflows (#2935, `workflows` in `docs/protocol/protocol.md`): Claude Workflow tool multi-Agent background scripts keep Agent Working after turn ends while waiting for completion,
  `state` still drives colors/count/watch/to-dos; App label Working (background) (`Session.stateLabel`);
  Agents rows/to-do cards add `WorkflowLine` below last text: "Background workflow ‘parity-audit’ · Verify 2/2 · 169/185+ · 14 min" (multiple add Waiting for N). Counts read done / started with `+` while the run runs (the script starts more as it goes), without it once it ended (`WorkflowRun.counts`). In a narrow row only the name is cut (`lineName`, tail); the phase, counts and time after it stay whole (cut in the middle of the whole line, 169/185+ read 9/185+).
  - **The run's card in the chat** (#3436, the web's `WorkflowCallCard` of #3434): the Workflow call that launched a run (`tool_use_id`) is drawn as `WorkflowCallCard` in `WorkStepView` instead of a tool row. `WorkStepView` is `Equatable` on its item, so the card reads its run itself: `AppModel.workflow(_:call:)` keeps one observable cell per (session, call), set only when that run changes (a push about another run or the rest of the session draws nothing). Head: mark, name, status chip, `169/185+`, a live clock (`Text(timerInterval:)`, ended: how long it took); under it "Now Verify: verify:island" while it runs, its summary (and a failed / stopped run's error) once ended; opened, the progress bar and the phase blocks. Open while it runs, folded to its head once ended; the reader's choice stays in the chat's `OpenTools`.
    While the hub lists the run (running or ended, a day), the call never folds into BeautifulUI's chips; its card remains inside the work summary until the reader opens it. A chat opened while a run goes on has that turn folded by the hub (`fold_work`): the fold whose `[ts, until]` holds the run's `started_at` is fetched so the card can be inspected (`WorkflowRun.foldsHoldingCalls`); one that did not come (hub restarting, offline, not connected) is asked for again after a wait that doubles from 5 s to a minute (`WorkflowCallCard.fetchFolds`), so the card appears without leaving the chat. No such run (older hub, a run over a day old): the call's row as before. The tool's verb / symbol / subject: Run Workflow, `Theme.Icon.workflow`, its `name` (else the one in its script's meta, else its script file's name without `.js` and the `-wf_<run>` Claude Code adds, as the node's `scriptName`).
  - **Phase blocks** (`WorkflowPhaseList`, shared by the chat card and the header): one block per phase (`phase_stats`; an older hub's are counted from the listed agents, declared phases first, then undeclared, then the agents without one), its head `done/started` with `+` while more may start there, lit (`Theme.aiTint`) where the run is; its mark a full AI dot there, a half-strength one (`Theme.aiMuted`) for a block with agents still running that the run moved past, a ring before a phase starts. A block with agents running (or the run in it) is open, one whose agents are all back folds by itself; the reader's tap wins (kept by the block's place and title). In the chat its head, 「n more」 and Show Fewer change what is under the head with the head held in place (`toggleInPlace` + `chatPinned`: the list is upside down, and 12 rows opening under a head pushed it up under the top bar). A phase back with failures gets a red ⚠ instead of ✓, and "n failed".
    Agents in the order they started (never reordered), at most 12 quiet rows a block, running rows always shown (#3434: a running row never folds, so a phase with 60 running lists 60), the quiet rest folded into "n more" where they are left out; a block the hub's newest-60 cap cut into says "n earlier ones not listed" and shows its last 12 rows. A row: a dot (working amber, back gray, failed red, stopped faint), its label, its time to the minute in secondary (tertiary read under 2:1 on white; so do the run's clock and "n earlier ones not listed") (a running one's moves on once a minute, `TimelineView(.periodic)` from its own start); on iPad and Mac also its model and latest tool. A tap opens its conversation read-only like a subagent; on the Mac the pointer over it says Open.
    The progress bar is `MeterBar` in parts (done, running, failed `agents_failed`, stopped once ended), `Theme.Size.bar` thick, with a legend.
  - **Motion only on change** (none on the first draw, none with Reduce Motion, nothing repeating, no spinner): counts `.contentTransition(.numericText(value:))` with `Theme.Motion.state`; the lit phase moves on with `matchedGeometryEffect` and `Theme.Motion.layout`; a new agent's row fades in (`.transition(.opacity)`, `Theme.Motion.reveal`); the status chip and an agent's dot change color with `state`. The run's time is `Text(timerInterval:)`, which the system ticks without drawing the card again; the agents' rows tick once a minute, all on one shared minute, not each second: on the Mac, 60 running rows each with a second clock kept the idle chat at 7.8% CPU (Debug, `ps`, 20 s). With minute rows, over 90 s (a minute tick included): 16 running rows 0.84%, 61 running rows (`MOCK_WORKFLOW=1`, `?step=5`) 0.82%, a chat without a workflow 0.25–0.30%; the difference is the run's one second clock (`docs/lessons/ios.md`). FrameStats needs an unlocked Mac (display links stop while it is locked) and has not been run. No `TimelineView(.animation)` anywhere.
  - **The chat header** (expanded, side, Mac top) has Background workflows above sub-Agents / the status line (`WorkflowSection`): a running `WorkflowCard` shows the same head, "now" line and progress bar, a tap opens the same phase blocks. Ended within the hour below as `WorkflowEndedRow`: name / done / started / time / end state, error line for failed / stopped, a tap opens its progress and phases (the same › turned down as everywhere else; an ended run's time reads "14 min" in the header, the card and the rows alike). Opening and closing them is at once with Reduce Motion. Whole section capped at 38% chat height (`Theme.Size.workflowShare`, #4532), scrolling inside so the chat stays. Under iPhone Duo's side bars, where the header never folds, a running card is its head line until tapped (`WorkflowCard.compact`, #2161). Screenshot `MOCK_WORKFLOW=crowded` (3 workflows, 10 running Agents) runs `testCrowdedWorkflowsLeaveRoomForTheChat`.
  - Watch: a tiny "workflow 169/185" chip in list / chat, its count rolling when either number changes (`numericText`, watchOS 10) and nothing else moving.
  Background N excludes `background` kind `workflow` (`Session.backgroundCount`); Live Activities unchanged, state already working.
  Lenient parsing: old hubs no `workflows` unchanged, unknown status complete, one bad entry (a run, an agent, a phase's stats) does not lose the rest (`WorkflowTests`, `WorkflowCardTests`). Mock `MOCK_WORKFLOW=1` (with `GET /mock/workflow?step=0..5`: 169→170, a new agent, Audit back with a failure, completed; 5: all 60 listed agents running) runs `PersonaUITests/WorkflowUITests`. Watch screenshots of one session: `-openWatchItem session:<session id>`.
- Background tasks: `run_in_background` commands/subagents not yet reported show N background tasks above input even after turn end, expandable description/command/elapsed,
  tap opens output file.
- Bottom tab bar persistent, no scroll collapse.
- Live status: working Claude spinner "Cogitating… 12s · ↓3.2k", tap output preview, terminal right.
- Terminal needs you (AskUserQuestion/permissions not caught by hooks, `ScreenForm`): parse screen dialog (`ScreenParser.dialog`: ☐ heading, │ quoted question,
  numbered choices and indented descriptions, "Type something.", "Chat about this"), render like question card titled Terminal has a question; choice sends number,
  Other answer… first number for Type something then text+Enter (`Resolution.preKeys`), Chat instead sends Chat about this number; raw terminal,
  keys/input under View terminal; unparseable dialog shows terminal directly. DEBUG `-showSampleScreen YES`.
  A command's permission prompt (the hub's `dialog.command`, #5266) reads as a tool approval instead: "Claude Wants to Run a Command", the agent's words,
  the command (`ScreenCommandAskBody`, eight lines until Show All N Lines opens it whole, fewer when the card is short, two in Beautiful UI; `CardCode`, #6605), the warning in plain words (Show Original: Claude Code's own), the countdown running
  (`ScreenCountdown`), Deny / Allow for its No / Yes (`ScreenParser.Dialog.allowDeny`). Mock: `MOCK_SCREEN_DANGER=before|after`.
- Interaction panel above input: questions (`QuestionForm`, iOS grouped list, question + rounded choice group, terminal-style numbered circles become checks, "(Recommended)" shown as Recommended badge but original sent; Other answer… last row becomes input on tap; single-question single-choice submits immediately, multiple title "Heading · 1/2", small Previous / Next / Submit below, auto-next after single choice; `-showSampleQuestion YES`), plans (Markdown + full screen, Approve and run automatically /
  Approve and auto-accept edits / Request changes with feedback), permissions (Allow / Deny / Always allow this session), terminal (screen + parsed choices + keys + text).
  A question whose interaction advertises `can_cancel` adds a visible **Skip** text action on the leading side of its action row (and its review), at least 44 pt tall, in both interface styles. It sends only `{"cancel":true}` through the existing resolve endpoint, cannot be sent twice or while an answer is in flight, and keeps the card, picks and typed answers with the error and Retry if it fails. Success removes the card and gives Android's **Question closed without an answer** receipt. Persona's own main-conversation `ask_user` no longer offers Handle in Terminal on Computer (there is no terminal); Agent cards keep it. Without `can_cancel` there is no Skip and the card is unchanged.
  Mock `MOCK_QUESTION_CANCEL=1` adds `can_cancel` to the network tests' Agent question and the main `ask_user` card for `MOCK_ASK_LONG=1|actions|swipe|review`; `/mock/resolved` records the exact resolution until `/mock/reset` restores both unanswered exactly once and clears those records together with their interaction history. `QuestionSkipTests` runs the main / Agent, absent / failed paths in both styles.
- Questions the assistant sees first (#544, iOS and Mac; `docs/assistant/assistant.md` "Assistant answers questions first", the web's rules in `docs/web/web.md`; `Shared/Delegated.swift`, `Views/Interactions/DelegateViews.swift`):
  a card with `delegating: true` shows **Assistant is looking** above its questions and can still be answered (the first answer wins). It gets no haptic or notification
  (`Delegated.announces`; Mac `MacAttention.lookingOnly`, watch `WatchInteractions`), and rings like a new card when the hub sends it again without `delegating`. The hub still counts it as pending, so lists and badges show Needs you meanwhile.
  `delegate` is shown as **What the assistant found** (What / Why now / Recommended / If not / Why you decide / Deadline, empty ones left out); a `recommend` that is a choice's own label (several questions joined with `；`) tags that choice **Assistant's pick** instead.
  `interaction_resolved` with `answered_by: "assistant"` leaves an **Answered by the assistant** card above the input (answer, reason, close button; the latest 50 are kept in memory), also when this device's own answer came second (the hub's `already` reply).
  **Change Answer** (sessions that take messages only) opens a sheet with each question's choices and Other answer…, sends only the questions you changed to `POST /api/v1/interactions/{id}/reanswer`, and `interaction_reanswered` marks it **You changed it** with the assistant's answer.
  Session info lists the session's questions the assistant answered (`GET /api/v1/interactions/history`, newest first); a hub without the route shows none. The history keeps the assistant's answer, so a change shows only on devices that saw it.
  Both requests name the card's or session's `manager` as `?manager=` (the question is no longer pending, so the hub routes by the manager named; docs/protocol/protocol.md "Per-manager routing").
  DEBUG `-openChangeAnswer YES` opens the sheet of the first answered card; mock `MOCK_DELEGATE=1` (`docs/operations/testing.md`).
- Codex / internal Codex fork cards (`agent`, `docs/integrations/codex.md` ("Questions and approvals")): Codex has a question / Codex needs permission, same `QuestionForm`;
  `AgentPermissionForm` shows reason/full command or changes/Deny/Approve, Always approve · Stop asking for commands beginning with … (`always_label`, absent hides),
  Deny with reason… opens input (stops turn and sends reason next message). Claude card unchanged; inability to type (computer draft etc.) keeps card,
  toast explains. BeautifulUI `BUIPermissionForm` asks Approve Bash?, Approve / Always approve (scope explained) / Deny,
  Codex reason above command, input means deny with reason; DEBUG `-showSampleCodexApproval YES`, `-showSampleCodexQuestion YES`,
  `CodexCardsTests` mock `MOCK_CODEX=1`, includes BeautifulUI.
- MCP forms and links (#2322, `docs/protocol/protocol.md` "MCP elicitation"; `Shared/Elicitation.swift`, `Views/Interactions/ElicitationForm.swift`): the iOS app alone declares
  `elicitation` in `client_hello`, with `platform` / `version` set to the two halves of its `X-Persona-Client` so the hub also lists these cards in `GET /api/v1/interactions`
  (`LiveConnection.helloFeatures`); the Mac and visionOS apps don't, so with only those open the request goes to the terminal. The rules are the web's and Android's
  (`Elicitation`: the bounded JSON Schema subset, validation, the answer), unit-tested on the web's cases (`ElicitationTests`). MCP Form Needs You: who asks (subagent, MCP server
  and its config source), the message, a row per field in the server's order (`JSONDecoder` drops key order, so `Elicitation.withFieldOrder` reads it off the raw event or
  pending list and keeps it in the app's own `app_field_order`): text, email, URL and date fields with their keyboards, `SecretField` for passwords (never prefilled, never kept,
  cleared as sent), Yes / No with neither picked, one choice or several as option rows; a bad value says so under the field and keeps Submit off. Decline / Submit, Cancel under
  them; a form outside the subset says so and offers Decline / Hand Back to Computer. MCP Link Needs You: who asks, where it opens, the whole URL, Open Link in the app's link card
  (the chat's `openLink`, as every link in a conversation; signing in again there is accepted), and Continue only once this card opened that exact http(s) link.
  What was typed and opened is kept per card in memory while it is pending (`ElicitationDrafts`, never a secret), so folding the card loses nothing. Receipts
  (`Answered.answerLines`) say declined, canceled or opened, or list the values with secrets hidden. Siri / Shortcuts never accept one (`ApproveFirstIntent`), and the
  watch summary leaves them out (`Interaction.showsOnWatch`: an older watch build would fail to decode the whole summary). Not pushed while the app is closed (#2433).
  `ElicitationCardTests` mock `MOCK_ELICITATION=1` / `=unsupported`.
- Input: above (at most 4 photos, resized 1600px; library videos uploaded as files to computer, message file card, protocol.md ("Sending videos"); / shortcuts; Send becomes red Interrupt while working).
- Files (#8361, protocol.md "Sending files"): + › Files and a scan's PDF become cards above the box (`FileAttachments`, `FileAttachmentsBar` in `Views/Chat/FileAttach.swift`), never words in the draft. Each is copied out of the picker first (`FileUploads.stage`), up to 10 per message, three uploading at once; ✕ cancels or removes, a failed card stays (tap to try again). They live in `ComposerAttachments.files` beside the videos, so a draft rebuilt (#2626) or a queued message taken back (#3022) keeps them. Sent, each is a `<persona-file kind="file">` tag after the words; the bubble draws it as `AttachedDocumentCard` (`Views/Chat/VideoCards.swift`; videos and recordings keep `AttachedVideoCard`), folding past four, opening the file viewer, dimmed when `FilePresence` says the computer no longer has it.
- Navigation subtitle: Machine · Model · State.

## New shell (ChatGPT-style, 2026-11-01)

Requested 2026-11-01: rearrange the iPhone shell like ChatGPT's iOS App: "chat is Persona (major changes), work is Agents (change the entry point)".
The pages are Assistant and Agents (requested 2026-11-01, formerly Chat and Work); new chats use `AssistantWelcome` (restored at the user's request 2026-11-02; previously no to-dos meant a blank page). The shell is the only navigation at compact iPhone width; it needs no appearance setting or launch argument. Regular-width iPad keeps its sidebar, and Mac / Vision retain shared root tabs.
#392: enabling Show messages on home in Lark settings adds a third Messages page at the top, otherwise two; the regular-width iPad adds a corresponding tab. It only browses chats and reuses existing chat pages; boundaries in [Lark migration](../extensions/lark-migration.md).
#1530: the private extension provides Lark gateway integration; entry points depending on `/api/v1/lark/…` appear only with hub feature `lark` (`LarkFeatures`, `App/LarkFeatures.swift`): Messages page and its switch / tab (stored toggle preserved, restored when the extension is installed), Settings › Connectors › Lark, Lark chats in Translation, native Lark document reader; native Moments reader additionally requires `lark_moments`. Without them, Lark document and Moments links open as ordinary websites; Inbox items with `lark` / `doc` / `moment` blocks only come from the extension and render normally. Empty features (before hello, demo mode), or only `lark_attachments` without `lark` (hubs that integrated Lark themselves before the extension), retain all existing UI. Mock `MOCK_NO_LARK=1` simulates no extension.
Only at iPhone compact width; iPad, open iPhone Duo, Mac and watch remain unchanged (iPad and open Duo use drawer ordering in Persona's sidebar, see iPad), except the open Duo's new conversation, which is `ShellHome` (#3680). Code in `Persona/Views/Shell/`, components in
`DesignSystem/ShellComponents.swift`, `ShellComposerBar.swift`, colors and sizes in `Theme.Shell` (the same neutral palette across styles).

| Part | Approach |
| --- | --- |
| Bottom | No tab bar |
| Top bar | Avatar · Assistant \| Agents (work shows Agents waiting for you) · right button. Assistant uses floating `AssistantTopBar` over the chat (`shellBar` in the shell); Agents uses `AgentsTopBar` over the system navigation bar, with button positions identical to the pixel. The top of the Agents page on a phone (search field, quick filters, the bar making way) is one component, `AgentsHeader` (#2478): the navigation bar never changes height, and the room for the taller shell bar is the list's own `safeAreaBar` (`Theme.Shell.agentsRoom`, not the navigation container, unchanged on returning from a chat, #340); both bars cap button scaling at xxxLarge (`Theme.Shell.topBarLargestTypeSize`), and so does the search field's text (what is typed and the placeholder, `AgentsSearchField.largestTypeSize`): its 44 pt capsule and 54 pt band stay the same at every size, so the fold, the pull and the rise keep their geometry. The field is UIKit's and sizes its words as it is made; the one in the list's row is made at launch, before the window's cap, so it fits them again on entering a window and on every text-size change (#10165, `AgentsSearchFieldTextSizeTests`). At rest with an empty query the field is the list's first row and folds with it; a retained query stays in the same row position in an opaque overlay while results scroll and the page returns to the front. Tapped, either field floats into the navigation-bar row (with ✕), and the list and the fade of `AgentsTopBar` follow the same animated value: the bar is gone before the field arrives and the sidebar cannot open; cancel restores it (no flash when opening a search result; checked frame by frame by `AgentsTopMotionTests` at the standard text size, xxxLarge and the largest accessibility size). The floating field publishes its actual animated progress, and the bar uses the later of its own and that published progress before becoming fully visible again at rest; a tab return during a close folds after the field leaves (#10166). The regular-width iPad uses `AssistantFullSearch`, whose field stays in its fixed bottom safe-area inset while results scroll, so there is no drawer fold to change; Mac and Vision keep their existing system search. No results says "No results for ‘…’", not "Start your first development session" (`ShellSearchTests`). Tapping Assistant again opens the former avatar menu (Knowledge, details, launcher, session info, interrupt…) |
| Top-left avatar | Formerly ☰. Requested 2026-11-01: the old centered avatar no longer fit after the bar change, so it becomes the top-left Chat history entry, displaying the avatar. Both pages (and dragging `ShellMovingBar`) share `ShellPersonaButton`: current avatar's state face (`PersonaDot`, identical to the old center one; five states in `docs/apple/persona.md` ("States")), based on the assistant page's current chat (`assistantCurrent`), identical on both pages and while dragging; tap opens the drawer. Same size as the right circular button (`Theme.Size.tapTarget`, grows with text). When Inbox needs you, a small pale-red bottom-right dot (`InboxDot`, `Theme.inboxDot`, requested 2026-11-04; the previous red tray badge outside the face was too prominent): no number or icon, diameter `Theme.Size.dotXL`, centered at 45° bottom-right and wholly inside the face (`dotShift`), with a thin page-background outline. Pale red keeps it apart from the strong red of errors, from Needs you's blue and from the avatar's top-right thought bubble / z. VoiceOver says Chat history, value "Persona, idle" (avatar name and state), plus "Inbox needs your attention". Chat, interaction, machine, connection and Inbox data are read only inside the button, preventing push-driven top-bar rebuilds (#178; `AgentsTopBar` is Equatable). `ShellAvatarTests`; use mock `MOCK_INBOX=empty` for no badge |
| Pages | One horizontal strip: drawer \| Assistant \| Agents (`ShellPager`, requested 2026-11-01). Drag left from Assistant or right from Agents to follow the finger to the other page; the selected switch segment tracks the finger. Release follows drawer rules (fast flick switches, otherwise past halfway), with the same spring. Tapping switches slides too. `AssistantView` stays mounted; `ChatsView` is created on first switch / drag, then retained, preserving position; at rest the other page is transparent and noninteractive. While dragging, page top bars hide and shell `ShellMovingBar` replaces them (avatar and switch stationary, right button morphs from New chat to New session), as page-local bars would partly cover each other |
| Horizontal gesture ownership | A UIKit pan (`DrawerPan`) decides from actual displacement after a few points: within 45° is horizontal, steeper belongs to list scrolling. Assistant: right opens drawer, left goes to Agents. Agents: its rows have no swipe (#1244; their actions are in each row's ⋯), so a drag pages from anywhere on the page, rows included: right returns to Assistant, left goes on to Messages when it is enabled (#2822). Agents no longer opens the drawer by right swipe; use the avatar. Code blocks and tables that can scroll in that direction get priority; dragging in text being edited never switches pages; pushed pages (system back swipe), search and an open drawer also disable paging |
| Sidebar | Lifted from Persona tab into the shell (`ShellState.drawer`), accessible from both pages. Assistant at root opens it with a full-page right swipe; both pages can tap the avatar (right swipe on Agents returns to Assistant). Main chat is first, as **Main Chat** with the main conversation's symbol (`Theme.Icon.home`) in the places' symbol column, under the head that shows the assistant's face and name (#9973; it carried the face and the assistant's name before, #2004, #3203, #6401; display only, session id unchanged), then a separator, Inbox (count), To-dos, Scheduled tasks, Knowledge, Machines, another separator, then the history: Directions (the leader conversations, `assistant.leaders`, in the hub's order under each direction's name from the direction tree `GET /api/v1/assistant/directions/tree`, else `GET /api/v1/assistant/leaders`, `AppModel.directionNames`, with its "N need you" from the tree's `counts.needs_you` (nothing when zero; its secretary group skipped, #4352) and the row's spinner while its own turn or anything under it runs (the tree's `running`, #6626; never "N working" beside it), both read with `DirectionTree.leader(_:)`; the heading reads Pinned, translated from the existing `Directions` key, #4331 each lead row also folds open to its to-dos, see "Pinned directions (#4352)" below and `docs/apple/sidebar.md` "Pinned directions", then **Recent** (#4873, #5195, `AssistantSidebarGroups.recent`): the 5 most recently active conversations that are neither the main one, a direction, the background supervisor nor a scheduled task's own chat (#6228: those stay under Scheduled Tasks and All Chats; a task chat a hub did not mark with `origin` is still known through `AssistantTaskChats`, #2838), whoever else opened them (the user, Lark /agent, the Inbox's AI twin…; a row Persona opened carries its source's mark, `ConversationSourceMark`, the symbol and name of its All Chats filter decided by `AutoConversations.source`), with no heading when there are none. The Recent heading folds the rows (`@AppStorage("assistant.sidebar.recent.folded")`, default open, the open conversation's row still shows while folded, #6228), then one **All Chats** row opening the full list with its filters and search. This phone's Pinned group, the Other Chats heading, the Automatic chats row and Show All are gone (#4331), and the hub's `directions` push (no payload) plus every (re)connect re-read the tree, keeping names, counts, order and new directions current without a relaunch (an older hub 404s: names still come from `GET /api/v1/assistant/leaders`, no counts). The long press on a conversation makes it a direction or ends one over the hub, `POST /api/v1/assistant/directions/{id}/make` / `end` (Pin / Unpin, #4331); both ask a second time in a `confirmationDialog` first (#6214) — unpin's message says the chat returns to Recent and its open tasks go back to the main chat's supervision, nothing closed or deleted — and the rows carry no pin glyph (the 常驻 heading alone marks them, matching Android and web). Each 常驻 row folds open to its to-dos through a trailing chevron (`AssistantSidebarDirection`, 44 pt target, spoken label + Expanded/Collapsed value); on the Mac it also carries a `.help` hover label saying whether it expands or collapses the direction's open tasks (#6214), since a pointer there needs a worded hint like the web's. Its "N need you" stays in the row and never moves for the menu. Main chat appears once, excluded from Directions and Automatic chats; its row remains before its session is listed or any other chat exists (tap opens main chat, or New chat if none exists; disabled until assistant configuration arrives). Its right side shows Working / Waiting for you / Unread; long-press offers Compact context / Clear context / New main chat, Copy session ID, Copy diagnostics, End while running… (Clear context only with hub feature `assistant_main_clear`, #5100); it cannot swipe left to archive. #4606 / #4916 moved it above Recent and renamed it Main chat; the owner wanted the #2004 layout, restored by #5110. Bottom theme-colored New chat pill (#1312) + Settings || Sidebar destinations | All push onto Assistant's stack (`ShellState.show` → `AssistantView.show`). Inbox and Machines use value-based pushes (`ShellRoute`, `ShellDestinations` registers them and their InboxItem / Lark / machine-detail / session destinations): a value-based push from a view-pushed page inserts the new page underneath it (docs/lessons/ios.md) |
| Settings | Full-screen sheet (`model.shellSettings`, in the model because style changes rebuild the tree and the sheet must return to the same page), × at the top left, as every presented page (#9653) |
| Empty chat | Shared welcome (`AssistantWelcome`: greeting, explanation of new / main chat, the token calendar, six suggestions; Liquid Glass two-column cards, BeautifulUI "You can ask me" list card). The first 2026-11-01 version showed only the input and was blank without to-dos; user restored the old design 2026-11-02. Above input: Agents waiting for you, then Inbox Needs you, at most 3 rows + "N more in Inbox" (`ShellNeedsYou`), hidden if empty; page background beneath this and the input, with welcome scrolling underneath and fading at the top rather than clipping. Rows adapt to space (2026-11-02, #283; 3 rows + input occupied half Duo's outer screen and cut suggestion cards): never cover the first suggestion (`NeedsRoom`: measures its lower edge, page height excluding keyboard, resting input height), reduce rows when needed, minimum 1. The shell's cards are a non-lazy `Grid`, so their edge is measured even below the page: at the largest accessibility text sizes the greeting fills it and Needs You keeps one row (#4073; a `LazyVGrid` never made those cards, and three rows covered the welcome); ordinary iPhone Liquid Glass 3, 17e BeautifulUI 2, Duo outer / SE 1. If hidden items include Agents, say "N more need you"; if only Agents remain, tap goes to Agents (they are not in Inbox). The token calendar (#9506, `Views/Assistant/TokenCalendar.swift`) replaced the online computers' and the iPhone's chips (labels that did nothing; the connection is in the title): every computer's tokens a day from `GET /api/v1/usage/days` (the same count as Today on the Machines page, docs/server/machines.md "Token usage history"), one square a day in `HeatGrid`, Monday on top, as many weeks as fit (19 on an iPhone, the year's 53 at most, the iPad reads the year), today outlined, the steps by rank among the days with usage (`HeatLevels`), the months over the weeks they start in and Monday / Wednesday / Friday at the left. Its line says today's and the last 30 days' tokens in the language's short numbers (`TokenFormat.short`: 32亿 / 3.2B / 32億); a tapped square shows its day and the four kinds, tapped again it goes back; the line opens **Token Usage** (`TokenUsagePage`): today / 7 / 30 days, the calendar with its key (no totals line over it, a picked day's line only), the last 7 days by the hour (`UsageHourGrid`, `GET /api/v1/usage/hours`), by computer, by agent (#10015: each agent's mark and name as in Agents, `AgentKind(usageAgent:)`, names from `spec/fixtures/agent-names`; usage the hub kept before it counted agents apart reads "Not Split (Before the Update)"; no section from an older hub) and by kind. Read when the page shows and when the app comes back to the front (`.tabActiveTask`), no timers; no card until the hub has history with usage in it (an older hub, no computers). Screenshots: `-showTokenPick busiest` (a day picked), `-openTokenUsage YES` (the page); the mock hub's `/api/v1/usage/days` and `/hours` are made up and fixed by the date (`MOCK_TOKENS=empty` none, `none` an older hub). `AssistantWelcomeTests` guards both layouts and drawer New chat: greeting, explanation and first suggestion visible above input; Needs you does not cover the first suggestion and focusing input does not change row count (also run on Duo outer screen); at the largest text size one row, under the greeting (`testNeedsYouAtTheLargestTextSize`) A first message whose start failed (#1368, as the web decided in #1122) stays where it was sent as the chat's failed bubble (`PendingBubble`): its red mark's menu has Resend and Delete, the reason sits over the input (with the readiness card for a refusal), the input stays empty and takes no new first message until it is resent or deleted, Health waits too. Kept across launches (`FailedStart`). Resend goes with the old op id only when the hub says that start is done or still running (`GET /api/v1/ops/{id}`), as a chat's Resend. `FirstMessageFailedTests` After Clear context (#5100) this page stays up with an empty middle and the composer at the bottom (`main-cleared`), not the centered welcome; `/clear` there is listed and does nothing, and the first message starts the new main chat. |
| Input | One row: [+] Ask Persona [microphone]; focused or nonempty it grows into a card, text above, then [+] [Hide Keyboard] … [Model · Effort] [microphone] [Send] (`ShellComposerBar`, shared with Agent chats, #1091). Long drafts and expansion on iOS (#5685): the line limit is the available height above the composer's bottom edge minus the window's top safe area × `Theme.Shell.composerHeightFraction` (22%) ÷ the current line height, floored and clamped to 3–8 lines, matching Android and web, and recomputed on keyboard, orientation and window changes. Below the limit it grows naturally; at the limit it caps its height and scrolls internally. Whenever the composer is focused, holds a multiline draft, or reaches the line limit (`ComposerGrowth.showsExpandButton`), the Expand button (`shell-composer-expand`) appears in the upper-right corner of the text area and reserves its trailing width; the full-page editor (`ComposerPageEditor`) offers Hide Keyboard (`composer-page-dismiss-keyboard`) and Collapse (`composer-page-collapse`) at the upper right and Send (`composer-page-send`) at the bottom, preserving the draft and cursor on collapse. While the keyboard is up, a Hide Keyboard button (`shell-composer-dismiss-keyboard`) sits beside `+` in the bottom toolbar row, committing active IME composition, dismissing the software keyboard and keeping the draft; the message list also dismisses the keyboard interactively on scroll (`.scrollDismissesKeyboard(.interactively)`). Mac and Vision retain their original composer height. Always the same view, preserving focus and Pinyin marked text while morphing. Model / effort is compact text like ChatGPT "6.1 Sol Medium" ("Opus 5.5 High", #1117): no icon or red, model in bold primary text (`secondaryStrong`), effort in secondary text with a small space; truncate long model names at the tail, never effort. Right-side space matches the circular Send / Stop button margins, making Model ↔ microphone and microphone ↔ Send / Stop gaps equal (`SessionModes.composerTitle` / `modelName`). GPT display names drop `gpt-` and word-format suffixes (`gpt-6.1-sol` → `6.1 Sol`), preserving version and variant; directly related model choices use the same format, custom display names take priority, raw model IDs, saved values and requests unchanged (#2200). The menu opens upward, nearest the button first: red Allow everything warning (`BypassWarning`, no input label), model, effort, then nested permission mode (`SessionModesMenu`; new chat `StarterModesMenu`). A new assistant chat's model section lists the launchers, each one's models a level down (#11472, as the running chat's model sheet and the web's model panel: the catalog's rows with its Default entry, else the configured model; a lock's reason; Reading models… or Retry), read from `GET …/launchers/{name}/models?purpose=assistant` when the composer appears (`AssistantStarterModels.loadLists`; sending never waits for them). With nothing picked the button shows Main chat's default, launcher and model (`default` of `?purpose=assistant`), and the start names neither launcher nor model, so the hub keeps no pick over the default; a model picked under a launcher goes as `launcher` + `model`, a launcher's Default entry as `launcher` alone, and the default's own row picks nothing (`AssistantStarterChoice`); Follow Main Chat default drops a pick. Model selection only for Claude Code in herdr panes (`/model`) and new assistant chats (a launcher and its model); existing assistant chats and Codex only show the current model. Codex / the internal Codex fork have no permission mode; app-server Codex has effort (`efforts`), others do not |
| Input motion | Timings and thresholds taken from Telegram iOS's input panel (behavior and numbers only, no code; it is GPL). The card opening on focus moves with the keyboard's iOS 26 spring (`Theme.Motion.keyboard`); a line more or less eases over 0.1 s (`composerGrow`, Telegram's main chat); Send comes into the round button's place from a point by the right edge (scale 0.001 → 1 on a 0.4 s curve, fade 0.2 s, blur 4 → 0) while Stop / Voice shrink to 0.2 and fade (`composerSwap…`), and Send shrinks to 0.75 while pressed. Quotes over the field slide in by their own height from under the card's clipped top edge, fading in 0.15 s and out 0.2 s, as the card grows on the 0.4 s curve (`composerAccessory…`). The mic, tapped, turns the composer's capsule into the dictation bar (`DictationBar`, after ChatGPT's): [✕] a wave and ■ ↑, ✕ where `+` was and ■ where the mic was tapped (#9619: [■] [↑] when voice or stop followed the mic on the line or send did in the card, [↑] [■] when the mic ended its row; kept in step with the composer while the bar is off, so the tap's frame reorders nothing, and after a close only once the bar has gone, `Theme.DictationBar.reorderAfter`; never animated), so a second tap in the same place stops and sends nothing, laid over the one-line composer, which stays under it so the capsule keeps its size and the field its draft (it takes no touches, and leaves the accessibility tree 0.45 s after the bar opens: rebuilding the tree on the tap's frame made that frame the bar's longest); the keyboard goes down first. ✕ drops what was heard; ■ puts it after the draft and sends nothing; ↑ puts it after the draft and sends (not past a send gate, not an empty draft), the bar staying up until the send has taken the draft so the composer does not open into the card for a frame. ■ and ↑ close the microphone at once and give the recognizer up to 1.2 s for its last words (`Dictation.finish`); after 150 ms the pressed button shows a spinner (VoiceOver: the wave reads "Transcribing…"). Dictation ending on its own keeps what was heard, as ■. The bar lets the audio session go 450 ms later on a serial queue, when the microphone is still free (`Dictation.stop(settling:)`): `setActive(false)` blocks the main thread for about 50 ms, which dropped frames in the bar's close. The wave is a `Canvas` drawn once per sample on `TimelineView(.periodic)` (every 90 ms), in a view whose layer Core Animation slides one 6 pt pitch to the left between samples (`WaveStripView`): between samples the app does no work, the render server moves the drawn strip; each tick redraws the strip shifted one pitch and restarts the slide from where the layer is on screen, so a late tick never jumps. A mask fades the oldest seven columns to 25 % at the left and lets the newest come in at the right. The ticks read `DictationMeter` (RMS of each microphone buffer in dBFS, −50…−14 mapped to 0…1, written on the audio thread, observed by nothing), so a louder voice redraws only the strip, never the composer; `DictationWave` smooths the level exponentially (50 ms up, 180 ms down); quiet columns are 3 pt dots, loud ones grow to 72 % of the wave's height and darken. While the bar is off the timeline stays but ticks once an hour, so a tap builds nothing. (A canvas redrawn every frame on `TimelineView(.animation)` and a SwiftUI offset animation both cost more app CPU than the composer before the bar; `docs/lessons/ios.md`, "The dictation bar's wave and measuring it".) The bar's parts are built with the composer and drawn while off at 0.1 % opacity (fully transparent, SwiftUI skips drawing them, and their first drawing landed on the frames right after the tap), so a tap only animates them (while off they also go without their accessibility labels and identifiers: XCUITest lists them although hidden, and the off ↑ answered UI tests' lookups of the composer's send, #7684): the bar comes in on a 0.42 s spring (bounce 0.16) with its buttons growing from 0.6 a beat later, and closes on a 0.32 s spring with the wave folding into the button that closed it (`Theme.Motion.dictation…`, `Theme.DictationBar`). ✕ ■ ↑ shrink to 0.75 while pressed and give a light haptic from a generator prepared when the bar opens (a cold one blocked the first impact for 100–600 ms on a simulator); the mic's tap gives none, as before. Reduce Motion: fades only, and the wave does not slide (every column at the current level, redrawn per sample). Measured against the composer before the bar: `PerfDictationTests` and the host-driven comparison in the PR (#5843). Held 0.3 s (`Theme.Hold.delay`; Telegram's 0.19 s took a slow tap for a hold that heard nothing, #9619) it dictates while held (`ComposerHoldMic`, `MicPress`, `DictationHold`): a strip over the card shows a blinking red dot, the time, what is heard so far and "‹ Slide to cancel" following the finger at half its pace; a circle follows the finger left. Letting go puts what was heard after the draft; sliding left 150 pt (100 pt or a 400 pt/s flick on release) drops it. There is no lock (Telegram's slide up): a tap keeps dictation on. The system taking the touch away (a call, an alert), or the mic leaving the screen under the finger, keeps what was heard, as letting go does. The heard words stay out of the field while held so the card does not change shape under the finger. Not taken: voice messages (Persona sends no audio), the interactive keyboard drag (the flipped list blocks it, #1823), an edit bar (Edit only replaces the draft), long-press send options. `DictationHoldTests`, `DictationBarTests` (a second tap where the mic was, a slow tap, a hold let go, a hold slid left), `TodoNotesDictationTests`; `-showDictationSample "<words>"` hears those words instead of the microphone (simulators), and `-showDictationHold <points>` draws a hold a few seconds in with those words, the finger that far left (screenshots) |
| Attach panel | On the iPhone the input's + opens a panel (`AttachSheet`, `AttachLayout`, tokens `Theme.Attach`, `Theme.Motion.attachTabFade` / `attachLens` / `attachBarSwap` / `attachPick…` / `attachUnpick`) in a system sheet, like the system's photo picker (#4312): it opens at half height (detent `.medium`) with the chat dimmed above it and drags to full (`.large`) with the system's grabber; scrolling the grid at half height grows it first, and a swipe down, a tap on the chat or × closes it. The composer's keyboard goes down as it opens. A header under the grabber has ×, the title (Recents, or the tab's name) and All Photos; the header and the tab pill stop growing at `Theme.Shell.topBarLargestTypeSize`, like the shell's top bars. The bar at its foot sits on the sheet's safe area (`safeAreaInset`), and the grid scrolls under it to its last row. Tabs float in a pill (62 pt tall; 160 / 240 / 300 pt for 2 / 3 / 4 tabs): Photos (recent photos and videos, three equal square columns 1 pt apart, thumbnails cropped to the center; the camera, where there is one, is the first cell, the same size as a photo; a mark in each cell's corner bounces as it fills with the pick's number (a cell takes taps only in its square: a landscape thumbnail filling it is laid out past it, and the cell to the right took the taps on the ring, #3702, `AttachGridTests`); the mark and a 45 pt corner round it pick, the rest of a photo opens it in the image editor below (a video's picks, #4963); up to 4 photos and 4 videos as the composer takes), Files (Browse Files: the system's picker, uploaded as before) and More (what + offers besides photos and files, the same list as its touch-and-hold menu below: Voice Chat, Take Photo, Scan Document, Glasses Photo, Record in Persona's main conversation, Mention a File or Session, Add as Context, an Agent's quick commands, extension prompts). Switching tabs, the old page fades out over the new (0.23 s); the highlight slides under the tab (0.4 s); tapping the tab shown opens the sheet to full. Photos picked, the pill cross-fades into a caption field (the draft itself) and Send, typed at once at either height; Send sends the photos with the draft, closing in any way keeps them in the composer (nothing picked is lost; the picks live in `AttachPanel`, so the sheet's `onDismiss` hands them back after a swipe down too). Typing the caption, the keyboard pushes the bar up through the sheet's safe area and the sheet goes to full height, so the field and what is typed stay in sight; a drag of the grid puts the keyboard away and the caption back down (#3704, #3004; `AttachCaptionKeyboardTests`, `AttachGridTests`). A photo opened from the grid (#4963, `AttachPhotoEditor`) is decoded off the main thread at 1600 px (from iCloud if it has to be; a spinner and Cancel until then) and shown in the image editor with the panel's caption (the draft) and Send under its tools: Select (Done once it is picked) picks it as edited and goes back to the grid, which draws the edit in its cell with a pen mark (VoiceOver: "Photo, edited"); Send picks it and sends everything picked once the editor has gone; Cancel leaves the pick, the caption and the photo as they were. The edits go with the outcome (`AttachPicks`) into the composer's strip, where they can still be changed. VoiceOver's activation of a cell still picks; Edit is its action (`AttachPhotoEditTests`, `AttachGridTapTests`). Limited access shows a line with Manage (the system's limited picker); no access shows Open Settings and Choose Photos (the system's picker, which needs none); All Photos in the header always opens the system's picker. Why not the system's `PhotosPicker` inline: the panel also holds the camera cell, Files and More, numbered picks capped by the composer's room, the editor, and picks as library identifiers for the existing upload path; the sheet, its detents and grabber are the system's. Not taken: the live camera preview (the cell opens the system camera), albums under the title, a video's preview (a tap on it picks it), swipe to pick several, pinch to change the columns, Lottie icons, Location / Poll / Contact tabs. Touch and hold + opens a system menu instead (`Menu(primaryAction:)`, #3225; the tap stays the panel, as Safari's tabs button and Mail's compose button): the More tab's list (`attachMoreSections`, drawn by `AttachMenuItems`), the most used nearest the finger — Voice Chat, Take Photo, Scan Document (#3720: VisionKit's document camera, `DocumentScanner`, where the device can scan; the pages go as the message's photos while they fit in the room left, else as one PDF uploaded as a picked file is, `DocumentScanner.delivery`), Glasses Photo, and in Persona's main conversation Record (#3723, #5243: the Recordings page's button, by its words; it starts the meeting with Recordings open, or reads "Recording… Open" and opens the one running); Mention a File or Session; Add as Context › Knowledge Note… / To-do… / Inbox Item… (#3721: one searchable picker, `ContextPickerSheet` / `ContextPickerModel`, over `kb/<kind>` (the latest projects, people, documents and meetings before anything is typed), `kb-search` (once words are typed), `todos` (open first) and `inbox`; the pick goes over the box as a quote, a `ContextRef` sent as `<persona-context>`. A note carries its summary, latest timeline and path in the knowledge base (`KnowledgeDetailView.contextRef`), a to-do its title, notes and id, an Inbox item what Hand to AI sends (`InboxHandOffSheet.contextRef`); a note's and a to-do's block in the sent bubble open it (`persona://kb/…`, `persona://task/…`), an Inbox item's unfolds, as the app has no address for one); an Agent's Quick Commands folded into a submenu (`Theme.Icon.quickCommands`, a command's "/" word as its second line); Extension Prompts folded too. Rows a chat or device can never use are left out (no camera, Persona's conversation has no agent quick commands until Clear context is offered); the main chat adds Clear context /clear to the quick commands when the hub has `assistant_main_clear` (#5100), and /clear while an Agent works is dimmed; Voice Chat always shows and says why when no call can start. The iPad's + is that menu on a tap, with Photos and Files first. A new conversation's composer (`StarterComposer` in the shell) is a conversation's here (#3805): the same panel, menu and rows through the shared pieces (`AttachPanel.opener`, `ComposerAttachments.addPicked`, `AttachMoreSection.common`, `ComposerCaptureSheets`), less what needs a conversation (Voice Chat; an Agent's quick commands); its text, quotes and attachments stay in `StarterDraft` (#2626) until the first message starts the conversation. Health's new conversation has the same composer on iOS in every style (#3825): the panel, the menu and Persona's new conversation's model button (`StarterModesMenu` with `ShellModes.health`: the launcher defaulting to Health's, the effort and, a level down, the permission mode); its uploads still go to `reports/`. The Mac and visionOS keep their menus (`ComposerRow`, or BeautifulUI's `BeautifulStarterComposer`). `-showAttachCamera YES` shows the panel's camera cell and `-showCameraRows YES` lists Take Photo and Scan Document in the simulator for screenshots; `PersonaUITests/AttachMenuTests`. The photos are fetched off the main thread, and a chat (or a new conversation's composer) warms the Photos library once in the background when access is already given, so the first + doesn't wait for the Photos database (#4250). A video picked in the panel is copied off the main thread into a directory of its own and deleted however its upload ends; a slow-motion one goes as its original. `AttachLayoutTests` |
| Chat | User bubbles share Agents-chat colors (`Theme.userBubble` from the interface style, #2647; previously shell-specific neutral gray) |
| Slow network | Small pill below the top bar (Waiting for network / Connecting…), shared by both pages; the page switch occupies the original title area |

Routing stays unchanged: all navigation still writes `AppModel.selectedTab` (notifications, Siri, hand to Persona, Inbox rows…); the shell treats `.assistant` / `.chats` as its pages,
`.mail` as "push Inbox on Assistant", `.settings` as "open Settings sheet", then returns to the original page. Foreground page has one source, `ShellState.page`;
writing `selectedTab` forwards immediately to the shell (not through `onChange`), and switch segments and dragging use it too: see `docs/apple/shell.md`.

Inbox-linked conversations use `InboxChatRoute` wherever `ShellDestinations` is enabled: the iPhone shell, regular-width iPad / open Duo (`padPlaces`), and the unified Sidebar layout (`embedded`). `pushInboxChat` is supplied under exactly the same condition and injected directly on every destination, as is `feishuOpen`, because an iPad split does not forward the root content's environment to those pages. It appends to `navPath` (the unified Sidebar's host path there, the current place's `NavigationPath` otherwise), so `ShellPlaces` saves that exact path, including `InboxChatRoute`, whenever the place is left. so `ShellPlaces` saves that exact path, including `InboxChatRoute`, whenever the place is left. The pushed `ChatView` does not yet expose the new place-sidebar button in its custom `devChrome`; that integration is follow-up work. This covers the timeline's handling and Agent rows, the draft's processing view, Hand to AI's View (main, new chat or Agent) and New Agent completion, Thread Agent rows (including an Agent inside a to-do sheet, which closes before the push), waiting-Agent rows in the inbox and on the new-chat page, and a summary started from an inbox source chat. Back, including the left-edge swipe, returns to the item, inbox list or new-chat page that opened it. A waiting card pushes only when its `ix.sessionKey.group` is the current hub's group and keeps that key unchanged; another group's card keeps `model.open`, which selects its own group's Persona for an assistant conversation. Sessionless cards still open as sheets, pushed chats keep `ChatView`'s default style, and Mac / Vision / classic layouts keep their previous destinations because the closure is nil.
A chat pushed over the new-chat page is discarded when `assistantCurrent` changes, because New Chat is not a `ShellPlaces` place. `AssistantCurrentNavigation` keeps pushed pages when the app automatically changes no current conversation to the main one (`old == nil`, nonnil `new == assistantMain`), including manager reconnects; an explicit user reveal still goes to the root. A real hub-group change forgets every saved place and clears the old hub's inbox and item pages, while initial pairing (`nil → group`) and reconnects do not. `InboxBackNavigationTests` covers these rules and every route fallback; `InboxBackTests` covers phone and wide edge-swipe return paths.

UI tests always use `launchPinned()` to fix the style (the run's, unless the test passes `-ui.style` itself, #5683) and `launchPersona()` for clean Demo Mode startup, without overriding the app's default navigation.
Shell tests: `ShellTests`, `ShellAvatarTests` (top-left avatar and Inbox badge), `ShellPagerTests` (paging), `ShellSwitchTests`, `ShellSearchTests`. DEBUG: `-openShellPage inbox|machines|routines|knowledge|profile`; `-showShellPagerAt 0.3` holds the strip 30% across for screenshots; `-showHomeState empty|error` (new-chat page: nothing needs you / a failed-start notice, for screenshots).

## Foreground-only work (polling and reporting read state)

Foreground-only work requires "the view is in the window and its page is foregrounded" (`Views/TabActivity.swift`): **polling**, **reporting read state**, and a chat's **display subscription**.
Previously the system `TabView` removed the old tab from the window on switching (`onDisappear`, cancellation of `.task`). The new shell's Assistant and Agents pages are a ZStack;
the hidden page is merely transparent and noninteractive, **still mounted**, so `onDisappear` never fires: without a signal it would keep polling and mark messages read for the user.

- Pass down `tabIsActive` (true outside any page): `ShellPageSlide` sets it from `ShellState.page` in the new shell; regular-width iPad and open Duo `TabView` use `TabActivityScope` on each tab's content.
- `.tabActiveTask` / `.tabActiveTask(id:)` cancels when the page goes behind or the scene becomes inactive, and restarts from scratch when foregrounded. `.onTabVisibility(appear:disappear:)` replaces paired
  `onAppear` / `onDisappear`, each called once; the system briefly remounting the old tab during a switch does not count as appearance. Its default deliberately ignores `scenePhase`: business lifecycles
  such as voice end on a real page departure, not Control Center or a background transition. Polling that must stop in an inactive scene opts into `onTabVisibility(sceneAware: true, ...)`; file-operation polling uses this option.
- Used for Lark chat's 60-second fallback polling and read reporting (catch up on foregrounding), and Machines-page (`MachinesView`) / machine-detail (`MachineDetailView`, including its computer-control and browser sections) polling, and the readiness pages' polling while a step can finish on another computer (`ReadinessPoll` in `ReadinessViews.swift`, which also requires the Mac app to be active).
- Shared `ChatView` transcript streams and visible/read ownership use paired visibility edges on iPhone as well as Mac. Hidden chats keep their store and scroll state but unsubscribe; returning resumes with the store's `since_seq`, without recreating the page or downloading all history.
- **Not for** generating replies, Agent tasks, dictation or timers (work-row stopwatch, terminal 250ms tick, header auto-collapse). These continue regardless of the foreground page; releasing a display subscription does not interrupt work on the hub.
- Under system `TabView`, timing matches window removal, so UI and behavior stay unchanged. New polling and read reporting must use these helpers, not direct `onDisappear` / `.task`.

## Persona top bar and sidebar (2026-10-30)

Inspired by dot and ChatGPT App (user-provided screenshots). Code in `Persona/Views/Assistant/AssistantTopBar.swift`, `AssistantSidebar.swift`.

- **Top bar** floats over the chat, with no system navigation bar or solid bar: a downward-fading fog (`TopFog`: page background + material, extending under the status bar), with chat scrolling beneath.
  Chat-list top reserves `AssistantTopBar.height` (70pt); the home page reserves the same via `contentMargins(.top)`. The bar stays fixed while scrolling (unlike the collapsible Agents header).
- **Footer fog** (#3696): the chat runs on behind the capsule row over Persona's composer (`ChatView.underDock`, with as much more room at the list's newest end, so nothing moves) and fades out under `FooterFog`, the page color along the top fog's eased curve, from `ChatView.newestGap` above the footer to solid at the list's edge; in Agents chats it ends at the composer's edge. Jump to Latest and the edge navigator are drawn over the fog. Rules: `docs/design/DESIGN_SYSTEM.md` ("3.12 Chat edges under floating chrome").
- Centered **Persona avatar + name capsule** (main and new chats use the assistant's name as the sidebar's head says it, with one assistant too, #9973; the look's name, default Persona, on a hub without a roster; side chats use the chat title).
  Beside a sidebar column that is out and whose head draws the face (the iPad and the open iPhone Duo, both layouts), the middle is the **name capsule alone**, the drawer bar's (`drawerRow`), with the same menu, the hub picker included (`AssistantTopBar.faceInColumn`, #9973): the screen shows the face once, and it comes back with the column folded. The avatar is the current avatar's **state dot**
  (`PersonaDot`): Idle / Thinking / Working / Needs you / Offline animations, see `docs/apple/persona.md` ("States"). Tap for Knowledge, Details (profile
  `AssistantProfileView`), Return to main chat (side chats); Copy directory (assistant cwd), Copy info (chat name, session ID, machine, cwd, launcher / model,
  `claude --resume` command, convenient for another Agent, `AssistantCopyText`), Copy session ID; launcher submenu, session-info card, Interrupt while working.
  Copy overlays "✓ Copied…" on the name capsule for 3 seconds (not global toast, which would cover the avatar). Slow / offline networks replace the name with spinner + Waiting for network / Connecting… / Updating…,
  and the menu's first item becomes Connection details.
- Left circular **Chat history** button opens the sidebar; right circular **New chat** button (disabled on the new-chat page). Both icon-only.
- **Assistant model menus** request `purpose=assistant` for both launchers and their models, including the new-chat
  composers and the typed-model sheet. They use the assistant catalog separately from New Agent's cached list: on a
  Paseo computer, a shell launcher can share a provider's name while running different credentials and models.
  Older hubs without the purpose filter keep the shell-launcher eligibility fallback; an explicit assistant capability
  takes precedence over agent kind. The menu's visual layout and the saved Main chat default are unchanged.
  New-chat effort choices come from that launcher's effective default model (the Main chat binding when it applies).
  Switching launchers resets the explicit effort to Follow Launcher; an unknown list offers only that inherited choice.
  Catalogs and cached choices are scoped to the manager, computer, launcher and settings revision; Health uses the
  default manager. A saved effort is checked against already known capabilities before the first message, including
  a suggestion or Resend. Sending never waits for a model-list request, and unknown or failed discovery preserves
  the user's saved choice for server validation. A confirmed unsupported level resets to Follow Launcher.
- **Sidebar** below the main page, 82% of screen width (at most 380pt). The main page covers it, remaining a same-size rounded card when slid right (thin dark-mode border separates it from the sidebar), dimming with progress (`Theme.Fill.scrim`, black 20% light / 45% dark, #334); sidebar stays horizontally fixed and rises only 8pt as revealed (`Theme.Motion.drawerLift`), with no scaling or fading. Tap the card or drag left to close.
  Content (2026-10-31, changed to match Doubao's sidebar from user screenshots): large avatar name (Persona) and a circular Search button on the right (becomes search field + Cancel);
  Main chat is the first row (Main Chat with the main conversation's symbol, identical to the new-shell drawer, #9973), then a separator; actions below: New chat, Scheduled tasks, Knowledge, Health (icon + text, no cards; Health in `docs/assistant/health.md` ("iOS and Mac entry points")); separator; Directions (the leader conversations in the hub's order, each with its "N need you" and its spinner while anything under it runs, as in the new shell) then Recent (the 5 most recently active other chats whoever opened them, except the scheduled tasks' own chats #6228, the others with their source's mark, #4873, #5195; the heading folds it, #6228; **Show Scheduled Task Chats** brings the scheduled tasks' chats and the background supervisor back by time, #8324: the hub's `recent_task_chats`, one value for all of the user's devices, off by default, switched in Assistant Profile › **Sidebar's Recent** or the ⋯ at the top left of All Chats, opposite Done, a menu with its check; `AppModel.setRecentTaskChats` shows it at once and puts it back if the hub says no; a hub without it shows neither; the app reads the assistant's settings again when it comes back to the front, so a change made on another device shows) and one **All Chats** row opening the full list; no inline Pinned group (#4331). Each row has one circular marker (pale chat bubble color chosen by session id, adjacent rows different) + one-line title,
  with a right-side state mark only for Needs you / Unread / Working (`RowStateMark`: a blue dot, a green dot, the amber spinner; the same mark on the main chat's row, the Directions rows and in All Chats, #4538); the last message appears only in long-press preview. Current chat has a rounded gray background. Bottom row: avatar + name + Online / Offline and machine
  (tap for profile), Settings on the right (drawer only; split mode has the tab bar beside it). Search lists only results (N chats). Long-press also copies session ID or ends the chat.
  **All Chats** (#3264, `AutoConversationsView`): every conversation, filtered along the top by All, Mine and each origin the hub reports (`conversation_origin`, session `origin`, `docs/protocol/protocol.md` ("Origin of assistant chats")): Inbox AI twin, scheduled tasks, Lark /agent, derived chats and extension-created chats. It opens on All every time (the last pick is not kept), and its rows carry the same source mark as the sidebar's (#5195): for a chat an extension opened, that extension's own icon once it is drawn, with "From <its name>" for VoiceOver and under the first message (#10358, `ConversationOpener.swift`; `docs/design/DESIGN_SYSTEM.md`). Same view in iPhone drawer, iPad split and Mac sidebar. Search still searches all. Old hubs without origins omit the source filters.
- **Gestures** (2026-10-31, rebuilt to match ChatGPT after build 49 feedback "not smooth, not following the finger, small area, swipes do nothing"): right-drag anywhere on the page opens the sidebar,
  using the same state as the button. UIKit pan (`DrawerPan`) decides after roughly ten points, from **actual displacement since touch-down**; within 45° of horizontal belongs to the sidebar
  (`AssistantDrawer.takes`; thumb diagonals typically 30°–40°, vertical scrolling steeper); when closed only rightward motion counts. Previous pan translation / velocity logic
  read translation 0 at that moment and velocity estimated from the last two samples, misreading a 30° swipe as nearly vertical and yielding to the list: "swipes do nothing".
  Horizontally scrollable content under the finger that is not at its left edge (wide code blocks, tables) gets priority; at its left edge, right-drag opens the sidebar. Every page scroll-view pan waits for the sidebar's decision
  (`shouldBeRequiredToFailBy`; it decides within the same few points where scrolling normally begins, so scrolling is not delayed); otherwise a 40° swipe's vertical component crosses the list threshold first,
  which steals it before the sidebar is asked (at the chat bottom it looks like no response); code blocks at their left edge also steal it for bounce. Pushed pages (profile, Knowledge, scheduled tasks) use system back gestures instead.
- **Release** matches system back: above 300pt/s follow velocity direction, otherwise open past halfway (including 0.2-second momentum projection, `settlesOpen`); snap with a non-bouncing spring
  (`Theme.Motion.drawer`) inheriting finger velocity so fast flicks do not pause before moving.
- **Finger tracking**: while dragging, only a small page-card modifier (`DrawerPageMotion`) reads progress, changing offset and opacity (2026-11-01 removed 6% shrinking like ChatGPT: it left top/bottom gaps and lowered the top bar relative to the sidebar); corners use screen radius (`Theme.Radius.page`). Requested 2026-11-02, switched to Codex App layering (#341): main page slides right above a horizontally fixed sidebar beneath, rising slowly from 8pt lower, fully opaque and same-sized throughout (no longer two rectangles moving together side by side). Corners, shadow and thin border fade in as fixed shapes,
  not redrawn per progress. Sidebar stays mounted in its own `UIHostingController` (`DrawerSidebarHost`), UIKit directly adjusts vertical transform and hides it when closed,
  avoiding per-frame SwiftUI. Tab bar fades with UIKit alpha rather than `.toolbarVisibility`: hiding it changes safe areas, relaying out the whole chat on the first drag frame and
  final closing frame. Sidebar reaches the bottom screen edge where the faded tab bar was, with home-indicator clearance. Per-frame main-thread timings before / after are in `docs/operations/testing.md` ("Sidebar dragging").
- Tests: `AssistantTopBarTests` (avatar menu / copying, left/right buttons, button and gesture opening, selecting a chat closes and switches, fixed top bar during long-chat scrolling, connection status; unit tests guard horizontally fixed, same-size, opaque sidebar across screen widths and drag progress, with closing / reopening following the same path).
  `DrawerSwipeTests` (mock `MOCK_DRAWER=1`): ±30° / ±40° swipes from screen center, over bubbles and after list flicking all open; left/right drags over code scroll it,
  then right-drag after returning to the left edge opens the sidebar.
  `-openAssistantSidebar YES` starts with sidebar open (screenshots); a fraction such as `0.5` holds halfway through dragging to inspect card corners and shadows.

## iPhone Duo (2026-10-30)

The iPhone Duo folds: closed it uses the outer screen (about 466×678 points, compact width), open the inner screen (about 669×951 points, regular width), and can also be half-folded, landscape or portrait.
On the outer screen and the inner screen in landscape, **the system places the status, navigation, toolbar and tab bars vertically along the screen edge**; on the inner screen in portrait the bars remain horizontal. Official guides:
`developer.apple.com/iphone-duo`, HIG "Designing for iPhone Duo". Build with Xcode 27 or later (`xcode` configuration, `docs/operations/config.md`);
Apps built with older Xcode do not extend under the status bar and camera on Duo.

- **Two layouts by width, not device model** (`Persona/Views/SplitNavigation.swift`). Compact (ordinary iPhones, closed Duo) keeps the original single
  NavigationStack with no behavior changes; regular (open Duo) uses `NavigationSplitView`: list left, opened page right, like Mail.
  Both share the same `path`, preserving the current page when folding / unfolding; half-folded, the system keeps both columns away from the crease. Agents, Inbox and Settings all use it.
  - But `RootView` swaps the entire tree between the new shell and tabs by width (`usesShell`), losing the Agents page's `path` (view state); iOS briefly changes width when taking a background snapshot,
    so switching Apps on an open Duo could return to "Select an Agent" in the right column (2026-11-04). Also, chats opened by left-column links do not write to `path`. The new Agents page therefore
    reopens `dev.openChat` (saved when opening a chat, cleared on returning to the list), one beat later (the outgoing tree's split view clears the path as it leaves);
    header-switched Agents in the same space are stored in `AppModel.chatSwitched`, and a rebuilt chat page on the same path opens that one directly. `ReturnToChatTests` uses `-swapTreeAt` on iPad to simulate a tree swap.
  - Since #2836 the snapshots build nothing again: while the app is away (inactive, in the background) `RootView` gives the pages below it the width class it had while it was the active app
    (`heldWidthClass`, in the environment, so the splits, the assistant's column and the detail presentations stay too, not only the shell / tabs choice), and on coming back the window's
    (folded meanwhile, the shell). Swapped there, every part lost its state: an open sheet such as New Agent with what was typed in it, uploads under way, picked attachments.
    The simulators take no snapshots in another width (a trip to the home screen keeps the sheet on the open Duo simulator without the fix too): that the hold works on the way to the background is checked on a real iPhone Duo open.
    `-swapTreeAt` alone flips only `RootView`'s choice (the tree swap above); with `-swapTreeAway YES` the whole app's width class flips instead, from above `RootView` (`TreeSwapFlip`), while the app counts as away,
    and the pages report they never saw it change (`probe.tree-inner`, `TreeSwapAwayTests`).
    A real fold or unfold while the app is active still builds the tree again, the pages' own width class with it (`-swapTreeWhole YES` flips the whole app's while it is active, `TreeSwapFoldTests`).
    What must outlive it is kept above the tree: `RootView` presents the Agents page's New Agent sheet (`\.newAgentShown`; the Mac keeps its own), and `AppModel` holds Health's report uploads
    per hub group (`healthReportUploads`), as #2626 keeps a chat's attachments. The tree on its way out lives on a moment after the swap (about two seconds on an iPad simulator), and its Agents page
    took the chat the new page asked for again, opening it where no one saw it: each tree `RootView` builds has a `BuiltTree` of its own, `ShownTree` holds the shown one, and a page
    of another tree (on its way out) leaves the chats asked for, and the remembered chat, to the shown tree's page (#2836).
  - Left-column `NavigationLink(value:)` replaces the right column (the left has its own never-pushed stack, forwarding values to the right); programmatically opened list pages must replace it too:
    `path = NavigationPath([value])` (in single-column mode the list is at the stack bottom, equivalent to append; Agents `openFromList`). Pages opened from details still append.
  - The first right-column page has no back button (the list is beside it): add `.splitDetailPage()` to the destination; Agents' custom chat back button hides for `\.splitDetail` (keeping its space so the title stays centered).
    With the list folded, the chat's leading button brings it back (#963): the sidebar's button (`Theme.Icon.sidebar`, "Show Sidebar", with how many Agents need the user), as on the
    main conversation, not a ‹ (#8603). What the button shows follows how the page was reached, not what it is: a chat picked in the list is a root and has the sidebar's button; the
    same chat pushed from inside another page (a waiting Agent on the main conversation, Inbox's) has ‹, which goes back to that page. On iPhone Duo open on its side the chat has
    neither (#3680): its actions are in the system's side bars, and the side column's first item, where Back is on a sub-page, is the sidebar's button
    (`ChatView.sideBarItems`, #11636), as on a place (`ListButtonItem`). Not the system's Show Sidebar, removed on the roots' column there too: iOS draws it in the bar's
    title row, which the chat removes while its header shows its own title, so an Agent's chat picked in the sidebar had no way back to it, and on a place it was not
    drawn again after a drill-down was popped. The chat's page starts `Theme.Space.xl` from the top whatever the bar holds (`ChatView.pinsSideTop`): folded or out, the
    header and the chat stay where they are.
  - The Separate layout's tabs at regular width (#8596): the split makes the floating tab bar's room in its columns, and UIKit held it a second time in the tab's own top safe
    area for a tab first shown after a switch (the Agents tab's column head 64 pt lower). `TabBarRoomOnce` (ChatsView) takes the tab's top beyond the window's own back off.
  - Split chat pages do not hide the tab bar (single-column pages do, to fill the screen).
  - Settings entry points all use values (`SettingsPage`); view-based `NavigationLink { page }` pushes the left column's own stack and opens in the narrow column.
- **Persona**: in regular width, the sidebar is `NavigationSplitView`'s left column (the shell's places and Settings at its foot), with the top-left button showing / hiding it; on iPhone Duo open the
  column's own button at its top hides it and the top-left one only brings it back (#2623, below); compact still uses a drawer.
- **The list column beside the page** (#2624, then #4206; iPad and open Duo; `SidebarWidth`, `SplitListMemory`): Persona's history (`assistant.list`), the Agents list (`agents.list`) and the Sidebar layout's column (`sidebar.list`):
  - **One width, slide only.** The column has one width: the iPad's `Theme.Size.sidebar` (400 pt), the open Duo's `Theme.Size.sidebarPhone` (320 pt, which leaves the page most of the 951 pt on its side). Hidden and shown, it only slides in and out, as Feishu's and ChatGPT's do; the page takes its new width once. Until 2026-11-14 a handle on the line (`SplitDividerHandle`) dragged the column wider or narrower (340–480 pt on the iPad, from 200 pt on the Duo); the column followed the finger, every row was laid out again on every frame and the drag felt slow, so the handle is gone (the owner, #4206). A width it kept (`<list>.width`) is no longer read.
  - **Beside the page or over it, by width alone** (`SplitPageMin`, `SplitColumns.fitsBeside`). The page beside the column is never narrower than `Theme.Size.splitPageMin` (400 pt, a large phone's screen): where the column would leave it less (the open Duo upright, 669 − 320 pt; an iPad mini upright, 744 − 400 pt) the column lies over the page (#4852), elsewhere beside it (the Duo on its side, every iPad on its side, an 11-inch iPad upright). It is said to SwiftUI as the detail column's least width (`navigationSplitViewColumnWidth`), which sets the split's `minimumSecondaryColumnWidth` itself. Turned to over, a column that was out goes away (as an iPad's sidebar does in portrait); turned back, it comes back as it was left beside the page. Over the page, a pick that changes the page slides it away (#4852; an Agent, a place, through the split's path, `SplitShowList.hideOverlay`), and so does a Sidebar layout pick that leaves the page as it was (the conversation while it is shown); a filter or a grouping leaves it. Nothing here sets the split from UIKit: SwiftUI sets `preferredSplitBehavior` and `minimumSecondaryColumnWidth` back on every update, and a value set from UIKit and taken back each time laid the split out again and again (`docs/lessons/ios.md`, "A split's posture belongs to SwiftUI").
  - **Slid by a finger** (`SplitColumnControl`, as Feishu's): from the window's leading edge while the column is away, from the line beside it while it shows. Drawn from pictures of the screen taken as the finger lands, the top and side bars in them: the column's (the window's leading edge to the line) and the page's (the line to the trailing edge; the whole window where the column lies over the page, which stays where it is and darkens as the column comes out, `Theme.Split.overDim`, the system's 0.2 measured). Each is stretched or squeezed between the line and its edge as the line follows the finger; both blur from nothing as the finger lands to their most at 80 pt (`Theme.Split.blurReach`), and a 3 pt guide line in the theme color (`Theme.accent`, `Theme.Split.guide`, as Feishu's) marks the line. Let go, a flick (500 pt/s) or past halfway settles it out or away, else it snaps back; the line springs there (`Theme.Split.settle`) and the pictures fade (`Theme.Split.fade`). The split itself changes at most once, under the pictures: coming out, as the finger lands (the column's picture is then of the column as it is); going away, as the finger lets go (where the column lies over the page, as it lands, for the page's picture without it). Nothing is laid out while the finger moves and the split's behavior is never set (an earlier drag laid the column over the page for the drag, and the Agents tab's split took its column away: the column slid on as an empty space, black in dark). The split's own edge swipe (`presentsWithGesture`) is off where this one is on: it laid the column over the page and left its state unkept (SwiftUI sets it back on its updates; set again, it lays nothing out). SwiftUI leaves out the system's Show / Hide Sidebar button with that swipe: the Separate layout's Agents list has its own Hide Sidebar where the system's was (`SplitHideListButton`), and with the list away the page's Show List and the chat's ‹ bring it back. Where the page can go back, its swipe back keeps the edge (the column comes from its button); the list's scroll, its rows' swipes and the page's drags wait for the drag to fail, which it does at once anywhere but on the line or the edge, or moving up and down.
  - **Shown or hidden is remembered** per list (`<list>.closed`; `SplitListMemory`): the column's own buttons and a finger's slide hide and show it, and the split starts as it was left (`SplitListMemory.startsClosed`). Only a person's choice beside the page is kept: not over the page (its going away there is a pick's or a tap on the page), not a turn's (beside by both SwiftUI's geometry and the split's own size, `SplitColumnState`: as a turn lays the split out either can have the new size first), not in a tab not shown (the Separate layout: it is turned too, its size not read until it shows), not a drag's moves (its end sets it), not in the background (the app switcher's snapshots lay the app out in the other orientation).
  - `SplitListTests` (the Sidebar layout on its side; `-openSplitFresh YES` starts the lists shown): one width, hidden and shown kept across launches, a finger's slide out of the way and back.
- **Readable width**: chats and the Persona home page are at most 760pt wide (`ChatView.readableWidth`), centered when wider; the top bar still spans the full width.
- **No screen dimensions**: the screens have different sizes and ratios; never use `UIScreen.main`. The long-press message action bar stays within the chat area (`MessageMenu.area`
  coordinate space); interaction forms have a maximum height of 55% of their container (`containerRelativeFrame`) and never more than lets the card fit the cards' area, so a lone card never scrolls there too (#6605), except a lone question card above the composer that grows past half the page (`docs/design/DESIGN_SYSTEM.md` 4.8, #6277); question, permission, plan and terminal dialog forms keep their answers outside their one scroll area (`CardForm`); image scaling uses the view's `traitCollection.displayScale`.
- **Toolbar buttons need both titles and icons** (`Label("新建会话", systemImage: …)`, "New chat"): vertical bars show only icons, and overflow menus show titles;
  icon-only buttons become blank menu items.
- **Vertical side bars**: when the system places bars at the side (outer screen, inner landscape; non-nil `toolbarVerticalEdge`, iOS 27.1 onward), the Agents chat page no longer draws its own header buttons;
  Back, Terminal, Git, Files and ⋯ go into the system toolbar, arranged below the status bar (`ChatView.sideBarItems`, each with title and icon). The content top keeps only a non-collapsing title
  (tap for session info), same-space Agents, loop / background tasks and the sub-Agent status line (`sideHeader`, the same second and third rows as the custom header). Horizontal bars (ordinary iPhone, inner portrait) retain the custom collapsible header.
  The pages that hold an Agents chat before it is there do the same (#9971, `DetailHeader`): a task still starting, one whose Agent did not start, a task,
  an Agent or a side session that is gone put their Back (the system's when pushed, a ✕ of their own in a sheet) and ⋯ in the system bar, so Back is at the top of the side column before and after
  the Agent comes up, never at the top left first (HIG "Designing for iPhone Duo": the top of the vertical axis is for Back or Close).
  A chat pushed onto another page, on its chat pane, keeps the system's own Back there (`ChatView.systemBack`, #9971): the system puts it at the top of
  the column, and iOS's swipe back from anywhere on the page works only on a page that shows the system's Back (`docs/lessons/ios.md`). Otherwise
  (files / Git back to the chat, a sheet's ✕, a root) the chat hides the system back button and supplies its own Back in the side column. Chat destinations must use
  `.splitDetailPage(ownsBack: true)`: an outer `navigationBarBackButtonHidden(false)` overrides the page's `true`, bringing back the system button
  and stacking two Back buttons (2026-11-04, TestFlight).
  With the keyboard open, this header becomes fog only (title, same-space Agents and sub-Agents hide until the keyboard closes): inner landscape leaves only about 390pt above the keyboard,
  and the header, mode buttons and input previously occupied it all, leaving no message visible (2026-11-04). The system draws its bar's title over the content, not in the side
  column, so it is removed while the header shows its own (`toolbar(removing: .title)`; the session's name showed twice, one above the other, 2026-11-08, TestFlight)
  and comes back while typing, when the header is only fog.
  Running workflows in this header are one line each, their head (name, status, done / started, time), and a tap opens the whole card: it never folds,
  and two full cards with the title and the tabs took half the outer display, leaving the chat a third (2026-11-09, #2161).
  Outside the shell, Persona's top bar remains custom at the content top; its fog extends horizontally under the side status bar (`BarFog`; limiting it to content width leaves a vertical edge in dark mode).
- **The shell's side rail** (2026-11-07, TestFlight: on the outer screen every button was at the top, out of reach of the hand holding the phone): with bars on the side, the shell's
  top bars (Assistant's, Agents', Messages' when it is on, and the moving bar while the strip slides) become a rail in that column (`ShellSideRail`; `ShellView` reads the edge once with `readsVerticalBarEdge` and
  hands it down as `\.shellRailEdge`; a page whose bar skipped it put the bar back across the top while it was in front, #2318). Following Apple's iPhone Duo guidance (navigation first, then the prominent action, the tab bar below), the face (the drawer) and the page's
  action (New Chat / New Session / Lark Settings) sit low in the column and, since #4231, the switch is the Duo's tab bar above them in the column's lower middle (`ShellSwitch(axis: .vertical)`:
  each page's symbol over its name, the chosen one on the Theme Color's circle, centred on the status bar, below), all within the thumb's reach; the status bar keeps the column's top. The pages drop the top room the bar had
  (`AssistantTopBar.room`, Agents' list `room`, Messages' top inset), so the content starts at the top. Menus (tapping the chosen segment), the badge and the drawer work as before; horizontal bars
  (ordinary iPhone, inner portrait) are unchanged. The rail measures the column as the page's width with the side's safe area less its width without: a `GeometryReader` that
  ignores the safe area reports no inset of its own on that edge.
- **The open Duo keeps the system's tab bar** (#10362): at regular width the tabs no longer hide the system's tab bar and
  nothing is drawn over them, so iOS places it as Apple's iPhone Duo guidance describes: along the foot upright, down the side
  column on its side, in Liquid Glass, with its own overflow and fold avoidance. The outer display keeps the shell's rail and
  its tab bar, described below; what the next item says about the open Duo (`TabSwitchOverlay`, the bar along the foot, its
  room, `PhoneTabBarHidden`) is history, its code removed. Tests: `TabSwitchLayoutTests.theTabsKeepTheSystemsBar`,
  `DuoOpenChromeTests` (the system's tab items on the open Duo, no drawn bar).
- **The Duo's tab bar on both displays** (#4231, the owner: first the corner design of #4206 drawn on 2026-11-11 / 12, then on
  2026-11-17, after a tab bar down the side of the outer display, a tab bar like it): in the Separate layout one component,
  the shell's switch as a tab bar (`ShellSwitch(axis: .vertical)` down the side, `ShellSwitch(bottomBar: true)` along the foot;
  `TabSwitchLayout`, `apple/Persona/DesignSystem/TabSwitchDock.swift`). Its items are the page's symbol over its name (Assistant,
  Agents, Messages when it is on; 56 pt each, the default text size at every size, a bar of fixed items), the chosen one's symbol
  white on a round fill of the Theme Color and its name in it, the others a line symbol over a plain name, all in one glass
  capsule with a hairline edge and a faint shadow; a page's count is white on red at its symbol's top trailing corner, as the
  system's tab bar badges (Agents' Needs You, `agentsWaiting`; Persona's, `TabBadges.assistant`; Messages has none, as its tab
  had none). Where the system's bars run down the side it stands in that column, centred on the status bar (the 84 pt inset
  less the 14 pt strip at the screen's edge, `Theme.Shell.sideColumnEdge`: the clock is 49 pt from the edge on both displays),
  in its lower middle: on the outer display's rail above the face and the page's action, which stay at the foot within the
  thumb's reach (#1057); on the open Duo, with nothing under it, their room left empty (`ShellRailButtonRoom`), so it stands as high above the foot on both displays (178.7 pt above the screen's foot at the default text size; one set of values, `Theme.Shell.sideRailFoot`, `sideRailSpacing`, `sideTabBarGap`) and stays put as the Duo folds or unfolds, at the shell's text size on both (`TabSwitchSideRail` scales as the pages' rails, `TabSwitchFootBar` too; `DuoTabBarAlignmentTests`). Its count is `TabBarCount`, 18 pt with 12 pt digits, ringed in the page's color, centred 2 pt inside the corner of the symbol's well (`tabBarBadgeSpot`). On the open Duo `RootTabs` hides the system's tab bar on every tab (`PhoneTabBarHidden`, set once
  at each tab's root, never toggled) and draws the bar over the tabs (`TabSwitchOverlay`), outside the splits and stacks, so it
  stays put on a push and a pop: on its side in the system's column down the right, upright along the window's foot, centred,
  where iOS puts a tab bar with the bars across the top (there is no side column to stand in; a column of names floating over
  the page took the composer's width and lay over the chat's newest message). Upright each tab's controller keeps the bar's
  room at its foot as `additionalSafeAreaInsets.bottom`, as the system's tab bar does (`TabBarRoomProbe`; a SwiftUI safe-area
  padding did not reach into the split's columns), so lists end above it and the composer stands on it at the whole width.
  Where the bars are is read in the same update as the tabs (`toolbarVerticalEdge`), so neither starts in the other posture.
  While the keyboard is up the bar fades away and its room goes. Merged (the default where the Duo is open) is unchanged; the
  iPad keeps the system's tab bar, an ordinary iPhone the top switch. Tests: `TabSwitchLayoutTests` (the placement, the sizes,
  the centring and the lower middle, the room, the choice kept), `DuoTabBarAlignmentTests` (both displays' rails laid out at every
  text size: the same bar, as far from the edge and the foot, at the same text size; the count on the well's rim),
  `DuoTabSwitchTests` (UI, each posture: one bar of symbols and names where it belongs, its foot 186.7 pt above the window's
  under its last item, the count at a symbol's corner, each item's page, the keyboard, Merged and Separate at once and after
  a relaunch). Built with Xcode 26 the bar edge is never read, so the open Duo on its side gets the bar along the foot.
- **One side column** (#9837, 2026-11-17, the owner: the tab bar lay over an Agent chat's Files and ⋯ on the open Duo on its
  side and on the outer display, and the system's search over Messages' Lark Settings at the outer display's foot). The
  system puts a page's own bar items down the column from its top: Back alone (a 48 pt glass), 12 pt under it the rest in
  one glass (48 pt items, 4 pt apart), the first from 174 pt under the safe area's top on the outer display (its camera is
  over the status bar) and 124 pt on the inner one, and a page's search at its foot (its top 38 pt over the safe area's foot),
  at every text size (iOS 27.2, `Theme.Shell.systemBar…`; the safe area's top is 0 on both). Our rail stands from the foot.
  - Each rail with the tab bar reports the bar's top (from the safe area's) to a `SideColumn` in the environment, which a scope gives
    the pages under that rail (`SideColumnScope`: the open Duo's tabs in `TabSwitchPlaced`, the shell's Agents page in
    `ShellView`); it is the room for the page's buttons from the system's first item (`TabSwitchLayout.systemBarTop`).
  - An Agent's chat keeps Back, ⋯ and the leading ones of the chat / terminal / Git / Files buttons that stand
    `Theme.Space.s` clear of the bar (`TabSwitchLayout.pageButtons`, `ChatView.sideButtons`); the rest are only in ⋯,
    which has them all. Measured: on its side terminal, Git and ⋯ (Files in ⋯); under the outer display's rail only ⋯
    beside Back. The tab bar is never moved for them.
  - Messages in the shell under side bars shows its navigation bar without its title, so Lark Settings goes with the
    system's items at the column's top as on the open Duo's tab; its rail keeps the face and an empty slot where the
    system's search stands (`ShellRailSlot`), so the tab bar stays where it is on the other pages.
  - The tab bar's chosen item is reached by a lens (`TabBarLens`): clear glass over the items in the glass styles
    (`lensSurface`), swelling and stretching along the bar as it glides with a bounce, fading as it lands; the chosen
    Assistant item shows the persona's face. With Reduce Motion the items cross-fade.
  - The lens's glide runs on the frames it is drawn in, not on the wall clock (`TabBarGlide.frame(at:)`, #10270): the
    chosen page is swapped in under the bar in the same frames (85–165 ms on the open Duo on its side in the simulator).
    Before it sets off, the lens waits where it starts through such frames (`longestWait`); once moving, a held-up frame
    moves it on by `longestStep` at most, so it never jumps. In the shell (the outer display) the moving rail stands in
    until the glide ends (`ShellPager.standsIn`), so the pages' own rails come back once nothing moves, not in its fade.
  - Tests: `TabSwitchLayoutTests` (what fits, the column's room, the glide's clock), `DuoSideColumnTests` (UI, each posture: no two buttons of
    the column over one another on an Agent's chat, Messages, a to-do and the main chat, also at the largest text size).
- **The inner screen's tabs** (#2623, 2026-11-10, the user, after ChatGPT's open layout; replaced on 2026-11-14 by the
  Separate layout's new look, #4206, "Sidebar layout (#6332)": the system's tab bar, the one sidebar's column; on 2026-11-16 the
  system's tab bar gave way to the Duo's own, #4231, above. What follows is the history; its code was removed for #8305): on a phone at regular width (iPhone Duo open, either way up; the iPhone app is upright only) `RootTabs`
  hides the system tab bar, which iOS drew down the side with the switch at its foot, and the pages draw `ShellSwitch` at their top (`pageTabSwitch`): Assistant in its floating bar in the face's place
  (tapping the chosen Assistant opens the face's menu, as in the shell; the link's state and a copy's confirmation show under it), Agents and Messages as a row over the page column
  (`PageTabSwitchRow`; Agents' list options stay on the list's ⋯). Assistant's sidebar column has the button that folds it at its top trailing corner, where Search was (`collapse`); the page's
  Chat History button only shows while it is folded, to bring it back. Search moves to the foot's row beside Persona's name and where it runs, and opens over the whole window as ChatGPT's (#3705, `AssistantFullSearch`): the field at the foot over the keyboard, × beside it, the conversations above, the latest first, narrowed as you type; a row opens its conversation. Its Chats | Agents scopes at the top look through the Agents too (#4206, "Sidebar layout (#6332)", Search). Both searches (this one and the sidebar's own) also find a chat by what was said in it (#6284, `AssistantChatSearch.swift`): when the hub lists `assistant_search`, the current assistant's `GET /api/v1/assistant/search` is asked 200 ms after the last key, each query cancelling the one before (`docs/protocol/protocol.md` ("Assistant chat search (#6282)")); the local matches (title, last message, a direction's name) show at once and the manager's results come first once they arrive, archived chats included. A row found by a message shows that sentence under its title with the match in bold (`SearchSaidLine`), starting a few characters before the match, fewer as the text grows (8, then 5 above Large, 2 from xxxLarge: `AssistantSearchMerge.lead(for:)`), so the match stays on the line; VoiceOver reads the sentence in the last message's place. Opening it lands on that message through the hub link landing (`MessageJump`, paging back as a link's `?at=` does, opening a folded turn that holds it): it comes to the middle under the quote's accent band, held `Theme.Quote.landingHold` (1.6 s, a quote's tap holds 0.7 s) and then faded, as the web rings its hit (#6446); a row found by its title opens as usual. While the manager's index is still reading older chats a line under the heading says so; offline, without the feature or on a 404 (an older manager) the local matches are the whole answer. The main conversation's
  long press holds the face's long-press items: whose Persona (several hubs), Knowledge, Details, Copy Directory, Copy Info (`MainChatPlaceItems`); the switch's Assistant, tapped while chosen, keeps the
  whole menu, as in the shell. Persona's badge rides on the switch's Assistant; the link's state shows beside the switch, a copy's confirmation as a toast. Pushed pages of Persona's column
  have no switch: Back goes to it (Agents' and Messages' rows stay over their pushed chats). On Agents the empty page has the row as an inset of the column's stack, and each
  page opened there carries its own (`PageTabSwitchInset`, from `SplitDetail.pageSwitch`): through `.splitDetailPage()`, and on a page `detailPresentation` pushes. An inset of the
  column's `NavigationStack` reached only its root page, so the switch went as soon as an agent was opened and nothing led back to Assistant (#3680). The chosen Agents segment in
  those rows has no menu, so it is a plain button, not an empty menu VoiceOver read as List Options (#3680). The Agents list beside an opened chat stays in sight and keeps
  following the hub: whether a page covers it goes by the page's layout (`ownsTop`), as the split's sidebar column reads compact on a phone (#1971, #3680). A new conversation on
  Persona's page is the iPhone's `ShellHome` there (what needs the user over the composer; `AssistantHome` stays the iPad's, #3680). `hidesTabBarWhenShown` puts back only what it found, so Back from a chat does not bring the bar down the side again.
  Settings covers the window there, one column across with its × at the top left (#9653), not a sheet over half of it (#3701, `PadRoutes`' full-screen cover).
  The iPad keeps its tab bar along the top and Settings as a sheet. `DuoOpenChromeTests` (taps and long presses only).
- **Switching pages under side bars** (2026-11-09, TestFlight: switching between Assistant and Agents stuttered on the outer screen): with bars on the side the two pages
  cross-fade in place instead of sliding (`ShellPageSlide`); a drag between them fades with the finger. A page that moved sideways under iPhone Duo's vertical bars, however
  it was moved (an offset, a render-only `transformEffect`, under a mask), made both pages' navigation controllers run a deferred transition and lay the page out twice on
  every frame: about 27 ms a frame and 140 ms of hitches a second through a switch on the simulator, against 58 ms and 2.8 ms in place (an ordinary iPhone's numbers).
  Horizontal bars keep the slide.
- **The drawer under side bars** (2026-11-10, TestFlight, #3755: opening and closing the drawer stuttered on the outer screen): with bars on the side the page stays
  where it is, dimmed, and the sidebar slides in over it from the leading edge (`AssistantDrawer.inPlace`), under the finger while dragged, with the page card's shadow
  and edge line along its side; a tap beside it or a drag back closes it. The page sliding aside to uncover the sidebar was the same bug as the pages' switch above:
  through three opens and closes on the simulator the main thread spent 35 ms a frame (p95), with 230 ms of hitches a second, against 1.1 ms and 2 ms with the page
  still (an ordinary iPhone, where the page still slides: 8 ms and 13 ms). Horizontal bars keep the page sliding off the sidebar.
- **Short outer screen** (about 678pt): on Persona's new-chat page, Needs you rows above the input shrink to the remaining space (see "Empty chat" under "New shell" above).
- **Simulator verification**: iPhone Duo has only the iOS 27.1 runtime (which supports only it). Folding / unfolding / half-folding can only be switched in Xcode Device Hub,
  with no CLI. Screenshots must specify the display: `xcrun simctl io <udid> screenshot --display=1` (outer) / `--display=3` (inner, 2853×2007 in landscape);
  otherwise the inactive screen may be captured (all black); calculate brightness first to confirm the active screen. Launch arguments work normally (`-tab`, `-openSession`, etc.).

## iPad (2026-10-31)

One App for iPhone and iPad (`TARGETED_DEVICE_FAMILY` 1,2). Previously only 1, giving iPad a phone compatibility window with enlarged buttons.

- Like iPhone Duo, layout follows width: regular (iPad full screen, wider split screen) uses `SplitNavigation` with two columns; compact (narrow split screen / Slide Over) uses one stack.
- iPad's left column uses `Theme.Size.sidebar` (340…480, ideal 400; system default is too narrow and wraps all Agents-card titles); Persona's chat list does too, preventing width jumps when switching tabs.
  iPhone Duo sets no width (the system splits at the crease).
- iPad supports all four orientations (required by split screen and Stage Manager); iPhone remains portrait only.
- The sidebar matches the iPhone new shell's drawer (requested 2026-11-05): Persona (main chat) first, then a separator, Inbox, To-dos, Scheduled tasks, Knowledge, Machines, then chat history (#2004 / #5110);
  bottom "New chat" pill + Settings, then the avatar, online status and machines row. Top tabs are only Persona, Agents (and optional Messages); Inbox and Settings are no longer tabs:
  Inbox, To-dos and Machines push into Persona's right column (`ShellDestinations`); Settings is a sheet (× at the top left, #9653; on the open iPhone Duo it covers the window, #3701). Routing still writes `AppModel.selectedTab`;
  `PadRouter` (`RootView.swift`) maps `.mail` to "push Inbox on Persona" and `.settings` to "open Settings sheet". Mac and Vision keep their shared root tabs.
  `IPadSidebarTests` guards this (portrait and landscape: ordering, bottom buttons, no Inbox / Settings tabs, Inbox opens on the right, Settings sheet can close).
- Welcome content is limited to `Theme.Size.readable` (560), not stretched across large screens.
- Detail pages opened from chats (task details, dock Agents, to-do strips, hub links `/todos?id=`) push into the chat's stack at regular width and fill the detail column,
  rather than sheets: `presentationDetents` has no effect at regular width and a sheet becomes a small centered form (feedback 2026-11-05, #2816). Use
  `detailPresentation` (`Views/DetailPresentation.swift`): still a sheet on phones, Mac / visionOS unchanged; replace a page's own `NavigationStack` with
  `DetailStack` (no extra stack when pushed). Where there is no stack to push into (a to-do opened from a session-info sheet), use at least `.presentationSizing(.page)`.
  Tests: `IPadTaskDetailTests` (iPad only, `MOCK_TASKS=1 MOCK_TASKS_NO_ASKS=1`).
- Left column reaches the screen top: the split view's left column starts below the status bar and cannot draw into it; that strip used to be window white, making the list appear short of the top (2026-10-31).
  `SidebarTopFill` behind the whole split view colors the strip to the left-column width (system sidebar color `Theme.systemSidebar` for glass, page background for other styles);
  it does not fill when the column is hidden (uses `columnVisibility`; the hidden column still reports its last position). `IPadLayoutTests.testSidebarReachesTheTop` checks pixels.
- Tests: `IPadLayoutTests` (iPad simulator only: native full-screen portrait / landscape, wide enough sidebar, chats open on the right); `InboxSplitTests` (split layout:
  switching between Lark chats shows the new chat's own messages and draft on the right, switching back preserves the draft). XCUITest's own landscape screenshots are rotated;
  inspect landscape by holding with `TEST_RUNNER_PERSONA_HOLD=<seconds>` and capturing with `simctl io screenshot`.

## Sidebar layout (#6332)

Settings › Appearance › Sidebar chooses how the app lays out Persona and the Agents (`NavigationLayout`, this device's
own UserDefaults key `app.navigation`, applied at once, Settings staying on its page): **Merged**, the web sidebar's
structure in one column, or **Separate**, everything above unchanged (the Assistant | Agents switch on an iPhone, the
Persona | Agents tabs on an iPad). The owner asked for it on 2026-11-13 (#6332, then named Navigation: Separate /
Sidebar). With nothing picked (#4206; the owner's delegate picked "Merged where wide", 2026-11-14) it is Merged at
regular width (the iPad, the open iPhone Duo) and Separate at compact width (an iPhone, the folded Duo), so folding the
Duo goes from the column to the shell; a pick holds at every width (`NavigationLayout.effective`), and the page checks the
one in effect. UI tests launch in Separate unless they name a layout (`launchPersona`). On an iPhone (and at any
compact width: iPhone Duo closed, an iPad's narrow split) the column is the drawer that slides in from the left; at
regular width it stays beside the page. The Mac (frozen) and Apple Vision Pro keep their own navigation.

- **The column** (`UnifiedSidebarColumn`, `Persona/Views/Shell/UnifiedSidebar.swift`), in the web's order: the assistant
  (its switcher); the main conversation held at the top while the rest scrolls (as the web's, #5581); one
  list, the Agents list (`AgentsList`, `sidebarRows`) with the Persona sidebar's rows over its own
  (`AssistantSidebar(part: .rows)`): **Pages** (Recordings, Inbox, To-dos, Scheduled Tasks, Knowledge, Health, Machines,
  Messages when it is on, the extensions' pages) and **Persona Chats** (Pinned, Recent, All Chats), each heading folding
  its section (remembered, `sidebar.fold.places` / `sidebar.fold.chats`; folded Pages keeps
  the Inbox and To-dos counts); then the Agents' heading with the list's ⋯, Personal Agents and New Session, the quick
  filters and the grouping, Needs You and the groups, the same rows as the Agents page. At the foot: "N need you ↓"
  (`SidebarNeedsJump`, the count the Separate layout's switch and tabs show, `agentsWaiting`) in a band of its own above
  the usage, so it never covers a row; a tap scrolls the list to the Agents' Needs You, the arrow says which way it is.
  Then the two fullest subscription accounts (`SidebarUsageStrip`, `UsageWindows.top`; a tap opens Machines), New Chat,
  and Search and Settings in one capsule (#4206, the owner). That New Chat is the only one wherever the sidebar has it at its foot beside the page (#4206, the owner):
  the page's top bar keeps its place empty, so its middle stays centred: the name capsule while the column's head shows the face, the face once the column is away (#9973) (`AssistantTopBar.newChatInSidebar`; the
  iPad's Persona column in the Separate layout too), and the Persona Chats heading has none. A phone keeps the top bar's.
  The parts share their dialogs through `AssistantSidebarUI`, presented around the column (`AssistantSidebarPresenters`):
  a sheet on a list row went with the row as it scrolled away. The head's face, the main conversation's symbol and the places' symbols stand on
  one vertical line, their names on another (#4206): the head and the main conversation (held over the list) start where
  the list's rows do, measured on the Pages heading (`AssistantSidebarUI.rowsLeading`; the list's own margin differs by
  device), the head's face moved by what centres it on the symbols' column and its name brought to the titles' line
  (`AssistantSwitcherHeader.faceGap`). The column does not make way for the keyboard (`ignoresSafeArea(.keyboard)`, #4206, the
  owner): typing in the page beside it, its foot stays where it is instead of riding up and squeezing the list.
- **Search** (#4206, the owner; it replaced #6332's field in the head, which filtered the column in place): the foot's
  Search opens the whole-window search (`AssistantFullSearch`, the open Duo's from #3705) at every width, with two
  scopes over the results: **Chats**, Persona's, as #3705's and #7123's (titles, last messages, and what was said, from
  the manager's index, opened at the message found), and **Agents** (every Agent's chat but sub-agents' and archived
  ones, matched as the Agents list's search matches, `Session.matchesScope(.all)`, with where the words were found under
  the title, else its state and computer; the latest 50). It starts on the Agents while an Agent's chat is in front
  (`AppModel.selectedTabNow`, which follows what is in front in this layout), else on the chats. A chat found opens as
  the column's rows do; an Agent found opens beside the sidebar (`AppModel.open`) once the search has gone. There is no
  "All" scope yet.
- **Its color** (`UnifiedSidebar.columnColor`, #4206, #4855): one neutral near-white / near-black down the whole column,
  as ChatGPT's (`Theme.Shell.drawer`, the shell drawer's), in every style and at every width: behind the head, the list
  (`AgentsListPage(column:)`) and the foot, in the strip over the column on the iPad (`SidebarTopFill(color:)`) and
  behind the phone's drawer. The system sidebar's grey-blue it had stayed under the head and the foot only, in the
  styles whose list paints the page's own color: three bands down the column.
- **Under side bars** (the open iPhone Duo on its side, `SidebarRoot` reading `readsVerticalBarEdge` into
  `\.splitSideBars`): nothing is above the columns there, so the column's head and the page's top bar sat at the
  display's edge, the mark under its rounded corner (#4853). Both columns start at least `Theme.Space.l` lower
  (`SplitNavigation.topPad`), as ChatGPT's open Duo: the head and the conversation's top bar stay on one line. The
  column's head never follows the page's own top (#8359): that is the root's navigation bar (0 on the conversation,
  82 pt on a place, 24 on an Agent's chat), and the head dropped by that much on every root but the conversation.
  `SidebarRootsUITests.testTheSidebarHeadStaysPutOnEveryRoot` holds the head, the main conversation and the Pages
  heading within 1 pt on the conversation, Recordings, Inbox and an Agent.
- **The page beside it**: the Agents page's split (`ChatsView` under `\.unifiedSidebar`, `SidebarRoot` in `RootView`;
  the Persona tab's in the Separate layout too). **Switching area replaces the page's root; only drilling down pushes**
  (#8359, docs/design/sidebar-navigation.md section 6): each sidebar item is a root of its own, the main conversation, a
  place (`ShellRoute`), Health or Messages (`SidebarRoute`), an Agent's chat (its row, a card, a route) or another page the
  Agents list opens. A pick replaces the root at once, with no push, pop or Back; the split's path (`\.unifiedPath`) holds
  only what is opened inside the page (an Inbox item, a machine, a recording), pushed with Back. Each root keeps its
  drill-down while the user is elsewhere, and the root in front picked again goes back to it (`SidebarPageRoots`, held by
  `UnifiedSidebar`). `AssistantView` without a stack of its own (`embedded`) stays the stack's root and draws the root picked
  (`sidebarRootPage`); `SplitNavigation(rootsSwap:)` counts the root as the first page. The conversation's sidebar button
  folds and brings back the split's column (`SplitShowList.hide`). The shaded row follows the root, also while drilled in
  (`UnifiedSidebar.atRoot`, `shownPlace`; an Agent's row from the root). Laid over the page (the open Duo upright) the
  column slides away on a pick, the only motion. DEBUG: `-probes YES` writes `probe.sidebar-page` (the root, the path, how
  many animated navigation transitions ran; `SidebarRootsUITests`). The conversation's `RootBarHidden` (which hides the
  stack's bar again after a pop) stops once the conversation is taken away: a hide still waiting from its last update hid
  the next root's bar, its title and, on the open Duo upright, the button that brings the sidebar back.
- **On a phone** (`SplitNavigation`'s drawer layout, `\.splitDrawer`, `SplitDetail.drawer`): the same column in the
  drawer (`AssistantDrawerLayout`, the shell's drawer: 82% of the width, at most 380 pt), the Persona conversation the root
  of one stack under it, and what the drawer opens that stack's first page. The conversation's bar has no Assistant |
  Agents switch: the persona's face opens the drawer (the inbox's mark on it, how many Agents need the user at its
  corner, `SidebarWaitingBadge`), its name in the middle holds the face's menu, New Chat on the right
  (`AssistantTopBar.drawerRow`). A first page has the same face where a back would be (`DrawerButtonItem`; an Agent's
  chat in its own header), and a drag to the right on it opens the drawer as on the conversation (the drawer's pan only
  yields to a second page's back swipe, `DrawerPan.firstPages`). A pick closes the drawer. Choosing Agents by itself
  opens the drawer at the Agents. While the drawer is in, the Agents list holds its updates as a covered list does, and
  the Persona rows keep the list they last showed (`SidebarFeed`). The conversation is the iPhone's (`shellChat`, the
  shell's new-conversation page). Behind the drawer is the column's color, so its head, list and foot are one surface in
  dark mode too. Settings is the sheet the shell shows it in (`PadRoutes`: the
  whole window only at a phone's regular width), so it opens and closes the same way in both layouts.
- **Routes** (`PadRouter(unified: true)`): notifications, links, Live Activities, widgets, quick actions and Siri /
  Shortcuts set `AppModel.selectedTab` and `openChat` as in the Separate layout and land on the same chat or page: an
  Agent's chat opens beside the sidebar (the Agents list takes `openChat` as before); `.assistant` goes back to the
  conversation (`ShellPage.conversation`); `.mail` opens the inbox and keeps it (the router's own tab change is only
  noted); `.feishu` opens Messages; `.settings` is the sheet over what is in front. Choosing Agents by itself
  (`agentsListReveal`) scrolls the sidebar to the Agents' heading and leaves the page beside it. `selectedTab` follows
  what is in front, so a relaunch goes back to the Agent's chat that was open.
- **The Separate layout at regular width** (#4206, the owner's new look; it replaced #6332's pixel-for-pixel tabs and
  #2623's switch drawn on the open Duo's pages): the system's tab bar at the top on the iPad; on the open Duo the Duo's own
  tab bar in its place, down the side column or along the foot (#4231, "iPhone Duo"), each tab the same column with its part of the one sidebar (`UnifiedSidebar.Part`), its own selection
  and its own page: Persona's tab the head, the main conversation, the pages and Persona's chats (in a list of their own,
  `personaList`), Persona's page beside it; the Agents tab the head and the Agents list, the "Choose an Agent" page beside
  it until an Agent is picked, which then is the page's root, put in place with no push (#8603, as the Sidebar layout's
  roots). Both feet are the Sidebar layout's (New Chat, Search, Settings); the Agents tab's New Chat and places open on
  Persona's tab. Each tab's column is kept as before (`assistant.list`, `agents.list`); an Agent asked for from outside
  opens on the Agents tab only. At compact width the shell is drawn as before.
- **The head's fold** (both layouts, beside a page; #4206 as the earlier one sidebar had it): the column's head ends in
  the button that folds it (`sidebar-fold-column`, `AssistantSidebar.foldColumn`); the page's sidebar button only shows
  while the column is away, to bring it back, its place kept empty meanwhile so the face stays in the middle. In the
  phone's drawer the face opens it, as before.
- Tests: `SidebarLayoutRoutingTests` (the router), `SidebarLayoutTests` (iPad UI: the column, a pick replacing the
  page, the routes), `SidebarLayoutPhoneTests` (iPhone UI: the drawer, a pick replacing the page, the face where a back
  would be, the setting switching both ways at once, nothing picked: Separate), `SidebarRootsUITests` (a pick replaces the
  root, a drill-down pushes, each root keeps its drill-down; iPad, open Duo, iPhone), `SidebarPageRootsTests` (the rule), `DuoTabSwitchTests` (iPhone Duo: Merged and Separate switched at once and kept, #4231). DEBUG: `-app.navigation sidebar` launches in the Sidebar layout, `-app.navigation ""` with nothing picked; `-openAppearance navigation` opens
  the setting's page.

## Home-screen quick actions (#3199)

Long-press the Persona icon: the last conversation opened on this device, then "Start Meeting" (a meeting recording, "Record" until #8731; `docs/apple/recording.md`), "New chat" and "Inbox" (`Persona/App/QuickActions.swift`). The system shows four at most, so "Agents" is not among them since #2191 (the tab bar has it), and the menu has one recent conversation, not two.

- **Recently opened** (#2191, `Persona/App/RecentChats.swift`): every chat that comes on screen (`AppModel.visibleSession`, an Agent's or a Persona conversation) goes first in a list of ten `SessionKey.path`s kept in `StateCache` (`recent-chats`; keys only, never a title or a message; never sent to a hub). The rules are shared with the web's ⌘K (`spec/fixtures/recents/README.md`). The menu lists the newest one the app still has (`RecentChats.homeScreen`), title and "computer · folder" (or "Persona"), with `Theme.Icon.recent`; tapped, it opens like a notification (`AppModel.open`). `shortcutItems` is set at launch, when a new conversation is opened and when the app goes to the background (the titles as they are then).
- Navigation only, except Record, which starts recording: Inbox pushes onto the assistant, New chat follows the drawer's New chat route (`ShellState.show(.newConversation)`), returning already-pushed pages such as Inbox to the root. Without hub pairing, remain on Welcome.
- If the App is open or backgrounded, use the scene delegate's `windowScene(_:performActionFor:)`; on cold launch the item comes through scene connectionOptions. `QuickActionSceneDelegate` handles only these two entry points; SwiftUI still manages the window.
- `QuickActionsTests` opens a demo chat, long-presses the home-screen icon, finds that chat on the menu (with Record, and no Agents) and taps it, then Inbox and New chat; a second test opens two chats in turn and checks the menu's order (the newer chat only, then Record, New chat, Inbox). Record is not tapped (it would record). `RecentChatsTests` runs the shared fixture. Cold launch is untested: SpringBoard launch cannot pass demo-mode arguments. Each simulator return to the home screen waits about 60 seconds for SpringBoard idle; the whole test takes about 7 minutes.

## App icon badge (#5824)

The number on the Persona icon (the iPhone's home screen, the Mac's Dock; Apple Vision Pro runs the same code) is the
tab badges added up: Persona's waiting questions (`TabBadges.assistant`) + Agents' Needs You (`TabBadges.agents`) +
the Inbox's unread items (`InboxStore.unreadCount`). One function, `AppIconBadge.count` (`Persona/App/AppIconBadge.swift`),
on every Apple app.

- **Same as the tabs**: nothing the tabs leave out (muted agents, an agent's finished turn not yet opened, open
  to-dos, an extension's badge, Messages) and nothing more; a quiet task's question counts with Persona's, as on its
  tab. Signed out, or every item handled, clears it.
- **Set by the app**: `AppIconBadgeWatcher` observes the numbers while the app runs and sets the icon only when the sum
  moves, 0.25 s after the first change of a burst (P7). iOS calls `UNUserNotificationCenter.setBadgeCount` on a serial
  utility queue (P1) and sends the number again each time the app becomes active: one set before the user allowed
  badges is dropped. The Mac sets `NSApp.dockTile.badgeLabel`. The hub's pushes carry no `badge`, so while the app is
  not running the icon keeps the last number set, and a build without push (no `aps-environment`) works the same.
- **The user's switch**: the system draws the number only with badge permission (asked with notifications after
  pairing, `PushRegistrar`) and its Badges switch on (iOS Settings › Notifications; macOS System Settings ›
  Notifications › Badge application icon). A Mac that never asked (Demo Mode only) has the Dock label set but not
  drawn (`lsappinfo info -only StatusLabel` shows it).
- **Tests**: `AppIconBadgeTests` (also in `PersonaMacTests`): the sum, the model's tabs feeding it, nothing while
  signed out, one call per burst, none when a burst ends where it started.

## Share to Persona (#4586, #1460)

In another App, Share › Persona sends text, a web page or link, up to 4 images and up to 10 files (PDFs, documents, videos, archives…), with an optional note, to the conversation or agent picked in the sheet, or starts a new chat with them (`PersonaShare/`, target `PersonaShare`). Android's ShareActivity has the same recent-first rules (#2515). It is separate from the composer's photo, camera and file pickers, which it does not change (#2282).

- **Send To**: the sheet lists Persona's main conversation, the targets shared to most recently that are still listed (latest first, at most 3), the one picked from the full list, and New Chat. "All Chats and Agents" opens the full list, searchable by title, machine and folder: Recent, Persona (the main conversation, New Chat, the user's own conversations newest first, at most 30; not archived, Health's or the automatic ones), and Agents (the ones that can take a message and are not offline, newest first, with machine and state as in the Agents list).
- **Where it starts**: on the last target when it was used within 24 hours and is still listed; otherwise on the main conversation, or New Chat when there is none yet. Sending is always the user's tap. The note is not focused when the sheet opens, and Send To comes right under it: the keyboard would otherwise hide where the share goes. A sent share records its target (a new chat as the conversation it started; at most 8) in the extension's own defaults; the app never reads them.
- **Sending**: as the composer does (images resized to 1600 px, JPEG; the same `client_msg_id` and op id on a retry), `POST /api/v1/sessions/{machine}/{session}/messages`; New Chat is `POST /api/v1/assistant/conversations` with the shared items as the first message, on the hub's default launcher.
- **Safari pages** (#2282): the extension declares `NSExtensionActivationSupportsWebPageWithMaxCount` and `NSExtensionJavaScriptPreprocessingFile` (`PersonaShare/SharePage.js`, a resource of both share targets), so Safari runs that script in the page first and hands over its `document.title`, `document.URL` and selected text as a `com.apple.property-list` item (`NSExtensionJavaScriptPreprocessingResultsKey`). The message is the title, then the link on the next line, then the selection quoted (`> `, at most 4,000 characters). Safari also hands over the plain link and its own text for the item; `ShareText.compose` (`Shared/ShareText.swift`, `ShareTextTests`) keeps each piece once, comparing them without surrounding spaces or a link's trailing slash. Everything else keeps the order it was shared in (each item's text, then its attachments' links and texts as they come); only the page goes first, so other apps (no script) send exactly what they did before. The property list is read before files are looked for: it is data and would otherwise go as a file.
- **Files**: pictures stay pictures (the first 4; more go as files). Anything else that is a file (a movie, a document or archive, a file's text: it has a name with an extension) is copied into the extension's temporary folder when the sheet opens, then on Send uploaded one after another as the composer's + › Files does (protocol.md "Sending files", 200 MB each; over that, the sheet names it and leaves it out) to the computer the target runs on: the agent's, the assistant's for Persona's conversations and a new chat. Their paths follow the text, quoted when they have spaces (`PathInsert.quoted`), and the chat shows them as file cards. A progress bar shows while they upload; a retry does not upload again what already reached that computer.
- **Size before Send** (#2282): under the shared files, the footer gives their total and the limit per file ("12.3 MB in total · up to 200 MB per file", `ShareSizeFooter`). From 50 MB in total (`ShareTargets.slowUpload`: about 40 seconds at 10 Mbit/s up) it adds that large uploads can take a few minutes and the sheet should stay open: the upload runs in the extension, which goes away with the app it was shared from.
- The extension connects to the hub itself; the App need not be running. It compiles all of `apple/Shared` as the watch does and reads the App-saved pairing from the keychain (`HubStore` "hubs" entry, App keychain group; `Keychain` takes the app's service from `PersonaAppBundleID`) through a keychain access that never writes (`ShareTargets.readOnlyKeychain`). It uses the default group (else the first), on the hub of it that answers first (`HubRoute.pick`), then lists with `GET /api/v1/assistant`, `GET /api/v1/sessions` and `GET /api/v1/machines`. No App Group, so no manual group setup on the developer website.
- Failure stays on the panel with a reason: unpaired, invalid pairing, cannot reach the hub, or the hub's own words for a refused send; the list has Try Again. No SSH tunnels; other groups of hubs and other managers are not offered.
- Rules: `Shared/ShareTargets.swift` (`ShareTargetsTests`), `Shared/ShareText.swift` (`ShareTextTests`); the sheet in the simulator: `ShareSheetTests` (the runtime CLI's `share` opens the system share sheet in the app, `fileMB` makes its files large enough for the size footer's slow-upload hint, `testLargeFilesShowTheirTotalAndTheSlowUploadHint`; against the mock hub, light and dark by `simctl ui appearance`).
- From the system's apps: `scripts/app/share-from-apps.sh [light|dark] [shots dir] [test | suite]` runs `ShareFromAppsTests` on a stock simulator (`sim-boot.sh --profile stock`) with its own mock hub. It seeds a picture with `simctl addmedia` and a PDF in Files' On My iPhone (the `group.com.apple.FileProvider.LocalStorage` container's `File Provider Storage`), then drives Safari to the mock hub's `/mock/share-page` (title "A Page Worth Reading"), Photos and Files: Share › Persona › Send, checking that the hub got the title and link together and once each, one picture, and the file's path.
- Release: the share extension has a new Bundle ID (`<bundle id>.share`), see `docs/operations/release.md`.

## Clipboard with your computers (#3064)

Send the phone's clipboard to a computer, or get a computer's clipboard onto the phone, through the hub and the computer's resident node
(the hub's `GET` / `PUT /api/v1/machines/{mid}/clipboard`, `GET` / `PUT …/clipboard/settings`; `Machine.clipboard` says whether it can, whether it is on,
and why not: `reason` in the reader's language, `reason_code` as a stable word that the app turns into its own sentence when `reason` is missing).
The server side, its errors and limits are described with the hub's clipboard routes (#3064); a hub without the feature sends no `clipboard`, which the
app treats as unsupported and off (no section, never offered).

- **Where**: the computer's page, right after Files, when the hub reports `clipboard` for it (`ClipboardSection`, model `ClipboardModel`; iPhone only for now,
  the Mac and visionOS apps later). The per-computer switch is off by default; a computer that can't (no graphical session) says why, an offline one says so.
- **Send Clipboard** reads `UIPasteboard` (`PhonePasteboard`: an image first, else text, else a link; images as PNG, or JPEG when the PNG is over 8 MB). iOS asks
  "Allow Paste?" for what another app copied. Beside it the system Paste button (`SystemPasteButton`, a design-system wrapper of `PasteButton`) hands the content over
  without asking: the tap is the permission. **Get Clipboard** writes the phone's pasteboard (never prompts) and previews the text or the image under the buttons.
  The image is decoded once, off the main thread, at the size the preview draws: 200 pt high at most, so as wide as its aspect ratio allows
  (`ClipboardModel.previewPicture`, #7925). For a readable picture, the phone receives the original bytes; unreadable image bytes show the
  existing `clipboard_bad_image` failure and no empty preview.
  Results are toasts; a computer's hidden content (password managers' concealed types) comes back as `concealed` and is not copied.
  Sizes are checked before sending, as the hub checks them (text up to 4 MiB, an image up to 7 MiB; an image whose PNG is bigger is tried as JPEG at falling
  quality), so a too large clipboard is said at once. The hub's refusals each have a sentence (`ClipboardFailure`): `clipboard_disabled`, `read_only`,
  `clipboard_unsupported`, `offline`, `too_large`, `clipboard_bad_image`, `clipboard_failed`; anything else shows the hub's own words. The switch's state
  follows the pushed `machines` list; the page also reads `GET …/clipboard/settings` when it opens.
- **Paste hint**: after the first paste the user allowed from Send Clipboard, a one-time notice says Settings › Persona › Paste from Other Apps can be set to Allow, with
  Open Settings (`openSettingsURLString`) and Don't Show Again; the flag is `clipboard.pasteHintDismissed` (written once).
- **Offer on return** (Settings › Phone Data › Clipboard, `clipboard.offerOnReturn`, off by default): coming back after the pasteboard changed elsewhere, a toast asks
  "Send what you just copied to <computer>?" with Send (`ClipboardOffer`). Only `changeCount` and `hasStrings` / `hasImages` / `hasURLs` are looked at, which never
  prompt; the content is read on Send. The change count seen when the app left lives in memory and `StateCache` (`clipboard-seen`, once per backgrounding), not
  UserDefaults. The computer offered is the one last sent to (`clipboard.lastMachine`) if it still shares its clipboard, else the first online one that does.
- **Shortcuts**: Send Clipboard to Computer and Get Computer's Clipboard (`docs/apple/intents.md`).
- **Limits**: iOS can't read the clipboard in the background, so phone → computer is always a tap (no automatic sync); computer → phone needs the app (or a shortcut) too.
- **Screenshots and tests**: `MOCK_CLIPBOARD=off|on|image python3 scripts/mock/mock_hub.py` (`MOCK_CLIPBOARD_FAIL=1`: every read and write fails as `clipboard_failed`); `-tab settings -openMachines YES -openMachineDetail macbook -openMachineClipboard YES`
  (scrolled to the section), `-showClipboardFetch YES` (runs Get on opening), `-showClipboardFetched text|image` (a self-contained preview),
  `-showClipboardHint YES`. `ClipboardSectionTests` (UI, `TEST_RUNNER_PERSONA_CLIPBOARD_HUB`)
  runs Get → Send → hint and the offer; with `MOCK_CLIPBOARD=image` and `TEST_RUNNER_PERSONA_CLIPBOARD_STATE=image` it checks fetched and DEBUG image previews;
  with `MOCK_CLIPBOARD=off` and `TEST_RUNNER_PERSONA_CLIPBOARD_STATE=off` it checks the default-off state
  (the switch and its explanation only, no buttons; the offer setting off). A hub without `clipboard` on a machine hides the section and never offers it; unit tests `ClipboardModelTests`, `ClipboardOfferTests`, `ClipboardIntentsTests`. The simulator could not answer "Allow Paste?"
  for what another app copied (its TCC store refused the answer, 2026-11-10), so the prompt itself is checked on a device.

## Effort and permission modes

Two small labels above the chat input: permission mode (Manual approval / Auto-edit files / Plan only / Auto / Allow everything) and effort (Auto / Low … Max),
like Claude Code's terminal bottom bar; tap to change (`SessionModeBar`, wording and icons in `SessionModes`). The new shell moves these into the input's bottom-right
"Model · Effort" button (#1091, see the input under "New shell" above). New Agent forms and the assistant home page above its input
also offer selection (unset means "Follow launcher"). New Agent offers a launcher that lists the permission modes a start on it takes
(`Launcher.permissionModes`, the node's `permission_modes`, #8261: Paseo's Grok, OpenCode or Codex, Codex's start included) only Use Launcher's (its own
approval, sent as no mode) and, where it lists it, Allow Everything, and only the efforts its model lists (`StartModes`); one whose own approval is all it lists
shows no mode field. A computer older than the list is told by the agent instead: a launcher with `source` `paseo` and an agent of its own, such as Grok, gets
Use Launcher's and Allow Everything (#8144). Paseo refuses Plan Only for these and drops the other modes. The mode and effort remembered per computer are
Claude Code's: such a start never sends them, never saves over them, and its picks are made on the sheet each time (`providerMode`, `providerEffort`); its
footer says the agent uses its own approval settings unless Allow Everything is chosen (and, where the launcher lists its modes, that the chat can switch
later), and its first message's hint names no Claude. Pickers name a launcher by its title (`display_name`: Paseo's label for its provider, a
Persona-managed provider's name), and the search finds it by that too. A running chat whose row lists two modes (`capabilities.permission_modes`,
`Session.listedModes`) offers those two rows alone in the header's mode menu (`HeaderModeButton`) and the composer's model menu (`SessionModesMenu`,
`SessionModeBar`): Use Launcher's, drawn as a mode not known (the plain permissions symbol in no tone, never Ask First's hand, since its own settings may
allow everything; no mode reported is this one), and Allow Everything, which asks first (`SessionModes.confirmation`); leaving it does not. Session Info
names the mode the same way and leaves out Context, Compactions, Chat and Output where the node says nobody gives them (`usage_unknown`). The web and
Android follow the same rule (`docs/design/DESIGN_SYSTEM.md`, "Permission modes a provider lists (#8261)"). Codex / the internal Codex fork sessions do not show them; app-server-driven Codex sessions (hub supplies `efforts`, `Session.setsEffortOnly`)
show effort only, with levels supported by their model (`SessionModes.effortOptions`, `docs/integrations/codex.md` ("Effort")). When the row carries `capabilities.modes`, that Boolean decides instead: `false` hides the controls for Claude outside Persona's terminals, and `true` enables them for a compatible runtime such as Orca or agy. An older hub that omits the field keeps the agent-based behavior (`Session.canChangeModes`; `docs/protocol/protocol.md` "Codex sources and capabilities").
For Claude Code sessions in herdr, the effort menu also has a Model section (#3119, matching web #2434; `SessionModes.switchesModel`):
the launcher's list as above (#3754; the fixed Default / Opus / Sonnet / Haiku until it is read), ticked by the running model's exact ID or the alias that stands for it, so a version is never shown as its family; choosing sends `POST …/modes {"model"}`, and the node enters `/model <id>` with message-sending safeguards and checks the screen; failure explains why.
While working, waiting for your answer or not running, this section is disabled and its title explains why; the title notes that the model also becomes the default for new sessions (Claude Code behavior).
The new shell's Model · Effort menu still sends `/model <id>` as a message.

- Display: each transcript user row has `permissionMode`, each reply `effort`; App changes take priority until the updated value appears in the transcript
  (node/modes.go). Assistant chats are an exception (#2416): App values persist in hub session state (`app_mode` / `app_effort`), always override the transcript and survive node restarts
  (previously the main chat's old transcript default locked the mode, and App changes reverted on node restart). Terminal shift+tab changes enter the transcript only on the next message; developer-info cards read the terminal bottom bar for live values.
- New sessions: `--permission-mode` / `--effort`.
- Changing running sessions: the headless assistant sends stdin control requests (`set_permission_mode`, `apply_flag_settings {effortLevel}`), effective on the next request;
  herdr Agents are operated like a person: shift+tab cycles to the target (default → acceptEdits → plan → auto), and the `/effort` picker moves to the level,
  then presses `s` (session-only). **Do not send `/effort <level>`**: it also saves the level as this model's default for all future sessions (`modelSettings` in settings).
- Effort changes wait until the turn ends and the terminal input is empty (otherwise `/effort` appends to someone else's unsent draft); herdr Agents can choose Allow everything only at creation, not in the cycle,
  unless their claude was started with `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions` or `--permission-mode bypassPermissions`, which puts it in the cycle:
  the node reads the arguments of the claude its hooks run under, once per process (`claudeCanBypass`, kern.procargs2 / `/proc/<pid>/cmdline`, no exec), and lists the session with
  `bypass_available` (#1379). The web and the app then offer it, red and asked again (`SessionModes.switchableModes`, `confirmation`); without it the app's header menu lists it disabled, with why (except Antigravity `agy` runtimes with `capabilities.modes`, which switch all five modes including Allow Everything directly via ACP with no restart or confirmation and are not `setsEffortOnly` even when `efforts` are listed).
- Allow Everything by a restart (#6513, the web's #6476; `docs/integrations/claude-integration.md` "Full access by a restart"): without `bypass_available`, a Claude Code in a pane
  Persona can type into (`SessionModes.paneClaude`, `bypassRestarts`) is offered it anyway, with "Restarts the session to apply it once this turn ends; the conversation carries on" under it
  and in the confirmation (`bypassNote`); the node restarts it once its turn is over. While the row has `bypass_pending`, every mode button keeps the mode the agent still runs in
  (the header capsule's look does not change) and says "The session restarts in Allow Everything once this turn ends": the header menu's title, VoiceOver's hint and the
  pointer's tooltip, a line over the composer menus' modes, and Allow Everything's own row. Picking the checked mode while `bypass_pending` sends `session.modes` without a confirmation
  (`SessionModes.appliesPick`, `MenuChoice.reselect`) so the node drops the queued restart directly. An assistant conversation's Allow Everything says "Takes effect from the next message".
- `/permissions` (#6513): in an agent's "/" list (`SlashMenuRow.permissions`; the BeautifulUI prompt bar's commands too) and in Persona conversations wherever a session's mode can be switched from here
  (`SessionModes.offersPermissionsCommand`, including Antigravity `agy` runtimes with `capabilities.modes`). Picked, or typed and sent (with nothing attached: `SessionModes.opensPermissions`), it opens `PermissionModeSheet`: the header menu's choices with Plan Only (`SessionModes.commandRows`), the current
  one checked, Allow Everything disabled with its reason where it can't be taken, `SheetClose` at the top leading corner, and a section footer only when Allow Everything's row does not already carry the pending restart note; a menu can't be opened from code, so it is a full-height sheet (a medium one hid Allow Everything and its note). A pick closes it and goes through
  `ChatView.requestMode` (its confirmation); nothing is sent to the agent. On the phone, Persona's shell composer has no "/" list so typing and sending `/permissions` opens the sheet (`PermissionsCommandUITests`); where a plain "/" list shows, it lists `/permissions` (beside `/clear` in the main conversation). `-openPermissionModes YES` opens the sheet
  and `-openSlashMenu /per` types that draft, for screenshots (with `MOCK_BYPASS_PENDING=1` for the waiting state).
- Agents header (#3719): the agent's mode at the end of the open header's second row, icon and color only (`HeaderModeButton`,
  `SessionModes.headerLook`, `Theme.ModeTone`): Ask First a quiet gray hand, Plan Only a teal clipboard, Auto-Edit a purple pencil, Auto a purple
  wand, Allow Everything a red shield; not reported yet the plain permissions shield (never a guessed Ask First). A tap opens the mode menu; going into
  Allow Everything asks first, and so does leaving it when `bypass_available` is false (no way back from the phone; assistant conversations and Antigravity `agy` switch directly without confirmation); the composer's model menu
  goes through the same `ChatView.requestMode`. Shown where the agent has a mode to see or change (`SessionModes.showsInHeader`): not Codex,
  TRAEX or a runtime without modes, not hosted pages (one-row header, #3255), not the folded bar. A row that lists its own modes (#8261, Paseo's
  Grok, OpenCode or Codex) shows its two rows instead, its own approval as the plain shield, a switch both ways (`SessionModes.menuOptions`,
  `headerLook(session:)`; see the New Agent paragraph above). Client-only: `permission_mode` is already in every
  session row (read off the transcript tail the node reads anyway), so no request, push or poll was added (measured in the PR).
- Assistant chats (Persona page, including the main chat) can choose Allow everything both at creation and while running (#2416); a red warning at the menu top explains consequences (`BypassWarning`). Persona does not start
  claude with `--dangerously-skip-permissions`, so a control request cannot switch to bypassPermissions: stop an idle process, then resume the next message with `--permission-mode bypassPermissions`;
  if answering or holding background tasks, replace the process on the first message after this turn.
- Auto effort (`auto`, the model's default). Tested on Claude Code 2.1.316: `--effort` rejects auto (warning, then uses default),
  settings.json `effortLevel` rejects it too (silently ignored), and the `/effort` picker has no Auto slot; only `/effort auto` accepts it, but also clears
  this model's saved default in settings. The three paths are therefore:
  - New sessions: omit `--effort`. Equivalent to Follow launcher, so new-session forms omit this option (`SessionModes.startEffortLevels`).
  - Headless assistant: `apply_flag_settings {"effortLevel": null}`, the same representation Claude Code itself uses for auto. Transcripts record the actual level;
    the node must not send it back as a new App choice (which would pin auto to that level). Only the App changes assistant effort; once Auto is selected, it remains auto.
  - herdr Agents: open `/effort`, move right from the leftmost slot to the one labeled "The default effort for this model", press `s` (`enter` for the slider picker).
    An approximation: the session is pinned to the current model's default and does not follow later model changes; if organization settings changed the default and the label cannot be found, the node cancels and errors.
  - Display: transcript replies record actual levels, never auto; the Auto label lasts only until the next reply, then shows its level (for example Medium).
  - Selecting auto in Settings › Claude Code Settings removes `effortLevel` (`claudeconfig.go`).
- While the picker is open, herdr reports the Agent blocked; after 1.5 seconds the hub would push it to the phone as a terminal dialog needing attention, so the node suppresses blocked while operating it.

### Claude Code ultracode (#1307)

The server contract is #3427 ([protocol.md](../protocol/protocol.md) ("Claude Code ultracode")): a session-only switch independent of the effort,
off by default, never remembered and never turned back on by itself after a resume. The behavior follows the web's (#3428,
[web.md](../web/web.md) "Claude Code ultracode"). Pure rules are `Shared/Ultracode.swift` (the watch compiles them for its rows' mark), requests
and what this device asked are `AppModel.setUltracode` / `dismissUltracodeLost`, views `Views/Chat/UltracodeViews.swift`.

- **Where**: the hub lists the `ultracode` feature (an older hub shows nothing), the agent is Claude Code (not TRAEX, Codex or a runtime's), its effort can
  change now (it takes messages, no read-only source, not offline), and it is not a Persona conversation (the main chat included: the server refuses those).
- **Session fields**: `ultracode` (omitted is unknown, drawn off; the info card says Unknown), `ultracode_note` (the node's reason), `ultracode_pending`
  (applies next turn) and `ultracode_lost_at`. An odd value never drops the session. `ChatCache` stays at `chats-v10`: the hub only rewrites the text of
  Claude Code's existing `/effort` output notice (same id, still a short notice), so a cached copy keeps the CLI's own confirmation until its turn
  moves on, and the notices from `ultra_effort_*` attachments are new ids that merge in; dropping every cached chat for that was not worth it.
- **Menus**: a section under the thinking effort in the shell composer's model menu (`SessionModesMenu`) and in the shared effort chip (`SessionModeBar`) on Mac / Vision:
  a `Toggle` whose subtitle is the line (what it does, "Applies next turn" while a change this device asked waits, "A change is queued for the next turn"
  for another device's, or the node's reason), and the Claude quota with its 80% warning as the section's header (`Ultracode.menuWords`; numbers the hub marked out of date add "Outdated · 2 hr ago" right after the quota, `Ultracode.quotaAge`, and New Agent's toggle fades the quota and draws that age as its own line, #1906). A menu
  draws a subtitle only from a second `Text` directly in the label (a `Label`'s two-`Text` title loses it, on the Mac too: the effort levels' details
  are not drawn) and colors no words. An iOS menu cuts a subtitle after three lines but draws a header whole, line by line, so there the node's reason
  goes first in the header and the subtitle keeps what the switch does or that a change waits; a Mac menu wraps a subtitle (`NSMenuItem.subtitle`) and
  draws a header on one line (a line break becomes a space), so there the reason stays in the subtitle and the header's parts are joined with " — ".
  The quota was a disabled item at first: drawn faint (about 2.3:1), it read as something not available. A menu that opens upward lists its items the
  other way round, so on iOS and visionOS the section is declared before the effort (`UltracodeMenuSection.leadsEffort`) and reads under it, next to
  the button without scrolling; a section's header stays on top of it either way. A Mac menu keeps the declared order.
- **Where the switch stands**: what this device asked while the request is out, while the node says it is queued, and until the node's push answers
  (`Ultracode.Ask.answered` is the session as its page drew it when the request returned; the pushes that came meanwhile land first, `landPendingSessions`).
  Grayed only while its own request is out; the node's reason replaces the line but does not gray it. An ask is forgotten at the first update of the
  lists after the session moved past it, or went (`dropSpentUltracodeAsks`, as the web's `useUltracodeAsk`): only hiding it let an old ask come back
  when another device queued a change later, and the switch said "Applies next turn" for what this device had asked long before. A failure, a refusal (`409 node_outdated`,
  `ultracode_unsupported`) too, puts it back and is said once in a toast (the menu has closed); nothing is kept, so asking again gets the node's answer
  again. A refusal never becomes the main chat's readiness card. A queued change never holds a message back (#5510), so sending is unchanged.
- **Marks**: `Theme.Icon.workflow` in `Theme.ai` after the effort on the composer's model button and the shared effort chip (read out as "…, ultracode on"),
  beside an agent row's title (`SlotRow` in both styles, `CompactSlotRow`, the watch's list; not on an offline row, which the node still remembers on;
  where a title can take two lines, `SlotRow`'s and the watch's, the mark is part of the text after its last word, `UltracodeMark.after`),
  and on a Codex thread at its `ultra` effort (no extra words: its effort says Ultra). It comes and goes with `Theme.Motion.state`, at once with Reduce Motion
  (`UltracodeMark.motion`); nothing repeats.
- **After a resume**: one line over the composer, "ultracode off after resume", **Turn Back On** (`{"ultracode": true}`; "Applies next turn" while it waits)
  and × (`{"ultracode_dismiss_lost": true}`, gone at once, back if the hub says no).
- **Info card**: a row "ultracode: On / Off / Unknown" under Thinking Effort; none for an offline agent, whose process is gone (as its row has no mark).
- **New Agent**: a switch under the thinking effort for Claude Code, off each time the sheet opens and never saved with the per-machine effort; only
  `"ultracode": true` goes with `POST /api/v1/sessions`. Grayed with why when the computer's `claude_version` is a known version before 2.1.284 (an unknown
  one may be tried). A start the hub refuses for it fails with "Couldn't start with ultracode: … Retry starts without it" and the switch goes off.
- **Quota**: the Claude subscription's 5-hour and weekly use from `GET /api/v1/usage/windows` (`UsageWindowsStore`): a chat that offers the switch keeps
  it no older than two minutes while it is in front (`keepFresh`, nothing extra while Machines or the Mac's bars poll), New Agent loads it when it shows
  the switch; only with data: an account read on the computers before a gateway's, fresh before stale,
  then the fullest. From 80% a warning, only for a switch that stands on or can simply be turned on.
- **Plan cards and to-dos** (#12, the web's rules in `web/src/lib/ultracode.ts`), only when the hub announces the feature: each task on an open plan card
  has an **ultracode** chip after its permission chip (AI color while on, quiet while off) that changes it before Start, and Edit has a switch per task
  that opens with the card's choices. Start sends `ultracode` only on tasks whose choice differs from the assistant's, so a plain Start still sends no
  `tasks`. On a computer whose known Claude Code is older than 2.1.284 the chip is grayed, a task that was on gets an orange "Starts without ultracode: …"
  line, and it starts with it off. Task cards, folded task lines and to-do rows show `UltracodeMark` after the title while one of the task's Agents
  carries the row mark (`rowMark`); before any Agent exists, task cards and lines show it when the plan task asks for it (to-do rows, as on the web,
  follow the Agents only). Done and failed-to-start to-dos show none. List rows
  (`summary=1`) omit `ultracode`, so `Session.preservingDetails` keeps the value already known. Rules: `apple/Shared/PlanUltracode.swift`; views:
  `apple/Persona/Views/Assistant/Tasks/PlanUltracodeViews.swift`; tests: `PlanUltracodeTests`.
- **Tests**: `UltracodeTests` (FakeHub: decoding with older hubs, the requests, where it shows, graying, the ask, the quota) and `UltracodeUITests`
  against the mock hub with `MOCK_ULTRACODE=1` (`TEST_RUNNER_PERSONA_ULTRACODE_HUB`, screenshots with `TEST_RUNNER_PERSONA_SHOTS`).

## Agent presets in New Agent and plan cards (#2225)

`AgentPreset` / `AgentPresetProfile` decode `tiers` and `tiers_profile` from New Agent's `GET /api/v1/machines/{mid}/launchers`; `AppModel` caches them beside the launchers, default and unavailable lists and clears them with those caches. An older hub, no active profile, or no tier this computer can run leaves **Preset** out and keeps the ordinary defaults and start body unchanged. The body never carries `tier`: a preset is only a way to fill launcher, model and effort.

When tiers exist, New Agent's Preset NavigationLink opens `AgentPresetPicker`, naming the active profile and showing each tier's name, description and launcher/model/effort. A launcher with blocked health has its reason on a disabled row, so it cannot be picked. Picking another row fills the three fields through the existing `restoredModel` path; manual launcher/model/effort edits unmark it, reselecting restores its defaults, and machine changes or recent-setup fills clear the pick. Explicitly choosing the list's default model through a preset remains an explicit `model` in `POST /api/v1/sessions` (`LauncherModelChoice.presetDefault`), rather than collapsing to Model Settings' default.

A preset with a model or effort waits for its own launcher's model-list request: Start is disabled and guarded, with **Loading the preset's model options…**, until that request settles. This prevents the previous launcher's list from dropping or replacing the preset's values. Once the list is available (or its absence/failure is known), the form names a locked or unlisted model and an unsupported effort that the start cannot send, using the launcher's or model's own settings for the omitted field; the existing list error and locked-model explanation remain visible too. Plan tasks decode and encode their optional `tier`. Their model chip gives an explicit model precedence, then shows `launcher · Preset <tier>` or `Preset <tier>`, then the existing launcher/Default Model label.

## Launcher health in New Agent (#1643)

`Launcher.health` (`apple/Shared/Models.swift`) is the hub's record for the computer the list is for (`docs/server/launcher-health.md` "In the App"); a record it can't read is dropped, never the launcher. Since #4437 the hub leaves out a launcher whose latest check failed, so the list the app reads carries `blocked` only for `not_in_terminal` and for the configured default; the rest are gone until a check passes.
`Launcher.block` / `LauncherChoices` (`Views/NewChat/LauncherAvailability.swift`, `LauncherHealthTests`) decide what is grayed out: only `blocked`, one line per failure class.
In `LauncherPicker` a blocked row is gray with the line (`LauncherBlockLine`) and an info mark; a tap opens `LauncherBlockedHelp` (why, How to Fix, the record under Details, Add Provider → `ProviderWizard`, Try Anyway picks it once the sheet has closed),
never picks it. The form says the line under the launcher row when the launcher that will run is blocked (Model Settings' default, or picked anyway); Start stays enabled. Run Agent Here picks a usable launcher first, lists blocked ones last in its menu under a section titled with the line (a picker menu drops an option's second line), and says it in its footer once picked.
Both read the list with `AppModel.launchersForStart` (fresh, not the cache). Add Provider is offered only for a computer in the hub-level group (the wizard saves there).
A Persona-managed provider whose own-service rule leaves it out on the computer (`own_service: skip` where Claude Code or Codex has a sign-in of its own, #8109) comes in the hub's `unavailable`, kept with the list (`AppModel.launcherUnavailable`). `LauncherPicker` lists it after the launchers under "Unavailable here", in the main chat model sheet's row (`UnavailableLauncherRow`, `Views/Chat/UnavailableLauncherRow.swift`: a lock, the name, the hub's reason), never picked; How to Fix under the reason pushes the provider's page (`ModelLauncherDetailView`, Settings › Model providers), whose Edit › Computers has the switch, and saving it reads the list again. As Add Provider, only for a computer in the hub-level group (Settings' pages read that hub); elsewhere the row says the reason only. Tests: `NewAgentUnavailableTests`, `NewAgentUnavailableUITests` (`MOCK_OWN_SERVICE_SKIP=1`).
`StartingNote` shows a coded start failure (`Session.startErrorCode`) as Didn't Start over the hub's whole sentence. Screenshots: `docs/operations/testing.md` ("Mock-data screenshots").

## Terminal Traex text starts in New Agent (#1842)

A known terminal Traex launcher (its `runtimeAgent` is `traex`, discovered in `zshrc` or explicitly typed without another runtime source) offers none of Claude Code's permission mode, thinking effort or ultracode choices. With a nonempty `FirstMessage.text` (the to-do context, then the typed text) and no attachments, Start makes one synchronous `POST /sessions` with the resolved launcher alias, `agent_type: traex` and that text as `prompt`, then opens the returned session without a separate ordinary message. Empty starts, attachments, unknown launchers and other runtime sources keep their existing negotiation and delivery paths. `StartModes` owns the control rule; `NewChatView.start` owns the narrow inline-start branch. Regressions: `PaseoPermissionModesTests`, `NewChatCheckTests`, `NewAgentAttachmentsTests`, and `MachinePagesTests/testNewTraexSessionStartsWithItsFirstMessage` against `MOCK_MACHINES=1 MOCK_TRAEX=1`.

## Recent setups in New Agent (#2134)

Every start from New Agent (the Codex and Claude Code paths, where the per-machine `launcher.` / `mode.` / `effort.` / `cwd.` keys are written) records its setup in `RecentSetupsStore` (`apple/Shared/RecentSetups.swift`, one `StateCache` file `recent-setups`): machine (`MachineRef.path`), folder, launcher (`""` follows Model Configuration), the model the start sent (`""` for the default), agent type, and Claude Code's mode and effort. Ids only, on this device; the rules (record, cards, a machine's model) are `spec/fixtures/recents/`, which `RecentSetupsTests` runs case by case.
- The form's top section **Recent** (hidden when joining a space) shows at most three cards on the machines offered (`RecentSetupRow`, `Views/NewChat/RecentSetupCards.swift`): folder · machine, then launcher · model · mode · effort; Allow Everything in `Theme.cautionText` with its mark; an offline machine's card gray with Offline and not pickable. A card's context menu has Remove from Recent and Clear Recent Setups.
- A tap fills every field and focuses First Message; it never starts. Another machine's card is applied once that machine's launchers and directories are in (`pendingFill`, after its saved defaults); the model is picked when `loadModels` moves to that launcher's list (`restoredModel`), before the list comes, so the efforts offered are the model's. A Codex card leaves the machine's Claude mode and effort as they were.
- Picking a machine (on open or by its card) also restores the model of its latest setup with the launcher it restores (`RecentSetups.model`); none: the default. No new per-machine key: Siri's `newChat.machine`, `launcher.<m>`, `cwd.<m>` are unchanged.
- UI flow: `RecentSetupsUITests` (`docs/operations/testing.md`, "Mock-data screenshots").

## An agent's live reply (#1309)

A Claude session Persona started sends its reply's lines as Claude shows them, before the transcript has them
(`docs/integrations/claude-integration.md` "Live replies (MessageDisplay)"); a runtime's session may send `delta` too. The behavior follows
the web's (`web/src/lib/live-text.ts`, `docs/web/web.md`).

- **Where it draws**: any chat of an agent (iPhone, iPad, the Mac app; not the watch) draws it under the newest items, the way the main chat
  draws the assistant's (`ChatLiveText`, `StreamingText` with its caret; the chat follows the end while it grows, `ChatStreamFollow`; while you
  read history it is held with the rest, `Frozen`). The running turn's words before it go into its work line meanwhile (`ChatLines`'
  `streaming`), as in the main chat, and the live work line stays above it. The main chat's look and rules are unchanged.
  This differs from the web on purpose: the web keeps a live turn's words in the flow with its steps and its live work row under the
  streaming words; here a turn shows only its latest words outside its folded work line, so the words before go into it once the new
  message lands anyway. Folding them when the new message starts streaming, not when it lands, keeps the chat from jumping each time
  the agent's next message comes (the main chat's reason, 2026-11-04); `ChatLinesTests.theRunningTurnsLatestWordsFoldWhileTheNextStream`.
- **Which deltas**: `AppModel.streamsAgentReplies`: a listed session that is not a Persona conversation, or a chat already given an agent's delta.
  An unlisted session's delta is handled as before (the assistant's rules), and its chat in the Agents style draws nothing. Both go through the
  same pacer (`DeltaPacer`, at most 5 draws a second, an empty delta at once).
- **Even reveal** (#6651): the model's text comes in bursts (a few dozen characters about every 350 ms, up to 2 s apart), so drawing what
  arrived showed a jump per burst. `StreamingText` reveals it a few characters a frame (`StreamReveal` in PersonaKit, driven by
  `StreamRevealClock`'s display link, which runs only while text waits, 10 times a second: each change re-measures the chat list, so the rate is the owner's trade-off between smoothness and power, #7135): a little slower than the stream's average so far,
  faster when more waits (waiting / 0.6 s), never slower than 20 characters a second. Only that view redraws per frame; the store and the
  list see the deltas as before. A first text over 80 characters (a chat opened mid-stream) shows at once; when the stream ends
  (`ChatStore.liveDone`), or the user holds the list, or the reply's item replaces it, the whole text shows at once. The Mac shows text as
  it comes. Measured with `PerfAssistantTests/testStreamingTrace*` (recorded model streams, `scripts/mock/stream_trace.py`).
- **What it shows** (`ChatStore.agentStreamed`, `Shared/LiveText.swift`, a port of `live-text.ts`, compiled for the watch too): the delta as it
  came (`agentLive`) is never drawn as it is, only what of it the newest items lack (`liveText`): the agent's latest message only (its words at
  the end of the transcript, nothing after them), only where a word ends, a cut item counting as its whole block (`full_size`); a late whole copy
  of the message before (its words, then a tool's result) shows nothing. Matching is by Unicode scalars and UTF-8 bytes like the node and the web.
- **Only the newest items touch it**: it is worked out again on a delta and on the newest items (`apply`, `replace`: pushes, the latest page,
  pulls), against the items from the latest exchange's opening on, never before the latest page from the end, with folds opened here folded again
  as the page brought them (`ChatStore.liveTail`). An older page (`prepend`) or an opened fold (`unfold`) does not change it.
- **No gap, no double**: a change that comes while the rows are being rebuilt waits for them and is published in the same pass
  (`agentLiveDue`), so the words leave the live copy as their row comes. `landed` / `clearLive` (the assistant's prefix rule and its 5 s hold) are
  not used for an agent's: the node sends `""` right after the items that replace it. A socket that starts over drops both (`dropLive`).
- **Only while subscribed**: the hub sends `delta` to a chat's subscribers alone, so the `""` that ends the live text while you are away
  from the chat goes to no one, and the hub sends it again on subscribing only while a message is being written. `AppModel.forgetAgentLive`
  drops an agent's live text (and what the pacer kept) when the chat's subscription ends and again when one starts, as the web's chat
  starts with none each time it opens; otherwise the words before a tool came back under a later message of the turn on returning.
- **Setting**: Settings › Claude Code Settings › Live Replies, per computer (`LiveRepliesModel`, `LiveRepliesSection`, the web's `LiveReplies.tsx`):
  `GET` on showing the page and on picking another computer, `PUT` on toggle with the new value shown at once; a failure puts it back and says why
  under the switch; with Apply to All Online Machines on, the other online computers change too (each through its own group's hub). An older hub
  (404) shows no section. On by default; a change reaches the sessions started after it. The copy says what it costs: the computer's terminal
  shows a reply a few lines at a time.
- **Tests**: `LiveTextTests` (the web's cases one by one), `AgentLiveReplyTests` (every order of the last delta, the items and the `""` with and
  without a rebuild between: never a gap or a double; the words after a tool; an older page, an opened fold; `AppModel`'s routing; leaving
  the chat and coming back),
  `LiveRepliesTests` (FakeHub). Screenshots: `MOCK_AGENT_LIVE=1` and `GET /mock/agent_live?step=…` (`scripts/mock/mock_hub.py`), the PR #42 review
  agent `-openSession macbook/a1f3c2d4-0000-4000-8000-000000000006`, the setting with `-tab settings -openClaudeSettings YES`.

## Session triage (Settings › Session triage, #3078)

Below Assistant, the same four toggles as web and Android (`GET / PUT /api/v1/agents/triage`, stored on the hub, consistent across clients, all off by default; meanings and whose tokens are spent in `docs/assistant/agent-work.md` ("Switches")):
master switch and Triage model (opens model configuration `agent.triage`), followed by cards for decisions, automatically continuing after errors, and scheduled wake-ups; disabled while the master switch is off.
Changes save immediately; failure restores the old value and explains why at the top; old hubs (404) show only unsupported. `TriageSettingsModel` / `TriageSettingsView`, shared with Mac.

**Ask for a Closeout** (#3612, the `closeout` switch) is one more row after Wake on Time, with a sample closeout card under it, off by default and disabled while the master switch is off like the other actions. `TriageSwitches.closeout` is optional: a hub that does not name the switch gets no row, and a save leaves the key out, so that hub keeps its value. **Ask for the Status Afterwards** (#3616, `closeout_ask`) follows it with a three-step stopped → question → report example; it also requires only the master switch, not Ask for a Closeout, asks once for one Agent-model turn, and stays hidden and unencoded when the hub omits the field. The frozen Mac app shows neither row; its saves still send back the values the hub gave.

## Knowledge selection (Settings › Knowledge Selection, #2217)

Below Connectors, iPhone and iPad only: the same document the web edits in Settings → Knowledge selection (`GET / PUT /api/v1/kb/selection`, stored on the
hub; what each rule and per-item setting means is in `docs/assistant/knowledge.md` ("Selection")).

- **Layout**: the chats and documents with a setting of their own first (they come before every rule), then the chat rules and the document rules, one
  section per priority level. A rule's row shows On / Off or its days and opens its options in a sheet (store what matches, days for text and attachments,
  whether an @all counts for mentions, the messages kept around your own); a level's menu in its header moves the level up or down, merges a rule into the
  level above, or gives a rule of a shared level one of its own.
- **Per-item settings**: Add Chat… / Add Document… list what the knowledge base already has (`/api/v1/kb/chats`, `/api/v1/kb/docs`); picking one reads
  its detail for the source identity (`meta.source` with `meta.source_id` for a chat, `meta.id` for a document) and opens its setting: Don't Read, the
  rules (a chat: only mentions and your own messages), or Store Everything with optional days. Mail, issues and pull requests are refused (always stored),
  and so is an entry without a recorded source. Chats a connector has not stored anything of yet are added on the web (`/api/v1/kb/selection/candidates`).
- **Saving**: there is no Save for the page. A change shows at once, then the page reads the hub's document again, applies the change to that (edits name
  rules and items, never positions, so another device's change in between is kept) and puts the whole document; a failed save puts the old value back and
  says why at the top. A hub without the route (404) and a document with a version or field this app does not know (`KBSelection.read` compares what it
  would send back with what it received) both show a notice and nothing to edit. History import settings (`history`) are sent back as read.
- **Code**: `KBSelection` (`apple/Shared/KnowledgeSelection.swift`), `KnowledgeSelectionModel` and `KnowledgeSelectionView`
  (`apple/Persona/Views/Settings/`), `KnowledgeSelectionTests`. Screenshots: the mock hub answers the routes (`MOCK_KB_SELECTION=missing|future|fail` for
  an old hub, an unknown field and a refused save); `-tab settings -openKnowledgeSelection YES` opens the page, `rule`, `chat` or `doc` instead of `YES`
  with that sheet up. Demo Mode keeps the document in memory.

## Inbox rows

For people scanning what needs them, one matter keeps its title, time and AI summary. Its metadata line says where it is
happening, who wrote the newest line and what they wrote: `where · sender: preview` in English, with the localized colon
in Chinese and Japanese. A direct chat whose `where` equals its sender names that person once. Without `where` (an older
hub, or mail), the existing source-specific line is unchanged. Search includes `where` in addition to title, sender,
preview and summary; it still does not search context or triage reasons.

When the manager sends the optional `now` key, `InboxNow` resolves the shared `spec/fixtures/inbox/now.json` table and
compose rules in the interface language. On an open item, the complete sentence takes the existing mark line below the
metadata, in the mark's current font and color (failure red, other states secondary), rather than competing with the
metadata or its count badge for width. Waiting has a static clock, only queued / looking / Agent-working spins, and states
that need the owner use the existing Needs you mark (failure stays red); FYI and folded states do not spin. Done rows keep
today's absence of a mark; their done_* words belong to the detail. Waiting includes its named person, short check time
and next step when present; failure uses only `handling.error`. An unknown or missing key keeps the existing
`InboxEventText.mark` line and draft behavior, including its row height. The frozen native Mac row keeps its previous
metadata and mark. `InboxNowTests` checks every shared state and row case in en, zh-Hans and ja, optional-field decoding,
fallback and `where` search.

## Translation (Settings › Translation, 2026-10-31)

With Automatic translation enabled, content not in the target language (Simplified Chinese by default) translates automatically; you can switch back to the original anytime.

- Scope: **Email** (on by default); **Lark chats** default to none (too many messages), translating only chats selected in Settings (`TranslationChatPicker`: pinned + recent chats, searchable),
  and only others' messages. Preferences stay local (`@AppStorage` `translate.*`, `TranslationPrefs`).
- The phone decides whether to translate (NaturalLanguage `NLLanguageRecognizer`, first 1500 characters; fewer than 6 letters skips detection; Simplified / Traditional Chinese count as one language, indistinguishable in short text).
  Results are cached by text in `Translator`; detection does not run inside row bodies.
- Translation runs in the manager (assistant process, `server/internal/assistant/inbox_translate.go`, moved with the Inbox from the hub on 2026-11-04; API forwarded through the hub, 503 if the assistant is absent),
  using the Text and email translation model purpose (user decision: no strong model needed, currently gpt-6.1-terra, about 2 seconds for two sentences).
  Email translates by reference (`POST /api/v1/translate/mail {account_id, uid, mailbox, target}`; the manager asks the hub to read mail, converts HTML with `mail.ReadableText` to paragraph-preserving text,
  body at most 12000 characters, excess marked `clipped`); Lark messages translate in batches (`POST /api/v1/translate {texts≤30, target}`; the App batches rows appearing within 300 ms).
  The manager caches in memory by model and content (model changes retranslate), the App caches for this run.
- **Translate on mail arrival, not opening** (requested 2026-11-01): the App sends Settings › Translation with device preferences to the hub (`PUT /api/v1/devices/{id}/prefs {"translate":{enabled,target,mail}}`,
  on connection and preference changes, `Translator.syncPrefs`). After each mail collection, the manager translates unhandled mail whose script differs from the target in the background (`foreignScript` in `assistant/inbox_translate.go`:
  by writing system; for Chinese targets, under 20% Han characters or any kana counts as foreign; Latin-script languages cannot be distinguished, so French is not translated for an English target), storing it in Inbox-item
  `translations[target language]` (also in the database, survives restarts). The list directly shows translated titles and summaries (translation icon by the title); opening mail retrieves this first via `translate/mail`, immediately.
  Only runs if some device enabled Translation › Email; disabling stops new translations.
- UI: mail titles and bodies become translations, with "Translated to Simplified Chinese · Show original" above the body; Lark bubbles have a gray translation line below (`LarkTranslationLine`).
- Screenshots / tests: `mock_hub.py` mail 504 is English; the Design review (f2) chat has English messages; `/api/v1/translate` returns "【译】原文" ("[Translated] original text");
  UI tests `TranslationTests`; DEBUG `-openTranslationSettings YES` opens Settings.

## Interface language (internationalization, #2457)

iOS, Mac, visionOS, watch and extensions support Simplified Chinese, English and 日本語 (shared rules in `docs/protocol/i18n.md`).

- **Entries**: String Catalog, development language `en`, with English source text or contextual English keys (#5090). Every entry has explicit en / zh-Hans / ja values; Chinese UI, unit tests and pixel / layout baselines retain their `zh-Hans` wording.
  `apple/Shared/Localization/Localizable.xcstrings` is shared across all targets (App, Mac, visionOS and watch compile it through `Shared/`; widgets add it separately in `project.yml`);
  shortcut phrases live in `Persona/Intents/AppShortcuts.xcstrings`; permission explanations in each target's `InfoPlist.xcstrings` (one each for iOS, Mac, visionOS, watch because wording differs). Runtime displays catalog text, not `Info.plist`: update both `Info.plist` and all three catalog languages for permission changes; `check` rejects en mismatches with `Info.plist` and `*UsageDescription` keys absent from the catalog (#4790).
  XcodeGen generates `knownRegions` from catalog languages; do not edit pbxproj manually. `Info.plist` `CFBundleDevelopmentRegion` is `en`: other system languages fall back to English.
- **Code**: SwiftUI `Text("…")`, `Button("…")`, `Label`, `.navigationTitle` already take `LocalizedStringKey`; every other user-facing `String` uses `String(localized: "…")`,
  App Intents use `LocalizedStringResource`. Interpolated sentences are one English entry (`"\(n) items"`), never assembled fragments; English plurals get one / other in the catalog.
  Dates and numbers use `FormatStyle`, never hard-coded `zh_CN` (machine formats still `en_US_POSIX`). When identical English has different Chinese or Japanese meanings, retain distinct contextual keys and explicit en values, or use `String(localized: "action.close", defaultValue: "Close")`.
  Non-UI Chinese (hub / Agent prompts and messages, protocol values, matching hub or terminal text, logs, previews, DEBUG pages) gets `// i18n-ignore` at line end; sample-data-only files add `// i18n-ignore-file: <reason>` within the first 20 lines.
- **After adding wording**: build once with `xcodebuild … SWIFT_EMIT_LOC_STRINGS=YES`, then `python3 scripts/i18n/apple_l10n.py sync <DerivedData>` adds new entries to the catalog (Xcode IDE builds do it automatically; CLI builds do not),
  then fill en, zh-Hans and ja, including symbols, numbers and names. `python3 scripts/i18n/apple_l10n.py check` (run by `scripts/test/test_apple_l10n.py`) checks every translation state, argument number/type, English key, unignored Chinese literal and explicit source-key reference. It also checks that every `Text`, `Button`, `Label`, `.accessibilityLabel`… literal in `Packages/BeautifulUI` has a key in the app's `Localizable.xcstrings`: the library looks its text up in the main bundle, so a missing key shows English in every language (#5574). The library itself is not changed; its keys live in the app's catalog.
  `apple_l10n.py strict <DerivedData>` remains available for compiler extraction. In `Text(condition ? "A" : "B")` (and `.accessibilityValue(…)`, `["A", "B"][i]`) both literals are `LocalizedStringKey`s: SwiftUI's `StringProtocol` overloads are `@_disfavoredOverload`, so the compiler types the literal branches as keys. Its extraction (`.stringsdata`) lists both, e.g. BeautifulUI's `"Show fewer actions"` / `"Show more actions"` (`SelectionActions.swift:220`) and `"Collapsed"` (`StreamingText.swift:244`, `SidebarNav.swift:181`), so each branch needs a catalog key. A branch that is a `String` value (`c ? title : "A"`) makes the whole expression a `String`, which is not looked up. `apple_english_keys_verify.py <base>` verifies that an English-key migration retains the existing translation trees after argument renumbering.
- **Choosing**: Settings › Appearance › Language: Match System (default) / 简体中文 / English / 日本語 (`AppLanguage`, `LanguageStore`, `Shared/Localization/AppLanguage.swift`).
  An in-App setting rather than only system per-App language: docs/protocol/i18n.md requires a common setting; Match System must use docs/protocol/i18n.md rules (French → English, Traditional → Simplified Chinese), which Bundle matching cannot provide;
  watch and extensions lack their own entry points; Mac's system per-App language is deeply buried. Both are the same value: the App writes its result to `AppleLanguages` (also used by the system's per-App language),
  and system-settings changes become a new choice on the next App open.
- **Dictation language** (#5005): on-device recognition (`SFSpeechRecognizer`) hears only the one language it is set to, and the interface language
  is not always the one spoken (an English iPhone whose owner speaks Chinese heard English). `DictationLanguage` picks it at each tap on a mic, first match wins:
  the owner's choice (Settings › Appearance › Language › Dictation, Automatic by default, `DictationLanguageSection`); the conversation's language, from the draft and the
  owner's own recent messages in that chat (`ShellComposerBar.dictationHints`, `DictationLanguage.ownerTexts`: not another session's, a channel's, a meeting's or Persona's
  injected lines) when `NLLanguageRecognizer` is at least 80% sure; the last sure choice (`dictation.language.recent`, written only when it changes), so a new chat follows the last one;
  the interface language; then `zh-CN`. Within a language the region comes from the owner's preferred languages, then the phone's region, then the language's usual one;
  only locales the phone recognizes are picked. Recognizing through the realtime voice service instead (mixed Chinese and English, #2083) was weighed in #5005: it needs the
  network, a set-up voice extension and the composer's audio leaving the phone, and its fallback would still need this choice. `DictationLanguageTests`;
  `-tab settings -openAppearance language` opens the page.
- **When effective**: Foundation chooses language at process start; `String(localized:)` cannot change at runtime (changing `Bundle`'s class or `AppleLanguages` only affected SwiftUI `Text`, leaving mixed Chinese and English),
  so choices **take effect next time the App opens**, explained in Settings; Mac has "Reopen now". Launch argument `-AppleLanguages (ja)` overrides for this run only, without saving (screenshots, tests).
- **Watch, extensions**: watch follows iPhone, which sends `language` in WatchConnectivity application context; watch writes its own `AppleLanguages`; before receipt, uses its system language.
  Widgets and Live Activities read App Group `persona.language.resolved`: extension processes run for hours, so their text uses `interfaceText(_:)` (`LocalizedStringResource` with that language)
  and `.environment(\.locale, LanguageStore.drawingLocale)`; App language changes affect their next refresh, watch next launch.
- **Tell the hub**: every HTTP request and WebSocket handshake carries `Accept-Language: <code>`; after connection send `{"type":"client_hello","locale":…}` (locale only; features / heartbeat / platform unchanged),
  `POST /api/v1/devices` includes `locale`, using this launch's interface language (`LanguageStore.launched`).
- **Code comments are English** (#5055): `python3 scripts/i18n/apple_comments.py check` (run by `scripts/test/test_apple_comments.py`) checks that comments under apple/ contain no Chinese or Japanese outside 「」; those brackets are reserved for exact localized text matched by code. Document references use English section titles, such as `docs/apple/ios.md` ("Interface language").
- **Tests**: scheme test actions pin `language: zh-Hans`, `region: CN`; unit, UI and pixel / layout tests all run in Chinese; `AppLanguageTests` covers parsing and storage.
- **Not covered yet**: hub-pushed text (Live Activity titles, details and terminal-needs-attention messages, push bodies, assistant replies) is localized by the hub; demo data (chats, names) is Chinese only.

## Design language

- Structure (2026-10-31, design in bui-adopt ARCH-PLAN):
  - `Shared/Design/`: tokens, also compiled by watch. `Theme` (font levels/emphasis, spacing, radius, line height, motion, color roles), `Icons` (`Theme.Icon`, `Theme.Brand`),
    `Palette` (base colors, also widgets/Live Activities), `StyleSpec` (composed value), `ColorSpec` (theme colors, migration), one file per style under `Styles/`.
  - `Persona/DesignSystem/`: iOS-only recipes/components: `Styling` (surfaces/buttons/page backgrounds from `spec.surface` / `spec.buttons`), `Components`
    (Chip, CountBadge, EmptyStateView, Monogram, `Loadable` / `LoadableContent`…), `Notice` (`NoticeBanner`: short notice atop page content,
    tone-colored caution/danger icon (default tone icon or specific meaning, e.g. waiting clock) + secondary gray text + optional button
    (below on phone, right on Mac), pale tone surface like chat `AttentionNote`; not dismissible, disappears when no longer applicable,
    used for Inbox Awaiting triage banner, #1773), `TextSize`.
  - **Add style**: a `ComponentSpec` in `Shared/Design/Styles/` (surface rendering, buttons, font design, component kit, canvas source, strokes, Lark cards,
    its own colors), `AppStyle` case, matching `component`. **Add theme color**: `ColorSpec` (accent, bubbles/text, darkening,
    washes manually tuned or derived), `ThemeColor` case. Other code reads `StyleSpec.current`, no `switch AppStyle` / `ThemeColor`.
    `StyleSpecTests` combines every style/color (including yellow avatar) and checks light/dark contrast.
  - Source checks (PersonaTests): `TypeScaleTests` (no system font styles/hard-coded sizes, emphasis via `labelStrong` rather than `.weight(` on a level,
    vertical grid spacing), `IconVocabularyTests`, `TokenRatchetTests` (per-file hard-coded padding/horizontal spacing/radius/hex colors/animations/fixed dimensions may only decrease;
    after reduction delete `TokenBaseline.json` and rerun to record baseline), `StyleSpecTests`. Debug warns on type checking over 150ms (release machine times out).
- Settings › Appearance (requested 2026-11-01, like Connectors): home group Avatar / Theme Color / Interface Style / Light & Dark / Layout (Light & Dark iPhone/iPad only, Layout iPhone only; App icon merged into Avatar 2026-11-05, no separate row, #3203), current value right, each opens its own page (`AppearanceSettingsView.swift`).
- Appearance has three independent choices (requested 2026-10-31: separate avatar, color and UI components; formerly style controlled colors/components,
  avatar overwrote accent, Clay then a baby avatar lost purple, Claude coral with Island cards impossible):
  - **Avatar** (`PersonaPicker`, `docs/apple/persona.md`): name, face, top-bar state dot/animation, no color change.
  - **Theme color** (`ThemeColorPicker`, `ThemeColor`, `ui.color`, default Match Interface Style): accent (actions, links, selections, tab tint),
    own bubbles/text, Lark links/own bubbles, Clay/Island background washes. Choices Match Interface Style (original colors, pixel-identical before separation),
    Match Avatar (only with avatar palette), System Blue, Coral (former Claude style), Lavender, Teal, Lark Blue.
  - **Interface style** (`StylePicker`, `AppStyle`, `ui.style`, default Liquid Glass): overlays/cards/buttons/fonts/chat components (BeautifulUI), expanded work card/timeline (`workList`), canvas source.
  - Choices compose `StyleSpec` (`StyleSpec.compose`, `StyleSpec.current`), read by Theme/Styling; changing any rebuilds RootView via `AppModel.look`.
  - **Canvas/card ownership**: glass styles (Liquid Glass, BeautifulUI) use system pages/cards; Lark uses its own light gray/white cards, unaffected by theme color;
    Clay/Island originally used colored washes (lavender/misty teal), poorly coupled to components (purple canvas/honey buttons), now **theme-color washes**
    (`ComponentSpec.Canvas.wash` / `.islands`, `ColorSpec.Wash`): Clay pages/cards/soft-block colored shadows, Island pages with white cards (wash cards in dark mode).
    Lavender/teal retain hand-tuned values; others derive from accent (light pages 90% white, cards lighter; dark pages darker than system grouped cells #1C1C1E).
  - **Claude is no longer a style**: only color differed from Liquid Glass, now Coral theme color, no duplicate style cards.
  - **Migration** (`AppearanceChoice`, first settings read without `ui.color`): old style → same style + Match Interface Style; Claude → Liquid Glass + Coral;
    selected avatar (formerly tinting) → Match Avatar. Writes both keys. Three visible changes: old Clay/Island+avatar users get avatar-color washes instead of purple/teal; Lark+avatar links/bubbles get avatar color; old Claude expanded work follows Liquid Glass timeline instead of gray cards.
  - `ThemeColorSettingsPage` sample uses live tokens (own bubble, reply card/link, two buttons), color dots below;
    style preview cards render each style + current theme color (Match Interface Style uses each own colors).

  | Style | Surfaces / buttons | Canvas | Own colors |
  | --- | --- | --- | --- |
  | Liquid Glass | Liquid Glass | System | System blue `#007AFF`, pale blue bubble `#DCEAFF` black text (dark `#1D3557`) |
  | Clay | Solid soft blocks: top-left highlight + bottom-right colored shadow, white stroke, rounded font, radius 24, soft buttons | Theme-color wash | Lavender `#7A5AF8` (dark `#8979EB`), purple bubble `#6759D3` white text, pale purple canvas `#EFEAF7` (dark `#0A0510`) |
  | Island | Floating white cards + large soft shadow, radius 26, soft buttons | Theme-color wash, white cards | Teal `#27747F` (dark `#2A9798`), ink bubble white text (reversed dark), misty teal canvas `#E6F1EF` |
  | Lark | White cards + hairline stroke, no shadow, radius 10, bordered buttons | Light gray `#F5F6F7` (theme-independent) | Lark blue `#3370FF`, pale blue bubble `#D1E3FF` black text |
  | BeautifulUI | Liquid Glass with BeautifulUI components | System | System blue |

  Coral (former Claude): accent `#D4683F` (dark `#CD6E45`), deep coral bubbles `#B24C25` white text.

  Readability floor (fixed once 2026-10-30, figures in persona-p1 themes-fix report; every style×color now must pass): white text on solid buttons/selected capsules ≥3.5:1 (dark accents cannot be too bright),
  own bubble white text ≥4.5:1, accent links on pages ≥3:1 (tightest wash pairing Coral/Clay 3.2:1); colored bubble gradients darken only (`Theme.deepening`), never brighten tops. Clay/Island solid buttons
  (`SoftButtonStyle`) use current tint, at least 44pt high: `.tint(Theme.danger)` Interrupt/Close Agent stays red in every style.
  Background-drawing scroll pages use `.themedGroupedPage()`, List/Form `.themedPage()`, otherwise Clay/Island/Lark canvas jumps on navigation.
  `Theme` reads current composition (Settings previews use `StyleSpec.compose`); panels `.surface(tint:interactive:in:)`,
  buttons `.themedButton(prominent:)`, list pages `.themedPage()`, plain rows `.themedRow()`, never direct `glassEffect` / `.glass`.
  RootView rebuilds on style/color with `AppModel.look` id (static tokens), current tab/chat already remembered. Lark framework colors
  (`LarkColor.blue` links/selections/new-message line, own bubbles) follow theme (Lark Blue restores gateway colors); card surfaces/strokes follow style; named Lark card-content colors unchanged.
  Debug `-ui.style glass|clay|island|lark|beautiful`, `-ui.color style|persona|blue|coral|lavender|teal|lark` (UserDefaults argument domain, Release supported, not DEBUG switches;
  `-ui.style claude` still Liquid Glass + Coral).
- **Light & Dark** (2026-11-03, #710): Settings › Appearance › Light & Dark, Match System default / Light / Dark (`ColorSchemeChoice`, `ui.scheme`, iPhone/iPad only).
  - Window-level `overrideUserInterfaceStyle` (`ColorSchemeChoice.apply`, every window/scene); choices through `AppModel.colorScheme`, new windows (launch,
    extra iPad scene) RootView `onAppear`. Sheets/dialogs/keyboards/WebView `prefers-color-scheme` follow window.
    Not `preferredColorScheme`: resetting nil may leave an open sheet's appearance; window `.unspecified` truly returns to system.
  - No whole-tree rebuild (`look` unchanged): trait-based dynamic colors redraw with window traits; `@Environment(\.colorScheme)` views update too.
    Explicit appearances unaffected (image editor/message menu dark; share cards/terminal own settings).
  - System surfaces still follow system: Dynamic Island/Lock Screen Live Activities, widgets/watch/notifications/launch screen (`LaunchBackground`).
  - App icons follow (requested 2026-11-03: light image for Light, dark for Dark): system chooses home-screen variant, App cannot,
    so Paper (Mochi/main icon) gets alternate sets `AppIcon-PaperLight` (both appearances light image), `AppIcon-PaperDark` (both dark, tinted unchanged),
    generated by `make_alt_icons.py` from `AppIcon`. `AppIconModel.follow` selects corresponding set for Light/Dark,
    Match System restores both-variant set (Paper main icon); avatar changes also respect choice (`AppIconChoice.iconName(for:)`).
    每次切换仍会触发系统的「图标已更改」提示。团子和奶蛙都使用默认图标，仅保留 Paper 的浅色、深色固定变体。
    `AppIconModel` remembers latest appearance: if changed during pending switch/system alert, rechecks and switches again to newest,
    never remains wrong (review of #714); `follow` also on every foreground (launch-argument appearance/interrupted switch),
    matched state does nothing, no alert. 系统仍记着已删除的备用图标时，主动切回默认图标；成功后不重复切换或提示。
    `AppGlyph` uses actually displayed home variant: Paper selected Light/Dark, other/Match System screen trait
    rather than window trait overridden by App. Debug `set-appearance` uses persisted `AppModel.colorScheme` too.
    `ColorSchemeIconTests` (demo) reads `probe.scheme` for actual page appearance, tearDown restores Match System after failure;
    iOS 26.0.1/26.3.1 simulators lack alternate icons (`supportsAlternateIcons` false), so verify home-screen switching on devices.
  - First verify Liquid Glass (lists, glass surfaces/buttons, sheets); other styles already adapt to system dark with same mechanism. Mac/Android lack option:
    Mac `NSApp.appearance` alone leaves List rows light (`MacSnapshot.colorScheme` comments), each window also needs `preferredColorScheme`, separate work.
  - Debug `-ui.scheme light|dark|system` (UserDefaults argument domain like `-ui.style`), `-tab settings -openAppearance scheme`.
- **Glow While Replying** (requested 2026-11-09): a switch in Settings › Appearance, off by default (`ComposerGlowPrefs`, `ui.composerGlow`, iOS only).
  When on, a band of color travels around the assistant's composer while its reply is being written (`SessionStatus.working`; not
  `blocked`, not Agent chats) and fades out when the reply ends. Drawn by the MarqueeGlow package (`apple/project.yml`, pinned
  `exactVersion`, `ComposerGlow.swift`): the halo is blurred once into a mask and only a gradient rotates per frame, so the chat scrolls
  at full frame rate under it. Colors `Theme.Shell.composerGlow`, width `Theme.Size.rule`, shape `Theme.Shell.composerRadius`.
  Reduce Motion keeps it still. Mac and Vision compile the same sources without the package (`#if os(iOS) && canImport`), so the switch is hidden there.
  The glow is applied inside `ShellComposerBar` (`glows`), before its animations, so it follows the surface when the line grows into the card.
- **Return Key Sends** (#8178): a switch in Settings › Appearance & Display, beside Glow While Replying, off by default (`ReturnSends`,
  `ui.returnSends`, iOS only: the Mac always sends on Return). Off, Return is a new line and the composer is unchanged. On, in the main
  conversation and Agent conversations (`ShellComposerBar`, the iOS `ComposerRow`, BeautifulUI's `PromptComposer`): the software
  keyboard's Return key reads "send" (`submitLabel`) and sends; a hardware keyboard's Return sends and Shift-Return (or Option-Return) is a
  new line; ⌘Return still sends. Return does what the send button's place does: send, the gate's action when a `SendGate` stands there (as
  the web's Enter nudges a blocked send), or nothing with nothing to send, never a blank line. While the keyboard is composing (marked
  text) Return confirms the composition and sends nothing (#2203). The full-page editor (Expand) keeps Return a new line, and BeautifulUI's
  bar leaves Return alone while its @ / / list or a menu is open. The fields are SwiftUI's vertical `TextField`, which never submits on
  Return, so the software key is read off the draft (`ReturnSends.breakSends`: an edit that only adds one line break, while the field holds no marked text), the line break taken
  back out of the text view as well as the draft before the send confirms what the field shows; a hardware Return is caught by
  `onKeyPress` first, with `GCKeyboard`'s Shift and Option (either side, `ReturnSends.lineBreakKeyHeld`) as the fallback when the press does not reach the field. A paste that brings a line break
  with other text never sends. The web has no setting (Enter sends with a precise pointer, `enterSends`) and Android none, so there is no
  shared key. `PersonaTests/ReturnSendsTests`; UI test `ReturnSendsUITests` (Return sends with the switch on, a new line with it off);
  `-ui.returnSends YES` turns it on for screenshots.
  Debug `-ui.composerGlow YES`; on the mock, `GET /mock/session_state?mid=server&sid=<assistant>&state=working` holds a reply open.
- State colors never follow style; one `Shared/StatusPalette.swift` shared by App/watch/Live Activities: Needs you blue (on every client, `docs/design/DECISIONS.md`), Working amber, Unread green, Seen gray, Not running pale gray, like herdr (`docs/protocol/agent-states.md`). Three values each: `dot` (dots/icons/fills), `text` (same hue darkened, small text on white ≥4.5:1), `tint` (pale background), via `AgentState.tone`. Questions/plans/permissions/terminal dialogs all Needs you blue; working spinner amber, accompanying text secondary gray. Non-Agent states use their own `Theme` tokens: `Theme.Diff` (diff/Git ±, counts, fills, body black), `caution` (connecting/stale/truncated/version mismatch, triangle not dot), `ai` (✦ AI summary), `loop` (loops/waiting), `positive` (success/connected/online/completed to-dos), `Hue` (categories such as memory types). Permission labels neutral except Allow everything red.
- Agent row (`SlotRow`): state dot + title + time; latest sentence; metadata "Needs you · Claude · Space › Tab · Machine", regular secondary gray. Only Needs you written in its blue; others dots only (Unread also bold title); former colored tab pill/state text/outlined Agent kind crowded first line and repeated status three times. Space counts colored dots + gray numbers.
- Tokens (`Theme.swift`, pages reference only): `Theme.Radius` badge 6/control 10/card 14/bubble 20/panel 22 plus capsule/circle;
  `Theme.Fill` well/chip/pressed (system quaternary/tertiary/secondary fill, visible dark too, no `Color.primary.opacity(…)`), plus drawer `scrim`;
  - `Theme.Font` levels at default size: display 28 bold, title 22, heading 17 bold, body 17, secondary 15, caption 13, sectionHeader 13 bold secondary,
    label 12, code 13 monospace; pages no direct `.subheadline` or `.system(size:)`.
    Mac same levels, Mac scale (#779, user selected body 13 on 2026-11-03): display 24 bold/title 18 semibold/heading 13 bold/body 13/secondary 12/caption 11/sectionHeader 11 bold/label 11/code 11 monospace,
    system fonts multiply `Theme.Font.scale` (`#if os(macOS)` in `Theme.Font`; Mac styles do not follow Dynamic Type, Small/Standard/Large body 12/13/14, one point each, #833; floor 11 so small 11pt levels remain 11, `Theme.Font.pointSize`; iPhone scale always 1). `Theme.Level.size` follows both, Markdown answers/profiles base `Theme.Level.body.size`, no fixed size.
  - Like BeautifulUI, hierarchy mainly weight/color: row titles bold body, section headers bold secondary caption smaller than row titles;
    Markdown H1 title, others heading separated by top whitespace; blocks code.
  - `Theme.Space` 4pt grid (2 4 6 8 12 16 24 32), all vertical `VStack(spacing:)`; horizontal beside button labels not globally changed.
    Page sides `Theme.Space.page` 16, aligned search/filters/group titles/cards/capsules, not individual paddings.
  - `Theme.Size` (requested 2026-10-31): named text-adjacent icons/dots/avatars/tap targets/thumbnails/label widths, default original points,
    `.scaledFrame(width:height:)` / `.scaledFrame(minWidth:…)` follows text up to 1.6× accessibility (44pt buttons not a third of screen).
    `.tapTargetFrame(alignment:)`: a hit area that grows like `scaledFrame` but never under `tapTarget` (44pt) at Small / smaller system sizes, for small controls hit wider than drawn (uploaded avatar's corner delete).
    Lines/dividers/progress ≤4pt stay unscaled `frame`; geometric sizes unchanged; watch keeps own sizes, not App text sizing.
  - Dynamic Type + Settings › Appearance › Interface Style › Text Size (Small/Standard/Large, `TextSize`, `ui.textSize`) offsets system one level via RootView `.dynamicTypeSize`,
    changing text styles, `@ScaledMetric`, MarkdownUI and BeautifulUI together, ratios intact.
  - Lark messages/cards retain own sizes mapped to levels, similarly scaled by `LarkTextBase.scale` (`Theme.textScale`).
  - Fixed-frame icons cap `dynamicTypeSize(...)`; AI Markdown baseline 17pt (MarkdownUI scales).
  - Exceptions: user-sized terminal grid, Dynamic Island grid, fixed-image share cards, small-circle numbers/badges.
  - Monospace only character comparison/copying: code/commands/paths/branches/hashes/IDs/versions/terminal/tool output. Chinese explanations/values system font
    (session info chooses by row: model/branch/directory/version monospace, others system + monospaced digits; memory editor/Knowledge recent messages system).
  - `Theme.Icon` (`Shared/Design/Icons.swift`): one meaning per SF Symbol, pages `Theme.Icon.copy`, not `"doc.on.doc"`; drawn with
    `ThemedIcon` / `IconTile` so each style's icon language applies (`IconSpec`; rules, exceptions and checks in `docs/apple/icons.md`).
    Symbols used in two files must register; errors triangles (`error` / `errorFilled`), octagon only critical machine alerts (`critical`); Copy `doc.on.doc`,
    Paste `doc.on.clipboard`, Interrupt `stop.fill`, model/launcher `cube` (`cpu` only processor), code files `curlybraces`, `</>` only Agents.
    `IconVocabularyTests` rejects direct registered names, shared unregistered symbols and retired error symbols.
    External names (Lark card icon tokens) retain Lark rendering; line ends `// icon: <reason>`.
  - `Theme.Brand` (Assets Brands, sources/terms `THIRD_PARTY_NOTICES.md`): Claude/OpenAI (Codex)/Lark,
    Gmail/Google G/Linux distributions. Official originals, no recoloring/redrawing except monochrome distro templates tinted distro color. Used in share avatars
    (Claude/Codex, internal Codex fork has no public logo, green terminal remains), Inbox source/watch Lark badges, Gmail/Lark Mail accounts,
    Sign in with Google (Google spec: light white/gray border, dark #131314, not accent), machine `OSBadge`.
  - Connector/mail-provider/iPhone-source marks through `BrandMark` (`apple/Persona/DesignSystem/BrandMark.swift`, #1003),
    no direct `Image(…).resizable()`: fixed `scaledFrame`, padding-free assets (Lark viewBox cropped to bird),
    optical compensation `Theme.Brand.optical` (circles/wide marks fill, full squares 0.88), equal apparent size/center in columns;
    iPhone icons use `AppIconShape` radius + hairline, outlining white Calendar/Health on white cards.
    BeautifulUI uses `BrandTile` white background matching `BUIIconTile`; circular source avatar badges `BrandBadge`
    (circle logos fill, others `Theme.Brand.inCircle`). Boxes: Settings `icon`, detail header `wellM`, email account `wellXS`,
    source `wellS`, sign-in `iconXS`; new marks measure opaque bounds after `rsvg-convert` and crop viewBox whitespace.
  Lock Screen Live Activities capped at `.large` (~160pt height, clips beyond).
- Motion (`Theme.Motion`): one meaning, one motion. A page picks the row for what is happening; a new meaning gets a token and a row
  here first, never an inline `.easeInOut(duration:)` (`TokenRatchetTests` counts those). Details and why: `docs/lessons/ios.md`, "Motion rules (#2943)".

  | What happens | Motion |
  | --- | --- |
  | A value or state changes in place (toggle, tone, selection, a menu or card opening) | `state` / `standard`: `.snappy` 0.25 |
  | Things take new places (rows in, out or reordered) | `layout`: `.smooth` 0.3 |
  | A block grows in place and the eye follows its rows | `expand`: `.smooth` 0.4 |
  | Something opens in the chat, or closes | Opens: `.unfolding`, `reveal` easeOut 0.28, a fade over all the space it takes; closes: at once, its space with it (#3005) |
  | A number changes | `.contentTransition(.numericText)` |
  | The finger lets go of a drawer / of something it dragged aside | `drawer(velocity:)` (no bounce) / `springBack` |
  | Moving with the keyboard | `keyboard` (the system's spring) |
  | An alert follows another (queued "already exists" questions) | `alertHandoff`: a 1 s wait before it asks; UIKit drops a presentation made while the one before is closing (#6113) |
  | The composer and the 「+」 panel | their `composer…` / `hold…` / `attach…` tokens, measured from Telegram |
  | The meeting recording's level meter (`RecordingLevelMeter`) | `recordingLevelFrame`: a 1/30 s clock; fixed-height bars scaled vertically (`Theme.Size.recordingMeter…`); one still frame with Reduce Motion, Low Power Mode or off screen |
  | A timeline row arrives in the live recording sheet | `reveal` with a small rise (`Theme.Recording.arrivalOffset`); opacity only with Reduce Motion |
  | The recording dot | `holdBlink`, steady with Reduce Motion (a pause glyph when paused: the state is never the color alone) |

  Red lines:
  - Leaving is quicker than arriving (`conceal` < `reveal`, `holdLeave` < `holdAppear`, `attachUnpick` < `attachPick`), and a
    panel, card or row leaving fades and moves but does not shrink. Only a glyph swapped inside one control shrinks (`composerSwapScale`);
    `composerAccessory` fading in quicker than out is Telegram's measured exception.
  - Under the finger nothing animates: a drag follows it frame for frame, and the animation starts at release with the finger's speed.
  - Only a few things move on their own, each listed with its reason in `apple/PersonaTests/ContinuousMotionTests.swift`: the Needs you
    dot's breath, the persona while it works, typing dots, loading skeletons, the voice orb in a call, the recording dot, the watch's frog.
    A loop changes only transform, opacity or color, never a frame, padding or font (that relays out every frame); it stops off screen,
    in the background and with the wrist down; the persona and the voice orb also hold still in Low Power Mode.
  - Reduce Motion (`accessibilityReduceMotion`, `Theme.Motion.reduced` outside a view): loops show one still frame (the working dot its
    static ring); changes land at once or cross-fade, without sliding or scaling. The registry test fails for a loop whose file, or its
    caller, never reads it.
  - No wobbling icons, spinning gears, pulsing avatars or spinning to-dos. Counts `.numericText`; no `.shadow` outside `.surface` (glass/Clay/Island already shadow,
  doubling otherwise); neighboring glass keys/top Back/⋯ use `GlassEffectContainer`.
  - An extension's View drawn again (#37, as on the web and Android; `Views/Settings/ExtensionMotion.swift`): under the same key, a `metric`, `stat_grid`
    or row value whose numbers alone changed rolls (`.numericText`, counting down when the number fell); any other change of a value or a badge's words fades in,
    and a badge's tone changes color (`Theme.Motion.state`). Blocks whose key is new fade in, gone ones fade out, and chart bars and lines move to the new values
    (`Theme.Motion.layout`). Nothing moves on the first draw or with Reduce Motion; at rest every value is exactly the extension's text. Mock: `MOCK_EXTENSIONS=1`,
    `-openExtension showcase -openExtensionPage motion`; `GET /mock/motion/next` draws its other step (`ExtensionMotionUITests`).
- Voice orb (`DesignSystem/VoiceOrb.swift`, #3506, voice design #3489): centered call-bar ball; pages pass only phase (`VoiceOrb.Phase`: not started/connecting/listening/hearing you/speaking/listening with Agent working/paused/muted/ended) and volume 0…1, no style decisions.
  - Motion: listening slow breath (`Theme.Motion.voiceBreath`, 1.6 s one-way), speaking audio beat (`voiceBeat`, 0.42 s one-way), matches Android `Voice.kt` (`VoiceOrbTests` reads/checks); hearing scales by volume, connecting/Agent working arcs (`voiceSpin` / `voiceLap`). `TimelineView` clock ≤30fps (`voiceFrame`, like Needs you dots/avatar glow), static phases paused; Reduce Motion or Low Power one static frame per phase (like avatar dot), Reduce Motion switches phase without scaling. One of few continuous animations, only during calls.
  - Rendering: Liquid Glass/solid styles highlight sphere, pale avatar accent (`Theme.persona`) listening, accent speaking; BeautifulUI flat disc + five volume bars like dictation, filled accent speaking, white bars (near-black on pale/yellow accent). Connecting arc accent, working arc amber `Theme.busy`; paused/muted/ended gray, `pause` / `voiceMute` icons on nonhighlight gray with white contrast ≥3:1 (`VoiceOrbTests` light/dark).
  - Size `Theme.Size.voiceOrb` 52 including glow/enlargement, sphere 0.7, text-scaled; everything inside frame, glow never enlarges with ball, gradient sphere edge to frame edge, not covering neighboring text/buttons. `voiceLevelBar` / `voiceLevelGap` / `voiceLevelHeight` scale proportionally; `Shared/Design/VoiceTokens.swift` also watch-compiled, no watch voice UI.
  - Speaking with `interrupt` is a button labeled Interrupt voice: stops speech only, not Agent (separate Stop Claude call-bar button); otherwise decorative, `disabled`/no keyboard activation, VoiceOver skipped, status in adjacent text.
  - Call icons `voiceMute` / `voiceUnmute` (same crossed mic, filled unmute, Phone-like toggle), `voiceEnd` (`Theme.danger`), `voiceCaptions` full captions, `voiceTimbre` voice; entry `voiceChannel`.
  - Mac continuous clocks relayout whole window each frame (`docs/lessons/macos.md`); no Mac voice yet, future animation Core Animation.
- One badge/count recipe (`Components.swift`): noninteractive `Chip` neutral (gray/pale: model/version/tab/direct chat),
  tinted (meaning + pale fill: `.tone` state, @me red, Recommended AI purple), outlined (Claude/Codex kind); `CountBadge` in the web's three looks (#4538):
  `.needs` (Needs you's text on its tint) for what waits for the user, `.neutral` (white on gray `Theme.countBadge`, the default) for a passive total,
  `.accent` (white on accent) for a mention or the selected item. Nothing at 0, 99+ over 99 (`CountBadge.text`, also for the tab and pane badges that are
  strings), VoiceOver reads the exact number or the host's `label`. Interactive filters/tabs/modes/sub-Agents are controls, not these.
- BeautifulUI (`apple/Packages/BeautifulUI`, vendored copy, source/update in `VENDORED.md`, no local library edits):
  **BeautifulUI style only** (`AppStyle.usesBeautifulUI`, Liquid Glass surfaces/theme colors), other styles retain Persona thinking/tool/work/input UI.
  Direct replacement 2026-10-30 (#93/#94) judged ugly by user, reverted and made this theme. In it:
  - `ThinkingState` (✦ + first line, expanded body right of guide).
  - Runs of ≥2 tools in expanded work group into chip row (`ChatLine.toolRun`, `ChatView.runLines`, `ToolRunRow`).
    Per-tool chip: one names target (Read WeatherStore.swift), multiple count (Run 6 commands), edits +N −N from shared `ToolDiffMemo`, failures red.
    Tap expands individual `ToolChipRow`, then tap params/output; single call direct row.
    Background calls stay separate so Background · Done / Failed remains visible (`ChatView.foldsIntoRun`).
    Jumping from background notice expands containing run first.
  - Diff/commands/output/Markdown code stay Persona `DiffView` / `CodeBlock`.
  - `BUITheme.persona(spec)` (`BeautifulUIBridge.swift`) derives colors/radius/type from current `StyleSpec`, injected RootView:
    - Tertiary text also secondaryLabel (tertiaryLabel only 1.7:1 on cards);
    - Code keywords `Theme.Hue.blue`;
    - Fonts ×1.2 (library desktop sizes), code ×1;
    - Entry animations off.
- Haptics: successful send/answer success, new interaction warning, error error.
- Icon: Paper (#709, chosen over several proposals 2026-11-03, replacing night/coral halo/four-point star): hand-drawn little white waving figure,
  heavy black outline/coral blush, Manus-like handmade icon, warm cream background (darker than website paper, clearer edge on light home screens); dark same figure on warm charcoal,
  tinted grayscale black. Same iPhone/watch/Mac image. Generated and selected, not script-drawn: `AppIcon.appiconset` `icon.png` light / `icon-dark.png` dark
  are source assets; `scripts/assets/make_icon.py` derives `icon-tinted.png` (grayscale, outer background pure black), watch light icon (system circular crop),
  Mac sizes and a temporary icon preview; replace both sources and run it plus `make_alt_icons.py` for Settings previews.
- **形象与 App 图标**（#6220；iPhone / iPad 可切换图标，手表和 Mac 保持一个图标）：
  - 只保留团子（`persona`，`AvatarDesign.mochi`，默认）和奶蛙（`nwverse`，hub 自带形象包）。`PersonaSelection` 在读取选择时把旧形象和未知 ID 归一为团子；`PersonaStore` 对本地缓存和 hub 列表使用同一白名单。
  - 团子和奶蛙都使用默认 Paper 图标。面罩、汇流、像素、晶面、软糖的绘制、备用图标和预览资源已删除；未上架的 Spark、Halo 绘制也不保留。
  - 旧偏好不因显示回落而删除，供回装旧版使用。原生迁移由 #6152 独立处理；迁移后的旧形象 ID 仍须经过这层白名单再显示，不能绕过回落策略。
  - `scripts/assets/make_alt_icons.py` 只生成 `AppIcon-PaperLight` / `AppIcon-PaperDark` 和 `AppIcons/app-icon-default` 预览，不再编译形象渲染器。
  - `ASSETCATALOG_COMPILER_ALTERNATE_APPICON_NAMES` 仅列出 Paper 的两个固定外观变体；`AppIconChoiceTests` 检查图标资源与构建配置一致。
  - `AppIconModel` 读取系统的 `alternateIconName`。已删除的备用图标会通过系统接口主动重置，失败时保留错误、下次前台重试；成功后不重复提示。
  - `PersonaSelectionTests` 在 iOS、Mac 验证旧选择回落和默认图标同步；`PersonaPickerTests` 验证两项选择、奶蛙下载、切回团子和旧选择启动后的回落。`persona-icon-note` 为当前图标的无障碍值，系统不能切换时为 `unavailable`。
- Settings top/welcome-name/sidebar-main marks use current avatar App icon (`AppGlyph`, home-screen light/dark via `AppIconChoice.homeScreenLook`, avatar packages own face), changes with avatar (#709; before #1312 always Paper, before #3203 followed separate App icon setting/sidebar hard-coded Paper).
- Brand coral (`Theme.brand`) only Persona marks: chat-home icon, Lock Screen four-point star. Actions use current `Theme.accent`.
  Both AccentColor assets (watch titles/tint, system dialogs) system blue; iPhone/iPad window `tintColor` follows theme (`WindowTint`, UIKit List icons, #1188); Live Activities cannot read style, stroke/buttons fixed system blue, button deep `#0071E3`, white contrast 4.7:1.

## Data flow

- Logic-layer choice (2026-10-31): pages with "load → display → edit → save" use MVVM: an `@Observable` `XxxModel` beside the page,
  with hub access, errors, toasts and navigation passed through `Deps` (`live` connects to `AppModel`); the view keeps only its own state (what is being edited, what is presented). The first is
  `AssistantRoutinesModel`. In event-heavy areas with ordering problems (chat pages, Lark chat pagination), extract pure calculations first: `ChatLines` (messages → rows),
  `AgentListOrder` (Agents list sorting and grouping), then convert to a reducer.
- Unit tests never connect to the hub: replace `APIClient.transport` with `FakeHub` (PersonaTests), which returns fixed JSON by "method path" and records requests; view models use real requests and decoding.
  Demo mode still uses the global `APIClient.local`.
- `AppModel` (`Persona/App`): pairing stored in Keychain; `LiveConnection` (WebSocket, exponential-backoff reconnect, resubscribing with `since_seq` after reconnect);
  one `ChatStore` per session (deduplicated by id, sorted by seq, `buildRows` produces display rows).
- Entering a chat page calls `subscribe`, leaving calls `unsubscribe`; the hub pushes `items` / `screen` / `sessions` / `interaction`.
- Phone tools: hub pushes `tool_call` → `PhoneToolRegistry.call` (writes require confirmation first) → `tool_result`. On connection, `register_phone` declares tools (`docs/apple/phone-tools.md`).
- Push: after pairing, request notification permission and register APNs, `POST /api/v1/devices`; when the hub has `apns.json`, it pushes new interactions and Agent completion (completion waits 1 minute, is skipped if seen on a computer or in the App, and waits for background tasks to finish; push policy in `docs/server/server.md`), with "Title" (no ✅; interrupted or failed is "⚠️ Title"),
  subtitle "Machine · Space › Tab", and the last reply as the body. No banner when the App is foregrounded and showing that session. Simulator automation (`-pairURL`) neither requests notification permission nor registers as a phone.
  Notifications allowed in Settings after a denial take effect when the App comes back to the front (`PushRegistrar.foreground`, #1861): it registers then, without a relaunch, and never shows the prompt itself; it waits for a launch or pairing request still open, so one answer registers once.
  A registration the hub no longer has is made again (#9612): a write for this device (`PUT /api/v1/devices/{id}/prefs`, `POST /api/v1/devices/{id}/live-activity`) answered 409 `push_registration_required` (the hub retired it, its credential revoked, #9611) registers once with that group's hub and then tries the write once more (`APIClient.request`, `PushRegistrar.registrationRequired`); another 409 within 10 minutes starts none. Coming back to the front also asks each hub, at most once an hour, whether `GET /api/v1/auth/tokens` lists a push route for this credential, and registers with one that lists none. A registration that did not reach a hub (the POST failed, the pairings could not be read yet, APNs gave no token) is tried again when the App comes back or, while it stays open, when due: 30 s, doubling to at most an hour, one registration in flight at a time.
  When the current hub confirms that this credential has no route, `PushHealth.missingRoutes` remembers the outage without confusing it with an older hub that supplies no route data. Persona's new and existing main conversations then show `PushHealthNotice`, a `NoticeBanner` over the composer in everyday language with one action: denied permission opens the system's notification settings; other confirmed outages open Settings › Notifications, whose detailed diagnostics and repair stay unchanged. Unknown or unchecked hubs, available routes and `lastFailed` do not show it. Returning to the front after registration also refreshes the cached system permission, so turning notifications off cannot leave the notice stale.
  Interaction pushes carry buttons (`NotificationActions`, category is the hub's `INTERACTION_<KIND>`): permissions "Allow / Deny", plans "Approve / Request changes" (changes can include typed feedback). Completion category `SESSION_DONE` supports direct "Reply" (#3449): unlock first, then send a normal message to that session in the background (new `client_msg_id`); failure produces a local notification explaining why. When a session appears on screen (`AppModel.show`), its old notifications (same `thread-id` and `machine_id`) are removed from Notification Center.
  Affirmative buttons require unlocking (`.authenticationRequired`); Deny does not. Plan approval from a notification uses `acceptEdits` (the whole plan has not been viewed, so commands still need individual approval),
  and "Approve and run automatically" is App-only. Buttons call resolve directly in the background without opening the App; failure (already handled, cannot reach hub) produces a local notification explaining why. Questions and terminal interactions have no buttons; tap to handle them.
  MCP forms and links are pushed too (#2433, `docs/protocol/protocol.md` "MCP elicitation"): the iOS app alone registers `features: ["withdraw", "elicitation"]` (`PushRegistrar.features`, the rule of `LiveConnection.helloFeatures`), so the hub pushes it these cards and waits for it while no app that shows them is open. Category `INTERACTION_ELICITATION` has no buttons: a tap opens the conversation, where the card arrives over the socket; the hub's withdrawal push takes it back like any card's. The alert names the MCP server and never carries the link or a form's values.
  What the hub gives for taking a to-do's recommendation with one tap (#2056, `docs/assistant/todos.md` "Waiting for the owner"; the App's rows are in "Closeout, orphaned tasks and answerable decisions (#3612, #3618, #3804)"): each to-do waiting for the owner has `ask: {outcome?, verdict, recommend?, asked_at}` in `GET /api/v1/todos` (a `{"type":"todos"}` event when it comes up, is taken or ends), and a decision with a recommended option is pushed with category `TODO_DECIDE`, whose Take Recommendation action takes it, and a top-level `todo: {id, asked_at, recommend, manager}`; from a manager that says how the push may answer it, `todo` also has `ask` and `lockscreen`, with `options` and `aps.mutable-content: 1` when `lockscreen` is `answer`, while `open_only` sends category `TODO_DECIDE_OPEN`, with no action (#10465); both are taken with `POST /api/v1/todos/{id}/accept {"asked_at"}` (409: already handled, refresh; 423 with a `code` such as `dialog_open`: the Agent's node refused it and nothing was taken, #7343; 502: the recommendation did not reach the Agent, nothing taken). The watch's answer-only credential cannot call either route yet.
  Settings › Notifications (`NotificationSettingsView`): "When you are needed" toggle, "When an Agent finishes" never / long tasks only / every time, Live Activities (Lock Screen and Dynamic Island / show details on Lock Screen / what each side of Dynamic Island shows, `IslandSettingsView`, `docs/apple/live-activity.md`), stored per device on the hub (`PushRegistrar.deviceID`). Changes save one at a time in the order they were made, each PUT the hub's last answer plus that one change, so a failed one reverts alone and no later PUT carries it (`NotificationSettingsModel.change`, #1913; the web's queue, #1285); quiet hours keep their own endpoint.
  Tapping a notification opens its session: use the completion-handler version of `UNUserNotificationCenterDelegate`, switch to the main queue to handle it, then call back (the async version returns on a background executor,
  UIKit receives completion off the main thread and a notification-triggered cold launch crashes).
- Watch pairing: iPhone sends pairing again after WCSession activation (sends before activation are lost), also using transferUserInfo for guaranteed delivery; the watch's unpaired page offers "Sync from iPhone"
  (sendMessage request, iPhone replies with pairing); iPhone Settings has "Sync to Apple Watch".
- Watch: iPhone uses WatchConnectivity to sync pairing and a summary (running sessions, pending interactions, Inbox items needing you); while open, the watch directly requests a hub refresh every 8 seconds,
  and answers / sends messages / interrupts directly through the hub too.
  UI hierarchy: each row's main content (question, session name, Inbox title) uses headline; type and location use footnote; messages and plans strip Markdown markers (`WatchText.shown`),
  bubbles fit text width, own messages align right; before any data arrives show "Loading"; when the hub is unreachable, a top row says "Can't reach the hub" + "Retry" (with old data, say how old it is).

## Demo mode (see every feature without a hub, 2026-10-30)

TestFlight has a public link and must pass Beta App Review: previously an unpaired App showed only the pairing page, leaving reviewers and friends who had not set up a hub unable to see anything (potential rejection under 2.1).
The pairing page's second button, "No hub? Try it first", enters demo mode: the App runs on built-in fake data and connects to no server (available in Release, not a DEBUG switch).

- **Below the data layer, no separate pages**: demo pairing uses `ServerConfig.demo` (`https://demo.invalid`, which never resolves, so even missed requests cannot reach the network).
  `APIClient.request` (and `image` / `fullItem`) routes it to `APIClient.local` instead of URLSession; `LiveConnection` opens no WebSocket for it,
  using a local socket made by `LiveConnection.makeLocal`. Both are registered and answered by `DemoHub` (`Persona/App/DemoHub.swift`), returning the same JSON as the hub,
  so pages, `ChatStore` and `AppModel.handle` do not know they are in demo mode. Missing endpoints return 404 "This data is not available in demo mode", with a `DemoHub miss` log in DEBUG.
- **Data**: `scripts/mock/make_demo_data.py` exports `apple/Persona/Resources/Demo.json` from `scripts/mock/mock_hub.py` (the mock hub for UI tests), then adds several Agents
  (covering every state and several spaces), Lark chat messages, email bodies and knowledge entries; the terminal is a Claude screen laid out for phone width (`phone_screen`; 96-column frames wrap every line on a phone).
  Everything is fictional; times are relative (`@rel:<seconds>`, `@unix:<seconds>`) and converted near "now" on load, keeping the demo fresh. Machines-page charts are calculated by `DemoHub` (a port of `mock_hub.metric_at`).
  When mock data changes, rerun the script and commit the JSON; `DemoHubTests` decodes every endpoint through `APIClient` into App models, catching model shape changes here.
- **Side effects**: messages to Agents / Persona echo locally, then after 1.5 seconds the "Agent" replies "This is demo mode…"; answering a question / approving a plan removes the card, the Agent keeps "working" and replies with the same text,
  with an extra "Nothing is actually sent in demo mode" notice; new Persona chats are created locally. Other actions that touch computers, Lark or email (new sessions, interrupt, close, terminal keys, sending email, run now…)
  fail with `APIError(code: "demo")`; `AppModel.show` displays a blue "Nothing is actually sent in demo mode" notice rather than a red error, and pages do not show success messages such as "Session closed".
  Local-state-only actions (seen, pins, mute, scheduled-task toggles, memory, handled Inbox items) work normally and are discarded on exit.
- **Indicator and exit**: the Settings tab has a "Demo" badge; its top says "Demo mode · All data is fictional", followed by "Connect my hub" / "Exit demo mode", hiding the server and watch sections.
  Exiting returns to pairing; successful pairing (QR scan, manual, `persona://pair` link) leaves demo mode automatically. Demo mode is stored in `demo.on` and survives App restarts, always resetting to shipped data on entry.
  Entering / leaving clears demo-session drafts and chat caches (demo session ids are the mock ids and can collide with drafts left by other UI tests on the simulator), and the Inbox; the Persona tab opens the demo main chat.
- Demo mode never requests notification permission, registers APNs, calls `register_phone`, sends pairing to the watch, or changes device credentials.
- Review notes (`app_store.external.review.notes`) explain entering demo mode from pairing.

## What the main chat still needs (#4193, onboarding M1)

With hub feature `assistant_readiness`, the App reads `GET /api/v1/assistant/readiness` (`AssistantReadiness`, `apple/Shared/AssistantReadiness.swift`;
state in `AppModel.readiness`, `AssistantReadinessModel`), using the same presentation as the web (`../web/web.md` ("What the main chat still needs")):

- Query while Persona is foregrounded (`.tabActiveTask`), when machines change, from `warm` responses, after chat creation is refused, and after saving a provider (`readinessChanged()`). Old hubs (no feature or 404) show nothing.
- Above the new-chat input (old `AssistantHome`, new-shell `ShellHome`), a `ReadinessCard` (design-system component, BeautifulUI list rows;
  at the accessibility text sizes its buttons and Details stack, the main one first, each on one line, `docs/design/DESIGN_SYSTEM.md` 1.2 rule 3, #5202):
  `setup` shows the first missing step; `rate_limited` shows only a card without blocking Send. In `setup`, a nonempty draft replaces Send with `SendGate` ("Connect a model first", "Add a computer first"…); tap opens that step, retaining the draft without queuing it.
- A main-chat rate-limit card uses the existing Later / close control. `AssistantReadinessModel.homeCardStep` suppresses the dismissed occurrence across ordinary refreshes without changing `current`, the recorded launcher health or the checklist's server-side dismissal; a recovery clears the local suppression, allowing a subsequent failure to appear. Dismissal is observable so the card disappears immediately. The hub's optional `occurrence` uses the main turn's time when available, a health-only subscription reset window otherwise, or the newest evidence time for other failures: a settled health record keeps the turn's identity, repeated health checks stay dismissed, and another turn in the same reset window is still new; older hubs use the step fields as a fallback. Because readiness describes the default launcher while `AssistantTopBar` can pick another launcher/model for this conversation, `ReadinessHomeCard` receives the main conversation's key and explains that the warning concerns the default model connection while this conversation uses another one, without exposing raw launcher IDs. A successful reply through another launcher is not recovery evidence.
- Actions (`ReadinessRoute`): add / confirm computer first opens Machines (switches to Add Computer after M3 lands); connect model opens provider wizard, replace key opens that provider;
  install / log in to Claude Code, wake computer and `local` sample questions open local instructions (`ReadinessHelpView`, not sent to a model); Try a message queries `?probe=1`. Unknown `action` shows text only.
- Hub-coded refusals to open a chat or send (`no_machine`, `claude_missing`…, `CodedFailure`) show `CodedFailureCard`, original text and request ID under Details, no toast; `assistant_offline` silently retries three times.
- A Persona chat's last turn with `turn_error_code` (`not_logged_in` / `auth_failed` / `model_route`) replaces the final "Not logged in" with a "Model not connected correctly" card and Fix it.

## Welcome screen and post-pairing checklist (#4195, onboarding M2)

- Welcome (`WelcomeView`) shows no commands: the main button is Scan to connect hub (Mac: Paste pairing link), followed by where to find the code (the screen after installing the hub, or Settings › Devices › Connect a new device on a connected device);
  No hub yet? opens the public page from build configuration `publish.getting_started_url` (Info.plist `PersonaGettingStartedURL`, hidden if absent), alongside Try it first and Other ways.
  Manual entry, SSH tunnels and command-line instructions appear only under Other ways in `PairView`. Expired or used codes (401 `invalid_token`) say "This code expired or was already used; generate another", with Scan again on iPhone; externally opened pairing links behave the same (`AppModel.showPairing`).
  On iPhone, **Choose from Photos** uses the system `PhotosPicker` without full-library access, even when `DataScannerViewController` is unsupported or unavailable. Vision accepts exactly one QR and feeds the same `PairConfirmation` path as the camera; unreadable, zero-code and multiple-code photos get localized errors without changing pairing state.
- After pairing (`AppModel.checklistPending`, after engine selection), when the hub has `assistant_readiness` and the user has not chosen Later, the page becomes `OnboardingChecklistView`:
  Computer, Claude Code, Model and Try a message, using readiness `state` for checkmarks, crosses, spinners and question marks, with the same actions as main-chat cards (`ReadinessRoute`). Later calls dismiss;
  when all are checked, show Get started. Settings top "N steps left to get started" (`ChecklistSettingsRow`, N counts unfinished computer / claude / model steps) returns to the checklist. Old hubs add no post-pairing checklist.

## Connect a new device (#3077)

"Connect a new device" at the top of Settings › Devices follows the web (#2731, `docs/web/web.md` ("Settings")): request a one-time temporary pairing code from the hub (`POST /api/v1/auth/pair-codes`, with `{"web": the hub address}`,
`docs/protocol/protocol.md` ("Temporary pairing")), so the new device does not need `personad hub pair` run on the hub machine. The card (`Views/Settings/ConnectDeviceView.swift`, logic in `ConnectDeviceModel`) has two modes:

- Persona App: pairing QR code (hub-generated `qr.rows`, `QRCodeView`), the temporary code itself (selectable), "Copy pairing link" (paste to pair Mac and Android).
- Browser: a login-link QR code (scan with the phone camera to open directly in the browser) and "Copy login link"; if this device connects to the hub through `127.0.0.1` / `localhost` (SSH tunnel), warn that other devices cannot open it.
- Countdown (every second); at 0 show expired and "Generate again"; while waiting for pairing, read the device list every 3 seconds. When a credential absent when the code was generated appears, show "Connected: name", refresh the device list, and offer "Connect another".
- For watches and devices connected with a temporary code themselves, the hub returns 403; the card shows the hub's message and "Retry". Old hubs without a QR code show only the temporary code and link.

Screenshots: `-openDevices YES -openConnectDevice YES` (`web`: open the Browser mode); mock hub `MOCK_PAIR_SECONDS` shortens validity, `POST /api/v1/auth/tokens` simulates a new device connecting.

## Add and confirm computers (#4196)

Like the web (#3934) and Android: add a computer in the App and confirm that it is yours (`docs/assistant/onboarding.md` 4.2, 5.3; APIs in `docs/protocol/protocol.md` ("Computer pairing codes", "Computer trust")).
One form (`Views/Machines/AddComputerView.swift`, logic in `AddComputerModel`): choose OS (Mac / Linux / Windows), enter a name (Chinese allowed, at most 40 characters, no id) →
`pair-codes` with `add` (added in the hub-level group) → one screen per OS: Linux shows the hub's one-line install command (otherwise falls back to the pairing command and says to install Persona Link first); Mac shows installation instructions and share / copy pairing link;
Windows shows the PowerShell line → countdown (the `ConnectDeviceText` algorithm; after expiry "Generate again" uses `{"machine": id}`), read the machine list every 3 seconds → when it connects and is pending confirmation, the same screen asks
"A computer wants to join: <name> (macOS). Is it yours?", "It's my computer" / "No". Errors use Chinese text by `code` (`AddComputerText.failure`); `tls_required` uses the hub's message, and unknown codes show the hub's `error`.

- Entry points: the Machines page's empty state and toolbar "+", the Agents empty state with no computers, Mac's File › Add Computer…; only shown when the hub has `machines_add` (otherwise the Machines page keeps its original text).
- The current assistant (`docs/assistant/managers.md` "The current assistant"): a new computer's `add.manager` is the roster's real id of the hub-level group's current assistant (the default's too, one assistant too). Before a roster is known (none kept from the last launch, and the hub has not answered in this run) with no kept pick, Next waits up to 5 seconds for the hub's answer (`AddAssistant.unknown`, settled by `AppModel.managerRosterUnsupported`, the hub's 404); a hub that says it has no roster (404) is settled at once and gets the request as before `add.manager`, as does one that never answers. A read that failed leaves the roster last read, or the kept pick; a new code for a computer names none. When the assistant the computer is under (the reply's `manager`; if missing, the hub's own assistant for a new computer and the listed one for a reused one) is not the one asked for, the waiting screen says "This computer was added under <name>." (several assistants only). With the current assistant offline (several assistants, roster known) the form is replaced by an explanation and Next is off. `no_manager`, `installing`, `assistant_offline` and `owners_unknown` show the hub's own words (`AddComputerText.failure`). The 3-computer limit is shown as the hub's `too_many_pending` words, with no list of the computers in the way (the hub counts every assistant's).
- Other pages can open it with `AppModel.openAddComputer()` (readiness `add_computer`), `AppModel.openConfirmComputer(ref)` (`confirm_computer`; without a machine, use `pendingToConfirm()`: online pending computers first),
  both setting `AppModel.addComputer`, presented by `.addComputerPresentation()` at the root; pages that are themselves sheets use `.addComputerSheet($request)` on themselves.
- Lists and the Machines page: `trust == "pending"` comes first, labeled "Waiting for your confirmation"; online offers "It's my computer" / "No", offline offers "Delete" (buttons only with hub feature `machine_trust`); `error_code` gets Chinese text
  (`AddComputerText.machineProblem`), with the hub's raw message in Machines-page Details. New Agent (and handing a to-do to an Agent, starting one from mail / Knowledge, all through `NewChatView.offered`) excludes pending computers.
- Screenshots: `-showAddComputer YES` (`add:<mac|linux|windows>:<name>` fills and submits; `<machine id>` opens confirmation for that machine; iOS uses `scripts/app/sim.sh`, desktop uses the Electron smoke); UI tests `AddComputerTests` (`docs/operations/testing.md` ("Readiness scenarios")).

## Mac signed in (2026-11-01)

Like WeChat's "Mac WeChat signed in": when Persona on a Mac is connected to the hub, the top of the Agents list says "Persona is signed in on Mac" (with a count for multiple Macs), with "Phone muted" on the right when enabled;
not shown during search. Tapping opens a card (`Views/Settings/DesktopPresence.swift`):

- Phone muted: while a Mac is online, this iPhone no longer receives "Needs you" or "Agent finished" pushes; the Mac alerts you instead. Reminders the assistant sends at your request still push. Stored in this device's push settings (`mute_while_desktop`), off by default.
- Each Mac's name and sign-in time, "Sign out of Persona on this Mac": revoke its device credential, and the hub disconnects it immediately; it must pair again to use Persona. A Mac connected directly with a pairing credential cannot be signed out individually.

Persona Link computers: the app declares `desktops_link`, so a hub that tracks Persona Link presence also lists computers where Persona Link reports you at them (unlocked and in use; `kind: "link"`, `machine_id`, `docs/protocol/protocol.md`). With a Mac app online, Phone muted works as above. With only Persona Link computers listed, it holds back only "Needs you", which Persona Link shows on that computer, and "Agent finished" still comes to the phone; "Needs you" comes back once none of them is in use. When such a hub connects to the computer (passive mode, `docs/operations/link.md` "Passive mode (the hub connects)"), it reads presence on its 30-second probe, so the list and muting can lag a lock or unlock by up to about 30 seconds. An entry with a `kind` this build doesn't know is left out (`DesktopDevice.shown`): neither its muting nor its identity is known. The Mac app's text and look stay as they were; Persona Link gets its own:

- Banner: one Persona Link computer says "Using “<computer>”"; once there are more computers it says "Using N computers". A computer with both the Mac app and Persona Link counts once, so Persona Link on a Mac whose app is signed in keeps the Mac app's line. The match is by name, an approximation for the banner only: a Mac app entry has no machine id, so two Macs sharing a name count once and a Mac whose app and Persona Link use different names counts twice. Exact counts need a machine id on Mac app entries in the `desktops` contract. The line is short because it shares the row with "Phone muted".
- Card: with any Persona Link entry listed, the title becomes "Computers in Use"; when every entry is Persona Link, the Phone muted footer says what Persona Link mutes and what brings "Needs you" back: locking that computer or quitting Persona Link, or with several computers, every one of them (`DesktopDevice.muteNote`). A Persona Link row shows "Persona Link · since <time>" (with the date when it isn't today), no Sign out; the first one has a footer on when it ends. A Mac running both the app and Persona Link shows both rows.

Placed at the top of the Agents list rather than the shared top bar: a persistent line in the two pages' shared bar would cover content; a list-top row scrolls away, like WeChat. Mac registers as a desktop, not a phone (`docs/archive/apple/mac.md` ("Mac is a desktop, not a phone")).
Screenshots: `-showDesktopSample YES` (fake one Mac with phone muting enabled; `link` for Persona Link on one computer, `links` on two, `both` for a Mac with the app and Persona Link plus a second computer with Persona Link), `-openDesktopSheet YES` opens the card.

## Tasks dispatched by the main chat notify only there (#1731)

Settings › Assistant › Task notifications toggle (hub `main_tasks_quiet`, on by default, `GET / PUT /api/v1/assistant`; hidden if the hub does not return the field).
When on, tasks dispatched by the main chat are quiet (to-do `quiet`, attached sessions also carry `quiet`); the hub sends messages into the main chat and the assistant tells you. The hub itself sends no push or Live Activity. Clients exclude them here:

- Live Activities: before merging, `AppModel.mergedFleet()` excludes quiet session entries and subtracts their counts (`FleetState.leavingOut`), covering fleet states predating the flag.
- Foreground notifications: `willPresent` shows no banner for quiet sessions.
- Badges: quiet-session questions count toward the assistant, not Agents (handled in the main chat); the drawer / Mac sidebar's To-dos count excludes quiet tasks (`TodoStore.badgeCount`), though the to-do list still includes them.
- Watch: both the phone-synced summary and the watch's directly fetched lists exclude quiet sessions and their questions (watch credentials cannot read to-dos and use only session `quiet`).
- Mac: notifications treat them as muted; the menu bar excludes them. The app icon counts their questions with Persona's, as the tabs do ("App icon badge (#5824)").

`AppModel.isQuiet` decides: first the hub's session `quiet`, then the current hub's to-dos (`QuietTasks`: quiet to-dos and their Agents, including ones detached from the to-do). Task cards in the main chat and the Agents-page list still display normally.

### Messages the hub injects into chats (#2358)

Task notices the hub sends to the assistant, along with scheduled tasks, follow-ups, wake-ups and node-restart explanations, are user text in the transcript (protocol in `docs/protocol/protocol.md` ("Messages injected by Persona")).
Apple clients do not draw them as user bubbles or show the model's instructions:

- Recognition: `Injected.of(item)` (`Shared/Injected.swift`) first checks the hub's `item.injected`; on old hubs without the field it recognizes text using the same rules as the hub's `prompts.ParseInjected`,
  tested by `InjectedTests` with shared fixtures `spec/fixtures/injected/cases.json`. App-sent messages (with `client_msg_id`) and messages from another session (`from`) do not count.
- Chat page: task notices and system explanations use `InjectedLine`, a centered capsule like the "Answered" row (`AnsweredLine`), with an icon and `line`. `InjectedLook` determines icons and colors, shared by phone, Mac and watch:
  finished is a green checkmark, waiting for you an accent-colored raised hand, interrupted or failed to start a red warning, node restart gray information.
  Tap a task notice for task details: use the hub's `todo_id`, otherwise find the newest same-name task created by this chat; without a match it is not tappable.
  Scheduled runs and follow-ups now use the source-mention rows below.
- When a turn has multiple messages, each gets its own row; the injected one uses the centered row above.
  Nor is it treated as delivery of a pending bubble (`PendingMessage.arrived`).
- Watch: a small centered line (icon plus `line`), no bubble (screenshots: mock `MOCK_INJECTED_WATCH=1`, watch `-openWatchItem session`).
- Chat-list previews use the assistant's words (`last_text`), which already exclude these messages.
- `ChatCache` bumped to v9: old cached items lack `injected`.

## Briefing card (#2320)

A morning or evening routine with `facts` on writes a `brief` item into its own conversation (`docs/extensions/bots.md`
("Morning and evening briefings")). iOS and iPadOS draw it as `BriefCard` (`Persona/Views/Assistant/Tasks/BriefCard.swift`)
with the web's sections, order and totals: waiting for you, the last hours (failed, then done, then "and N more"), to verify,
and can wait and in progress when there are any.

- **Reading**: `FeedItem.brief` decodes `BriefBody` (`Shared/Brief.swift`) when the card is drawn, as `today` is. Every
  section and field is optional (hubs since 2026-11-07 wrote different shapes), a row without an id is left out on its own,
  a total never counts fewer than the rows listed, and `outcome` and `stage` stay strings: an outcome this build doesn't
  know gets Got It, a stage it doesn't know shows the hub's word. A body that can't be read leaves out that card only.
  Builds before this one decode the kind as `unknown` and skip it.
- **Folding**: in compact width the done and failed rows fold into the heading's totals until it is tapped. Regular width
  (iPad, and the frozen Mac app through the shared view) shows them, as the web does.
- **Buttons** (`BriefActions`, one for the app, keyed by row and round: a card drawn again, or the same round in another
  briefing, keeps what happened): Accept / Got It / Take Recommendation post the row's `asked_at` exactly as the hub sent it
  (`APIClient.acceptTodo`, for #2056 to reuse), Handled posts the question's id (`APIClient.laterHandled`). Taken, they
  say Accepted, Noted, Recommendation taken or Marked as handled; a 409 says Already handled; a 404 (a hub rolled back past
  the routes) puts "The hub is too old for this button" in the button's place; any other failure (a 502 for a
  recommendation that didn't reach its Agent, a 503) shows the hub's message as a toast and leaves the button usable. A tap
  while a request is out does nothing. A decision with nothing recommended has no button, and a row without `asked_at` has
  it disabled.
- **Opening**: rows to verify, rows in progress and done rows open the to-do (`TasksStore.detailTask`). A waiting card or a
  question opens the chat it came from (`machine_id` / `session_id`, which the hub adds from #2320). From an older hub, a
  waiting card the app still has among its pending cards opens its chat, and otherwise the row has no Open. Failed rows open
  nothing, as on the web.
- **Recommended setup** (#5804; `docs/assistant/assistant.md` ("Recommended setup")): the first briefing of a week may
  carry `setup`, the hub's computed lines about mechanisms that are off. `BriefBody.setup` keeps the lines that have an
  id and words, each mechanism once, and the card draws them last, under a heading and the web's one sentence that
  nothing was changed. Each line is the hub's `text` as it came; the app adds no number and turns nothing on.
  **Open Settings** is on a line only when the app has the page its mechanism is turned on at (`BriefBody.Setup.page`):

  | `id` | Opens |
  | --- | --- |
  | `triage_decide`, `triage_auto_continue`, `triage_wake` | Settings › Session Triage (`SettingsPage.triage`) |
  | `supervisor_tier` | Settings › Models › the background supervisor's use (`ModelPurposeRoute`) |
  | `task_trust` | no button: a kind of task is remembered with a plan card's own checkbox, as the line says; Auto-Start Tasks only lists and removes them |
  | `standing_authorizations` | Assistant Profile › Standing Authorizations (`SettingsPage.standingAuthorizations`, after `SettingsPage.assistant`, so Back goes to the profile; #7002) |
  | `leaders`, `review_sample`, `delegate`, `independent_review`, a newer one | no button: the app has no page for it, and the line itself says where it is turned on |

  The hub's `link` is the web app's page and is not used. A hub from before sends no `setup` and the card is as it was.
- **Not drawn**: the watch reads only assistant view cards from the feed, so it gets nothing.
- **Tests**: `BriefCardTests` (the first, the current and a newer body; a feed with unreadable bodies; opening; every answer
  the buttons can get, with `FakeHub`; the setup lines, their pages and a briefing with them as one feed line) and
  `BriefCardUITests` against the mock's `brief` item (`MOCK_TASKS=1 MOCK_TASKS_NO_ASKS=1`; `TEST_RUNNER_PERSONA_SHOTS` /
  `TEST_RUNNER_PERSONA_SHOTS_APPEARANCE` save it folded, unfolded and answered). `MOCK_BRIEF_SETUP=1` adds the setup
  lines to the mock's briefing, in the hub's own words; `AvailabilityFollowupsUITests` (with `MOCK_RULE_SUGGESTIONS=1`
  too) checks which lines have Open Settings and the line under the open plan, and saves both
  (`availability-<appearance>-…png`).

## Closeout, orphaned tasks and answerable decisions (#3612, #3618, #3804)

What the hub says about stopped work, drawn on iPhone, iPad and Vision. The wire is in `docs/assistant/agent-work.md`
("Closeout", "Protocol") and `docs/assistant/todos.md` ("Nobody is working on this", "Decisions the owner can answer"); the
web's `CloseoutCard.tsx`, `lib/closeout.ts` and `DecisionRow.tsx` are the reference for behavior and wording. The native
Mac app is frozen: it decodes the same models and draws none of this (`#if os(macOS)` at each entry).

- **Models** (`Shared/Closeout.swift`, `Shared/TodoAsk.swift`, `Shared/Tasks.swift`): `WorkState.closeout` and
  `WorkState.artifacts`, `TodoItem.orphaned`, and the decision's fields on `TodoAsk` (`question`, `background`,
  `why_owner`, `options`, `cost_of_waiting`, `default`, `deadline`, `responses`, `response`). Every new field is optional
  and decoded on its own: one an older hub leaves out, or one in a shape this build does not know, is as if it were
  not there, and never drops the work state, the ask or the to-do. Session lists carry only where a closeout stands;
  `did`, `verified` and the artifacts come with the session's own read (`AppModel.refreshSessionDetail`), which the card
  asks for again when a list row replaced them.
- **Closeout card** (`CloseoutCard`, `Persona/Views/Chat/CloseoutCard.swift`): in the chat's header, under the
  workflow section, for a stopped Agent session with a closeout whose status this build knows
  (`Session.shownCloseout`: not while it works, not on a subagent's page or an assistant conversation). It is in the
  header and not a row of the list, so it is above the transcript and outside message rendering (super rule 2 is
  untouched), and it folds away with the header as the list scrolls. The head says the status with "The agent's own
  report" under it; a tap anywhere on it (one button at least `tapTarget` tall at every text size, the card's padding
  inside it) folds the details, which start folded under an open workflow section: what changed, artifacts (a web link
  opens in the chat's link sheet, "PR #12" for a pull request), merged and deployed, verified, still needs. Each claim
  shows what the Agent said and the hub's finding in one of three states that differ in words, mark and color:
  confirmed (the check, `Theme.positive`), contradicted (the warning mark on the caution chip, `Chip` with
  `.review(.caution)`) and not checked (a dash, secondary). In the chat the details scroll inside past a share of the
  page (`Theme.Size.workflowShare`); without that cap (Settings' sample) they take their own height. They stack label
  over value at the accessibility text sizes.
- **The raw block** (`ChatLines.hidingCloseout`, `Closeout.strip`): while the card shows, the last reply is drawn without
  its `<closeout …>…</closeout>` block: exactly the last block, the blank lines it leaves and a code fence that held
  nothing else. A reply that was only the block is not drawn; an earlier turn's block and a reply followed by a new
  message stay as written.
- **List rows** (`Session.listPreview`, `Session.closeoutPreview`): today's line (`workLine`), then "merged · not
  deployed" where the Agent said or the hub found it ("merged ✓" for a finding, "✗ says merged, isn't" for a claim the
  hub found untrue), as one string the row truncates. A turn nobody can call finished arrives with an empty phase and a
  line (`WorkState.notFinished`): the row shows that line as it is, with no check mark and no wait. When the hub gave no
  line the closeout's own words stand in ("Not done: …", "Blocked on: …"), never the raw block.
- **Nobody is working on this** (`OrphanedLine`, `TodoItem.orphanedLine`): an open to-do with `orphaned` says so with
  the reason (closed after an error, computer offline, closed) in the caution color with the warning mark, under its
  state on the To-dos row and the to-do's page, and first in the task board's subtitle. Not while the task waits for the
  user, and not on a done to-do. (The conversation's full task card, `TaskCard`, is no longer drawn since its dispatch
  links, #3711.)
- **Decisions** (`DecisionRow`, `DecisionAnswers`, `Persona/Views/Assistant/Tasks/DecisionRow.swift`): wherever an open ask
  was drawn with its one button (the Needs You tray, the task board; `TodoAskRow` picks), a
  decision with two or more options shows the question, the hub's background line, each option with what it leads to
  (the recommended one first with the Recommended tag), "Taken by default at 17:30" or "Never taken by default" with
  what waiting costs, then the options as buttons: as many as fit (`ViewThatFits`), the recommended one the only
  prominent one, "Other…" (opens the task) and a ⋯ menu with the remaining options, the hub's fixed answers ("You Decide",
  "This Isn't What I Asked For", "Tell Me the Background First", each with what it does) and Complete Directly (#3038).
  At the accessibility text sizes the group is one column. When `why_owner` is `irreversible` or `visible_to_others` an
  option takes two taps: the first arms it ("Confirm: …"). Every answer is `POST /api/v1/todos/{id}/accept` with the
  round's `asked_at` and `option` or `response` (`APIClient.acceptTodo`); state is kept by hub, to-do and round, so the
  three surfaces agree. An option taken ends the row: "Answered: …" in the buttons' place. A fixed answer does not
  (the hub keeps the round open after "Tell Me the Background First", and ends it after the others, which the to-dos
  read after the answer then take away): the row says "Sent back to Persona" above its buttons, as it does when the
  hub's ask carries `response`, and the options, Other… and ⋯ still answer it. A 409 says Already handled; any other
  failure shows the reason as a toast, sends nothing twice and leaves the buttons (and an armed option, or "Sent back")
  as they were. One request at a time: while an answer or Complete Directly (`BriefActions`, which ends the task) is
  out, its control spins and the whole row is disabled; completed, the row goes as `BriefAcceptRow`'s does. An armed
  option keeps a button even when its longer "Confirm: …" leaves room for fewer, in the last place
  (`DecisionRow.buttonIndices`), and "Other…" and ⋯ never cut their labels. The default line is worked out once per
  round and day (`DecisionWaitingLines`), not in `body`. In the Needs You tray it is what a folded decision row opens
  into (#5569): without the title the line above already says, and with Later and Don't handle under its options as
  `BriefAcceptRow` has them there (`TodoAskRow`'s `deferrable`). A decision held for the briefing says so, and a
  suggested rule's line (`RuleSuggestionLine`) sits under it, as on `BriefAcceptRow`. An ask without options, and every
  ask on the Mac, keeps `BriefAcceptRow`.
- **Not built**: the `TODO_DECIDE` notification's option actions (the push payload carries `options` when its
  `lockscreen` is `answer`, and `TODO_DECIDE_OPEN`, no actions, when it is `open_only`, #10465); the watch.
- **Tests**: `CloseoutTests` (decoding, claims, the rows' line, the stripped block, nothing new without a closeout),
  `DecisionRowTests` (the ask's fields, the default line and its memo, every answer with `FakeHub`, the row staying
  answerable after a fixed answer, the row held during Complete Directly, the armed option keeping its button,
  orphaned to-dos), and cases in `SessionWorkTests` and `TriageSettingsTests`. DEBUG
  `-showCloseoutSample confirmed|contradicted|blocked|rows|orphaned|decision|armed` shows each state with made-up
  content and no hub.

## To-dos (V1, #10465)

The To-dos redesign (`docs/design/proposals/todos-redesign.md`, its V1 frames in `docs/design/prototypes/todos/v1/`) is
built in stages behind the hub's `todo_view`; this section grows with it.

- **Mock hub**: `MOCK_TODO_VIEW=1` serves the design prototype's to-dos (`docs/design/prototypes/todos/data/todos.js`)
  as a hub with the view does: `view` on every row of `GET /api/v1/todos`, the list's `counts`, the item and places
  routes, Someday and restore, and today's answers to its asks (`scripts/mock/todo_view_mock.py`). A screenshot lines
  up with its V1 frame when it is taken soon after the mock starts (its start plays the prototype's 10:14) and the app
  runs with `-AppleLanguages "(en)"`. `MOCK_TODO_VIEW=old` is a hub from before the view (405 for the item and places
  routes, as an older hub answers them), `late` one whose hello names the view only after a reconnect, and
  `MOCK_TODO_MANAGERS=2` gives the Weather App's to-dos to a second manager. `MOCK_MANAGERS=1` lists one manager, for
  the path that reads the hub's counts and places as sent; the default roster lists several.
  `docs/operations/testing.md` has the details, including `GET /mock/churn?todos=1` for measuring a push on the list
  and where the mock is not a hub.
- **The data** (`TodoStore`, `apple/Persona/Features/Todos/`): `GET /api/v1/todos` is decoded into its rows and, beside
  them, each row's `view` (by to-do id) and the list's `counts` (`TodosPage`); a view that does not decode is none.
  `TodoItem` gains only facts (`part_of`, `after`, `restored_at`, `someday_until`); the store keeps `views`, `counts` and
  `places` apart from `todos`, so a change to a view alone (a run's latest line) redraws nothing that reads the rows and
  does not refresh the watch or the widgets. A 304 applies the views and counts read with that tag again. Views and
  counts are read whether V1 is on or not (V1 coming on in a run has them); everything else is done only while it is on:
  - the `todo-view` StateCache entry beside `todo-items`: the views, counts and places, with the numbers the pages
    showed, so the first frame draws them, and when the hub last answered a read of the list (`readAt`, said while it
    is out of reach). It is written at most every 15 s while a run's line moves (`Throttle`) and
    at once when the scene leaves the front; forgetting the hub removes it whether V1 is on or not, and an entry none
    of whose views is for the kept rows is not used at all;
  - the first V1 launch without it: `viewsRead` stays false until the first read ends, so the list does not file its
    rows twice, but no longer than a second (no connection yet, or a hub that does not answer);
  - the places, read with the list while the hub lists `todo_places`, again only when the list changed; a hub without
    the route, or whose hello stopped listing it, has none (All only);
  - one timer, to the earliest `next_change_at` of any row, reads the list again (coalesced) through the app's hub, only
    while the scene is in front. A read that still shows that moment (the hub's clock behind) is tried again after 5 s
    and 30 s, then left to the next push, and the timer never reads sooner than 5 s after its last read. The timer is
    the store's, not the page's: the drawer's and sidebar's counts change by time too.

  Tests inject all of this through `TodoStore.V1` (whether V1 is on, the features, the cache, the client the store's
  own reads go through, and `Timing`); only `TodoStore.shared` follows the scene (`followsScene`). When the gate
  changes, `TodosV1` tells the store (`v1Changed`), which counts, writes the cache, arms the timer and reads the places
  from what the last read had, or lets the numbers go. `TodosAPI.swift` has the item, places, Someday, restore and
  origin-route calls (the last with its op id and a timeout above the hub's five-minute relay).
- **Filing and counts.** `TodoFiling` files a row by its `view` when its section is one this build knows, else by the
  app's own rules (`TodoTriageLogic`, frozen) mapped onto the V1 sections (Needs you → Needs you, Working → Working,
  PR to merge → Waiting, Other open → To do, Just finished → Just finished, Shelved → To do with the Inactive mark,
  Done → Done); a section's rows go by `rank`, then `manager`, then `id`. Every To-dos number comes from
  `TodoStore.countsSource` (`TodoCountsSource`): the hub's `counts` and places with one manager; with several, counted
  from the current assistant's filed rows (`TodoCountsFallback`, the hub's rules, replayed against
  `spec/fixtures/todo-counts`) with only its own project places; less the answers given on the device that the hub's
  view does not show yet. The rows are filed there once (`countsSource.filed`, the current assistant's in the list's
  order), for the V1 list to take its sections from, so the list and the numbers cannot disagree. A row filed
  by the app's own rules (a manager older than the hub's features say, or a section this build does not know) is
  filed again when one of its Agents' sessions changes (each read through its own cell, so only those are watched)
  and each minute while the scene is in front, since those rules read the clock and no `next_change_at` says when.
- **The gate and its memory.** `TodoFeatures` reads `todo_view`, `todo_places`, `todo_timeline` and `todo_asks` from each
  group's hello and remembers them per group (`hubTodoFeatures.<group>`, written only when they change); before a
  group's hello in this run the remembered value is used. `TodosV1` is on while the hub-level group's hello listed
  `todo_view` in this run, or, before that hello, its last one did; once on for a group it stays on for that group
  until the app quits, so a hello without it while a manager reconnects never swaps the page back, and such a hello is
  not remembered as a downgrade (the first hello of a run is). It follows the owner's pick of the hub-level group:
  picking an older group's hub turns it off, picking back a group it was on for turns it on without another hello.
  Signing out of every hub forgets the remembered values, and a group that goes while others stay (signed out of, or
  merged into another: `TodosV1.forget`) loses its own, since the app's stored pick, written only when the owner picks
  a hub, can still name that group at the next launch. The gate turns on at the first hello that lists `todo_view`, also in the
  middle of a run: holding that change while To-dos is on screen, so a visible page never swaps, is the page's job
  (`TodosPlacePage`, below); the gate stays a plain value. It is off in every Release build until the switch-on
  (`TodosV1.released`), and on the iPhone and iPad only (`TodosV1.platform`): visionOS keeps today's To-dos. The data
  and model files under `apple/Persona/Features/Todos/` compile on every platform, since `TodoStore` (shared with
  visionOS) names them; the V1 views are iOS only (`List/`). The hub's `todo_send_state` marker is not read yet.
- **The list** (`apple/Persona/Features/Todos/List/`, iOS only; DESIGN_SYSTEM 3.4 "To-dos list"). `TodosListModel`
  (`Deps`: the store, the hub, the clock, the projects' names) follows the store and lays the list out once per change
  (`TodosListLayout`, pure): the sections for the bar's choice (`TodosBar`, `TodosChoice`), the grouping and the sort,
  from `countsSource.filed`, less the steps their parents carry. Each row reads its own cell (`TodosListModel.Row`),
  whose content (`TodoRowContentLogic`: times said from now, the mark, line 2 through `TodoLineText`, the door, line 3,
  and the row in words for VoiceOver) is worked out again only when its facts, its view or the minute changes; a line
  key this build does not know says the section's word, so the mark stays. While the list is scrolled or out of sight
  (`Hold`: the page reports its scroll phase and when it is out of sight) a new filing waits, and so does what each
  row says, since a new line can wrap or a door come and go and a row's new height moves every row under it; a row
  gone from the store keeps its cell until then. A row that comes to need the owner lands at once, alone, onto the
  list as drawn; the numbers (`countsSource`) are never held. When the rest lands, the rows that changed group are
  drawn in place at once and fade in (`Theme.Motion.conceal`), so nothing slides across the list. A row in another
  group or fold is a new view there (another section's `ForEach`), so a trigger it observes cannot start the fade (it
  starts with the value as it already is, and the view it leaves sees the change first), and an `onAppear` that
  starts one comes a frame after the row was drawn whole. The landing marks the row instead (`Row.entering`), which the
  new view reads from its first frame, drawn hidden, and clears inside the animation as it appears. A mark not taken
  (a row that landed out of sight) is cleared at the next landing and when a scroll starts, so nothing fades in under
  a finger; with Reduce Motion nothing is marked. The page's minute
  (`tick`, also as the page comes back to the front) says the times again. Rows open today's to-do page until the to-do's own page arrives. On iOS 26 the section bar is pinned over the
  list; iOS 18 has no scroll edge effect to draw behind it, so there the bar is the list's first row, as the Inbox's.
  `TodoWorkspace` (one for the app, `TodoWorkspace.shared`, outside the pages' trees) keeps the place and where each
  place's list came to rest, written only while the scene is active and only from the tree the window shows; a page
  made anew puts its list back there once, and a page a to-do popped back to keeps its own place. The grouping and the
  sort are kept on the device (`todos.v1.grouping`, `todos.v1.sort`); the bar's choice is not.
- **The rows' writes** (`TodoListActions.swift`, `TodoOutbox`; DESIGN_SYSTEM 3.4 "To-dos list"). The tick, the trailing
  swipe (Done, Someday; Reopen on a closed row), the long-press menu (Today, Date…, Project, Someday, Copy Link,
  Delete…) and New To-Do; which a row offers is `TodoRowActionList`'s, so the swipe, the menu and VoiceOver's actions
  agree. A row (`TodoListCell`) takes its cell and the list's model, never closures, so a list update that leaves it
  alone skips it (P6); its confirmations are presented from it. A tick, Someday and a delete are held in `TodoOutbox`
  (`apple/Persona/App/TodoOutbox.swift`) for `Theme.Motion.todoUndo` (`todoUndoAssistive` while VoiceOver or Switch
  Control runs) and only then sent, so Undo sends nothing; a reopen, Remove from Someday and a new to-do go at once,
  and Remove from Someday inside Someday's window takes that write back instead. Each write keeps its op id
  (`X-Persona-Op-Id`) from the tap. The outbox is the StateCache entry `todo-outbox`, read off the main actor and
  written in order by `TodoOutboxFile` (numbered snapshots, so a late older one never wins); it is made only once V1
  is (`TodoStore.V1.answered` is called only while V1 is on), and forgetting a hub or unpairing drops its writes
  (`TodoOutbox.forgetHubs`, only while the build may show V1, so a Release build before the switch-on never makes it;
  a write a DEBUG launch with `-showTodosV1` kept for a hub forgotten without it goes only to that hub, and expires).
  - **Sending.** The writes whose window ended go; the shown hub's every write goes at once when the app leaves the
    foreground, under a background task that lasts until every send under way is answered. How an answer is read
    (`TodoOutbox.outcome`): a 404 on a to-do's own route (`not_found` or `todo_not_found`; not the hub's `no_manager`)
    is a to-do already gone, which is what a change to it comes to; nothing reached the hub (no connection made, no hub,
    `assistant_offline`, 408, 425, 429, `delivered: "none"`) sends it again when the hub next answers a read; another
    4xx is a refusal, dropped and said in a toast (a new to-do's with **Edit**, which reopens its sheet with what was
    typed); anything else may have been done. A tick, Someday or a delete of unknown outcome is sent again, being the
    same write twice, at most `TodoOutbox.tries` times, then said. A new to-do is never sent twice on its own: the hub
    keeps no receipt for a create without images (`server/internal/hub/todo_receipts.go`), so one of unknown outcome,
    or one the app was ended with while it was out, is "Not sure it was saved" until a read lists a to-do with its
    title and notes made since (then the hub's row is its row), with Try Again and Remove.
  - **Sent again.** A write the app was ended with, or one that could not reach the hub, goes once the hub answers a
    read of the list, and only if its to-do's `updated_at` is still the one the list had at the tap (`Entry.base`):
    one whose to-do changed meanwhile is let go, and said unless the to-do is already as it asked; one whose to-do is
    gone is let go quietly. A change older than 12 hours, or a new to-do that never reached the hub after a week, is let
    go.
  - **Drawn until it shows.** The hub's answer is kept on disk as answered (never sent again) the moment it comes,
    before the list is read again; the write stays until a read of the list begun after that answer has landed
    (`TodoStore.readsBegun`, `hubAnswered(read:)`): until then the list draws it (the tick struck through, a new to-do
    "Saving…", first in To do) and `TodoCountsSource` counts it (`TodoStore.V1.pending`: a deleted to-do out of every
    number, one put in Someday counted there, a new one in To do), so the numbers and the rows never disagree. A change
    that could not reach the hub says "Not on your hub yet" on line 2. Undo while a write is being sent draws it taken
    back at once and sends its reverse once the hub has it; Undo on one the hub has, or may have, sends its reverse,
    to the hub it was made on. A new to-do's own row takes the place of the one drawn for it in the same step, without
    fading in. Today, Date… and Project are the to-do's plan, drawn at once and saved as its page saves it
    (`reschedule`, `refile`); Today and Date… keep a timed due's time of day.
  - **Offline.** While the titles say the hub is out of reach (`ConnectionStatus.shown`) the list keeps the last read,
    under "Can't reach your hub · showing what was saved ‹time›", the time the hub last answered (`TodoStore.readAt`,
    kept with the views at least once a minute); the line registers with `titleSaysLink()`, so the shell's pill stays
    away. Tests: `TodoOutboxTests`, `TodosListWritesTests`, the UI suite's `testTickAndUndo`, `testSwipeSomeday`,
    `testDeleteAndUndo` and `testOfflineIsSaidOnce` against `MOCK_TODO_VIEW=1`.
- **Which page.** `TodosPlacePage` draws the V1 list while the gate is on, else today's `TodosView` (frozen), deciding
  once, when the page is made (`TodosPageChoice`): a hello that turns the gate on while To-dos is up (a manager that
  connects late), or while a to-do is pushed over it, never swaps it; the next time To-dos opens it is the V1 list. The
  drawer's and sidebar's To-dos row and the folded Pages heading show N on the V1 path (`TodoStore.sidebarCount`),
  today's open count otherwise, also while a page made before the gate came on still draws today's list
  (`TodoWorkspace.holdsToday`), so the page and the drawer never say two numbers. DEBUG `-showTodosV1 YES` stands for
  the release (`TodosV1.allowedHere`); V1 still needs a hub that lists `todo_view`. With `-probes YES` the page exports
  what it draws and whether the gate is on (`probe.todos-page`). The store's one-minute clock for rows filed by the
  app's own rules runs while a counted row is filed so (the drawer's N included, whether or not the list was drawn) and
  follows whatever changes that, another assistant made current included (`TodoCountsSource.byRulesChanged`).
- **404 and 405.** An older hub answers 405 for `GET /api/v1/todos/{id}` and `GET /api/v1/todos/places` (the `PATCH` and
  `DELETE` routes of `/api/v1/todos/{id}` match the path) and 404 for the other new routes: `TodoNotThere` takes both
  as "not there". A 404 with `todo_not_found` is the to-do itself gone. `GET /api/v1/todos` answering 404 still hides
  the To-dos entrance, as before.

## Quiet supervision rounds (#2092)

Persona conversations on iOS, iPadOS and macOS share the web's whole-round visibility contract. A `system` / `supervise`
input with `level: triage` (including legacy inputs without a level) and all following work, answers, recaps and
continuations become one compact **Supervision record**. The next actual input always starts outside it, even when queued.
This is independent of New Conversation Layout; Agent conversations retain their existing presentation.

Opening the record restores the original injected input, answers and retained transcript rows. Ordinary work can still
be expanded separately, including server-folded ranges. Successful Persona `notify_user`, `ask_user`, accepted/needs-user
`task_review`, a successful `supervision_finish` receipt with user visibility, explicit user-level input or retained
`fold.supervision_user` promotes the whole round. A pending live `ask_user` stays visible before its result arrives.
Failed calls, arguments without success receipts and silent finishes do not establish or reverse promotion.

The separate streaming answer stays hidden inside a closed quiet record, including text waiting for its transcript
item after a turn stops. Frozen history uses its captured live turn, not a later turn's activity. Retained promotion survives overlapping
page merges and cache serialization; the chat cache schema changes with these fields. Tests: `ChatSupervisionLinesTests`,
`ChatSupervisionTests` and `SupervisionRoundUITests`. Screenshots use `MOCK_TASKS=1 MOCK_SUPERVISION=1` on a fresh test device;
`-showSupervisionOpen YES` opens the record, and `-openAllWork YES` exposes its internal work for inspection.

## Background rounds (#4314, leaders in #6595)

In the assistant's **main** conversation and its **leader** conversations (`AppModel.assistantLeaders`,
`ChatLines.foldsBackground(…leaders:)`), a round whose first input comes from another conversation (source kind
`conversation`), a scheduled follow-up (`followup` with the task's title; not a usage-limit continuation, a wake-up or an
untitled follow-up, which continue the user's own work) or a scheduled task (`patrol`) folds like a quiet supervision round: one muted
line, `@<source> · Background round` (`@<source> · Running in the background` while it runs), drawn by the same
`SupervisionRecordLine`. `<source>` is the label the source mention uses (`MessageSource.name`). The source comes from
`MessageSource.of`: `Item.source`, then the `message-source` metadata block, then the hub's `injected` mark. The round
lasts until the next actual input; recaps and compactions inside stay inside. A message the user queued while it runs
joins the same transcript turn, so `ChatLines.lines` cuts the turn at each input: the user's message and what follows it
are never folded in. Other side conversations and Agent sessions are unchanged. The hub's interject note
(`prompts/assistant/interject.md`, `<persona-context kind="context" title="插话">`) is dropped in `ContextRef.scan`, so
it never renders as an owner-bubble quote; `WatchText.shown` likewise leaves no 〔插话〕 mark on the watch.

Opening the line shows the whole round, history included (super rule 2.4). Unlike a supervision record it shows the
input as its source block, not as raw input: a supervising conversation's message shows its text; a scheduled task's or
a follow-up's prompt (`injected.detail`, or the raw input) is never shown while folding is on, in a folded round or a
visible one; its block shows only the one-line `injected.line` (`ChatLine.hidesPrompt`). A round stays shown by exactly
the supervision rule (`Supervision.visible`): a successful `ask_user` / `notify_user`, a pending live `ask_user`, an
accepted / needs-user `task_review`, a `supervision_finish` receipt with user visibility, or `fold.supervision_user`.
A completed round with a nonblank final reply also stays visible unless it successfully finished silent, except for a
routine cross-conversation report to the main conversation: that reply stays inside the expandable background record,
just as on the web. Explicit owner-visibility actions above still open and notify. The reply uses the usual bubble and
the work stays collapsed. See `docs/assistant/assistant.md` "Background rounds".

While folding is on, the main chat's Needs You tray groups task rows by the conversation that supervises them (the
to-do's `origin`) when they come from more than one: the conversation's own group first without a header, then each
other one under its name (the direction's name, else the conversation's title, else "Another conversation") in the
tray's header style. Rows that are not tasks belong to the conversation's own group. Rows, their order within a group
and the count are unchanged (`ActionTrayContent.grouped`); with one group the tray is a single list as before.

**Task cards, fused with the web (#5569, `docs/design/task-cards.md`).** What changed on iOS:
- **Needs You rows are folded.** A plan, a reply draft and a task with an open owner ask (a decision or a to-do to verify) are one line each (mark, title, kind word, a second line, ›); a tap, or the chevron, opens that row in place into its card (`AssistantActionBar.opening`): `PlanCard`, `MessageCard` or `BriefAcceptRow`. Several rows may be open; a card drawn once stays in the tree (folded to height 0) so a plan's "Start these directly" and a draft's edit survive folding. The tray grows to `box380` while a row is open and scrolls the row to the top; the open plan's Start / Edit / Don't Start are pinned under the list (`PlanActions`, choices in `PlanChoices`). The conversation's own ask rows keep their old behaviour (the card is in the chat footer; the row closes the tray), and task and Agent rows without a card open their detail. DEBUG: `-openNeedsRow plan:|task:|message:` opens the tray with the first row of that kind opened (the Release check refuses the name anywhere in the binary's `strings`: it is in `RUNTIME_CLI` in `scripts/release/debug_switches.py`, not in the whole-line list `scripts/release/debug_switches.txt`).
- **Progress rows (§3.2).** The status word is the right-hand column in its tone (red for interrupted and failed, `cautionText` for a usage limit, the Needs You tone), a second line carries the reason (usage limit: "You've hit your usage limit. · Resets 15:20", from `Session.usageLimitWhen`; interrupted: the error's reason; starting: "Starting the agent"; needing you: what it asks; working: its step; finished: its last words), and a › ends every row that opens something. Marks: a `caution` clock for a usage limit, ⓘ for interrupted, a hollow circle with a `caution` dot for starting. The usage limit's reading is `Shared/UsageLimit.swift` (the web's `lib/usage-limit.ts`: `turn_error_resets_at`, `turn_error_resumes`, `work.resets_at`, a 10-minute grace after the reset); `AssistantEntries.todoStatus` is unchanged (the shared fixture does not know the grace).
- **Decision rows (P1, P4).** `BriefAcceptRow` has no "Actions" menu: when its buttons do not fit one line the secondary ones (Open, Complete Directly, Later, Don't handle) wrap in their own row and the main one has the line below to itself. Later and Don't handle show only on the tray's decision rows (`decision:` arg of `BriefAcceptRow`, an owner decision or a `needs_user` review): Later hides the round for this app session (`TasksStore.laterDecisions`, keyed `decision:<manager>:<id>:<round>` like the web's `decisionLaterKey`); Don't handle is `POST /todos/{id}/decision-dismiss` with the hub's own `decision_review.review_at` string (never a `Date`: sub-second digits matter) and the manager header, a six-second Undo toast (`decision-undo-dismiss` with the receipt), and a 409 reloads the to-dos. `AssistantEntries.split` leaves a hidden decision out (`decisionHidden`) unless a live Agent question gives the task its status. The words are one pair on every surface: "Later" / "Don't handle".
- **Plan card (§3.1).** Start is full width, Edit and Don't Start under it when the three do not fit one line; a task's brief (`PlanTask.brief`, decode-only, never sent back) shows folded under its "why" (`PlanBriefView`: who asked, a summary line, Show All).
- **Receipts (§3.3).** Gone since #8237: the chat draws no receipt, review or report (`ChatLines.drawnFeed`); the hand-off card says a failed start or an interrupted turn in place (`DispatchHandoffCard.interruption`), Retry, Remove and Continue are on the Needs You rows (`AssistantActionBar.actions`, `TasksStore.retry`), and finished tasks are the Finished entry's rows (#10922; In Progress's Just Finished group until then; `docs/assistant/unified-entry.md` "Receipts").
- **Hand-off cards (D6).** See `docs/assistant/unified-entry.md` ("What a dispatch opens").
- Removed (P7): the unused `TaskCard`, `TaskNowLine`, `TaskLine`, `TaskSentList`, `TaskProgressLine`, `TaskOpenButton`.

The hub's `main_background_folded` (`GET` / `PUT /api/v1/assistant`, on unless changed; an older hub that omits it
counts as on and hides the switch) is Settings › Assistant › **Fold background rounds**. Off, the main conversation
and Needs You look exactly as before. Tests: `ChatBackgroundRoundsTests`.

## Agent follow-up settings (#7005)

The web's Settings › Assistant groups "Starting tasks" and "Agent follow-up" (`web/src/views/settings/AssistantSettings.tsx`,
`AgentAutoNaming.tsx`) on iPhone and iPad, as one page one tap below the profile: **Assistant Profile › On Its Own ›
Agent Follow-Up** (`AgentFollowUpView`, `AgentFollowUpModel`; Settings search finds it, `SettingsPage.agentFollowUp`). The
profile shows the row only when the hub sends at least one of its fields, and the page shows each row only for a field the
hub sends (`GET /api/v1/assistant`, the value in effect). Who it is for: people who hand work to Agents from Persona.

| Section | Row | Field (default) | Control |
| --- | --- | --- | --- |
| Starting tasks | Start What I Ask Right Away | `start_requested_tasks` (on) | switch |
| | New Tasks Prefer | `task_workspace_preference` (`prefer-existing`) | menu: Existing / New Workspaces |
| Follow-up | Main Chat Supervises Its Own Tasks | `main_supervises_own_tasks` (off) | switch |
| | Follow Up Every Agent Persona Starts | `agent_tasks` (off) | switch |
| Tidying up | Stop Idle Agents After | `idle_close_hours` (12; 0 never; at most 8760) | menu: Never, 1, 2, 4, 8, 12, 24, 48 hours, 1 week, Custom… |
| | Archive Closed Sessions After | `archive_closed_days` (3; 0 never; at most 3650) | menu: Never, 1, 3, 7, 14, 30, 90 days, Custom… |
| Terminals | Close an Agent's Terminal When It Ends | `close_ended_terminals` (off; #10355) | switch |
| Session names | Fix Unhelpful Session Names | `agent_auto_naming` (on) | switch |
| | Name Changes per Session (while on) | `agent_auto_naming_max` (1; 1–1000) | menu: 1, 3, 5, 10, Unlimited (1000, the hub's most), Custom… |
| | Update Names Regularly (while on) | `agent_auto_naming_interval_hours` ≠ 0 (off; on starts at 24) | switch |
| | Update Every (while regular) | `agent_auto_naming_interval_hours` (1–8760) | menu: 6, 12, 24 hours, 2 days, 1 week, Custom… |

- **Values**: a menu lists a value set elsewhere (the web, the assistant) among its presets, worded in the largest whole
  unit (`AgentFollowUpText`). Custom… asks for a whole number in an alert, within the hub's bounds. Each footer says the
  defaults in one sentence; with regular updates on and at most one name change the footer says to raise the limit, as
  the web's note does.
- **Writes**: a change shows at once and is one sparse `PUT /api/v1/assistant` with that field alone, naming the manager
  that answered the read (`X-Persona-Manager`, the default's too: the hub takes the session-name settings only from a
  request that names its manager, so those rows need `manager` in the read). The hub's answer is kept for that field
  and handed to `AppModel.assistantConfig`; a refusal puts the page's value back with a toast. A control waits while its
  own write is out.
- **Not here**: `inbox_signals` (Tell supervisors about new messages) moves to the Inbox's own settings (#4524), and
  `default_permission_mode` has no page in the app yet.
- **Mock hub**: the default mock sends every field with the hub's defaults and checks a `PUT` as the hub does;
  `MOCK_LEGACY_HUB=1` has none of them. Debug launch: `-openAssistantPage agentFollowUp`.
- **Tests**: `AgentFollowUpTests` (reads, each change's request and header, the hub's answer, rollback, the menus'
  words) and `AgentFollowUpUITests` against the mock hub (light / dark shots with `TEST_RUNNER_PERSONA_SHOTS`);
  `AssistantSettingsModelTests.testMainSupervisesOwnTasksSwitch` on this page.

## Background supervisor (#5083)

The hub's built-in background supervisor (`docs/assistant/unified-entry.md`, "Background supervisor") on iPhone, iPad,
Mac and Vision Pro. Nothing here shows on a hub from before it.

- **Its conversation** is the one `GET /api/v1/assistant` names as `background_supervisor`
  (`AssistantState.isBackgroundSupervisor`, never the main conversation). The hub opens it with `origin: "task"`, so it
  is in All Chats under Scheduled Tasks and never in the sidebar's Directions or Recent (`AutoConversations`,
  `AssistantSidebarGroups`; Recent leaves it out by its id since #6228 every scheduled task's chat (`origin == "task"`, plus the ones `AssistantTaskChats` knows) stays out of Recent and in All Chats). Its row has no Archive (swipe, menu, the Mac's Delete key), no Pin and no End, in the
  sidebar's search results and in All Chats; Rename and the copy items stay. The hub refuses Archive and Pin itself;
  End it does not refuse, the row only does not offer it. The hub makes the conversation on first use, usually while
  the app is open, and only the assistant settings say which one it is: a new assistant conversation with
  `origin: "task"` that the settings held do not list has them read again, once for a conversation
  (`AppModel.unreadTaskConversations`), so the row has the three actions no longer than that request takes.
- **The switch**: Assistant Profile › Agent Follow-Up › **Main Chat Supervises Its Own Tasks** (on the profile itself
  before #7005) is the hub's `main_supervises_own_tasks` (`GET` / `PUT /api/v1/assistant`), off by default: while it is
  off the background supervisor follows up the tasks Persona hands out. The page writes it like its other rows
  ("Agent follow-up settings" below) and hands the hub's answer to `AppModel.assistantConfig`. A hub that omits the
  field has no switch.
- **Its model**: Settings › Models lists the use `assistant.supervise` after the categories and the other uses set
  apart. A hub since #5806 lists it in its catalog: the list reads it there, where the catalog puts it, and sends no
  request by its id (`ModelConfigurationModel.asksApart`). A hub before #5806 leaves it out, so `load` asks for it by
  its id once the catalog is in and appends it (`listing`); a hub without it answers 404 and the list has no such row.
  Its page says "Follow Main Chat" where Lark /agent says "Follow Agents", and saves to the use's own record.
- **What each use is for** (#5806): a use's `description` (the manager's sentence in the request's language, #5086) is
  drawn under its name in the list, above the model (`ModelPurpose.purposeNote`). A use without one, and every use
  of a hub from before #5086, has the row it had.
- **Its reviews in the main chat**: a review is the feed kind `review` in the conversation that planned the task,
  whoever reviewed, so `ChatLines.mergeFeed` places it and `ReviewFeedLine` draws it as one line that opens the task.
  Nothing marks the reviewer in the feed item, and the line does not name one.

Mock hub: `MOCK_BACKGROUND_SUPERVISOR=1` adds the conversation (with `MOCK_ORIGINS=1` for the other sources); the
switch and the model use are there without it, and `MOCK_LEGACY_HUB=1` has none of the three.
`MOCK_SUPERVISE_LISTED=1` is a hub since #5806: the catalog lists the use and every built-in use has a description.
Tests: `BackgroundSupervisorTests`, and `AssistantSettingsModelTests.testMainSupervisesOwnTasksSwitch` against the mock hub
(on the Agent Follow-Up page).

## Standing authorizations (#7002)

The web's Settings › Assistant › Standing authorizations (`docs/web/web.md`, "Standing authorizations") on iPhone and
iPad, in the assistant's profile: **Assistant Profile › On Its Own › Standing Authorizations**, after Auto-Start Tasks,
with how many there are. Settings search finds it (`SettingsPage.standingAuthorizations`), and the briefing's setup line
for `standing_authorizations` opens it. The frozen Mac app compiles the same views.

- **Who it is for**: anyone who said "always go ahead" to an Agent's merge, push, deploy, send, delete or spend. A row
  reads as a sentence: the actions in the web's words ("Merge changes and deploy to a test environment"), "In
  weather on MacBook" (the folder's name and the computer's), then the conditions and the day it was granted. The full
  folder, the owner's own words, the source and whether the authorized work can be undone are on the grant's page,
  with **Lasts: Until you revoke it** (grants have no expiry). Unknown actions and conditions show as the hub wrote
  them (`StandingAuthorizationText`). The source is words, not a code: the hub's own sentence for a suggested rule
  confirmed with Always (in the owner's language when it was made) and the usual words the assistant writes (main chat,
  a chat, Settings) are worded in the app's language whichever of en, zh-Hans or ja they were written in
  (`StandingAuthorizationText.source`); anything else shows as written, as on the web.
- **Requests**: `GET /api/v1/assistant/standing-authorizations` and `DELETE …/{id}` through the client of the assistant
  the page first read for (`StandingAuthorizationsModel`, `currentAssistantSettings`). A 403 says only a device with
  full access can see them; any other failure shows Retry, and a failed refresh with the list on screen is a toast.
- **Revoke**: a trailing swipe, the row's menu or the grant's page, always behind a confirmation dialog that names the
  scope, says done work is unaffected and that a new grant is asked for in the main chat (destructive **Revoke
  Authorization** and Cancel; section 4.4 of `docs/design/DESIGN_SYSTEM.md`). Confirmed, the row leaves the list at
  once (the grant's page closes first); a failed DELETE puts it back where it was with a toast saying it is still in
  effect, and a read while the DELETE is out leaves it out. There is no Undo: the hub has no route to restore a grant,
  and an old id can't be added again.
- **Mock hub**: `MOCK_STANDING_AUTHORIZATIONS=1` seeds two synthetic grants (`MOCK_STANDING_AUTHORIZATIONS=forbidden`
  answers 403, `=fail` makes every DELETE fail); with `MOCK_RULE_SUGGESTIONS=1`, Always adds one. DELETE answers as the
  hub does, `{"ok":true,"revision"}`. Debug launch: `-openAssistantPage standing`.
- **Tests**: `StandingAuthorizationsTests` (decoding, words, the model's read, revoke and rollback with `FakeHub`) and
  `StandingAuthorizationsUITests` against the mock hub (list, page, confirmation, revoke; light / dark shots with
  `TEST_RUNNER_PERSONA_SHOTS`).

## Suggested standing rule and held decisions (#5802, #5803)

The Apple part of two follow-ups of `docs/design/proposals/main-chat-availability.md`, on iPhone, iPad, Mac and Vision
Pro through the shared views. Neither shows against a hub that does not send the field.

**A rule after the same answer three times** (`docs/assistant/unified-entry.md`, the section of that name). The hub
attaches `suggestion` (`RuleSuggestion`, `Shared/RuleSuggestion.swift`) to an open plan, to a to-do's decision and to a
question a supervisor passed up. `RuleSuggestionLine` draws it as one quiet line under the card:

| Card | Where the line is | Whose manager answers |
| --- | --- | --- |
| Open plan (`PlanCard`, in the chat and the action tray) | Under the card's surface | The plan's conversation's (`AppModel.feedManager`) |
| A to-do's decision (`BriefAcceptRow`, or `DecisionRow` for one with options: the task board, the action tray) | Under the row's buttons | The to-do's own (`TodoStore.named`) |
| A question card (`InteractionPanel`) | Under the form, inside the panel; the form's cap leaves it room | The one that reported the card (`Interaction.manager`) |

- **What it says**: the hub's `text` (what was noticed), then a sentence the app words from the fields, saying exactly
  what Always creates and for what (`RuleSuggestion.createsText`): the entry in Auto-Start Tasks for that directory and
  computer; a standing authorization for the action, directory and computer, with its conditions and the words it
  would keep as the owner's (`scope.quote`); or, for `delegate`, that Always stores no rule and turns nothing on and
  only puts the suggestion away for 30 days. That sentence names no key and no place: the hub's own line above it
  already says what the setting does.
  A condition or an action the build does not know is shown as the hub's word.
- **Two buttons**, single-line: **Not Now** and **Always**
  (`POST /api/v1/assistant/rule-suggestions/{id}/not-now` / `always`). While one is out, neither takes a tap.
- **After Always** the line becomes what was made and where it is listed and removed: Auto-Start Tasks in the
  assistant's profile; for a standing authorization, Standing Authorizations in the assistant's profile (#7002); for `delegate`, that nothing was stored or turned on and that it is turned on in the assistant's
  configuration file on the hub's computer, with the key in brackets, as the hub's recommended-setup line for the
  same mechanism words it. The result is kept by card with the suggestion it answered
  (`RuleSuggestions.made`), so it stays when the hub sends the card again without the line. A to-do's card is its
  decision round (the to-do's id with the ask's `asked_at`): the result is not drawn under the to-do's next
  decision, and another suggestion on the same card is offered in the result's place.
- **Answered or stale**: Not Now takes the line away. A 404 (the answers no longer earn it, or it was answered on
  another device) clears it without a message. An answered suggestion is also hidden on every other card that still
  carries it, for an hour (`RuleSuggestions.quietFor`), which bridges the time until the app's copies are without
  it; the hub itself keeps it quiet for 30 days. After an answer the to-dos are read again (the hub words a decision
  on every read) and a pending card comes again without the line. For a plan the hub sends no event, and a feed read
  asks only for what came since the newest item, so the app takes the answered suggestion off every plan it holds
  itself (`TasksStore.ruleSuggestionSettled`); otherwise the line would come back after the hour. A 404 on Always
  after an Always whose answer was lost on the way is cleared like any other 404, without saying a rule was made
  (the app cannot tell the two apart). A 409 (the standing authorizations changed meanwhile) and any other failure (403 for a device
  without full access, 503) say so in a toast and leave the line.
- **Not offered**: a suggestion without an id or words, or whose `creates` this build does not know. It cannot say
  what the tap would make, so it draws nothing and the card is as before; the card itself is never dropped
  (`RuleSuggestion` decodes without failing).
- **Not drawn**: the watch, the notifications' quick actions and the card previews in lists show the card without
  the line (`SessionlessCardSheet` draws the panel, so it has it). A plan's line follows the plan as the app last
  got it: the `task_plan` event of a new plan and every read of its feed carry the suggestion.

**A decision held for the briefing** (`docs/assistant/todos.md`, "Decisions that can wait"). A to-do's `ask` with
`for_briefing: true` was not pushed and comes with the next briefing. `TodoAsk.forBriefing` reads it and
`BriefAcceptRow` says "Comes with your next briefing" under the row's meta line, wherever a to-do's decision is drawn
(the board, the action tray; `DecisionRow` says it above its buttons). It is answered as any decision, and Needs You counts it as before. The
briefing's own rows never carry the mark.

Mock hub: `MOCK_RULE_SUGGESTIONS=1` (with `MOCK_TODO_ASKS=1` the PR check's decision, the network tests' question card,
and with `MOCK_TASKS=1` the open plan; the two routes answer once, then 404) and `MOCK_ASK_HELD=1` (with
`MOCK_TODO_ASKS=1`, the PR check's decision is held); `python3 -I scripts/mock/test_availability_followups_mock.py`
checks them. Tests: `RuleSuggestionTests` (reading from each card, a hub from before, odd shapes, the wording, and every
answer the two buttons can get with `FakeHub`) and `TodoDecisionCompletionTests.aDecisionHeldForTheBriefingSaysSo`.
`AvailabilityFollowupsUITests` runs against the mock with `MOCK_TASKS=1 MOCK_TASKS_NO_ASKS=1 MOCK_BRIEF_SETUP=1
MOCK_RULE_SUGGESTIONS=1` (the plan's line and Not Now; it skips against a mock without them). The to-do's decision, the
question card and the held mark have unit tests only.

## Source mentions in the assistant conversation (#2130)

Display provenance does not change model input roles. Relayed conversation messages and automated updates use the
shared `SourceMessage` component on iOS and macOS, and `WatchSourceMessage` on Watch, never a right-aligned user bubble.
The existing single-conversation navigation and Assistant / Agents tabs stay unchanged: no group title, participants,
announcement or per-message avatar.

- `MessageSource.of` follows the source contract in `docs/protocol/protocol.md` ("Message source"): explicit `source`
  wins, then persisted `kind="message-source"` context, existing `injected`, and exact legacy relay envelopes.
  Kinds are `conversation`, `patrol`, `followup`, and `supervision`; daily/interval followups map to `patrol`.
  `conversation_id` and `title` are optional. Unknown kinds remain external sources, and malformed optional
  fields do not drop an item. `Item.source` retains the JSON object for cache round-trips.
  The first persisted metadata block is authoritative: `{"author":"user"}` or malformed metadata prevents nested
  source inference. Provenance blocks never become selectable Context cards; genuine selected contexts remain available.
- With an older server, only a complete leading relay context with the fixed non-user delivery sentence is recognized.
  Exact scheduled envelopes reuse `Injected`; supervision requires both its exact header and non-user notice.
  App-sent text with `client_msg_id` is never heuristically reclassified. A bare clock, clipboard, title or quoted
  protocol example is not a source. This fallback is a presentation heuristic, not authentication.
- The accent `@source` label appears only on the first adjacent row of the same source identity (kind + conversation
  id, or title when no id exists). A reply, genuine user input, work row or date separator starts a new run. Grouping
  is prepared after final row ordering rather than in lazy-row lifecycle callbacks.
- Long content starts at three lines with Expand / Collapse; short messages have no extra control. The source side
  uses design-system surface/type/color tokens in both appearances. These attribution corrections apply with the
  optional new conversation layout on or off. Genuine user bubbles are unchanged.
- Source rows are excluded from the genuine-input edge navigator and pending-message text matching. `ChatCache` v12
  prevents old cached copies without provenance from keeping the user-bubble presentation.
- Verification uses `MessageSourceTests`, `MessageSourceUITests`, and the loopback-only synthetic fixture
  `scripts/mock/message_sources_hub.py` (`--legacy` omits source metadata, `--watch` restricts the session inventory).

## New conversation layout (#1666)

Persona's main conversation on iPhone and iPad always uses this layout (#2477); there is no settings entry or switch.
Legacy `ui.newChatLayout`, `ui.newChatLayout.fold` and `ui.newChatLayout.lines` values are ignored, including launch arguments. They are not rewritten, so older versions retain their preferences.
The Agents chat, the Mac and Vision Pro keep their platform-specific presentation (`ChatLayoutSettings.layout(assistant:)` returns `ChatLayout.off` there).
`ChatView` hands rows `\.chatLayout` through the environment; no row observes UserDefaults (`Persona/Views/Chat/ChatLayout.swift`).

- **Who talks to whom**, from the hub's existing fields only: feed `routed` that Persona wrote (`from` `assistant`, #1542) reads "Persona → @task" (`RoutedExchange`; other platforms retain #1542's folded `AgentSentLine`), what the user sent a task (`routed` without `from`) "Me → @task" in the user's box (`UserRoutedExchange`), feed `report` reads "task → @Persona" (`ReportExchange`),
  while supervision, scheduled runs, follow-ups and wake-ups use the source mentions above, on the source's side, never where the user's messages are.
  Task notices, node-restart and Inbox lines keep their centered line. The addressee is in the accent; there is no group name, member list or avatar per speaker.
- **Boxes**: every message sits in a box, the user's pale blue (`Theme.ChatBox.user`) and Persona's, its exchanges' and its notes' pale gray (`Theme.ChatBox.persona`), words in the primary color in both modes (`ChatLayoutTests` checks the contrast). The answer streams in the box it settles in.
- **Folding**: what Persona and a task say to each other always starts with its first three lines and Expand / Collapse under it (`ExchangeText`, measured against the whole text, so a short one has no button). Expanding one message does not change the default for other exchanges.
  No evidence list under a report (it was not tappable); web links in the words are links (`ExchangeText.linked`).
- **Right-edge navigator** (`ChatEdgeScrub.swift`), like the ChatGPT desktop app's: a finger held still for a moment (`Theme.Motion.edgeScrubHold`, 0.3 s) in the band along the list's right edge (`Theme.Size.edgeBand`, inside the list's side margin, below the top bar) shows one tick per message the user typed
  (not injected lines, other conversations' messages or /loop ticks), a light tap as it opens and a selection tick per message passed, with the picked message's first two lines beside it. Sliding up or down picks another message; letting go brings the picked message to the middle of the list and lights it up.
  Letting go before the finger has gone past `Theme.Space.s` to another tick than the one it opened on only closes the navigator (`EdgeScrubState.moved`): a finger held still trembles, and dense ticks are a few points apart.
  It is a UIKit recognizer: it fails at once for a touch outside the band and for any move past `Theme.Space.s` during the hold. A swipe that starts in the band therefore scrolls the list, or pages and opens the drawer when it goes sideways (`DrawerPan`), as it would anywhere else (#2615).
  The list's scroll view pan waits for it to fail (`shouldBeRequiredToFailBy`), which is why the hold is needed: without it, the navigator took ordinary scrolls that started at the edge. The system's back swipe is at the other edge.
  With more messages than fit, the newest that fit are shown. `-showEdgeScrub N` holds it open on the Nth newest message for screenshots.
- Tests: `ChatLayoutTests` (unit: fixed defaults, ignored legacy preferences and launch arguments, platform presentation outside Persona's conversations, patrol kinds, tick placement, a release only after a move counts as a pick, links, contrast); `NewChatLayoutTests` (UI, mock `MOCK_EXCHANGES=1`, see `docs/operations/testing.md`): headers, three-line folding despite legacy preferences, no evidence list, removed settings entry and deep link, a held slide along the edge, a quick swipe at the edge that only scrolls the list; `ChatFollowTests.testPersonaReopensWhereItWasLeft`: the reading position after a restart.

## Connection status (slow / offline networks)

Like Telegram: network trouble shows no banner; **the page title becomes "small spinner + status"**, then silently returns when recovered (requested 2026-10-29; the old list-top
`ConnectionBanner` was removed).

- Three states (`Persona/App/ConnectionStatus.swift`): **Waiting for network** (`NWPathMonitor` says the phone is offline), **Connecting…** (network available but WebSocket disconnected,
  reconnecting, or connected but ping unanswered for 5 seconds / answered after more than 5 seconds = slow network), **Updating…** (just connected, catching up: complete only after pending interactions and backlog for open chats arrive,
  at most 8 seconds). Unpaired and backgrounded Apps (socket intentionally disconnected) are not problems.
- Debounce: trouble must persist for 1.5 seconds before showing; reconnects completed within a second, such as foregrounding or switching networks, show nothing; restore the title immediately on recovery.
- **A connected socket drops: stay silent first** (invisible hub restart, #1634, `docs/operations/hub-restart.md`): `LiveConnection` records the drop (`drop`),
  and during grace (`inGrace`: 8 seconds normally, 30 seconds after `hub_restarting` or close code 1001) still counts as connected: titles, status rows, Settings dots and the assistant page's device icons remain unchanged,
  messages send normally (failures retry with the same id within grace, without becoming clock bubbles). If grace expires without reconnecting, show the title status immediately, with no additional 1.5 seconds.
  Reconnect timing (`LinkDrop.retryDelay`, `Shared/Reconnect.swift`): during restart first at 150–400 ms, then every 0.5–1 second, no backoff for 30 seconds; ordinary disconnect first at 300–600 ms, then doubling from 1 second to 20 seconds (±20%).
  Resume: remember hello's `event_stream` and the largest received `event_seq` (`EventCursor`, retained across reconnects, cleared on hub change / re-pairing, not persisted); put them in the socket URL on reconnect.
  When hello says `resumed: true` (`ReconnectPlan`), do not refetch interactions, Inbox, to-dos, tasks, Lark or the Agents list, and do not show "Updating…"; just register the phone and resubscribe to open chats (`since_seq`) as usual,
  keeping the answer being generated until the hub's replayed `delta` continues it. Non-`resumed` reconnects do the full refresh as before.
- Slow network: only a timely pong clears "Connecting…" (late pongs still count as slow); ping every 5 seconds while slow so recovery is detected quickly,
  otherwise the title flashes every 20 seconds (observed). The socket still waits for 45 seconds with no messages before declaring it dead and reconnecting.
- Network recovery (path changes from unavailable to available) reconnects immediately without waiting for backoff (up to 20 seconds); switching networks (Wi-Fi ↔ cellular) pings immediately, exposing a dead connection on the old interface within seconds.
- During hub self-update (`hub_update` `restarting` / `confirming`, `docs/operations/hub-update.md`), disconnects are expected: "Connecting…" becomes "Hub updating…", and the opened explanation describes the restart.
- Placement (`ConnectionTitle` / `ConnectionLabel`): center of the navigation bar (`.principal`) in Agents (normally no title, title area empty), Inbox, and Settings › Machines;
  in the Persona tab, the name capsule below the avatar becomes the status (menu adds "Connection details"); the chat's expanded header keeps the session name but replaces the subtitle with status (like Telegram), while its collapsed small capsule is replaced entirely.
  Tap status for an explanation (`ConnectionDetails`: cause, last failure, seconds until retry, last connection, queued message count, "Retry now").
- **Addresses in Connection Details** (#11390): under the explanation and its lines, **Addresses** lists one line per address the app tries for the
  hub-level group, in the order tried (`HubRoute.candidates`: the center first, a hub's paired address before its added ones, then the addresses only the
  mesh lists). Each line is the address, then what its last try came to as a `Chip` (**Answers**, **Certificate Not Trusted**, **Not Your Hub**,
  **Couldn't Connect**, **Unknown**) and the time of the last try, bare beside it (VoiceOver says "Tried at <time>"); the address the socket is on says **In Use** (an outline `Chip`, as Settings › hub › Addresses),
  and one not tried yet in this launch says "Not tried yet". What each word means, and where it comes from: `docs/operations/network.md` ("Client-added candidates", iOS).
  The outcomes come only from tries the app makes anyway (no probe, timer or polling of its own) and are kept in memory (`HubAddressBoard`, `apple/Persona/App/HubAddressBoard.swift`);
  the lines are worked out when a try ends, not in the details' `body`, which runs every second, and are drawn by their own view (`ConnectionAddresses`) with the address as their id.
  The title keeps saying "Connecting…": nothing on the primary screens changes.
- Offline actions:
  - **Send messages**: when unreachable, make no request; the bubble has a clock and "Sends automatically when connected" (tap the clock for "Retry now / Cancel send"); requests failing offline / on timeout also become waiting,
    with no error alert. After reconnect and "Updating…" completion, send automatically in composition order (for unopened chats, first fetch one page of latest messages; delivered ones appear in the transcript and are removed automatically).
    Resends use the same `client_msg_id`, preventing duplicates. Waiting messages persist (`Outbox.swift`) and survive App termination;
    so do sent ones still waiting for their transcript copy (back as sent, not sent again), unless the chat's cache already has the copy (#1455).
    Waiting has no limit while the link is down. While it is up, a message whose sends keep coming back waiting (no answer, or its chat missing from the session list)
    fails after a minute (`OutboxWait`), and one retry per group runs every 10 seconds until then; when the hub still does not list the chat, all its waiting messages fail with one toast.
    Waiting messages older than a week do not come back at launch.
  - **Op id**: messages carry `X-Persona-Op-Id` (the message id, equal to `client_msg_id`); each answer / approval tap gets one;
    the hub executes it once (`docs/protocol/protocol.md` ("Operation receipts")). A duplicate request returning `202 {"state":"pending"|"unknown"}` is not success: pending keeps retrying, unknown is treated as "May already have been delivered".
    Retrying a "May already have been delivered" message first checks `GET /api/v1/ops/{old id}`: `done` means sent, no resend; otherwise resend with a new id as before.
    A message the hub refused with nothing typed (`delivered: "none"`: a draft in the computer's input box, a dialog open…) goes again under a new id too, on Resend and on Clear and Send:
    the hub answers the refused id with that refusal without asking the computer again, so a Resend after clearing the box on the computer kept getting the old alert (#6425).
  - **Approve / answer**: if a request dies in transit (connection refused / reset, timeout, 502 / 503 / 504 not issued by the hub), keep the card submitted and retry with the same op id for at most 60 seconds (`OpRetry`), then restore the card and show a notice.
  - **Other one-shot actions**: never queue them (computer state may already have changed). Offline cards remain unchanged; failure restores the card,
    with a blue "Can't reach the hub / No network; that action did not take effect" notice (at most one per 4 seconds), not a red error.
  - Offline / unreachable-hub errors (`Error.isOffline`) are hidden in pages like timeouts (`Error.shown` is nil); background refresh and history-loading failures show nothing, as the title already explains it.
    The watch lacks this title and keeps showing errors as before.
- Debugging: `-showConnection waiting|connecting|updating` pins a state; add `-openConnectionDetails YES` to open the explanation, and
  `-showConnectionAddresses YES` to give the hub-level group's addresses every outcome in turn, made up and kept (screenshots; real tries are not taken while it is on);
  `-simulateOffline YES` makes NWPathMonitor treat the network as unavailable (the simulator always has the Mac's network). Mock slow / offline switches are in `docs/operations/testing.md`.

<a id="ssh-隧道直连不通时的后备891"></a>

## SSH tunnels (fallback when direct connections fail, #891)

When a device cannot reach the hub address, connect through an SSH server. For the mechanism, server setup and security, see [network.md, "Connecting to hub through SSH"](../operations/network.md#经-ssh-隧道连-hub直连不通时的后备); this section covers the App side.

- **Code**: the tunnel itself is in the shared package `apple/Packages/PersonaSSH` (swift-nio-ssh + Network.framework, shared by iOS and Mac, not linked into the watch);
  App-side `Persona/App/SSHTunnels.swift` (one tunnel per hub, `SSHKeys`, routing `AppModel.sshRoute`),
  `Shared/HubTransport.swift` (maps hub addresses to tunnel loopback addresses, `SSHRoute` configuration), `Persona/Views/Settings/SSHTunnelView.swift`.
- **Only outgoing URLs change**: `APIClient.url`, chat-item URLs, `LiveConnection` sockets, port-forwarding address construction and the Feishu video and voice player's stream URL (`LarkMedia.streamURL`) pass through `HubTransport.resolve`;
  `ServerConfig`, the pairing registry, `currentConfig` and all comparisons still use the hub's own address, so multi-hub grouping, routing and credential upgrades need no tunnel awareness.
  `HubRoute.healthProbe` uses `direct: true`, always probing the direct route.
- **Routing**: with usable SSH settings in a group (host, user, confirmed fingerprint), `routePicker` always has a value: before every connection, `HubRoute.answering` probes direct routes (even a single address),
  and only if none answer calls `SSHTunnels.open`; "Always use SSH" opens the tunnel first. When restoring a closed SSH tunnel used last time, that tunnel races the direct probes and the first success is used (#11671); an already-routed tunnel keeps direct-first selection so `watchDirect` can still move back. While tunneled, `watchDirect` probes direct routes every 2 minutes and calls `reroute()` when they work again.
- **Configuration** lives in `PairedHub.ssh` (per-hub pairing registry in Keychain); private keys are separate in Keychain (`ssh.identity`, `AfterFirstUnlockThisDeviceOnly`).
  SSH settings entered on the pairing page are saved with the newly paired hub; the tunnel opened for pairing is handed directly to it (`SSHTunnels.adopt`), and device-credential upgrading continues through it.
- **Status** (consistent with existing wording): connection details add "Route: Direct / Via SSH tunnel (user@host)"; tunnel startup failures show "SSH tunnel: <reason>";
  Settings top says "Hub connected · Via SSH tunnel"; the active hub in Settings › hub has an "SSH" tag and footer "Connected (via SSH tunnel)", with the SSH tunnel row showing Not configured / Fallback / Always / In use.
- **Background**: `suspendLinks` closes tunnels unless a call or an outside action still owns them (`closeTunnelsForBackground`). Closing SSH also cancels pending authentication and removes its start immediately; late completion cannot affect a newer start or relay lease. Foregrounding reroutes and rebuilds each group. `HubLink.api` requests await `LiveConnection.waitForRoute` before resolving their outgoing address, independently per group and without waiting for the socket hello. Cancelled route attempts cancel their waiting requests. Actual recovery latency is network-dependent; the old approximately one-second observation was simulator-only.
- **Debugging**: `-seedSSHTunnel <base64 SSHRoute JSON>` seeds SSH settings for the first hub; `-seedSSHKey <base64 32-byte seed>` uses a supplied test key (DEBUG only),
  `-tab settings -openSSHTunnel YES` opens the first hub's SSH tunnel page. End-to-end test setup is in `docs/operations/testing.md` ("SSH tunnels").

## File page (#5827)

The Files page (`Views/Session/FilesPane.swift`: an Agent's Files tab, and the computer page's Files row at the home) walks the computer's folders. The pure parts are `FilesNavigation` and `FileFind` (`FilesNavigation.swift`, `FilesNavigationTests`).

- **Where it goes**: it starts at the session's folder (the computer page: `~`) and goes anywhere the node lets it read, above the session's folder included. Back and Forward walk the folders shown, like a browser (50 kept); Up opens the parent; the path bar names every folder from the home (a house) or from `/`, each a button, with the session's folder tagged **Session** and **Read Only** after a folder outside the home (or in `~/.persona`, `~/Library`). Uploads, renames, moves and trash are not offered there; the node refuses them anyway (protocol.md "Changing files"). A folder listed in the last minutes shows at once on Back / Forward while it is read again.
- **Find**: the field under the path bar finds names under the folder shown, folders included (`fs/find`: Git's files in a repository, else a bounded walk), every typed word in the name, best first (whole name, start, word start, anywhere; folders first, shorter paths first). The web's Files panel ranks by the same rule.
- **Info** (long press › Info, ⋯ › Info for the folder shown; `FileInfoSheet.swift`): kind, location, size or item count, created / modified / attributes changed, permissions in words and as `rw-r--r-- · 644`, owner and group, Copy Path. A symlink adds what it says, its real location and **Go to Real Location**. No permissions on a Windows computer; an older hub or node shows what the listing had and says what to update (protocol.md "File details and links").
- **Symlinks** in the list carry an arrow on the icon and "Link → target"; a broken one the error mark and "Link broken · target isn’t there" in red, and a tap opens Info instead. A tap on a working link opens what it leads to with the path bar on the link's path.
- **＋** (a folder that takes changes; #5828): New Folder and New Text File, each asking for a name (a taken one is refused, nothing replaced; the new item is marked, a text file opens), then Upload from Photos and from Files. **Download** in the selection bar and a folder's long press: one file as it is; several items or a folder are zipped on the computer first (`files/zip` into a temporary folder there, "Zipping on the computer…" in the transfers bar), then saved through Save to Files.
- **Copy, duplicate, compress** (#5830): a row's long press offers Copy To… (the folder picker in copy mode, Copy Here, with New Folder in its bottom bar), Duplicate ("name 2.ext" beside it) and Compress ("name.zip", several "Archive.zip", a taken name numbered), each marked in the list when done. While selecting, the count replaces the find field and the bar under the list has five actions, each its mark over its word: Download, Copy To, Move To, Compress, Trash.
- **Permissions** (#5831; `FilePermissionsSheet.swift`): Info's Change Permissions… and a row's Permissions… open switches for read, write and run by the owner, the group and everyone else, the code under them and what it was before; Save asks first, saying what changes for whom and the codes before and after (`files/chmod`). Not for links, folders outside the home or a Windows computer.
- **Open Terminal Here** (#5832, ⋯ for the folder shown and a folder's long press, where the hub has `pane_cwd`): a new terminal tab on the computer in that folder (in the session's workspace when the page is a session's), shown over the page as the Terminals page shows one.
- **Show in Files Page** (the preview's ⋯, where the preview was opened from the Files page or an Agent's chat): the Files page at the file's folder with the file marked for a moment.

## Editing a file (#5829)

**Edit**, the first item of the preview's ⋯ where the hub writes files (`file_write`), opens a text file in the editor (`FileEditSheet`, full screen): UTF-8 text up to 1 MB read whole; otherwise Edit is off with the reason under it (too large, not text, not read whole). The editor (`Code/CodeEditor.swift`) is the system's `UITextView` on TextKit 1, whose line fragments give the gutter where each line starts: line numbers beside it, the viewer's colors (`SyntaxLexer`, worked out off the main thread after a pause in typing, put on the lines in view only), the system's find and replace (`UIFindInteraction`) and undo, and a keyboard bar: Find and Replace, Undo, Redo, Indent (the file's own: a tab, or its smallest run of spaces), brackets and quotes, Hide Keyboard. No third-party editor.

- **Save** sends the text with the file's revision as opened and the hash of what was opened (`files/write`, protocol.md "Changing files"): the computer writes only over that version. When it changed meanwhile, an alert offers **Save as Copy** ("name 2.ext", the first free name), **Discard Mine and Open the New Version**, **Overwrite Anyway** (asked a second time) and **Keep Editing**.
- **Line endings and a byte order mark** go back as the file had them; the editor works in LF (a file mixing CRLF and LF is left as it is).
- **Cancel** with changes asks to discard; the sheet does not swipe away while there are changes. After a save the preview reads the file again.

## File viewer (2026-10-31)

Files mentioned in chats, tool rows and the chat's Files page all open in `FileViewer` (`Views/Chat/FileViewer.swift`, `Persona/Code/`).

**Link only paths that can actually open.**
- **Previous problem**: every path recognized in messages (`FilePaths`) became a file block, read from the session's computer when tapped. Paths on another computer (such as the hub's `~/.persona/…`), or deleted files, only showed "Can't open this file".
- **Now**:
  1. When a message row appears, pass its paths to `FilePresence`;
  2. A moment later, batch one `GET /api/v1/machines/{mid}/stat` by "machine + session directory", resolving relative paths against the session cwd;
  3. Only file / dir / denied paths become links.
- **Cache**: by (machine, directory, path), existing paths for 30 minutes, missing paths for 2 minutes (the Agent may create them soon), unavailable checks retried after 1 minute.
- **If checks are unavailable**: before the answer arrives, with old nodes lacking the API, or offline, paths remain plain text; uncertain paths are not linked.
- **Counts**: at most 40 candidate paths in body text (`PathMarkdown` / `PathRefs`); file blocks recognize at most 12, displaying the first 6 existing ones.
- **Tool rows** (Read / Edit files) skip checks: the tool just used the file.

**Body paths are links** (#1706, same rules as web #1669, `apple/Packages/PersonaKit/Sources/PersonaKit/PathRefs.swift`, #2095):
- **Recognized**: `~/…`, absolute paths with at least two segments (`/tmp` does not count), relative paths with a directory and a letter-starting extension (`src/a.ts`; not `deepseek/v4.1`).
  Optional line suffixes: `:12`, `:12:5`, `:12-20` (start line), `#L12`; `stat` receives the path without line numbers.
  Names may be in any script (#8386). Outside backticks a path stops at whitespace, at punctuation other than `_ . @ + -` (full-width punctuation too), and before a non-ASCII word right after an extension (`/tmp/a.md里`, meaning "in /tmp/a.md"); a relative path starts with an ASCII folder; sentence-final `.` is excluded. A whole backticked span counts as one path (may include Chinese and spaces); if it does not exist, check paths within the span and select existing non-overlapping ones, longer first at the same position. Same rules and cases as the web (`web/src/lib/path-refs.test.ts`, `PathRefsTests`).
  URL fragments, `and/or` and version numbers do not count; code blocks, indented code, existing links and formulas in replies remain unchanged.
- **Rendering**: existing paths in replies (`LinkedMarkdown`) become Markdown links (`persona-path:<path>?line=N`, `PathLinkURL`), colored like other links (`.persona` link color); an entire backticked path's inline code is tappable.
  User bubbles (`PathText`) use underlined links in `Theme.UserBubbleInk.link`: accent on the main conversation's box and in light bubbles, bubble text color in colored bubbles (accent is invisible there).
  Before answers or when checks fail, plain text; conversion to a link changes only color, not font size or line layout.
- **Streaming**: the currently written chunk is not recognized (its final word might be an incomplete path); completed paragraphs (`StreamChunk`) are checked and linked normally.
  That chunk is made whole for the moment before it is parsed (`StreamRepair` in PersonaKit, ported from the web's `repairStreamingMarkdown`, #2916): an open fence closed (indented as its opener, so a fence in a list item closes in it), a half-written table row and a last line of only list / heading / quote markers held back, a table header given its delimiter row (indented as the header; a table also right under a paragraph line, as cmark-gfm reads it, but not in a lazy line of a list item or quote), and on the last line `**` `*` `~~` and code spans closed, a half-written link or image shown as its text. `_` is never closed (as the web): in coding replies it is mostly `src/_app.tsx` or `_private`, and closing it set the rest of the line in italics until the line ended. It only cuts or adds to the last line, so finished blocks stay byte for byte the same. The caret ` ▍` is added only in prose (`StreamRepair.end`), never inside a fence, a table or a formula; the chunk has its own small memo (`StreamTail`), not `MarkdownMemo`.
- **Opening**: the chat's `openURL` recognizes `persona-path:` and opens `FileViewer` (`FileRef.line`). With a line number, show source directly (rendered Markdown, JSON trees and tables have no lines), read through that line (at most 8 extra pages), scroll it to the upper third of the screen and highlight it (iPhone `Theme.lineFocus` background, Mac selected row). Directories open as directories.
- **File blocks**: inline paths no longer repeat as capsules below the message; non-inline ones (only in code blocks, beyond the count limit) keep capsules; document cards and image / video / audio capsules remain.
- **Not done**: a long-press menu on individual paths (copy path / open in Files) and Mac hover "path · Line N": links inside SwiftUI `Text` have no individual long-press or hover handling; message long-press opens the whole-message action bar. Copy path is in the file viewer's ⋯ menu.

**Code blocks, inline code and quotes** (#2984, as Telegram draws them; `Views/Chat/ReplyBlocks.swift`, the `.persona` theme; the frozen Mac keeps its hover Copy and plain quotes):
- **Code block** (`ReplyCodeBlock`): a title bar (`Theme.codeBar`) on top of the block with the fence's language (nothing when it names none) and Copy, which copies the whole block (trailing line breaks dropped) with a light haptic and the "Code copied" toast. The code under it is monospaced (`.fontDesign(.monospaced)`: the style's font design set on the root won over MarkdownUI's monospaced variant, so code blocks were drawn in the body font) and scrolls sideways as before, so a sideways drag on a wide block still goes to the code. A block still being written gets its bar at once.
- **Inline code**: on no background (#9477), monospaced as the code blocks are, at 0.94 em (`ReplyMetrics.inlineCode`; SF Mono's x-height is the body's, its wider letters made it look larger), in `Theme.Reply.codeInk`, a step quieter than the body; words in an ink of their own (a quote, H4–H6, an agent's quiet words) keep theirs for their code, and code in a written link is the link's (`InlineCodeInk`). Chinese and Japanese inside it fall back to the body's system face (PingFang SC / TC / HK, Hiragino Sans), SF Mono's own cascade. Where a code span meets a Han, kana, Hangul or bopomofo letter directly, a thin space (U+2009, about 0.12 em) goes on that side before parsing (`InlineCodeGap` in PersonaKit, beside `CJKEmphasis`), none beside a space or CJK punctuation; Select Text's overlay adds the same room, which copies as nothing. The style's font design in the environment replaces every run's font, monospaced and named ones included, so the blocks that draw words (paragraphs, headings, table cells) clear it and give it to their runs as their family instead (`RunFonts`); body text keeps the style's design. In a chat a tap copies it ("Copied"): its spans are made links of their own (`persona-copy:`, `CodeCopyURL` / `CodeCopyMarkdown` in PersonaKit) that the chat's `openURL` answers; the code style is applied after the link's, so they keep the code's ink. Blue is for what opens: a path's code span that is there is linked as its words (`PathMarkdown.linked(plainCode:)`), not as code, so it stays a blue link that opens the file. A long press still lifts the message. The Mac keeps its inline code as it was.
- **Quote** (`ReplyQuote`): the accent bar on the left of a faint accent card (`Theme.quoteBackground`), a quote mark at its top right (`Theme.Icon.quoteMark`); its words keep clear of the mark and the chevron by a room that grows with the text size. One more than a line taller than `Theme.Size.quoteFold` (about four lines, scaled with the text) is folded with its end faded: a tap on it opens it, its chevron folds it again. A reply still being written (`replyStreaming`) never folds its quotes.

**Math and diagrams** (#1295, `apple/Packages/PersonaKit/Sources/PersonaKit/MathMarkdown.swift`, `Views/Chat/RenderedMarkdown.swift`): chats, plans, Markdown in Files, and Knowledge person pages render formulas (KaTeX 0.16.22) and Mermaid diagrams (12.3.0), both MIT and bundled under `Resources/Render/` (with their LICENSE files), without networking.
- **Recognized**: ```math (also latex / tex / katex) and ```mermaid code blocks; `$$…$$`, `\[…\]` over standalone lines or as a line's sole formula are block math; `$…$`, `\(…\)` are inline math. `$` immediately followed by whitespace is not an opening delimiter; preceded by whitespace or immediately followed by a digit is not closing (pandoc rules); Chinese or Japanese inside disqualifies it (except `\text{…}`); `\$` is a dollar sign, so "$5 to $10" remains text. Code blocks, inline code and indented code are untouched.
- **Rendering**: a hidden WKWebView loads bundled `render.html` (CSP allows only bundled scripts, styles and fonts; a rule list blocks all network requests), renders one item at a time and snapshots it; content is passed as arguments to `renderMath` / `renderMermaid`, never concatenated into the page. Images are cached in memory and on disk by (kind, source, light/dark, font size). Formulas render black as template images following text color, sharing one image across light/dark; diagrams render once for each mode (Mermaid default / dark themes). Inline math uses MarkdownUI inline images (`![…](persona-math:…)`, `MathInlineImages` attached at the root); block math and diagrams replace code blocks.
- **Failure**: block math and diagrams fall back to code blocks with KaTeX / Mermaid's message below; inline math shows monospace source. Tap diagrams for full-screen viewing (`ImageViewer`).
- **Streaming and the queue** (#2916): a ```math or ```mermaid fence still open in the block a reply is writing draws as plain code until it closes (`StreamTail`, `StreamRepair.plainOpenFence`); before, its half-written source went to the renderer at every draw and showed a spinner or "Couldn't draw this". Requests for one picture share one drawing; a block's request is dropped when its view's task is cancelled before its turn (one already drawing finishes and is kept); inline formulas wait as before (MarkdownUI's inline images would show the TeX source for a cancelled load). Failures are kept by key, at most 256.

**Images viewed this turn and document cards** (#1543, `Views/Chat/TurnAttachments.swift`, `ViewedImages.swift`):
- **Viewed images**: images the Agent viewed during the turn (rules in `docs/protocol/protocol.md` ("Images viewed this turn"); `ViewedImages` follows the hub's `transcript.ViewedImages`) appear in their own row between work rows and the reply body (`ChatLine.viewed`). New streaming images only extend this row horizontally; reply text does not reflow and following stays intact.
  - No explanatory text, only thumbnails: multiple images use a horizontally scrollable 64pt-high strip extending to screen edges with a fade on sides containing more; one image uses an 80pt-high thumbnail. Screenshots and webpage screenshots have a small bottom-left marker.
  - Width follows aspect ratio (¾ to 1.6 times height): phone screenshots narrow, desktop screenshots wide; dimensions from lazy-loaded images reserve space before arrival.
  - Names and sources ("Screenshot · Studio", "Home.png · 480×300") appear only in long-press previews, Mac hover and the gallery.
  - Thumbnail sources: embedded images use `ChatImageLoader` (lazy loading, size-based downsampling, memory + disk cache); path-only images use M0 `file/thumb` (`FileThumbnails`).
  - Tapping opens this turn's gallery (`ImageViewer(items:)`): swipe left/right, save, share, copy image; paths additionally offer copy path and "Open in Files".
  - Folded work uses `fold.images`; expanded work uses actual tool calls, with identical results.
- **Document cards**: Word / Excel / PowerPoint / iWork / PDF files mentioned in replies (`FileThumbnails.isDocument`) become cards below reply text, not capsules: first-page thumbnail (M1 `FileThumbnailIcon`), filename and type ("Word document"). Tap for file preview, long-press to copy path. Other files keep capsules.

**Links inside previews**

The file viewer opens http(s) links within its own presentation layer: on iPhone the link card sits above the file preview; on Mac the link panel is inside the file preview window. Closing the link returns to the original file and preserves the reading position; the chat's underlying link drawer must not wait until the file closes to appear. Both chat file blocks and the Files page pass the original `SessionKey` to the preview, so AI editing / adding context inside links still belongs to the original session; standalone previews without a session can also browse websites. Development services on localhost / machine addresses are forwarded through the hub to the computer holding the file; other URL schemes keep the platform's existing policy.

**Display**

| File | Display |
| --- | --- |
| Code, config, logs | Language highlighting, line numbers, monospace without wrapping, horizontal scrolling. Line numbers stay fixed on the left; long-press a line to "Copy line / Copy all" |
| JSON | Source first; after the whole file is read (at most 24 MB) and parsed, switches to a collapsible tree view (key order unchanged), see below |
| Markdown | Rendered by default with the chat's renderer (`.persona` theme, `FittedTable`, `Links`); code blocks use language highlighting. The menu offers "View source" |
| Images, PDFs, directories | As before |
| Video | `video/*` or common video extensions use the native player; buffer the index and the segments needed to start playback, then play while reading, with pause and seeking. Seeking to an uncached position reads segments near the target keyframe directly; failures and unsupported codecs explain why. Closing the preview pauses the player, cancels reads and clears temporary caches |
| Audio | System-playable audio such as mp3, m4a, aac, wav, flac, aiff, caf (`audio/*` or extension, #1298), downloaded to this preview's temporary file before playback (`AudioFileView`): filename, play / pause, skip forward / back 15 seconds, draggable progress and remaining time, 0.75–2× speed; unsupported formats explain why. Closing the preview stops playback and clears the temporary file |
| HTML | .html / .htm default to an isolated preview (`HTMLFileView`, #1297): a separate WKWebView with no persistent website data, **scripts off, network off** (a rule list blocks every load except the page itself and data:). The page cannot navigate itself; tapped links use the App's link card; a line at the top states what is enabled. The menu offers "Allow scripts", "Allow network" (this preview only, reloads after changes) and "View source". Referenced files in the same directory (images, stylesheets and their url() / @import, fonts, scripts) are first read from that computer through the hub and embedded as data: (`HTMLResources`, at most 64 files, 8 MB each, 24 MB total), with networking still off; the top line says how many were read. Local dev-server site previews use port forwarding, not this viewer |
| Jupyter notebooks | .ipynb (nbformat 4, #1296) shows cells after reading the whole file (`NotebookFileView`): Markdown cells use the chat's Markdown style; code cells use the notebook's language and show "In [n]", followed by outputs: stdout / stderr (stderr orange), result text, images, errors (name, message and traceback with color codes stripped). HTML results (pandas tables) render in the same isolated WKWebView as HTML previews (scripts and network disabled, height follows content, `HTMLSnippetView`), with "Show text version" below and automatic text fallback on load failure. Outputs longer than 60 lines are initially folded. The menu offers "View source"; unreadable notebooks (nbformat 3, invalid JSON) show as text with an explanation |
| Tables | CSV, TSV, xlsx use a native table (`TableFileView`, #1294), see "Tables" below; the menu offers "View source" for CSV and "Open with Quick Look" for xlsx |
| Office, iWork, RTF | docx, doc, xlsx, xls, pptx, ppt, pages, numbers, key, rtf use system QuickLook (#1293): downloaded through `…/file/download` to this preview's temporary file, then embedded in the page (iOS `QLPreviewController`, Mac `QLPreviewView`), without another presentation layer. The menu can "Share" the file (open in another App, save to Files). Closing the preview clears the temporary file. `.key` sent as text (PEM private key) still displays as code; iWork documents saved as folders (packages) are still listed as directories |

**Thumbnails on the Files page** (#1293): for images, PDFs, videos and the documents above, when a row appears it requests a thumbnail from the computer holding the file (`GET …/file/thumb`, hub feature `file_thumb`, `docs/protocol/protocol.md` ("File preview")), sized to the row's tile × screen scale, at most three concurrently (`FileThumbnails`). Results are stored in memory and the cache directory by (hub group, machine, path, size, modification time): a changed file changes the key and is requested again. Every copy on disk is also checked once per launch with its ETag (`If-None-Match`, 304 keeps it), so a node that draws differently (#1303, JPEG orientation) replaces old pictures of unchanged files. If the machine cannot render it (`no_thumbnail`: Office on Linux, missing tools), or an old hub lacks the feature, the icon stays and no further request is made in this launch. Code and text get no thumbnails. A dark-page document thumbnail is not used; the icon is shown instead (in Mac dark mode, Quick Look gives RTF and text a dark background, making a black square in the list). Items with only a path (file cards in chats; `FileThumbnailIcon` passes size 0 and no time) are stored by path alongside the node's ETag and checked the same way: 304 reuses the old image, while a file regenerated at the same path gets a new one. The card's tile can be enlarged with `width` / `height`, and the image fits proportionally.

**Binary files return only metadata**: when the hub has `file_stream`, the Files page reads with `stream=1` and binary files return only size, type and revision; images and PDFs are then fetched in full through `…/file/download` (no longer capped at 2 MB), QuickLook still downloads to a temporary file, and videos read segments on demand (#706, see "Video" above). Old hubs still return the entire file as base64.

**Compare** (#1299, `apple/Packages/PersonaKit/Sources/PersonaKit/DocumentText.swift`, `Views/Session/CompareView.swift`): changed PDFs, docx, pptx, xlsx and images on the Git page no longer say "No text changes"; they compare the two versions, like the web (#1452):
- **Which versions**: unstaged changes compare the index (INDEX) with the working-tree file; staged changes compare HEAD with the index; files in a commit compare "hash^" with "hash". The old version is written to the node's cache by `GET …/git/blob` (hub feature `git_blob`), then read through `/file/download`; working-tree files are read directly. Added files have no old version; deleted files have no new version. Without this hub feature, the existing text diff remains.
- **Documents**: extract text and split it into units (PDF by page, Word by page break, slides one each including notes, spreadsheets one worksheet each), compare line by line (`CollectionDifference`), and put each change under its page / slide / worksheet with two lines of context on either side; unchanged text folds into "… N unchanged lines". If the text is identical, explain that formatting or images may have changed. At most 20,000 lines per version.
- **Images**: side by side, or the new image over the old one with a draggable divider.
- "Open old version / Open new version" at the bottom opens that version in the Files viewer (QuickLook, tables, PDF…).

**Tables** (#1294, `apple/Packages/PersonaKit/Sources/PersonaKit/TableParsing.swift`, `Views/Chat/TableFileView.swift`):
- **Read**: CSV / TSV parses RFC 4180 (quoted delimiters, newlines, `""`); the Files page keeps its paging and feeds the parser page by page, with a row split at a page end continued by the next page; the next page loads when scrolling within 60 rows of the end. `.csv` uses semicolons if the first row has more semicolons than commas (regions using commas as decimal separators). xlsx downloads fully through `…/file/download`, then reads zip (Compression for deflate) and OOXML itself: shared strings, inline strings, numbers (floating-point tails removed), TRUE / FALSE, error values, and date formats (built-in 14–22, 45–47 and custom formats containing y/m/d/h/s) displayed as `yyyy-MM-dd [HH:mm]`. No third-party library. Old .xls and Numbers still use QuickLook. At most 200,000 rows per worksheet.
- **View**: freeze the first row at the top (as headers) and row numbers on the left (Excel row numbers; the header is row 1); draw only visible columns. Estimate column width from the widest content in the first 300 rows (CJK counts as two cells), clamped to 56–240 pt; drag a header's right edge to resize. Numbers align right. Tap a cell to show its position (B12) and full content below (selectable), with copying; long-press offers "Copy cell / Copy row" (rows are tab-separated, preserving separate cells when pasted into a spreadsheet). Multiple worksheets get a switcher across the top.

Video does not use the first 2 MiB of binary data returned by the old API. `VideoFileModel` uses a two-byte Range request to `GET …/file/download` to discover file length and version, then `VideoFileResource` (`AVAssetResourceLoaderDelegate`) reads the ranges requested by the player. `VideoFileCache` aligns chunks to 256 KiB and writes read ranges into this preview's sparse temporary file; only recorded cached ranges are handed to the player, and logical file length does not mean downloading is complete. MP4 indexes at the end can be requested directly without downloading everything in between; seeking uses the index to find a nearby keyframe, never guesses a byte offset from a time ratio. The player controls prefetching, with a preferred forward buffer of 3 seconds; there is no separate task downloading the whole file from the start.

Authentication goes only in HTTP headers; playback URLs use a local custom scheme containing only a random identifier and filename, never the hub address, remote path or credentials. Later requests carry `If-Match`, and every chunk validates `Content-Range`, length and ETag; changed files, ignored ranges, incomplete chunks and old nodes produce errors, never silently fall back to whole-file transfer, and authenticated requests may not follow redirects. Closing the page cancels all resource requests and network tasks, releases the player, deletes the sparse cache, and checks task and preview generations to prevent late results from reviving it. Actual codec support depends on the system; H.264 / HEVC MP4 is recommended. Nothing is rewritten, transcoded or additionally generated as HLS. Both the hub and the node on the video's computer must support `file_download`.

**JSON tree view**
- Formatted display: keys green, strings, numbers and constants each have their own colors.
- Tap a container row to expand or collapse; the triangle on the left shows its state.
- By default, only the first few levels expand, stopping at about 400 total rows; a 6000-item array starts collapsed.
- Long-press a row to "Copy value / Copy path (`.forecasts[3].rain`)".
- The menu has "Collapse all" and, for smaller trees (up to 50,000 nodes), "Expand all".
- Invalid JSON (JSONL, a half-written file) displays as text with an explanation.

**Languages** (`SyntaxLanguage`)
- Swift, Go, Python, JS / TS (including JSX / TSX), Rust, Shell (bash / zsh);
- YAML, TOML, INI / .env, JSON, SQL;
- C / C++ / Objective-C, Kotlin, Java / Gradle / Groovy / Scala, C#, Ruby, PHP, Lua, Dart, Protobuf, GraphQL, Terraform / HCL, CMake;
- HTML / XML (plist, SVG, storyboard), CSS / SCSS / Less, Dockerfile, Makefile, Markdown source, diff.

Detection order:
1. Extension;
2. Filename (Dockerfile, Makefile, Podfile, .zshrc, .env…);
3. First line (`#!/usr/bin/env python3`, `<?xml`).

**The highlighter is a custom lexer (`SyntaxLexer`), with no new dependencies.**
- Why not tree-sitter: a C grammar library for every language; more than twenty makes a large package.
- Why not highlight.js: needs JavaScriptCore or a WebView, both slow and heavy.
- How it works: a generic loop over UTF-8 bytes; languages are just tables (keywords, types, constants, comment and string syntax). YAML, TOML, HTML, CSS, Dockerfile, Makefile, Markdown and diff each have a small specialized handler.
  - Keywords use byte-hash lookups, without building a String for each word.
  - Block comments, multiline strings, HTML comments and tags, Markdown code blocks and Dockerfile continuation lines carry state to the next line through `SyntaxState`.
- **Colors**: design system `Theme.Syntax` (GitHub colors). Code backgrounds are identical across interface styles and theme colors, so syntax colors do not change either; every color has ≥ 4.5:1 contrast against the code background in light and dark modes (`SyntaxPaletteTests`).
- **Pitfall**: in `emit(s, i, word(…, &i, …))`, Swift reads the argument `i` by position before calling `word`, leaving the macro's `!` outside the token; store `word`'s result in a variable first.

**Performance**
- **Fetching files**: the node reads text in pages of at most 2 MB, ending at a line boundary, and scrolling further reads the next page (`offset` / `next`). JSON is read in full because it must be parsed as a tree.
- **Splitting lines**: split bytes into lines in the background first (`CodeDocument`), then display plain text immediately.
- **Highlighting**: background processing from the top in chunks of 2000 lines; after each chunk, refresh the visible lines. At most 100,000 lines or 8 MB are highlighted; the rest stays plain text with "Only the first N lines are highlighted".
- **Very long lines**: display truncated after 4000 bytes (minified JS / JSON is often one line).
- **Code view** (`CodeLinesView`): creates labels only for visible lines and reuses them while scrolling, so tens of thousands of lines are as light as ten. Not the old "whole document in one UITextView" approach, whose layout cost grows linearly with file size.
- **Markdown**: split between top-level blocks into chunks of about 12 lines (`MarkdownChunks`, never inside code blocks or indented lines), placed in LazyVStack so only visible chunks are laid out. Highlighted code blocks are cached by content.
- **No text selection**: rendered Markdown does not enable text selection. Measurements showed that enabling it slowed opening by about 40% (607 ms versus 435 ms), and long frames when a chunk entered the screen grew from about 40 ms to about 60 ms. To copy, switch to source view or use "Copy content" in the menu.

**Measurements** (2026-10-31)
- **Environment**: iPhone 18 Pro simulator (iOS 27, Mac M1 Ultra), `-O` optimized Debug build, `MOCK_FILES=1` mock hub over local loopback.
- **Method**: `FileViewerPerfTests` with `-showFilePerf YES -showFileAutoScroll YES`, timed from opening the file; scrolling runs at a constant speed inside the App, down for 4 seconds then up for 2 seconds.
- **Why not XCUITest drags**: before each gesture XCUITest captures an accessibility snapshot on the App's main thread, itself causing 40–100 ms long frames.
- The "Scrolling" column below gives frames per second and the longest frame during those scrolling seconds; the simulator runs at 60 Hz.

| File | Size | First page received | On screen | Highlighting finished | Scrolling at 4000 pt/s |
| --- | --- | --- | --- | --- | --- |
| Go, 20,000 lines | 795 KB | 178 ms | 273 ms | 257 ms | 60 fps, longest 16 ms |
| JSON, 6000 entries (two pages) | 2.3 MB | 312 ms | Source 439 ms, tree 494 ms | 460 ms | Tree with 6000 items expanded: 60 fps, longest 22 ms |
| Log (first page 2 MB, 5 MB total) | 5 MB | 306 ms | 432 ms | No highlighting | 60 fps, longest 16 ms |
| Markdown, 4176 lines (with tables and code blocks) | 169 KB | 258 ms | 566 ms (rendered) | 365 ms (source) | 52–58 fps, longest 38–54 ms |

Two more points about Markdown:
- Scrolling at reading speed, 1500 pt/s, gives 52–58 fps with a longest frame of 38–50 ms; slow frames all happen when a new chunk enters the screen (MarkdownUI laying out tables and code blocks).
- At both speeds, the first second of scrolling has one frame of about 0.24 seconds.

Source view, code, JSON and logs all stay at 60 fps.

**Tests**
- **Unit tests**:
  - `SyntaxTests`: tokens in each language, multiline state, detection, wide-character offsets;
  - `SyntaxPaletteTests`: syntax-color contrast;
  - `FileViewerTests`: line splitting, continuing colors across pages, chunked highlighting and limits, JSON parsing, default folding, row text, Markdown chunks, batched path lookups and caching.
- **UI tests** `FileViewerUITests` (requires `MOCK_FILES=1`):
  - File blocks only for existing paths;
  - Go opens with line numbers;
  - JSON is a tree, with expandable arrays;
  - README is rendered.

## Developing Persona (#9460)

Settings › About's switch that records the device's logs, errors and energy use (MetricKit, thermal state, battery,
requests and socket frames a minute) and exports them as one zip for an Agent: [developing-persona.md](../operations/developing-persona.md).
Code in `Persona/DevMode/`; shared code reports through `Shared/DevProbe.swift`.

## Error reports (#5621)

iPhone, iPad and Mac crashes go to the hub's `error_reports` (format and rules: [protocol.md](../protocol/protocol.md) "Error reports"). Everything is in `Shared/ErrorReports.swift` (the report builder, the disk queue and the sender); `AppModel.flushErrorReports` wires it to the connection. There is no UI.

- **Crash**: an uncaught `NSException` handler, installed first at launch (`AppDelegate` on iOS, `applicationWillFinishLaunching` on the Mac), writes the report synchronously (kind `crash`, level `fatal`, `previous_launch` true: it is sent at the next launch) and then hands the exception to the handler that was there before. On the Mac it rarely fires: AppKit catches an exception raised on the main thread (it logs it and the app goes on) unless `NSApplicationCrashOnExceptions` is set, so only exceptions on other threads reach the handler.
- **Content**: the exception's name (`message`) and reason (≤ 1 KB), and `callStackSymbols` as frames, innermost last, at most 64: the symbol (or the image's address and offset) as `function`, the image as `module`, `in_app` for the app's executable (`Persona.debug.dylib` too), PersonaKit and PersonaSSH. There is no `file` (not symbolicated on the device). `release` and `build` come from the bundle, then `os`, `os_version` and `device_model` (the hardware model identifier; the simulated one on a simulator); `source` is `ios` or `mac`. No device id, name or account.
- **Queue and sending**: one JSON file a report in Application Support `error-reports/` (`<time ms>-<id>.json`), at most 50, at most 7 days old. A file, not UserDefaults: it is written right before the app dies. It is sent only to the hub of the group the hub-level pages show (the one picked, else the first), after each hello and each `machines` message; when its hello's `features` has no `error_reports`, the queue is deleted. Batches of ≤ 20 reports and ≤ 240 KB through `APIClient`; 2xx deletes the batch (and `{"enabled": false}` deletes the rest); 4xx drops it, except 408 / 429; 5xx or no answer keeps it for the next time. A retry sends the same `id`.
- **New Chat check** (#2189): New Chat taken from a Persona conversation watches for the new-conversation page (`NewChatCheck`, Release too). When it has not appeared 1.5 s later, a report of kind `log`, level `warning` goes into the queue with one line of labels only: `new_chat_check from=<main|other> current_early=… current_late=<main|other|none> drawn_late=<chat|home>` (no text, no ids). The latest line, with its time, is also kept in UserDefaults `diag.newChat` and shown at the foot of Settings › About ("Last New Chat Check"), which has no such row until one was written: a hub without an `error_reports` extension drops the queue.
- **Not yet** (#5621 phase 2): signal crashes, MetricKit crash and hang diagnostics, the Mac's main-thread exceptions (`NSApplicationCrashOnExceptions`, or reporting what AppKit catches), breadcrumbs, the watch (through the phone) and visionOS.

## Extension pack switches (#1830)

On iOS and visionOS, an extension pack's Enabled switch explains when members follow or will be restored (or, on an older hub, that only the pack changes), asks before disabling enabled members, and then uses the existing dependent confirmation if the hub requires one.

## Extensions and connectors of several managers (#1827)

[managers.md](../assistant/managers.md) ("Grouping"): with more than one installed manager, the hub's merged lists stamp each item with
its `manager`, and every manager lists its own built-ins, so the same id comes once per manager. With one manager, or a hub without a
roster, none of this draws and the pages are as before.

- **Settings placement** (#2522): Settings has one Connectors entry. Its page reuses `ConnectorsSection`, including all manager groups, and ends with Records sent as you. The ledger is a secondary destination; `[.connectors, .sent]` keeps Back and the Mac selected Settings row on Connectors, including legacy launches and native settings links.
- **Roster**: Settings › Extensions (`ExtensionsModel`) and Settings › Connectors (`ConnectorsListModel`) read `GET /api/v1/managers`
  with their list, and again on `{"type":"managers"}` for Extensions. `ManagerRoster` (`Persona/Views/Settings/ManagerGroups.swift`)
  has `installed` (the default first, then the roster's order, installing ones left out), `isMulti`, `owner` / `named` (no stamp, or a
  manager the roster doesn't have installed, is the default's) and `groups(items)`. A roster that can't be read keeps the last one; a hub
  without one (404) groups nothing.
- **Grouping**: each manager's items are a section under `ManagerHeading` (dot, name, count, and "Offline · last synced …" while
  offline), empty groups left out; the list's footer goes under the last group. In Connectors the device, mail, Lark and Lark Bot rows
  stay in the Connectors section and the hub's other connectors follow, grouped. Rows are keyed by id and manager (`listKey`).
- **Routing**: navigation carries the manager (`SettingsPage.plugin(id, manager:)`, `.connector(id, manager:)`,
  `.pluginPage(…, manager:)`, `MessagesChatsRoute` / `MessagesThreadRoute`; nil for the default's). Each page's model keeps its
  manager and makes every request through `APIClient.manager(_:)`, which adds `X-Persona-Manager` (nothing for the default): an
  extension's details, switch, settings, secrets, auto-update, provided features, sign-in, its connectors' Show in Inbox; a connector's
  status, sign-in and sign-out, switches, Delete Saved Data / Knowledge, GitHub's sign-in source and Inbox options, and Messages.
  Extension pages get the manager too (`ExtensionPageView(manager:)`, `?manager=` on their actions). Another manager's extension or
  connector page adds that manager's name to its status line (`ManagerRoster.name`); the default's pages are as before.
- **Kept apart**: sign-in states (`ExtensionAuthStore`) and unsaved settings drafts (`ExtensionDrafts.Key`) are kept by id and manager.
  The market (installs, upgrades, uninstall) is still the default manager's; its pushes are matched by manager (below).
- **Tests**: `ManagerGroupsTests` (decoding, roster order and ownership, grouping, one manager, a hub without a roster, the header on
  every request of another manager's extension, sign-in, connector and Messages pages and none on the default's). UI test
  `ManagerGroupsUITests` against the mock hub's three managers (`MOCK_EXTENSIONS=1`, `TEST_RUNNER_PERSONA_MANAGERS_HUB`, shots with
  `TEST_RUNNER_PERSONA_SHOTS`; with `MOCK_MANAGERS=1` add `TEST_RUNNER_PERSONA_MANAGERS_ONE=1`). By hand: `-tab settings -openExtensions
  YES`, or `-openExtension <id> -openExtensionManager m-3fa21c` for 工作's copy.

## Inbox lineage

An item's **Thread** section reads `GET /api/v1/inbox/tree/inbox/{id}` and shows its ancestors followed by its direct children. A to-do or Agent-session row uses the title already known locally; without one it shows only its kind, never the raw id. A fallback title identical to the kind is likewise said once. Rows keep their existing destinations: another item, the to-do, or the Agent conversation. Tree responses do not carry merged-list manager stamps, so the loader applies the opened item's manager to every tree item before either action uses it.

- **Not the same matter** appears on every ancestor and child row whose pair ends in an Inbox item. It sends `PUT /api/v1/inbox/{lower-item-id}/parent` with the body JSON `null`, without a confirmation step, then shows the success toast and rereads the visible tree. Failures use the app's existing error feedback.
- **Same matter as…** appears below a nonempty Thread section and in the item detail's More menu. Its system sheet searches open items in the same manager, newest first, by title or `where`, excluding the item itself and its direct children. Each row shows the title and a second line of `where · sender` (one part when the other is absent, and no repeated name). A choice sends `PUT /api/v1/inbox/{current-item-id}/parent` with the bare body `{"kind":"inbox","id":"chosen-id"}`, shows the linked-title toast and rereads the tree. Neither body is wrapped in a `parent` field.
- `InboxLineTests` covers row titles and the lower end of every relationship; `InboxSameMatterPickerTests` covers candidates and the second line; `InboxParentTests` pins both request-body shapes and manager routing. `InboxLineageTests` exercises the visible controls against the existing `MOCK_INBOX_LINE=1` fixture (`lark:f1:100 → lark:f2:200 → lark:f3:300`), with `TEST_RUNNER_PERSONA_SHOTS` captures on phone and wide layouts.

The frozen Mac layout keeps its existing lineage rows and does not gain these two new actions.

## The inbox, notifications and computers of several managers (#1850, #1863, #1919, #1834)

The rest of [managers.md](../assistant/managers.md) "Clients" on iOS and the Mac. `AppModel.managerRosters` keeps each group's roster
(`AppModel+Managers.swift`), read on its hub's hello, on each `{"type":"managers"}` and on a refresh; `managerRoster` is the hub-level
group's. With one installed manager, or a hub without a roster, nothing below shows. What the pages list of several managers' things
(the current assistant's alone) is in "The pages show the current assistant" below.

- **Inbox items are their id and their manager** (`InboxItem.key`): the store, the bulk arithmetic (`InboxBulk`), the reply and status
  spinners, the Mac's selection, item windows and Quick Open use it. An answer about one item is put back with the manager asked
  (`InboxStore.put(_:manager:)`), and a bulk answer is stamped with it (`InboxBulkResult.stamped`), skipped ids too. A request
  about one item (details, Done, Mute, a reply, its suggestion, hand-off and Trail) names its manager unless that is the
  roster's default (`AppModel.itemManager`, as the web's `itemHeaders`), and names its stamp until the roster is read (the
  default may have moved); the Rules and Muted lists name their stamp. An item's
  rule sheet reads and saves its manager's rules and moves only that manager's items (`InboxPolicyTarget.manager`).
- **An item's Lark chat is its manager's**: its emoji and quick reactions (`LarkMedia.catalog`, one per manager), its messaging
  source (`MessagingSources.source(for:manager:)`: a named manager's list is read through it when its chat, a profile or a Moments
  post opens, until one is found this launch; the default's stays the hub's), its drafts (`LarkDrafts.Key`, and the Lark user
  its outbox draft opens under, `LarkOutboxAccounts`; the default's under the names they had) and the clients of what opens
  inside it (thread, composer, profile, cards: `AppModel.larkAPI`, through the chat's group, `larkGroup`). A Moments post
  pushed from an item sets its manager on its page. Tests `LarkChatManagerTests`, `LarkDraftsTests`, `LarkOutboxStoreTests`.
- **Rules and Muted per manager**: both lists are the current assistant's, read with it named, the default too (the
  device's idea of the default may be stale); an offline one says in the list's place that they show once it is back.
  Each entry is stamped with the manager it came from (`InboxPolicy.manager`, `InboxMute.manager`), so Edit, Remove and
  Unmute go to it. Another assistant becoming current starts the list over (`readKey`), and a read for the one before
  doesn't land (`InboxRulesModel`, `InboxMutesModel`; tests `InboxManagerListsTests`).
- **Bulk actions per manager**: Done on several, a section's Clear all and Clean up all send one request per manager
  (`InboxBulk.byManager`). One that fails puts back only its own items and the toast says how many weren't changed and why
  (`InboxBulk.partly`); only when every one fails is it the old error. Undo after one manager's clean-up sends that manager's `undo`;
  after several managers' it brings back what moved with the Done request.
- **The inbox's list**: the current assistant's items and waiting agents, with no manager headings ("The pages show the
  current assistant" below).
- **Push switch per manager**: a manager's page (`ManagerDetailView`) has this device's Notifications switch
  (`ManagerPushViews.swift`): `muted_managers` sent alone (`APIClient.setMutedManagers`), shown only when the prefs answer has the
  field, put back if the hub says no. The Mac's own banners (`MacAttention.managerMuted`) skip a muted manager's agents and cards
  of the hub-level group (another group's hub keeps its own prefs); its Dock badge still counts them. A prefs read already on its
  way when a page saves doesn't put the old prefs back (`AppModel.savedNotifyPrefs`).
- **The built-in assistant in the switcher** (#9055): its page (`ManagerDetailView`) has **Show in Switcher** (on: the roster's
  `hide` is false), sent alone as `PATCH /api/v1/managers/assistant {"hide": …}` (`APIClient.hideManager`,
  `ManagersModel.showInSwitcher`), shown at once and put back if the hub says no (`409 builtin_in_use` while it is the default
  or has a computer, in plain words). It can't be turned off while the built-in one is the default, and its footer says so
  (`ManagersText.switcherLocked`, `switcherNote`); turned off while it is in use, the footer says it is listed for now and why.
  An older hub answers yes and keeps nothing: the list read after it still says shown, the switch goes back and a toast says to
  update the hub. While the hub marks it `hidden` its row in the list carries a Hidden chip. Tests `ManagersModelTests`.
- **Push grading** (#8157): the last section of Settings › Notifications, after this device's own rows, is the hub's
  push grading switch (`APIClient.pushLnMode` / `setPushLnMode`, `GET` / `PUT /api/v1/push/ln-mode`, the legacy wire route); its footer says it holds
  for every device and what still notifies. A hub without the route, or one that does not answer, shows no section; a change
  shows at once, and a refusal puts it back and says why. `docs/server/server.md` "Push grading (push urgency filtering, `hub/push_grading.go`)".
- **Quiet hours**: Settings › Notifications has this device's quiet hours (a switch, then From / Until), only when the prefs answer
  has `quiet`; each change sends `quiet` alone with this device's time zone (`APIClient.setQuietHours`), as the web does. Hours set
  in another zone: the section's footer names that zone instead of this device's (`QuietHours.elsewhere`) until they are changed here.
  The footer also says a chat bot's nudges about unanswered cards wait while any device is in its window (the hub's `quietNow`).
  The Mac's own banners follow the window too (`QuietHours.active`, read like the hub's `activeAt`): no "agent finished", and
  Needs You without a sound and passive.
- **Computers of an offline manager**: `Machine.manager` decodes; an offline computer whose manager the roster has offline
  (`ManagerRoster.offlineManager`) says so on its card and has no Reconnect on its page.
- **Extension pushes**: `extension_operation`, `extension_auth` and `plugin_ui` badges carry their event's `manager`
  (`ServerEvent`); none is the hub's own assistant. An operation is kept by its manager and id (`ExtensionOperation.storeKey`; the
  market pages show the default's), a sign-in by id and manager, and another manager's badge is left out of the default's places.
- **Tests**: `InboxManagersTests`, `ManagerPushTests`, `ManagerGroupsTests.aComputerIsOfflineWithItsManager` and
  `.aPushIsThePagesOwnUnlessAnotherManagerSentIt`,
  `ExtensionOperationsTests.anotherManagersOperationIsKeptApart`, `MacAttentionTests.aManagerSwitchedOffHereRingsNoBanner`.

## Starred messages (#11993)

On iPhone and iPad, touch and hold a user message or reply in an Assistant or Agent conversation and choose **Star**.
The same row shows the saved mark, and **Unstar** offers Undo. **Pages › Starred** searches the current assistant's
snapshots by words, conversation title and speaker. The list is shared with the Web app through the existing Assistant
manager API. Chats use compact references. The saved text is read while the Starred page is open.

**Open Original** checks the exact transcript and highlights the saved message when it is still present. Positional
messages also need a matching record fingerprint. A deleted conversation or original leaves the words available for
Copy. A failed connection offers a retry. Older assistants that do not offer the API have no Starred entry or menu action.

## Saved prompts (#9436)

The user's own `/` prompts (web #7329, `docs/web/web.md` "Saved prompts (#7329)"): the instructions they send again and
again, kept on the hub as the current assistant's settings, so the phone, the iPad and the web have one list. iOS uses the
web's routes (`docs/protocol/protocol.md`, `/api/v1/assistant/saved-prompts`) and adds no hub change.

- **One list** (`Persona/App/SavedPromptStore.swift`, types, API and rules in `Shared/SavedPrompts.swift`): read on hello,
  on `assistant_saved_prompts` when it is the shown assistant's (no `manager`: the default one; `SavedPromptStore.concerns`),
  and after the current assistant or hub changes, which first forgets the list before so another assistant's prompts are
  never offered. Until a read answers the `/` lists have none; an assistant from before them (404) has none and its page
  says so; a failed read keeps a list already read.
- **The `/` lists**: the plain composer's (`SlashMenuRow.slashRows`, also in chats that have no commands of their own) and
  BeautifulUI's prompt bar (`promptCommands`) list them after the chat's own commands and before extensions' quick
  prompts, as `/name` with **Saved**; picking one types its text into the box and never sends it. A saved `/commit`
  beside the chat's own is a row of its own (ids `saved:<id>`). Persona chats, which have no agent commands, list the saved prompts (the main chat after its `/clear`).
- **Assistant Profile › In Chats › Saved Prompts** (`Views/Settings/SavedPromptsView.swift`; both styles): each prompt's
  `/name` and its text's first two lines, New Prompt, and per row (context menu, swipe, VoiceOver actions) Edit, Move Up /
  Move Down (`PUT …/order`) and Delete, confirmed through `accessibleConfirmationDialog` (an alert at the largest text sizes). Delete and a move show at once and come back with the reason when the
  hub refuses; a list changed on another device meanwhile (409) is read again with a word. A change that answers after
  the current assistant or hub changed leaves the new list alone, and a move that answers after a delete made meanwhile
  does not bring the deleted prompt back.
- **The editor** (`SavedPromptEditor`): Name after a fixed `/` and Text; `sheetCancel`'s ✕ leaves, `SheetConfirm`'s ✓ saves (a spinner while it runs; #11183). It says the hub's rules before asking it
  (`SavedPromptRules`: a taken name at once, the others once something is typed or Save was tapped); a count shows past
  80% of 8,000 characters; the hub's own refusal shows in the sheet and keeps what was typed.
- Android supports saved prompts (#11667, `docs/android/android.md` "Saved prompts"). Save as Prompt… on a message's menu is a separate feature.
- DEBUG: `-tab settings -openAssistantPage prompts|prompt|newPrompt` (`promptsEntry`: the profile scrolled to In Chats).
  Tests: `PersonaTests/SavedPromptsTests.swift`; UI suite `SavedPromptsUITests` against the mock with
  `MOCK_SAVED_PROMPTS=1` (`TEST_RUNNER_PERSONA_MOCK_HUB`; skips without it) swipes a row, checks Delete asks first and
  Cancel sends nothing, and opens the editor, checks its ✕ and ✓ and that the ✕ saves nothing, light and dark;
  `TEST_RUNNER_PERSONA_SHOTS=<dir>` keeps `saved-prompts-<light|dark>-<swipe|confirm|editor>.png`.

## The pages show the current assistant

[managers.md](../assistant/managers.md) "The current assistant" ("Pages", "Counts") on iOS and the Mac: with several installed
managers every page lists the current assistant's things, with no manager headings. One rule says whose a thing is
(`AssistantScope`, `App/AssistantScope.swift`; `AppModel.assistantScope`, nil where nothing is filtered): its `manager`, and
the default's for none or for one the roster doesn't have installed. Only the hub-level group has a scope; another group's
computers, agents and cards are on every assistant's pages, as before (each hub has its own managers). Until the roster is
read after a launch, the kept pick counts by itself (the snapshot usually has the roster; without one only what names the pick
is listed), and a hub that answers it has no roster filters nothing (`AppModel.managerRosterUnsupported`).

- **The lists stay whole**: `AppModel.machines`, `allSessions`, `interactions`, `InboxStore.items` and `TodoStore.todos` keep
  every manager's, so a notification or a link opens another assistant's item as itself and what its page looks up (its
  session, its computer) is found. The pages read `scopedMachines()`, `scopedInteractions`, `scoped(_:)`,
  `InboxStore.scopedItems` and `TodoStore.scopedTodos`.
- **Agents** (`ChatsView`): the list is paged by the hub, so a hub whose hello has `sessions_manager` is asked for the
  current assistant's sessions (`SessionListQuery.manager`, set per group in `RuntimeSessionLists.managed`; its counts and
  its sections' pages name it too). The manager is part of the query, so another assistant is another list read from its
  first page (`rescope`, also on a hello: the feature may come with it). A hub with bounded lists but without the filter
  is read as a complete copy instead (the `legacy` query: every row once, then what changed since) and filtered by each
  row's `manager`, as a hub from before bounded lists is: its pages, totals and counts would be every manager's. A
  complete copy is never asked with a manager (it is the app's rows of that hub). The list's partition carries the scope (`AgentListProjectionCache.Partition`), so a
  switch starts the list over at once, with nothing held back. The computer menu offers the current assistant's
  (`MachineChoice.manager`), New Session its computers, and a switch clears a computer filter that is not its own and
  pops what was pushed from the list.
- **Computers** (`MachinesView`): the current assistant's.
- **Inbox**: the page's snapshot holds the current assistant's items and source chips (`InboxProjection.Filter.scope`), the
  agents waiting are its own, and Done's history and day counts are read from it (`InboxStore.historyOwner`; the history is
  one manager's). They start over when another assistant becomes current, and a page of the one before doesn't land
  (`InboxStore.scopeChanged`). Pull to refresh asks the current assistant to collect. The Mac's list is the same
  (`MacInboxState.sections`). The mailbox filters and New Email are the current assistant's mail accounts
  (`InboxMailAccounts`, kept per assistant). A switch drops the selection, an item's page, the Muted and Rules sheets
  and a source filter (a mailbox or a connector of the one before would leave the list empty); a mail being written
  stays, with the manager and the accounts it was started with (`ComposeDraft.manager`), and is sent through that one.
  A collection that fails on pull to refresh says so, as the default's does.
- **To-dos**: `TodoItem.manager` decodes the merged list's stamp; the list, the calendar and the count are the current
  assistant's, a new to-do names it, and a change to one names the to-do's own manager whichever is current
  (`TodoStore.named`; its decisions and a briefing's Accept too). The list is always read merged. Link to To-Do on an
  agent or a project offers the to-dos of the assistant whose computer it is on, and makes a new one for that
  assistant (`TodoStore.openTodos(of:)`, `Owner.of`), whichever is current.
- **Knowledge**: the page asks through a client for the current assistant (`APIClient.currentAssistantSettings`,
  `KnowledgeView`'s `scopedAPI`), reads again when it changes, and an entry's page keeps the client it was first read
  with (`capturedAPI`), so its editor writes to the assistant it was opened for. Recordings are this device's own and
  are synced to the default assistant whichever is current, so their part of the knowledge base
  (`/api/v1/kb/recordings…`: a recording's minutes on its page, on the knowledge page and on the watch, the Mac's list
  of recordings) names no manager whichever client asks (`APIClient.recordingsRoute`).
- **Scheduled tasks**: the page keeps the client it first read with (`AssistantRoutinesModel`): pushed on the Persona page
  it closes with a switch, and where it stays (Settings) it starts over on the one current now (`restart`).
- **Offline**: the hub's merged lists leave an offline manager's items out, so in place of their empty page the inbox
  and the to-dos of an offline current assistant say "<name> Is Offline" with when it last synced and that the page
  updates once it is back, under its avatar with the offline mark (`AssistantOfflineNotice`). Its computers and sessions stay listed, offline, as the hub keeps
  them.
- **A conversation's feed and what it offers** (the feed's read, a reply draft's Send and Do Not Handle, a plan's Start
  or Don't Start, a briefing question's Handled) name the conversation's own manager (`AppModel.feedManager`), whichever
  assistant is current: some of those routes name no conversation, so the hub would take them to the default. The
  default's conversations name none, and with one installed manager or no roster nothing is named.
- **Counts**: the tab badges, the app icon's number (their sum), the sidebar's Inbox and To-dos counts, the home page's
  Needs You and the main conversation's dock count the current assistant's (`AppModel.countBadges`,
  `InboxStore.unreadCount` for the Inbox's badge and row, `TodoStore.badgeCount`). The
  switcher shows beside every other assistant what waits there (`AssistantBadges`, `AppModel.assistantBadges`): its agents
  waiting (a session with a pending card or in Needs You once, quiet and muted ones left out, a card on no session by
  itself; Persona's own conversations by their cards only) plus `inbox_needs_you`. The header's face carries a mark while
  another assistant has something waiting (`AssistantWaitingDot`; beside the name where the face isn't drawn), and VoiceOver
  says both. With several assistants the name beside the Persona face elsewhere (the sidebar's foot, the Mac's status
  line) is the current assistant's, and a voice call's captions name the assistant of the conversation they are said in
  (`AppModel.assistantShownName`); with one assistant these keep the persona's name (#4712). The inbox counts are kept beside the rosters (`AppModel.inboxNeedsYou`; a `ManagerRoster` carries none), so a
  count that moves writes no roster and draws nothing that reads one. On a hub with bounded lists the app holds only the
  pages it showed, so the sessions in Needs You are read for the counts (`AppModel.readNeedsYou`: one request for every
  manager's, at most every two seconds while the hub's `sessions_list` hints come); not while its list is a complete
  copy (a grouping, pins or mutes, a hub without the manager filter), which has every assistant's sessions already.
- **A switch** is the current assistant itself changing (`AppModel.assistantSwitches`: a pick in the switcher, or its
  manager leaving the roster), which is what closes pages and clears what was chosen. The default moving onto or off
  the current assistant is none: what a request names changes, who is shown doesn't. Until the hub answers its roster
  after a launch, a kept pick, and the roster the snapshot kept (`AppModel.rostersKept`), hold the rows to themselves,
  but nothing this section adds is asked by them (`AppModel.rosterAsked`, `AssistantScope.asked`): the Agents list is
  asked without a manager, the sessions in Needs You are not read, and Done's history, Rules, Muted and a new to-do
  name none. The hub may have one manager by now, where no request names one; the list is asked again once it answers.
- **Unchanged**: the watch, widgets, Live Activities, Siri and Shortcuts, the Health page and notifications go by the
  default assistant or the item's own manager. The Mac's menu bar and Dock menu, Quick Open and Apple Vision Pro's shell
  still list every manager's.
- **Tests**: `AssistantScopeTests` (whose a thing is, the kept pick, the model's lists, the tab badges and the dock, the
  Agents list's query on a hub that filters and on one that doesn't, the inbox, the to-dos and what a change names, the
  knowledge base, scheduled tasks), `AssistantBadgesTests` (the counts, the roster's count kept apart, a push while the
  roster is read), `InboxManagerListsTests`, `AssistantSwitcherTests`. UI suite `AssistantSwitchPagesUITests` (iPad
  only, against the mock hub as `AssistantSwitcherUITests`): with an inbox item or the scheduled tasks of one assistant
  open, another is picked in the sidebar, and the item is gone and the page is the new assistant's.

## The Persona page's manager (#2664)

[managers.md](../assistant/managers.md) "Clients": the interface calls a manager an **assistant**, and one of them is the
**current assistant**, picked in the sidebar's header (`AssistantSwitcherHeader`, `Views/Assistant/AssistantSwitcher.swift`; iPhone,
iPad and the Mac, where the header is one line in the title bar's row). The header shows the Persona face chosen in Appearance
with the state's dot at its foot, the current assistant's name with an arrow, and "Online · <host>" / "Offline · last synced …".
It opens a popover (`AssistantSwitcherList`): one row per manager, the installed ones (the default first, then roster order) and
then those still installing, each with its first letter on a disc in its color (`AssistantSwitcherText.hue`: the default's is
`Theme.ai`, any other's the sum of its id's UTF-8 bytes modulo five over blue, green, orange, pink, indigo, the same on every
client; the letter in the panel's color) and the state's dot at the disc's foot, its state line, a Default chip and a check on
the current one or, beside any other with something waiting, its count ("The pages show the current assistant" above), on a
solid panel (`Theme.card`); an offline row is dimmed as a whole (`Theme.Fade.offline`) and can still be
picked. A manager the roster marks `hidden` (the built-in assistant while the user hides it and it isn't used, #9055;
Settings › Assistants) has no row (`AssistantSwitcherText.rows`); it still counts as installed (`ManagerRoster.installed`,
`isMulti`), so the pages still scope by assistant, its badge still counts and the Default chip still shows. The dot
(`AssistantStateDot`) is green online, gray offline and orange while installing, ringed in the color it sits on;
Settings › Assistants keeps its dot beside the words. Under them **New Assistant** (asks for a
name, then the install code sheet of Settings › Assistants) and **Manage Assistants…** (Settings › Assistants). A row still
installing opens its install code: the one this header made while it lasts, else a new one (`ManagersModel.showCode`; the hub
keeps no code to show again). What the list presents (the name alert, the code sheet, Settings) waits until the popover is
gone (its `onDisappear`), not for a fixed delay: UIKit refuses a presentation while the popover is still leaving. At the
accessibility text sizes the header leaves out the face so the name fits, and the list gives way to the screen's width. The
header and its list appear once a roster is known (read in this run, or kept from the last launch) and the current assistant
can be shown, including with one installed assistant (an offline one counts, one still installing doesn't). A one-assistant
roster has no Default chip, and **New Assistant** and **Manage Assistants…** stay below it. On a hub without a roster,
`AppModel.showsAssistantSwitcher` is false and the sidebar keeps the persona's name as its title.

The pick is the manager's id per group (`AppModel.assistantPicks`), kept on this device across launches (`AssistantPickStore`,
the app's defaults); no pick follows the hub's default. A pick whose manager leaves the roster, or that the roster marks
`hidden`, goes back to the default (`ManagerRoster.canPick`; a hidden one can't be picked either), and one that becomes the
default stays picked. Each group's snapshot keeps the roster and the picked manager's settings, so a launch
starts on the current assistant before the hub answers. Switching closes the Persona page's pushed pages (`assistantSwitches`:
they read the assistant before and would write to the next); re-reading the same assistant (back online, the default moving
under a pick) doesn't. While another assistant is current, the default's settings are read once a run all the same
(`defaultAssistantReads`), since Health and the channels follow the default.

CarPlay's voice follows the conversation open in the app, else the current assistant's main conversation (it extends the app's
screen, `CarPlayVoiceSession`); Siri and Shortcuts, the Watch, widgets, Live Activities and the Health page keep following the
default assistant.

- **Requests**: `AppModel.api` and `api(_:)` carry the pick (`APIClient.showing(_:)`); a request names it in `X-Persona-Manager`
  only on the assistant's own routes (`APIClient.assistantRoute`): not one conversation's (`/api/v1/assistant/conversations/{id}/…`,
  which go by the hub's record of their manager), not the health space (`space=health`, a health start included), and nothing
  for the default. An item's client (`manager(_:)`) names only its own manager.
- **What the page shows**: its manager's main conversation, settings and lists (`AssistantLists.Shown`: a conversation without a
  `manager`, or with one the roster doesn't have, is the default's). The health space stays the default's: its list, a new
  conversation in it, and its machine for uploads and checkup reports (`AppModel.healthMachineID`). What was read of each
  manager is held for this run (`assistantByManager`; the default's is still the one saved), so going back shows it at once.
  A read for the manager shown before doesn't land (`AppModel.showsPage(of:)`), nor does its readiness. The hub relays another
  manager's conversation list and new conversations without `manager`: the client stamps them with the manager it named.
  A first message that failed to start stays on its manager's page, one per page (`FailedStart.Message.manager`). A main
  conversation is only started once the shown manager's settings are read; before that a start is a plain conversation. A
  launcher picked on one manager's page is dropped on a switch; with several managers Health keeps its own, falling back to
  the default's. A change of the default manager, or the shown one coming back online, reads the page again (and, with another
  manager shown, the new default's settings for Health and the channels). A channel's target (Settings) lists the default's
  conversations.
- **Offline**: the page shows what it last read in this run, a note above the composer ("<name> · last synced <time>. Messages
  can be sent once it's back.", `ManagerOfflineNote`) and every composer is off; suggestions are hidden and Ask Persona says so
  instead of sending. A conversation of an offline manager opened anywhere has its composer off with the same note
  (`AppModel.offlineManager(of:)`).
- **From elsewhere**: opening a conversation (a link, a notification, the Inbox's hand-off to Persona) opens that conversation
  and leaves the current assistant alone (`AppModel.openAssistantConversation`); its requests go to its own manager, as the hub
  routes a conversation's. The top bar names that assistant (`otherAssistant(of:)`); one not listed yet keeps the item's manager
  (`assistantItemManagers`) for its links, its offline note and that name. It stays on screen through later pushes and re-reads,
  and gives way to the current assistant's main conversation only when the user switches. A hand-off sent as a plain message goes to the item's manager's main conversation. A summary started from an extension's chat page stays on the default
  manager.
- **Tests**: `ManagerSwitchTests` (the route rule and real requests, the pick's start, fallback and one manager, the pick kept
  across launches and through a change of the default, each manager's settings held, a manager leaving the roster or hidden,
  the lists, another assistant's conversation opened without switching, an offline manager) and `AssistantSwitcherTests` (the
  rows and their words, a hidden one left out but still counted, the colors, the kept roster, a row still installing); UI suite `AssistantSwitcherUITests` (New Assistant → name →
  code sheet, Manage Assistants… → Settings › Assistants, a row still installing → code sheet; against the mock hub with
  `TEST_RUNNER_PERSONA_SWITCHER_HUB`). By hand, against the mock hub (`MOCK_MANAGER_NAS=1`):
  `-openAssistantManager m-3fa21c` (工作, no main conversation yet) or `m-8b04d2` (家里, offline) makes that manager the current
  assistant once the roster is in; `-openAssistantSidebar YES -showAssistantSwitcher YES` opens the sidebar with the list.

## Screenshot banner

Requested 2026-11-17 (#9508): a screenshot of a chat carries Persona's brand where the top bar is, the current assistant's avatar and name at the left and GitHub's mark with `nwverse/persona` at the right (#10172, 2026-11-18; until then "(Persona)" followed the name), while the screen keeps its bar. Who it serves: everyone who shares a screenshot of a conversation; nothing changes on screen and nothing to set up. iPhone and iPad; web and the desktop app cannot (their screenshots are the operating system's), Android is to do separately.

- **Where**: the Assistant chat's bar in all its forms (the shell's row, the Sidebar layout's drawer row, the iPad's bar; `AssistantTopBar`) and the Agent chat's header, open and folded (`ChromeHeader` in `ChatView`). Not the iPhone Duo's side rail or side-bar header, and not Inbox, Knowledge or Settings: those use the system's navigation bar, which cannot be moved into the layer below, and the Inbox mostly shows other people's messages.
- **How** (`apple/Persona/DesignSystem/ScreenshotBanner.swift`): iOS leaves a secure text field's drawing view out of screenshots, screen recordings and mirroring (AirPlay and the like) and keeps it on screen: what keeps passwords out of pictures. `screenshotBanner` hosts the bar in its own `UIHostingController` inside that view, taken from a `UITextField` with `isSecureTextEntry` (public API only: the field's first subview), and draws the banner's hosting controller under it, both over an opaque backdrop that carries on to the screen's edges where the bar meets them (behind the status bar). On screen the bar covers the banner; in a capture the bar's layer is gone and the banner shows. The field stays alive: switching its `isSecureTextEntry` turns the layer off and on (`ScreenshotBanner.beginReveal()` / `endReveal()`). No such view on some future system: the bar is drawn as before and screenshots show it (`ScreenshotBanner.isAvailable`, which also hides the switch).
- **The name**: the one the top bar says for whose page it is (`AppModel.assistantTitleName`, #12243): the named manager on a hub with several, else the current assistant (with one too, as the sidebar's head and the bar show it), else the persona's name; in the Assistant chat the bar hands its own (another assistant's conversation names its own; a side conversation's banner keeps the assistant's name, not the conversation's title). Before #12243 the banner read `assistantShownName`, which keeps the persona's name with one assistant, so a screenshot said "Persona" (cut to "Pers…" on a narrow display) where the screen said the assistant's name.
- **The banner**: `PersonaBrandLockup` `.banner` (`PersonaBannerRow`): `PersonaAvatar` at `Theme.Size.avatarM` and the name in `Theme.Font.display` at the leading page margin, so the avatar sits where the bar's own face is; at the trailing margin GitHub's mark (`BrandMark`, `Theme.Brand.github`: Primer Octicons' `mark-github` as published, a one-color template) and `PersonaBrandLockup.repository` (`nwverse/persona`, spelled out nowhere else in the app) in `Theme.Font.title`, mark and words in the secondary color, centred on the name. The slim row has the name in `heading` and the repository in `secondaryStrong`. In the bar's row of buttons (the Assistant chat) or centred in the header (the Agent chat). It grows with the text size to the bar's cap (`Theme.Shell.topBarLargestTypeSize`). The repository is why the banner is there, so it goes last: `ViewThatFits` over the large row with it, the slim row with it (a folded header, the bar's largest text sizes, the narrowest iPad split), and the slim row with the mark alone. In a row with the repository the name asks for no more than its first two characters and the ellipsis, so a long name is cut at its tail and the repository stays whole; the mark stands alone only where not even the slim row has that much room. A picture, never a link: it takes no touches. The same component signs exported pictures (`.mark`, #9468, still with "(Persona)").
- **On screen** the bar is unchanged at rest; what scrolls under its strip no longer shows faintly through, since the backdrop under the bar is opaque: the page's color down to where the Assistant bar's fog starts to fade (`AssistantTopBar.fogSolid`, then out over `Theme.Space.m`; `BarFog` under both carries the fade on as before), the page's color and the header's material under the Agent header (the banner over the page's color alone, so no second blur). The bar takes the touches in its strip, as a hosting view does (a pass-through also lost the bar's own buttons, measured on a prototype).
- **Settings › Appearance & Display › Persona Banner in Screenshots** (`ui.screenshotBanner`, on by default, this device only). Off, the bar is drawn as it always was, on screen and in screenshots.
- **The bar's environment**: its hosting controller gets the page's values one by one (`IslandEnvironment` and the shell's, the tab's, the chat's link and port openers), never `.environment(\.self, …)` (see `LayoutIsland`). The look is the window's, not the copy's: the bar's and the banner's hosting controllers draw in their window's light or dark (its `traitCollection.userInterfaceStyle`, followed as it changes) and take it as their traits too, since the copy is only as new as SwiftUI's last update. With the copy alone, an Agent chat opened for the first time once drew its header and banner light above the dark chat (#9829), as the drawer's sidebar did (#4496).
- **Accessibility**: VoiceOver reads the bar as before; the banner's hosting view is `accessibilityElementsHidden` and takes no touches (SwiftUI's `accessibilityHidden` on the banner left its text in the tree, on the prototype).
- **Pictures the app takes of itself**: `drawHierarchy` and `snapshotView` leave the layer out as a screenshot does. The iPad's sidebar drag (`SplitColumnControl`, its pictures of the page) and DEBUG renders (`RuntimeCLI`) reveal the bars while they take them, after the screen updates while a bar is in the layer (`ScreenshotBanner.hasLayers`: a picture of the frame already drawn still leaves it out); `layer.render(in:)` draws everything and needs nothing. UI tests launch with the banner off (`launchPinned` adds `-ui.screenshotBanner NO` unless the test names it; `TEST_RUNNER_PERSONA_SCREENSHOT_BANNER=1` turns it on for a run): XCUIScreen's screenshots are the system's.
- **Checking it**: `simctl io … screenshot` and `recordVideo` read the display's frame buffer, not the system's capture, so they show the bar either way. `ScreenshotBannerUITests` takes XCUIScreen screenshots (the system's, outside the app) and checks the bar's control is gone from them with the switch on and there with it off; `ScreenshotBannerTests` covers the layer with in-app pictures. On a device: take a screenshot (side and volume-up) and a short screen recording of a chat; both show the banner, with no black block where the bar was.
- Also in captures: the App Switcher's card may show the banner; a screenshot taken while the pager slides between Assistant and Agents shows the stand-in bar (`ShellMovingBar`), not the banner.

## DEBUG launch arguments (Debug only; Release checks exclude these strings)

`-pairURL persona://pair?...` (pair without scanning a QR code, no notification permission prompt), `-openSession <machine>/<session>`,
`-showNewChat YES`, `-tab dev|mail|settings`, `-openAssistant <machine>/<session>`, `-newAssistant YES`, `-openDictationControl YES` (the Dictate to Persona intent; combine with `-showDictationSample "<words>"` in the simulator), `-openAssistantSidebar YES|0.5` (Persona tab sidebar open / halfway open), `-openAssistantManager <id>` (that manager as the current assistant once the roster is in), `-showAssistantSwitcher YES` (the sidebar's assistant list open, several assistants), `-openFirstImage YES` (open the viewer for the first image in the chat), `-showScrollState YES|visible` (chat-list position and following state, readable by UI tests; `visible` draws it), `-openTerminalPane YES`, `-showPad YES`, `-showKeyMods locked,once,held` (set the terminal key bar's ctrl / alt / shift to these states), `-terminalWrap NO`, `-terminalTheme ink|midnight|persona|solarized`, `-terminalInputMode direct|field` (terminal direct typing / input field), `-terminalTipsSeen YES` (skip "Terminal tips"), `-openTerminalTips YES` (show them again), `-focusTerminalInput YES` (shows the keyboard in direct-typing mode), `-openShortcutsPanel YES`, `-openTerminalSettings YES`, `-showLark YES` (go directly to Inbox), `-larkFocus <messageId>` (scroll to that message), `-larkProfile <userId>` (show this person's profile card in the chat), `-larkPicker YES` (show the reaction picker for the last message), `-larkMoments <postId>` (open a Moments post), `-openInboxFirst YES` (open the first unhandled Inbox item), `-larkTop YES` (show the Lark chat from its earliest message, for screenshots of long cards), `-addMail YES` (open Manage email accounts › Add email account), `-tab settings -openHubUpdate YES` (Settings › hub, `docs/operations/hub-update.md`), `-tab settings -openSkills YES` (Settings › Assistant › Skills), `-tab settings -openAssistantPage assistant|routines|routine|intervalTask|cronTask|followup|bypassTask|newRoutine|memory|memoryFile|memoryEdit|places|place|newPlace|deletePlace|about` (pages under Settings › Assistant; `routine` opens the first scheduled task, `intervalTask` / `cronTask` / `followup` the first interval / cron / one-time task, `bypassTask` shows the "Allow everything?" confirmation in the editor, `newRoutine` shows the creation sheet with the "End-of-day review" template, `memoryFile` / `memoryEdit` opens the first memory / its editor; `places` is Saved Places, `place` opens the first place's editor, `newPlace` the new-place sheet, `deletePlace` the list's Delete confirmation for the first place). `scripts/app/sim.sh` wraps building, installation, pairing and launch (`SIM=<udid>` changes the simulator; concurrent agents must use separate simulators or overwrite each other's installs). It leaves the watch app and the share extension out of the build unless given `--with-watch` (`docs/operations/testing.md`, "Development builds").
Check history rendering: `-loadAll YES` (load all history and scroll to the top), `-showFromSeq <seq>` (display from the turn containing this user message; scroll positioning can drift in long histories),
`-dropUsers YES` (hide that turn's user bubbles to fit its process rows and answer on one screen). `-openPane terminal|git|files` opens the chat page's terminal (from the bottom) / Git / Files;
`-openNeedsRow plan:|task:|message:` (the Needs You tray with the first row of that kind opened in place; use with `-openAssistant`), `-simulateTap <machine>/<session>` runs the notification-tap handler 4 seconds after launch; `-tab settings -openNotificationSettings YES` (Settings › Notifications; add `-openIslandSettings YES` for Dynamic Island); `-tab settings -openSentAsYou YES` (Settings › Sent as You, #5278); `-showFleetSample needsYou|working|done` (`-fleetDetails YES`, `-fleetIsland b|c|custom:<left>,<right>`) starts a Live Activity with fake data; `-showTestNotification permission|plan` shows a local notification matching an interaction push 3 seconds after backgrounding (UI tests use it to test buttons); `-openFilePath <path>` (open this file after opening the chat; `-openFileLine <line>` jumps to that line), `-showFilePerf YES` (write file-open timings and per-second frame rates to Caches/file-perf.log; `-showFileAutoScroll YES` scrolls the file at constant speed, `-showFileAutoScrollSpeed 1500` changes the speed); `-showMenu YES` opens the long-press action bar on the last reply (with `-showFromSeq`, that turn's reply); `-openWelcome YES` forgets pairing and exits demo mode at launch, stopping on the pairing page (`DemoModeTests`); `-openChangeAnswer YES` opens Change Answer on the first **Answered by the assistant** card (#544).

`-showSideSessions chip|sheet|side|<machine>/<session>` (with `-openSession` for the first three): sample side sessions under that session, and the sheet or the first side's page opened (#7944, "Side sessions" above).

`-showVoiceOrbs YES`: without connecting to the hub or opening the microphone, show a sheet listing every phase of the voice orb (fake fluctuating volume) and two large orbs; combine with `-ui.style`, `-ui.scheme` for screenshots (#3506).

`-showCloseoutSample confirmed|contradicted|blocked|rows|orphaned|decision|armed`: without a hub, show one state in a sheet with made-up content: a closeout card the hub confirmed, one it contradicted, a blocked one, the Agents rows' line with the merged / deployed suffix, "Nobody is working on this", a decision row, and a decision whose first option is armed (#3612, #3618, #3804). The same states on their real screens come from the mock hub: `MOCK_CLOSEOUT=1` (sessions with closeouts; `-openSession server/<id>` opens one, and `-tab settings -openTriageSettings YES` shows the closeout and status-afterwards switches; `MOCK_TRIAGE_NO_CLOSEOUT_ASK=1` omits only the latter as an older hub does) and `MOCK_ORPHANED=1` with `MOCK_TASKS=1` (orphaned to-dos). `MOCK_WORKFLOWS=1` with `MOCK_CLOSEOUT=1` puts a running workflow's section over the blocked session's card; `MOCK_DECISIONS=1` with `MOCK_TODO_ASKS=1` gives the decisions their options, background, default and fixed answers (with `MOCK_TASKS=1 MOCK_TASK_DECISION=1`, the tray holds three, two of them two-tap ones).

Tests that live updates do not interrupt interaction (`LiveUpdateTests`) use two arguments: `-openChatsFresh YES` clears the chat cache on disk at launch (steps pushed during the previous run do not remain);
`-perfCounts YES` counts how often each view body is computed and how long each hub push takes until the main thread is idle, exposing JSON in hidden text `perf.counts` for UI tests
(`PerfSignpost.swift`; the same instrumentation emits os_signpost, so Instruments Points of Interest shows every body and push).

## Lark message rendering (`Persona/Views/Inbox/Lark/`)

Reuses the Lark gateway web app's message rendering (the upstream message renderer), with `parts` forwarded unchanged by the hub:

Sending does not wait for Lark (`LarkOutbox.swift`): tapping Send clears the input and draws the message at the bottom of the chat immediately (a small spinner beside the bubble, like Lark), while the background sends messages one by one in composition order;
after sending, it waits up to 20 seconds for Lark's own copy (same text, sent by me, no earlier than the send time) and replaces it. Ordering applies only to send requests: waiting for copies happens independently alongside them, so the next message does not wait for the previous copy (#851, both chat pages and topic panels). Unsent messages stay in place with a red failure icon beside the bubble (`Theme.Icon.errorFilled`); tapping it can retry, return the text to the input, or delete it. Sending and failed rows share the same list as Lark messages, ordered by composition time (`LarkChatRows`): later sent messages and others' later replies appear below them; retrying does not move a row.
Read receipts (`LarkReadReceipts.swift`): beside my messages, direct chats say "Read / Unread"; groups show a progress ring (a checkmark once everyone has read). Without receipts (not fetched yet, absent from the Lark gateway, or an old hub without the route), nothing is drawn.
Only my visible messages are queried (`POST …/conversations/{id}/read-states`, at most 100 per request), at least 3 seconds apart per conversation; fully read messages are not queried again. When this conversation gets live updates (or the once-a-minute foreground fallback poll), visible messages not fully read are rechecked; after an old hub returns 404, no more queries during this visit.
The same applies to sending the AI twin's draft from the Inbox: tapping Send closes it and marks it handled; failure puts it back among unhandled items, and reopening the reply page restores the same text (`InboxStore.unsentReplies`).

| File | Lark gateway counterpart | Content |
| --- | --- | --- |
| `LarkModels.swift` | contracts `MessageView` / `MessagePart` | Wire types, API |
| `LarkMedia.swift` | `LarkRemoteImage` / `image-viewer` / `message-video` / `chat-file-preview` | Authenticated media loading (memory + disk cache, ImageIO downsampling), emoji directory, full-screen gallery, player, Quick Look |
| `LarkInline.swift` | `card-style.ts` / `card-markdown-text.ts` | Lark palette, inline runs (bold / italic / strikethrough / inline code / links / dashed terminology underline / embedded emoji images), flow layout |
| `LarkMessageContent.swift` | `index.tsx` `renderPart` / `rich-blocks.ts` | Rich-text block tree (paragraphs, ordered / unordered lists, quotes, headings), images, stickers, file cards, video, voice, code blocks (lightweight highlighting), system messages |
| `LarkCard.swift` | `card-renderer.tsx` | Card elements: header theme colors, markdown, column_set (weighted / fixed / auto column widths + bisect / trisect / stretch narrow-screen reflow), table, list, collapsible_panel, note, text_tag, icon, buttons (links / callbacks), select / input / picker (read-only), overflow, img, chart (Swift Charts)… |
| `LarkSummaryCards.swift` | `SummaryCardView` | Forwarded-message bundles (tap for all messages), group / person cards, video meetings, calendar, polls and other summary cards |
| `LarkChatView.swift` | messenger bubbles | Avatar, sender + bot badge, consecutive messages from one sender (by id) grouped, time only in centered separators (day change or gap over 5 minutes, `LarkChatLayout`), quoted replies, reactions, edited marker; cards / images / files have no bubble. Topic groups (`is_topic`) use one card per topic like the Lark gateway web app (`LarkTopicCard.swift`: avatar, name and time on each card, not chat-grouped; content taller than 320 pt folds into "Expand"; below are the latest two replies and "View N earlier replies"; replies load only when the card is visible, at most 3 concurrently, and not again if the reply count has not changed (`LarkTopicReplies`); "React" and "Reply" below; tapping the body or replies opens "Topic details"; recalled and edited topics and recalled replies follow the same "Message Tweaks" recall-protection and edit-history display as chat messages, #1087) |

Key points:
- Inline content becomes one `Text` (`Text("\(a)\(b)")` interpolation); emoji images embed inline through `Text(Image(uiImage:))`, initially showing `[label]` before loading.
  Changes to `LarkMedia.emojiVersion` trigger redraw after loading.
- Card `plain_text` in Markdown contexts must decode entities such as `&nbsp;` and `<br>` / `<a href>` / `<font color>` (markup left by the Lark gateway).
- Card callback buttons are enabled only when `actionMethod == 2` (through the Lark gateway's card-actions API); select / input and other form controls show only their current values.
- Custom `Layout` can receive `.infinity` for `proposal.width` (ScrollView / ideal-size measurement); check `isFinite` before converting to an integer or it crashes.

## Settings by scope (#5236)

The shared iPhone, iPad and Mac settings page has three groups:

| Scope | Entries |
| --- | --- |
| Current Assistant's Settings | Models, Assistant Profile, Channels, Connectors, Extensions, Knowledge Selection, Meetings, Agent, Machines (platform and hub capabilities decide availability) |
| This Device | Appearance & Display, Language & Dictation, Notifications, Translation, Storage, Health, This iPhone, About |
| Hub | Hub Connection, Hub Version, Assistants, Avatar, Devices & Pairing |

Whenever the hub answers a roster, with one or many installed assistants, the first heading is **<name>'s Settings**,
with the current assistant's first-letter avatar and state dot plus **Switch**. Switch opens the same assistant
popover/list as the sidebar. The first group's footer is **Only <name>'s own settings are shown. To change another
assistant's, switch to it first.** A legacy hub without a roster keeps the neutral **Current Assistant's Settings**
heading, without Switch or that first-group footer. The second group has no footer; the third group's footer is
**These two groups are the same whichever assistant is current.** It covers both This Device and Hub. The health space
remains bound to the default assistant. The avatar is shared across devices; theme color, interface style, text size,
list layout and light/dark appearance remain device preferences. The layout page is titled **Agents List Layout**.

Models replaces the two separate model-provider and model-configuration entries. Common purpose/model choices come first;
providers and keys sit under Advanced, and a purpose's runtime/provider choices are folded in its editor. Agent collects
Claude Code, Codex and Session Triage. Applying runtime settings to all online computers affects only this assistant's
computers in the current Hub. Connectors, extensions, markets, channels and knowledge selection use a captured assistant
request context; switching assistants returns Settings to the list, and a late response cannot replace the new scope.
An explicitly manager-bound detail link still opens that assistant's copy without changing the app's pick.

An extension purpose with candidate support shows the lists it requested, their most recent use, and each model's provider
or reason for being unavailable. Choose the extension’s lists, the purpose model alone, or an ordered list of directly
callable providers and model IDs. Leave an ID empty to follow that provider’s default. Saving uses the current revision. When the revision changes elsewhere, the editor keeps the draft until you confirm a reload.

The rest of the first group follows the same rule. Skills, Auto-Start Tasks (`TaskTrustModel`), Outlook sign-in on Add Mail
Account, an extension form's launcher picker, each page's Subscriptions entry (`SettingsPage.subscriptions(state:manager:)`)
and the extension operations a page reads (`ExtensionOperations.reload(manager:)`, which replaces that assistant's alone) go to
the assistant the page opened for. An assistant without the chat connector (its manager answers 404) shows it Not set up, with
no Message Tweaks. Links from a page of the default assistant's things name it by its id while another one is current
(`AppModel.settingsRoute`): an extension page's settings link opens its own page's copy (`ExtensionViewModel.Outcome.settingsOwner`),
Messages' Settings the default's chat connector page (`SettingsPage.feishu(manager:)`, which says whose it is), and the voice
entry the default's voice extension. What runs on the default assistant whichever is current keeps the default's model
settings (`ModelSettingsOwner`): Translation's model (with a line saying so while another assistant is current), a voice
call's model, the inbox triage banner's Set Up, and the /agent model on the default's chat connector page. The badges on the
ways to Settings count the default's extension entries only while it is current, since Settings shows the current
assistant's under Extensions. While another assistant is current, that route may keep the default assistant's explicit id
on the page's requests. Extension operation and auth storage canonicalize the id to the default/nil key, so a push with
no manager still updates the explicitly routed page; another assistant's id remains a separate key.

About shows the app version and a folded Diagnostics section containing the build commit and last new-chat check.
Devices & Pairing retains the device list, pairing again, Watch sync and confirmed disconnect. Mobile permissions,
Health, dictation and lock-screen privacy remain available in their appropriate device pages.

Settings search matches localized entry names and common aliases (for example API Key, dictation, lock screen, cache,
Watch, and commit), then navigates directly to the destination. It changes no settings. Legacy SettingsPage values,
ModelLaunchersRoute and existing DEBUG page-opening arguments continue to work. For a search-state capture, the existing
`-openAppearance` argument accepts `search:<query>`; no new DEBUG switch is introduced.

Regression coverage: `SettingsNavigationTests`, `AssistantSettingsScopeTests`, `SettingsResourceScopeTests`,
`ManagerSwitchTests`, `SettingsFollowAssistantTests`, `SettingsHomeTests`, `SettingsOwnerLinksTests`, `ModelSettingsOwnerTests`,
`SettingsSubscriptionsOwnerTests`, `SettingsBadgeTests`, `ExtensionOperationsTests`, and `SettingsNavigationUITests` /
`ModelsSettingsHierarchyTests`. A remembered assistant pick
routes settings before the roster arrives; only a confirmed legacy-roster 404 suppresses it, without erasing the pick.
These tests do not claim that an installed TestFlight app was updated.

## Engine selection

Each machine uses one terminal engine: herdr (herdr in the terminal), Herdr App (terminals opened in the desktop App), or Orca; the default is herdr. Only sessions in the selected engine are shown. Choices come from the `engines` reported by the hub (names and descriptions unchanged), never hard-coded. User-facing instructions are in `docs/integrations/engines.md`.

After successful QR-code or manual pairing, first choose an engine for the hub machine; if there is no local machine, use the first in the list. One must be selected to continue; there is no Skip: preselect this machine's current engine (herdr if never selected), and confirm with `PUT /api/v1/machines/{id}/engine`; a save failure keeps this page open and asks to retry (requested 2026-11-01). If the hub does not report engine detection data (old hubs), skip this step to avoid a page that cannot save or exit. Settings › Machines › Machine details › Engine can change it at any time; saved changes while offline take effect on connection. Missing detection data shows "No detection data yet", never treats unknown as not installed.

Both mock hub and demo mode support selection and machine pushes. For screenshots, first open the pairing page with `-openWelcome YES`, then open a `persona://pair` link to the mock hub to enter the real post-pairing flow; settings details keep using `-tab settings -openMachines YES -openMachineDetail server`. No new DEBUG arguments.

The To-dos page (iOS / visionOS) and the assistant's tasks share `TodoItem` / `TaskState` in `apple/Shared/Tasks.swift`. The To-dos page uses the state labels, sorting and colors specified in `docs/assistant/todos.md`, and its sections and related rows follow `docs/assistant/todo-status-sop.md`.

For a to-do whose optional `kind` is exactly `concern`, iOS adds one compact neutral **Concern** chip on open and closed rows, beside a short title or directly below a title that needs its existing two lines. It never replaces the row's state word, counts or actions. An accepted open concern keeps its open status words, tone and checkbox rather than Accepted / Done presentation, and the list omits its Accepted review chip while its report remains open. Open concerns never merge with related to-dos or one another, including transitively; closed concerns group normally. A missing or unknown `kind` is an ordinary to-do, so older hubs and cached items keep their previous behavior (#3582).


### Orca workspace containers in Agents (#1399)

By Repository uses the manager-owned `/api/v1/workspaces` catalog for Orca containers, including existing workspaces with no Agent. The host's project ID groups independent root workspaces under one project heading; workspace titles are not project names. Workspaces keep their host titles, order and source-local parent IDs; a parent can be in another Orca project on the same computer, in which case the child stays under that parent's tree. Every level has its own child fold, and status counts and Mac keyboard traversal include descendants only while their branches are open. Quiet workspace trees retain their structure. Search and quick filters keep the ancestor path of visible Agents. Other grouping modes retain their existing behavior.

On iOS By Repository keeps its structure, a card per repository with its worktrees' lines (#2531, `RepoTreeLine` and its lines in `RepoTreeRows.swift`; the Mac keeps its sidebar cards), and draws its agents in the display's rows like every other grouping (#3753, a tester's decision that reversed #3182's one-line tree: one look whichever grouping is picked; one line per agent is Compact List). In the grouped list (Avatar, Two Lines, Compact List) each repository is a card; under State Cards and the BeautifulUI style's capsules its lines sit on the page, over its agents' cards.
- **Repository:** each one is a card of its own. Its header is the folder, the name and the fold chevron, with no ⋯ and no to-do mark. Its actions are in the header's long-press menu: the space's open / pin / mute / rename / link to-do and its to-do when its agents are all in one herdr space, otherwise Mark All as Seen.
- **Worktree lines:**
  - ⌂ and a Primary badge for the main checkout, no leading mark for the others so the name and indentation carry it like the web sidebar (#6644); the branch (or Detached HEAD, or the repository of a child from another project) in small type after the name when it says more than the name.
  - At the end: N agents, Idle, or nothing for a worktree whose agents are all in its child worktrees; then the chevron.
  - A worktree the host lists opens its first agent (or its page) on a tap, folds from its own chevron (a 44 pt square, its chevron where the header's is), and holds Open / New Session / New Child Worktree / Rename / Copy in its long-press menu.
  - The host's catalog worktrees (Orca's) take the same lines; theirs alone keep the long-press menu, as their cards had it.
  - A folder two repositories claim (a Codex App chat names its project's folder, above the repository whose main checkout
    the folder is, #5715) is a worktree line in each, each with its own agents: the repository whose main checkout it is keeps
    the path as the line's id, the other's gets `@<repository id>` after it (`RepoTree.build`), since the list keys rows,
    folds and quiet state by that id. Mock: `MOCK_SHARED_FOLDER=1`.
- **Agents** are the display's rows (`SlotRow`: Avatar, Two Lines, State Cards; `CompactSlotRow`: Compact List), with what they show in every grouping: the title, the time, the latest words, where it runs, the model's short name. They sit under the line they belong to at its level (a worktree's, as a tab's agents sit under the tab's row; a level in under the header of a repository with its main checkout alone), moved in by the tree's levels (`TreeIndent`), and keep the long-press menu (`AgentRowMenu`).
- **Folded lines:**
  - Idle agents sit behind one quiet line: "N idle · their tabs ›", or "Hide N Idle" once opened.
  - Child worktrees sit behind one line like a worktree's: "N child worktrees", their state counts and the chevron.
- **44 pt:** every line (header, worktree, fold) is its whole row and at least 44 pt tall (`RepoTreeLine`, `ButtonRowInsets`), the agents' rows too (the list's least row height).
- **Beside the chat** (regular width, By Repository), the open worktree's line or the open agent's row is washed blue (`Theme.Tree.selected`); while the open agent's repository is folded into Idle Repositories its folded line carries the wash in grouped rows and Compact List, and unfolding transfers it to the agent's row so exactly one is selected; the floating rows have no wash, as in the other groupings, and every display sets the selected accessibility trait on the folded line or open agent's row.
- **UI tests:** `AgentRepoTreeUITests` (`MOCK_WORKTREES=1 MOCK_ORCA_CHILDREN=1 MOCK_MANAGERS=1`, `PERSONA_WORKTREE_HUB`) and `AgentsTouchTargetUITests` (every grouping and display; `MOCK_AGENTS=1 MOCK_WORKTREES=1 MOCK_ORCA_CHILDREN=1 MOCK_DESKTOP_APPS=1`, `PERSONA_TOUCH_HUB`).

On the Mac, repository browsing follows Orca's workspace-card geometry: one project heading, a card with its title, an optional Primary badge, then project/branch identity, compact Agent rows with short activity ages (`now`, `5m`, `3h`, `2d`) and a child-count chip. Cross-project children show their repository badge only when their repository differs from the direct parent; descendants in that same repository omit the repeated badge, even under a different outer project heading. Stable project IDs take precedence over path/name fallbacks for older catalogs. Detached checkouts keep the technical label `Detached HEAD` in every interface language. Nested cards advance by twenty points, and selection shades the selected workspace and its ancestors. Detached Git checkouts show the host's short HEAD identity; the node bulk-reads it only when needed, with a bounded optional request and no terminal inspection. Host `unread`, `sidebar_status` and `workspace_kind` values drive workspace presentation separately from Agent state; older nodes fall back to the sessions they report. Reply previews and repeated location metadata stay out of the list. Multiple Agents fold behind a compact icon/count strip. Keyboard traversal follows those exact Agent and child folds, including idle sessions. Conversation navigation, context menus and session actions remain available.

The catalog is read every ten seconds while this list is visible, independently of the Workflow board toggle. Offline discovery uses the manager's existing snapshots. Transient failures retain the last successful catalog; older hubs fall back to the session-derived tree. The list creates no synthetic Agent and starts or stops no process. iOS and Mac share this model; the Mac draws the cards, iOS the tree above. This does not update an already installed TestFlight binary.

Clicking a worktree's title or branch opens its first direct Agent in the sidebar's displayed order, including when its Agent strip is folded. Clicking a specific Agent opens that conversation. Selection replaces the previous conversation; it does not open a separate worktree details page. An Agent-free worktree stays selected with the normal empty pane and keeps New Session and New Child Worktree in its context menu. The separate chevrons only fold Agents or children. Explicit workspace IDs take precedence over directories. Provider history without an Orca binding is associated by exact directory only when the catalog has one workspace at that path on that machine and hub; ambiguous history stays unassigned. Read-only history retains its original capabilities.

Right-click a worktree title, branch or direct Agent row on Mac, or long-press it on iPhone, for the same worktree commands: Open Worktree, New Session, New Child Worktree, Copy Path, Copy Worktree Name and, when present, Open Parent Worktree. It uses that exact workspace as the initial code source and parent, including a nested or Agent-free container. The form can change either choice independently; a folder needs a Git code source. Successful creation refreshes the sidebar catalog and opens the existing New Session form for the new child using the server-acknowledged parent. All regions use the owning worktree's identity. An Agent row also retains its session commands. A title with one Agent offers those commands directly; with multiple Agents it offers named submenus so session actions have an explicit target. Each descendant keeps its own menu scope. The UI names these containers worktrees; protocol and accessibility identifiers retain their workspace names for compatibility.

New Session also offers a searchable workspace picker, with full ancestor paths and an explicit Choose by Folder fallback. A chosen workspace sends its source ID and directory to the existing session-start route. New Child Worktree separates Code Source (the repository and base commit) from Parent Worktree (any Orca workspace on the same computer, including empty folders and other projects). On a phone, the selected parent's name has its own line above the ancestor path so a deep breadcrumb cannot hide the selected destination. The node advertises `choose_parent` before the client enables independent parent selection. With `automatic_name`, Create Worktree is ready without entering a branch or path on every supported engine. The native form keeps those overrides in Advanced, collapsed initially. An unnamed create sends `auto_name: true`. Orca receives a generated creature seed without a branch override and owns its prefix, retired-name/collision policy and first-work rename. Other supported hosts use a unique node-generated branch and directory through their existing creation route; subsequent naming follows the host’s capabilities. Explicit branch/path inputs retain their existing validation; older nodes keep the manual form. A created worktree appears through discovery even if the New Session form is cancelled before an Agent starts. Stale explicit launch targets and missing parents are refused rather than silently redirected.
