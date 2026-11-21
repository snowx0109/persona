package hub

// Golden tests for every prompt the hub writes (docs/assistant/prompts.md): representative inputs, the output compared byte for
// byte with testdata/prompts/<name>.golden. After changing a prompt on purpose, regenerate with
//
//	UPDATE_GOLDEN=1 go test ./internal/hub -run Golden
//
// and read the diff before committing it. All inputs are placeholders.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/assistant"
	"github.com/nwverse/persona/server/internal/assistantwire"
	"github.com/nwverse/persona/server/internal/connector"
	"github.com/nwverse/persona/server/internal/kb"
	"github.com/nwverse/persona/server/internal/prompts"
	"github.com/nwverse/persona/server/internal/proto"
)

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "prompts", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v (UPDATE_GOLDEN=1 writes it)", name, err)
	}
	if got != string(want) {
		t.Errorf("%s differs from %s\n--- got ---\n%s\n--- want ---\n%s", name, path, got, want)
	}
}

// capturedCall is one request the fake model from replies answered.
type capturedCall struct{ System, User string }

// goldenSnapshot is a chat service's connector with a live state (connector.Snapshot): what a plugin serving Feishu
// hands the assistant's snapshot.
type goldenSnapshot struct{}

func (goldenSnapshot) ID() string   { return "chatservice" }
func (goldenSnapshot) Name() string { return "Chat service" }
func (goldenSnapshot) Manifest() connector.Manifest {
	return connector.Manifest{Capabilities: []connector.Capability{connector.CapSnapshot}}
}
func (goldenSnapshot) Snapshot(context.Context, string) (connector.SnapshotData, error) {
	at := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }
	return connector.SnapshotData{
		Chats: &connector.SnapshotChats{
			AtMe:  []connector.SnapshotChat{{Name: "发布群", Unread: 3, Preview: "@我 周五灰度能不能提前到周四？这条预览很长很长很长很长很长很长很长很长很长很长很长"}},
			Other: []connector.SnapshotChat{{Name: "张三", Unread: 1}, {Name: "李四"}},
		},
		Meetings: []connector.SnapshotMeeting{
			{Title: "周会", Start: at("2099-01-01T11:00:00+08:00"), End: at("2099-01-01T12:00:00+08:00")},
			{Title: "评审", Start: at("2099-01-01T09:30:00+08:00"), End: at("2099-01-01T10:00:00+08:00")},
			{Title: "早就开完的会", Start: at("2000-01-01T09:00:00+08:00"), End: at("2000-01-01T10:00:00+08:00")},
		},
	}, nil
}

// goldenKB is a knowledge base with one chat, its summary and two projects.
// goldenKB is the knowledge base h's assistant keeps once it connects (withKB), with a few projects and chats.
func goldenKB(t *testing.T, h *Hub) *kb.Engine {
	t.Helper()
	e := withKB(t, h)
	st := e.Store
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	for _, p := range []*kb.Project{
		{Slug: "demo-release", Name: "示例发布", Kind: "short", Status: "active", Tagging: kb.Tagging{Importance: "P0", Relevance: 95}, OneLine: "周五灰度", Updated: now},
		{Slug: "demo-infra", Name: "示例基建", Kind: "long", Status: "active", Tagging: kb.Tagging{Importance: "P2", Relevance: 40}, OneLine: "", Updated: now},
		{Slug: "demo-old", Name: "做完的项目", Kind: "short", Status: "done", Tagging: kb.Tagging{Importance: "P1", Relevance: 80}, Updated: now},
	} {
		if err := st.PutProject(p); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.PutSummary(kb.KindProject, "demo-release", "# 示例发布\n\n## 目标\n周五灰度")
	c := &kb.Chat{ID: kb.ChatID("lark", "f1"), Source: "lark", SourceID: "f1", Kind: "group", Name: "发布群", Created: now,
		LastAt: now, OneLine: "在推示例发布", Tagging: kb.Tagging{Importance: "P1", Relevance: 70}, Projects: []string{"demo-release", "missing"}}
	_ = st.PutChat(c)
	_ = st.PutSummary(kb.KindChat, c.ID, "# 发布群\n\n## 这是什么\n发布协调")
	m := &kb.Chat{ID: kb.ChatID("mail", "acc1-sender@example.com"), Source: "mail", SourceID: "acc1-sender@example.com", Kind: "mail",
		Name: "示例来信", Created: now, LastAt: now, Tagging: kb.Tagging{Importance: "P3", Relevance: 10}}
	_ = st.PutChat(m)
	return e
}

func goldenMachines(h *Hub) {
	for _, mc := range []MachineConfig{
		{ID: "server", Name: "服务器", Kind: "local"},
		{ID: "laptop", Name: "笔记本", Kind: "ssh", SSHHost: "laptop.example"},
		{ID: "desk", Name: "台式机", Kind: "ssh"},
	} {
		m := newMachine(h, mc)
		h.machines = append(h.machines, m)
		h.byID[mc.ID] = m
	}
}

var (
	snapHeader = regexp.MustCompile(`^- 现在：\d{4}-\d\d-\d\d 周[日一二三四五六] \d\d:\d\d（([^）]+)）\n`)
)

// normalSnapshot replaces the clock in the snapshot's first line.
func normalSnapshot(t *testing.T, s string) string {
	t.Helper()
	m := snapHeader.FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("snapshot header: %q", s[:min(len(s), 200)])
	}
	return "- 现在：YYYY-MM-DD 周X HH:MM（" + m[1] + "）\n" + s[len(m[0]):]
}

func snapshotHub(t *testing.T, full bool) *Hub {
	h := newTestHub(t)
	h.cfg.PublicURL = "http://hub.example.com:18660" // the default names this machine, and it shows in the links
	putAssistantSettings(t, h, assistant.Settings{Timezone: "Asia/Shanghai"})
	goldenMachines(h)
	if !full { // no live state: chats and calendar left out
		connectAssistant(t, h)
		return h // no database: the assistant's schedule is its default routines, no follow-ups
	}
	// The chat service's live state, as the manager's connectors hand it to the assistant in its process.
	fakeExtOf(h).onApp(assistantwire.AppConnectors, assistantwire.ConnectorsSnapshots, func(raw json.RawMessage) (any, error) {
		var a struct {
			TZ string `json:"tz"`
		}
		_ = json.Unmarshal(raw, &a)
		d, err := goldenSnapshot{}.Snapshot(context.Background(), a.TZ)
		return map[string]any{"snapshots": []connector.SnapshotData{d}}, err
	})
	h.byID["server"].online = true
	h.byID["laptop"].online = true
	now := time.Now()
	h.byID["server"].sessions = []proto.Session{
		{SessionID: "aaaaaaaa-1111", Cwd: "/code/demo", Title: "修登录", Status: proto.StatusBlocked, UpdatedAt: now},
		{SessionID: "bbbbbbbb-2222", Cwd: "/code/demo", Title: "写文档", Status: proto.StatusDone, UpdatedAt: now},
		{SessionID: "cccccccc-3333", Cwd: "/code/demo", Title: "跑测试", Status: proto.StatusWorking, UpdatedAt: now, SpaceName: "示例空间", SlotName: "测试"},
		{SessionID: "dddddddd-4444", Cwd: "/code/old", Status: proto.StatusDone, UpdatedAt: now.Add(-48 * time.Hour)},
	}
	var working []proto.Session
	for i := range 9 {
		working = append(working, proto.Session{SessionID: "w" + string(rune('a'+i)), Cwd: "/code/w" + string(rune('a'+i)), Status: proto.StatusWorking, UpdatedAt: now})
	}
	h.byID["laptop"].sessions = working
	goldenKB(t, h)
	at := time.Date(2099, 1, 2, 1, 30, 0, 0, time.UTC)
	seedTasks(t, h, // the assistant's, which it reports to the hub
		assistant.Task{ID: "a", Title: "看 CI", Repeat: "once", At: at, Enabled: true},
		assistant.Task{ID: "b", Prompt: "巡检一下各台电脑上的 Agent 有没有卡住的，这句话会被截断", Repeat: "interval", EveryMinutes: 30, At: at, Enabled: true},
		assistant.Task{ID: "c", Title: "早报", Repeat: "daily", Time: "09:00", Enabled: true},
		assistant.Task{ID: "d", Title: "停了的", Repeat: "once", At: at, Enabled: false},
		assistant.Task{ID: "e", Repeat: "interval", EveryMinutes: 120, At: at, Enabled: true, Prompt: "x"},
	)
	connectAssistant(t, h)
	return h
}

// The snapshot is the assistant's (internal/assistant, context.go), read through the hub as a node asks for it: a side
// conversation's context is the snapshot alone.
func TestGoldenAssistantSnapshot(t *testing.T) {
	ctx := context.Background()
	snapshot := func(h *Hub) string {
		res, err := h.asst().Context(ctx, assistant.ContextParams{SessionID: "side-session"})
		if err != nil {
			t.Fatal(err)
		}
		return res.Text
	}
	golden(t, "assistant_snapshot.full", normalSnapshot(t, snapshot(snapshotHub(t, true))))
	golden(t, "assistant_snapshot.bare", normalSnapshot(t, snapshot(snapshotHub(t, false))))
}

// The main conversation's context starts with a line saying so (needs Postgres for the saved main conversation).
func TestGoldenAssistantContextMain(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()
	if err := store.PutSetting(ctx, "assistant", assistant.Settings{Main: "main-session", Timezone: "Asia/Shanghai"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.PutSetting(ctx, "assistant", nil) })
	h := snapshotHub(t, false)
	h.store = store
	setManagerJSON(t, h, "assistant.owner", "示例用户")
	raw, _ := json.Marshal(map[string]string{"session_id": "main-session"})
	res, err := nodeAsks(ctx, h.byID["server"], "assistant.context", raw)
	if err != nil {
		t.Fatal(err)
	}
	text := contextOf(t, res)
	prefix, rest, ok := strings.Cut(text, "\n\n")
	if !ok {
		t.Fatalf("%q", text)
	}
	golden(t, "assistant_context.main", prefix+"\n\n"+normalSnapshot(t, rest))
	raw, _ = json.Marshal(map[string]string{"session_id": "side-session"})
	res, _ = nodeAsks(ctx, h.byID["server"], "assistant.context", raw)
	golden(t, "assistant_context.side", normalSnapshot(t, contextOf(t, res)))
}

// contextOf is the text of the node's assistant.context answer, as it goes on the wire.
func contextOf(t *testing.T, res any) string {
	t.Helper()
	raw, _ := json.Marshal(res)
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Text
}

func TestGoldenBotMessage(t *testing.T) {
	golden(t, "bot_message.persona", prompts.RenderBotMessage(prompts.BotMessage{Persona: true, Owner: "示例用户", Text: "今天有什么会？",
		Quote: &prompts.BotQuote{Who: "Persona", Text: "上午 10 点有周会。"}}))
	golden(t, "bot_message.agent_files", prompts.RenderBotMessage(prompts.BotMessage{Text: "看下这个日志",
		Forward: "11-02 10:00 张三：构建挂了\n11-02 10:01 李四：我看下", Files: []string{"/tmp/persona-uploads/build.log"}}))
	golden(t, "bot_message.only_files", prompts.RenderBotMessage(prompts.BotMessage{Persona: true, Files: []string{"/tmp/a.pdf", "/tmp/b.png"}}))
}

// A message to a plugin's chat app bot (docs/extensions/channels.md): plain text and text answers (WeChat), Markdown without
// answering there, what came that cannot be passed on, a picture alone.
func TestGoldenChannelMessage(t *testing.T) {
	golden(t, "channel_message.plain", prompts.RenderChannelMessage(prompts.ChannelMessage{Persona: true, Name: "微信", Owner: "示例用户",
		Plain: true, Asks: true, Text: "明天有什么会？"}))
	golden(t, "channel_message.quote_unsupported", prompts.RenderChannelMessage(prompts.ChannelMessage{Persona: true, Name: "微信", Plain: true,
		Asks: true, Text: "看下这个", Quote: "张三\n合同明天要签", Unsupported: []string{"文件「合同.pdf」", "一段视频"}}))
	golden(t, "channel_message.markdown_files", prompts.RenderChannelMessage(prompts.ChannelMessage{Persona: true, Name: "Example Chat",
		Text: "看下这个日志", Forward: "11-02 10:00 张三：构建挂了", Files: []string{"/tmp/persona-uploads/build.log"}}))
	golden(t, "channel_answer_again.feishu", prompts.RenderChannelAnswerAgain(prompts.ChannelAnswerAgain{Name: "飞书"}))
	golden(t, "channel_message.only_unsupported", prompts.RenderChannelMessage(prompts.ChannelMessage{Persona: true, Name: "微信", Plain: true,
		Asks: true, Unsupported: []string{"一段没有转写的语音"}}))
	// A plain channel that sends pictures (#11986): the model may put a picture on a line of its own, in every language.
	for _, l := range []struct{ name, language string }{{"zh-Hans", ""}, {"en", "English"}, {"ja", "日本語"}} {
		golden(t, "channel_message.images_out."+l.name, prompts.RenderChannelMessage(prompts.ChannelMessage{Persona: true, Name: "WeChat",
			Plain: true, Asks: true, ImagesOut: true, Language: l.language, Text: "Show me the chart"}))
	}
}

// A mention is public even when its turn runs in the owner's main conversation. Its origin and gathered context are
// data, and the caution not to disclose private context follows the caller's language (#11130). Decode the fixture as
// JSON so a host without the new fields fails by losing behavior, rather than by failing to compile this test.
func TestChannelMentionMessageCarriesOriginAndLocalizedPublicCaution(t *testing.T) {
	for _, tc := range []struct {
		name, language string
		caution        []string
	}{
		{"zh-Hans", "", []string{"其他对话", "其他会话", "邮件", "健康", "明确"}},
		{"en", "English", []string{"other chats", "sessions", "mail", "health", "explicitly"}},
		{"ja", "日本語", []string{"チャット", "セッション", "メール", "健康", "明示"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, _ := json.Marshal(map[string]any{
				"Persona": true, "Name": "Example Chat", "Language": tc.language, "Public": true,
				"Chat": "Release chat", "Topic": "Release 1.3", "Quote": "We ship on Friday.",
				"Context": "Alice: The migration is done.", "Text": "What changed?",
			})
			var in prompts.ChannelMessage
			if err := json.Unmarshal(fixture, &in); err != nil {
				t.Fatal(err)
			}
			got := prompts.RenderChannelMessage(in)
			golden(t, "channel_mention_message."+tc.name, got)
			for _, want := range []string{"Release chat", "Release 1.3", "We ship on Friday.", "The migration is done.", "What changed?"} {
				if !strings.Contains(got, want) {
					t.Errorf("origin/context lost %q:\n%s", want, got)
				}
			}
			for _, want := range tc.caution {
				if !strings.Contains(got, want) {
					t.Errorf("public caution lost %q:\n%s", want, got)
				}
			}
		})
	}
}

// Closing a finished task (docs/assistant/todos.md "Automatic completion"): the judge's system prompt and its input, with and without hints.
func TestGoldenTaskDone(t *testing.T) {
	golden(t, "task_done", prompts.RenderTaskDone(prompts.TaskDone{Owner: "示例用户"}))
	golden(t, "task_done.delivery", prompts.RenderTaskDone(prompts.TaskDone{Owner: "示例用户", Delivery: true}))
	// The reason in the user's language: the apps show it on the task (#3180).
	golden(t, "task_done.language", prompts.RenderTaskDone(prompts.TaskDone{Owner: "示例用户", Language: "English"}))
	golden(t, "task_delivery", prompts.RenderTaskDelivery(false)) // handed to a task's agent after its prompt (#3306)
	// The user's files, copied to the agent's computer (#2437).
	golden(t, "task_attachments", prompts.RenderTaskAttachments(prompts.TaskAttachments{Copied: true,
		Files:    []string{"/home/me/.persona/uploads/terminal/2026-11-09/ab12-notes.txt"},
		Missing:  []string{"/tmp/report.pdf（over 50 MB）"},
		Pictures: []string{"/home/me/.persona/uploads/terminal/2026-11-09/cd34-screen.png"}}))
	golden(t, "task_done_input.hints", prompts.RenderTaskDoneInput(prompts.TaskDoneInput{Title: "修登录崩溃",
		Prompt:   "iPad 横屏时登录页崩溃，修好并补测试，走 PR 合并后发 TestFlight",
		Replies:  []prompts.TaskDoneReply{{Machine: "box", Text: "修好了，PR https://github.com/o/r/pull/901 已合并，TestFlight 构建号 202611041200"}},
		MergedPR: true, Build: true}))
	golden(t, "task_done_input.two_agents", prompts.RenderTaskDoneInput(prompts.TaskDoneInput{Title: "用新 logo 设计默认形象",
		Prompt:  "按新 logo 重画默认形象的全部表情",
		Replies: []prompts.TaskDoneReply{{Machine: "box", Text: "没有全改完，还差三个表情"}, {Machine: "mac", Text: "预览图放在 /tmp/preview.png"}}}))
	golden(t, "task_done_input.follow_up", prompts.RenderTaskDoneInput(prompts.TaskDoneInput{Title: "修任务启动失败",
		Prompt: "任务启动失败时一直显示正在开，修好",
		Replies: []prompts.TaskDoneReply{{Machine: "box", Ask: "再把「全部 N」挪到这一行最左边",
			Text: "挪好了，PR 已合并，TestFlight 构建号 202611040923。没截图的：Mac。"}}, Build: true}))
	// The user only asked whether it is done: the report is in the turn before (#3111).
	golden(t, "task_done_input.check_in", prompts.RenderTaskDoneInput(prompts.TaskDoneInput{Title: "新空间加模型",
		Prompt: "给新空间加上新模型，部署并验证",
		Replies: []prompts.TaskDoneReply{{Machine: "mac", Ask: "好了吗？", Text: "好了，全部完成。",
			Earlier: "加好了：PR https://github.com/o/r/pull/77 已合并，已部署，接口实测返回新模型列表。"}}}))
}

// What a finished task came to (todo/outcome, #2820): the system prompt, and the task with two agents' last replies.
func TestTodoOutcomeGolden(t *testing.T) {
	golden(t, "todo_outcome", prompts.RenderTodoOutcome(prompts.TodoOutcome{Owner: "示例用户"}))
	golden(t, "todo_outcome_input", prompts.RenderTodoOutcomeInput(prompts.TodoOutcomeInput{Title: "修登录页横屏崩溃",
		Prompt: "iPad 横屏时登录页崩溃，修好并补测试", Replies: []prompts.TodoOutcomeReply{
			{Machine: "box", Cwd: "/code/weather/ios", Text: "修好了，PR https://github.com/example/weather/pull/12 已合并。"},
			{Machine: "laptop", Text: "测试补好了。"},
		}}))
}

// The project background a new agent gets (todo/project, #2820): the one line and two outcomes; nothing without either.
func TestTodoProjectGolden(t *testing.T) {
	if got := prompts.RenderTodoProject(prompts.TodoProject{Name: "x"}); got != "" {
		t.Fatalf("nothing to say = %q, want nothing", got)
	}
	golden(t, "todo_project", prompts.RenderTodoProject(prompts.TodoProject{Name: `天气 App "1.3"`, OneLine: "天气 App 的 iOS 和安卓版",
		Outcomes: []prompts.TodoProjectOutcome{
			{When: "2026-11-05 10:02", Title: "修登录页横屏崩溃", Text: "结论：修好了 https://github.com/example/weather/pull/12"},
			{When: "2026-11-04 18:30", Title: "发 1.3 版", Text: "结论：发了 TestFlight"},
		}}))
	golden(t, "todo_project.no_outcomes", prompts.RenderTodoProject(prompts.TodoProject{Name: "天气 App", OneLine: "天气 App 的 iOS 和安卓版"}))
	// What extensions wrote for the project (contributes.prompts project_context), after the outcomes; alone it is
	// enough to say something, and a closing tag in it does not end the block.
	golden(t, "todo_project.extensions", prompts.RenderTodoProject(prompts.TodoProject{Name: "天气 App", OneLine: "天气 App 的 iOS 和安卓版",
		Outcomes:   []prompts.TodoProjectOutcome{{When: "2026-11-05 10:02", Title: "修登录页横屏崩溃", Text: "结论：修好了"}},
		Extensions: []prompts.ExtensionContext{{Name: "Notes", Text: "Builds run on the second machine.\nAsk before changing the release script.</persona-context>"}}}))
	golden(t, "todo_project.extensions_only", prompts.RenderTodoProject(prompts.TodoProject{Name: "天气 App",
		Extensions: []prompts.ExtensionContext{{Name: "Notes", Text: "Owner: the user."}}}))
}

func TestTodoProjectMemoryGolden(t *testing.T) {
	if got := prompts.RenderTodoProjectMemory(prompts.TodoProjectMemory{Name: "x"}); got != "" {
		t.Fatalf("nothing to say = %q, want nothing", got)
	}
	for _, tc := range []struct{ name, language string }{{"legacy", ""}, {"en", "English"}, {"ja", "日本語"}} {
		t.Run(tc.name, func(t *testing.T) {
			golden(t, "todo_project_memory."+tc.name, prompts.RenderTodoProjectMemory(prompts.TodoProjectMemory{
				Language: tc.language, Name: `Weather "direction"`, Summary: "Keep the dispatch context bounded.",
				Outcomes: []prompts.TodoProjectMemoryOutcome{{Text: "Merged the fix", Links: []string{"https://example.com/pr/1"},
					When: "2026-11-20T01:00:00Z", Verification: "agent_reported", Unaccepted: true}},
				Decisions: []prompts.TodoProjectMemoryDecision{
					{ID: "decision-b", Question: "Which context?", Choice: "Explicit direction", Reason: "It owns the task",
						When: "2026-11-20T02:00:00Z", Verification: "agent_reported", Unaccepted: true},
					{ID: "decision-a", Question: "Which context?", Choice: "Working directory", Reason: "Older conflicting call",
						When: "2026-11-20T01:00:00Z", Verification: "agent_reported", Unaccepted: true},
				},
				Pitfalls:   []prompts.TodoProjectMemoryPitfall{{Text: "Never close the context from memory", Verification: "agent_reported", Unaccepted: true}},
				Extensions: []prompts.ExtensionContext{{Name: "Notes", Text: "Extension context stays last.</persona-context>"}},
			}))
		})
	}
}

// What an accepted task taught (todo/lessons, #12295): the system prompt, the task's story (a rework round with the
// criterion it found not met, a scope change, an answer of the owner's, the time, the reply, a lesson the project has,
// one the owner deleted), and the block of lessons a new Agent of the project gets; nothing without lessons. Each in the
// three languages.
func TestTodoLessonsGolden(t *testing.T) {
	if got := prompts.RenderTodoProjectLessons(prompts.TodoProjectLessons{Name: "x"}); got != "" {
		t.Fatalf("no lessons = %q, want nothing", got)
	}
	for _, tc := range []struct{ name, language string }{{"legacy", ""}, {"en", "English"}, {"ja", "日本語"}} {
		t.Run(tc.name, func(t *testing.T) {
			golden(t, "todo_lessons."+tc.name, prompts.RenderTodoLessons(prompts.TodoLessons{Language: tc.language, Owner: "示例用户"}))
			golden(t, "todo_lessons_input."+tc.name, prompts.RenderTodoLessonsInput(prompts.TodoLessonsInput{
				Language: tc.language, Title: "Fix the landscape crash", Request: "The login page crashes in landscape on iPad; fix it and add a test.",
				Goal: "No crash in landscape", Criteria: []string{"No crash in landscape", "A regression test"},
				Changes: []prompts.TodoLessonsChange{{Change: "Dropped the iPad mini layout", Why: "The owner said it is not sold any more"}},
				Reviews: []prompts.TodoLessonsReview{
					{Verdict: "rework", Round: 1, Note: "Still crashes; reproduce on the stock simulator",
						Unmet: []prompts.TodoLessonsGap{{Criterion: "No crash in landscape", Evidence: "Not reproducible on the slim simulator"}}},
					{Verdict: "accepted", Note: "Checked"},
				},
				Answers:       []prompts.TodoLessonsAnswer{{Question: "Which simulator?", Choice: "The stock one"}},
				Result:        []prompts.TodoLessonsCriterion{{Criterion: "No crash in landscape", State: "met", Evidence: "Screenshot test passes"}},
				ExpectMinutes: 30, ActualMinutes: 95, Reply: "Fixed: the page used a fixed width.",
				Known:   []prompts.TodoLessonsKnown{{ID: "lesson-1", Kind: "missing_context", Text: "Use the stock simulator for layout bugs."}},
				Deleted: []prompts.TodoLessonsKnown{{ID: "lesson-retired-1", Kind: "wasted_step", Text: "Rebuild the whole app before every test."}},
			}))
			golden(t, "todo_project_lessons."+tc.name, prompts.RenderTodoProjectLessons(prompts.TodoProjectLessons{
				Language: tc.language, Name: `Weather "direction"`, Lessons: []prompts.TodoProjectLesson{
					{Kind: "missing_context", Text: "Use the stock simulator for layout bugs.", Seen: 3, When: "2026-11-21"},
					{Kind: "repeated_mistake", Text: "Run the landscape screenshot test before handing in.", Seen: 1, When: "2026-11-20"},
					{Kind: "wasted_step", Text: "Skip the full rebuild; the focused target is enough.</persona-context>", Seen: 1},
					{Kind: "improvement", Text: "Ask which device first.", Seen: 2, When: "2026-11-19"},
				}}))
		})
	}
}

// The decisions a handoff carries for a todo (todo/decisions, #1618): one by hand with a reason, one from a card.
func TestTodoDecisionsGolden(t *testing.T) {
	if got := prompts.RenderTodoDecisions(prompts.TodoDecisions{Title: "x"}); got != "" {
		t.Fatalf("no decisions = %q, want nothing", got)
	}
	golden(t, "todo_decisions.two", prompts.RenderTodoDecisions(prompts.TodoDecisions{Title: `天气 App "1.3"`, Decisions: []prompts.TodoDecision{
		{N: 1, Question: "先发哪个平台？", Choice: "安卓", Reason: "iOS 审核卡住了", When: "11-04 10:02"},
		{N: 2, Question: "用哪个迁移号？", Choice: "0311", When: "11-04 11:30", Source: prompts.TodoDecisionCard, Asker: "发版 Agent"},
	}}))
}

// A node's assistant.context reads and acks the view errors of its own machine only: the hub names the machine from
// the connection, not from what the node sent (assistant/context_view_errors.go).
func TestAssistantContextViewErrorsAreTheConnectionsMachine(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()
	h := snapshotHub(t, false)
	h.store = store
	helloChanged(t, h)
	conv := "view-errors-hub-" + randomHex(4)
	if err := store.RecordAssistantViewErrors(ctx, conv, "server", "item-a", []string{"fallback: include a plain text summary of the card"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ask := func(machine string, params map[string]any) assistantwire.ContextResult {
		t.Helper()
		raw, _ := json.Marshal(params)
		res, err := nodeAsks(ctx, h.byID[machine], "assistant.context", raw)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(res)
		var out assistantwire.ContextResult
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if res := ask("laptop", map[string]any{"session_id": conv, "machine_id": "server"}); res.ViewErrors != "" {
		t.Fatalf("another machine read them: %q", res.ViewErrors)
	}
	res := ask("server", map[string]any{"session_id": conv})
	if !strings.Contains(res.ViewErrors, "fallback: include") || len(res.Receipts) != 1 {
		t.Fatalf("%+v", res)
	}
	ask("laptop", map[string]any{"session_id": conv, "machine_id": "server", "ack": res.Receipts})
	if again := ask("server", map[string]any{"session_id": conv}); again.ViewErrors != res.ViewErrors {
		t.Fatal("another machine's ack consumed them")
	}
	ask("server", map[string]any{"session_id": conv, "ack": res.Receipts})
	if after := ask("server", map[string]any{"session_id": conv}); after.ViewErrors != "" {
		t.Fatalf("after the ack %q", after.ViewErrors)
	}
}
