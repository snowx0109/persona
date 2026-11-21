package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/kb"
	"github.com/nwverse/persona/server/internal/prompts"
	"github.com/nwverse/persona/server/internal/proto"
)

const lessonsAnswer = `{"lessons":[
	{"kind":"missing_context","text":"登录页要在 iPad 原版模拟器上复现，瘦身的复现不了。","same_as":""},
	{"kind":"repeated_mistake","text":"改完布局先跑横屏截图测试再交。","same_as":""},
	{"kind":"rule","text":"不是四种之一的不记。","same_as":""}]}`

// lessonsEnv is outcomeEnv with lessons on (unless off) and the task supervised from conv1 with one criterion.
func lessonsEnv(t *testing.T, on bool) (*Service, *fakeHub, Todo) {
	t.Helper()
	s, f, todo := outcomeEnv(t, nil, outcomeReply)
	// The agent's session names its computer and finished its turn, so a rework reaches it.
	f.setSessions("box", proto.Session{MachineID: "box", SessionID: "s1", Title: "修登录", Cwd: "/code/weather/ios",
		Status: proto.StatusDone, LastText: outcomeReply})
	f.outcome = `{"outcome":"修好了","links":[],"decisions":[],"pitfalls":[]}`
	f.lessons = lessonsAnswer
	if on {
		setSettings(t, s, func(v *Settings) { v.TaskLessons = new(true) })
	}
	s.superviseTodo(context.Background(), todo.ID, "conv1", []string{"横屏不再崩溃"}, 0)
	return s, f, todoNow(s, todo.ID)
}

func lessonsAsked(f *fakeHub) []modelCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.lessonsAsked)
}

// acceptAfterRework records that the task was sent back once with a criterion not met (as task_review's rework writes
// its ledger; this store keeps no feed for the rework's message to the agent), then accepts it with task_review.
func acceptAfterRework(t *testing.T, s *Service, todo Todo) {
	t.Helper()
	if _, _, err := s.updateSupervision(context.Background(), todo.ID, func(v *Supervision) bool {
		v.Round++
		v.State = reviewRework
		v.Reviews = append(v.Reviews, TaskReview{At: time.Now().UTC(), Verdict: reviewRework, Round: v.Round,
			Note:   "横屏还会崩，先在原版模拟器上复现",
			Checks: []ReviewCheck{{Criterion: "横屏不再崩溃", Met: false, Evidence: "瘦身模拟器上看不出问题"}}})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	reviewTask(t, s, todo.ID, map[string]any{"verdict": reviewAccepted, "note": "看过了",
		"checks": completeReviewChecks(todoNow(s, todo.ID))})
	s.tasks.outcomeRuns.Wait()
}

// With lessons on, a task accepted after a rework round teaches its project: the model reads the rework, its unmet
// criterion and the reply, and the lessons of a known kind go into the project's memory.jsonl with their source and
// time. The next new Agent in the project gets them after the project's own block.
func TestTaskLessonsLearnedFromAReworkedTask(t *testing.T) {
	s, f, todo := lessonsEnv(t, true)
	acceptAfterRework(t, s, todo)
	asked := lessonsAsked(f)
	if len(asked) != 1 {
		t.Fatalf("the lessons model was asked %d times, want once", len(asked))
	}
	for _, want := range []string{"横屏还会崩，先在原版模拟器上复现", "没达到：横屏不再崩溃（瘦身模拟器上看不出问题）", "rework · 第 1 轮：", "注意 iPad 模拟器要用原版的"} {
		if !strings.Contains(asked[0].User, want) {
			t.Fatalf("the model's input lacks %q:\n%s", want, asked[0].User)
		}
	}
	live := kbEng(t, s).ProjectLessons("天气-app").Live
	if len(live) != 2 {
		t.Fatalf("lessons = %+v, want the two of a known kind", live)
	}
	for _, l := range live {
		if l.Type != kb.ProjectMemoryLesson || l.TodoID != todo.ID || l.Verification != kb.ProjectMemoryAccepted || l.Seen != 1 ||
			l.RecordedAt == "" || l.LastSeenAt == "" || !slices.Equal(l.Sources, []string{todo.ID}) {
			t.Fatalf("lesson provenance = %+v", l)
		}
	}
	got := s.projectContext(context.Background(), "box", "/code/weather")
	block := `<persona-context kind="context" title="项目 · 天气 App · 经验" url="">`
	at := strings.Index(got, block)
	if at < 0 || !strings.HasPrefix(got, `<persona-context kind="context" title="项目 · 天气 App · 背景"`) ||
		!strings.Contains(got[at:], "- 该知道的背景：登录页要在 iPad 原版模拟器上复现，瘦身的复现不了。") ||
		!strings.Contains(got[at:], "- 反复出现的错误：改完布局先跑横屏截图测试再交。") {
		t.Fatalf("project context = %q", got)
	}
	// The same completion is never learned from twice, whatever calls it again.
	s.learnTaskLessons(context.Background(), todo, *todoNow(s, todo.ID).DoneAt)
	if n := len(lessonsAsked(f)); n != 1 {
		t.Fatalf("a completion already learned from asked the model again (%d)", n)
	}
}

// Off (the default) nothing runs: no model call, no lesson, and the next Agent's context is what it was, even when the
// project has lessons from before.
func TestTaskLessonsOffLearnsAndHandsOutNothing(t *testing.T) {
	s, f, todo := lessonsEnv(t, false)
	ctx := context.Background()
	acceptAfterRework(t, s, todo)
	if n := len(lessonsAsked(f)); n != 0 {
		t.Fatalf("lessons off asked the model %d times", n)
	}
	e := kbEng(t, s)
	if l := e.ProjectLessons("天气-app"); len(l.IDs) != 0 {
		t.Fatalf("lessons off wrote %+v", l)
	}
	before := s.projectContext(ctx, "box", "/code/weather")
	appendTestLesson(t, e, "天气-app", "lesson-old-1", kb.LessonImprovement, "先跑单个测试。", time.Now())
	if after := s.projectContext(ctx, "box", "/code/weather"); after != before || strings.Contains(after, "先跑单个测试") {
		t.Fatalf("lessons off changed the context:\nbefore %q\nafter  %q", before, after)
	}
	// Turned on, the same lesson is handed out.
	setSettings(t, s, func(v *Settings) { v.TaskLessons = new(true) })
	if on := s.projectContext(ctx, "box", "/code/weather"); !strings.HasPrefix(on, before) || !strings.Contains(on, "- 更好的做法：先跑单个测试。") {
		t.Fatalf("lessons on: %q", on)
	}
}

// A task accepted at once, in time, with nothing asked of the owner has nothing to learn: no model call.
func TestTaskLessonsSmoothAcceptanceMakesNoModelCall(t *testing.T) {
	s, f, todo := lessonsEnv(t, true)
	reviewTask(t, s, todo.ID, map[string]any{"verdict": reviewAccepted, "note": "看过了", "checks": completeReviewChecks(todo)})
	s.tasks.outcomeRuns.Wait()
	if n := len(lessonsAsked(f)); n != 0 {
		t.Fatalf("a smooth acceptance asked the lessons model %d times", n)
	}
	if l := kbEng(t, s).ProjectLessons("天气-app"); len(l.IDs) != 0 {
		t.Fatalf("lessons = %+v", l)
	}
}

// Without a model nothing is learned: no lesson is invented from the story itself.
func TestTaskLessonsWithoutAModelInventNothing(t *testing.T) {
	s, f, todo := lessonsEnv(t, true)
	f.mu.Lock()
	f.lessons = ""
	f.mu.Unlock()
	acceptAfterRework(t, s, todo)
	if n := len(lessonsAsked(f)); n != 1 {
		t.Fatalf("model asked %d times", n)
	}
	if l := kbEng(t, s).ProjectLessons("天气-app"); len(l.IDs) != 0 {
		t.Fatalf("a failed model call wrote %+v", l)
	}
}

func appendTestLesson(t *testing.T, e *kb.Engine, slug, id, kind, text string, at time.Time) {
	t.Helper()
	stamp := at.UTC().Format(time.RFC3339Nano)
	if _, err := e.UpdateProjectLessons(slug, func(kb.ProjectLessons) ([]kb.ProjectMemoryRecord, error) {
		return []kb.ProjectMemoryRecord{{ID: id, Type: kb.ProjectMemoryLesson, Kind: kind, Conclusion: text, TodoID: "todo-old",
			Verification: kb.ProjectMemoryAccepted, RecordedAt: stamp, Seen: 1, FirstSeenAt: stamp, LastSeenAt: stamp, Sources: []string{"todo-old"}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A lesson the project has is taught again, not stored twice: the one the model named, else one with the same text.
// Its count goes up, the to-do joins its sources and the owner's text is kept. The same to-do never counts twice, a
// lesson the owner deleted is not learned again (in the words of any of its versions, or named in same_as), and a
// completion already learned from writes nothing.
func TestPlanLessonsMergesAndHonoursDeletions(t *testing.T) {
	now := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	old := kb.ProjectMemoryRecord{ID: "lesson-a", Type: kb.ProjectMemoryLesson, Kind: kb.LessonMissingContext,
		Conclusion: "The staging hub needs its own token (owner's words).", Verification: kb.ProjectMemoryOwner, Seen: 2,
		RecordedAt: "2026-11-01T00:00:00Z", FirstSeenAt: "2026-10-01T00:00:00Z", LastSeenAt: "2026-11-01T00:00:00Z",
		Sources: []string{"todo-1", "todo-2"}}
	same := kb.ProjectMemoryRecord{ID: "lesson-b", Type: kb.ProjectMemoryLesson, Kind: kb.LessonWastedStep,
		Conclusion: "Run the focused test before the whole suite.", Seen: 1, RecordedAt: "2026-11-02T00:00:00Z", Sources: []string{"todo-3"}}
	cur := kb.ProjectLessons{Live: []kb.ProjectMemoryRecord{same, old},
		Retired: []kb.ProjectMemoryRecord{{ID: "lesson-x", Type: kb.ProjectMemoryLessonRetired, SupersedesID: "lesson-c",
			Kind: kb.LessonImprovement, Conclusion: "Always rebuild everything."}},
		RetiredVersions: []kb.ProjectMemoryRecord{
			{ID: "lesson-c", Type: kb.ProjectMemoryLesson, SupersedesID: "lesson-c0", Kind: kb.LessonImprovement, Conclusion: "Always rebuild everything."},
			{ID: "lesson-c0", Type: kb.ProjectMemoryLesson, Kind: kb.LessonImprovement, Conclusion: "Rebuild the app from scratch each time."}},
		IDs: map[string]bool{"lesson-a": true, "lesson-b": true, "lesson-c": true, "lesson-c0": true, "lesson-x": true}}
	key := lessonKey("todo-9", now)
	got := planLessons(cur, "todo-9", key, now, []foundLesson{
		{Kind: kb.LessonMissingContext, Text: "Staging needs a separate token.", SameAs: "lesson-a"},
		{Kind: kb.LessonImprovement, Text: "run the focused test, before the whole suite", SameAs: ""},
		{Kind: kb.LessonImprovement, Text: "ALWAYS rebuild everything", SameAs: ""},
		{Kind: kb.LessonRepeatedMistake, Text: "Check the landscape layout before handing in.", SameAs: "lesson-gone"},
		{Kind: kb.LessonImprovement, Text: "rebuild the app from scratch, each time", SameAs: ""},
		{Kind: kb.LessonImprovement, Text: "Start every build clean.", SameAs: "lesson-x"},
		{Kind: kb.LessonImprovement, Text: "Start every build clean.", SameAs: "lesson-c0"},
	})
	if len(got) != 3 {
		t.Fatalf("plan = %+v, want two merges and one new lesson", got)
	}
	a, b, n := got[0], got[1], got[2]
	if a.SupersedesID != "lesson-a" || a.Seen != 3 || a.Conclusion != old.Conclusion || a.Verification != kb.ProjectMemoryOwner ||
		a.FirstSeenAt != old.FirstSeenAt || !slices.Equal(a.Sources, []string{"todo-1", "todo-2", "todo-9"}) || a.TodoID != "todo-9" ||
		!strings.HasPrefix(a.ID, key+"-") || a.LastSeenAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("merge by same_as = %+v", a)
	}
	if b.SupersedesID != "lesson-b" || b.Seen != 2 || b.Kind != kb.LessonWastedStep || b.FirstSeenAt != same.RecordedAt {
		t.Fatalf("merge by text = %+v", b)
	}
	if n.SupersedesID != "" || n.Seen != 1 || n.Kind != kb.LessonRepeatedMistake || n.Verification != kb.ProjectMemoryAccepted ||
		!slices.Equal(n.Sources, []string{"todo-9"}) {
		t.Fatalf("new lesson = %+v", n)
	}
	// Written, the completion is learned: planning it again writes nothing.
	for _, r := range got {
		cur.IDs[r.ID] = true
	}
	if again := planLessons(cur, "todo-9", key, now, []foundLesson{{Kind: kb.LessonWastedStep, Text: "Something new."}}); again != nil {
		t.Fatalf("a completion learned from twice: %+v", again)
	}
	// A to-do already among a lesson's sources does not count it again.
	cur2 := kb.ProjectLessons{Live: []kb.ProjectMemoryRecord{old}, IDs: map[string]bool{"lesson-a": true}}
	if again := planLessons(cur2, "todo-2", lessonKey("todo-2", now.Add(time.Hour)), now, []foundLesson{{Kind: kb.LessonMissingContext, Text: "x", SameAs: "lesson-a"}}); again != nil {
		t.Fatalf("the same to-do taught a lesson twice: %+v", again)
	}
}

// The model reads the newest deleted lessons with their tombstones' IDs, each text once and at most lessonsDeletedMost,
// after the project's lessons; with none deleted its input has no such list.
func TestLessonsInputListsTheNewestDeletedLessons(t *testing.T) {
	todo := Todo{Title: "修登录", Supervision: &Supervision{}}
	known := kb.ProjectLessons{Live: []kb.ProjectMemoryRecord{{ID: "lesson-live", Kind: kb.LessonImprovement, Conclusion: "先跑单个测试。"}}}
	for i := range lessonsDeletedMost + 2 {
		known.Retired = append(known.Retired, kb.ProjectMemoryRecord{ID: fmt.Sprintf("lesson-retired-%d", i),
			Type: kb.ProjectMemoryLessonRetired, SupersedesID: fmt.Sprintf("lesson-%d", i), Kind: kb.LessonWastedStep,
			Conclusion: fmt.Sprintf("删掉的第 %d 条。", i)})
	}
	// The owner gave a later lesson the text of one deleted before and deleted it too: the model reads that text once.
	known.Retired = slices.Insert(known.Retired, 1, kb.ProjectMemoryRecord{ID: "lesson-retired-again",
		Type: kb.ProjectMemoryLessonRetired, SupersedesID: "lesson-again", Kind: kb.LessonWastedStep, Conclusion: "删掉的第 0 条"})
	in := lessonsInput("", todo, nil, nil, time.Time{}, nil, nil, known)
	if len(in.Deleted) != lessonsDeletedMost || in.Deleted[0].ID != "lesson-retired-0" || in.Deleted[1].ID != "lesson-retired-1" ||
		in.Deleted[lessonsDeletedMost-1].ID != fmt.Sprintf("lesson-retired-%d", lessonsDeletedMost-1) {
		t.Fatalf("deleted = %+v", in.Deleted)
	}
	got := prompts.RenderTodoLessonsInput(in)
	if want := "这个项目已经记下的经验：\n- [lesson-live] improvement：先跑单个测试。\n\n用户删掉的经验（不要再学）：\n- [lesson-retired-0] wasted_step：删掉的第 0 条。\n- [lesson-retired-1] wasted_step：删掉的第 1 条。\n"; !strings.Contains(got, want) {
		t.Fatalf("input = %q, want it to hold %q", got, want)
	}
	if none := prompts.RenderTodoLessonsInput(lessonsInput("", todo, nil, nil, time.Time{}, nil, nil, kb.ProjectLessons{})); strings.Contains(none, "删掉") {
		t.Fatalf("an input with nothing deleted lists deleted lessons: %q", none)
	}
}

// A new Agent gets the fresh lessons, the most repeated first, then the newest; at most two of one kind, five in all
// and within the runes; a lesson not taught for 90 days is left out.
func TestLessonsToShowOrderAndBounds(t *testing.T) {
	now := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	lesson := func(id, kind string, seen int, ago time.Duration, text string) kb.ProjectMemoryRecord {
		at := now.Add(-ago).Format(time.RFC3339Nano)
		return kb.ProjectMemoryRecord{ID: id, Type: kb.ProjectMemoryLesson, Kind: kind, Conclusion: text, Seen: seen, LastSeenAt: at, RecordedAt: at}
	}
	day := 24 * time.Hour
	live := []kb.ProjectMemoryRecord{
		lesson("new-once", kb.LessonImprovement, 1, day, "a"),
		lesson("old-thrice", kb.LessonImprovement, 3, 30*day, "b"),
		lesson("stale-many", kb.LessonMissingContext, 9, 91*day, "c"),
		lesson("third-improvement", kb.LessonImprovement, 2, 2*day, "d"),
		lesson("mistake", kb.LessonRepeatedMistake, 1, 3*day, "e"),
		lesson("waste", kb.LessonWastedStep, 1, 4*day, "f"),
		lesson("context", kb.LessonMissingContext, 1, 5*day, "g"),
		lesson("mistake-2", kb.LessonRepeatedMistake, 1, 6*day, "h"),
	}
	var ids []string
	for _, r := range lessonsToShow(live, now) {
		ids = append(ids, r.ID)
	}
	want := []string{"old-thrice", "third-improvement", "mistake", "waste", "context"}
	if !slices.Equal(ids, want) {
		t.Fatalf("shown = %v, want %v", ids, want)
	}
	long := strings.Repeat("长", 700)
	if got := lessonsToShow([]kb.ProjectMemoryRecord{lesson("long-1", kb.LessonImprovement, 2, day, long),
		lesson("long-2", kb.LessonWastedStep, 1, day, long), lesson("short", kb.LessonMissingContext, 1, day, "short")}, now); len(got) != 2 ||
		got[0].ID != "long-1" || got[1].ID != "short" {
		t.Fatalf("runes bound = %+v", got)
	}
}

// Only a lesson of a known kind with text is kept, each text once, at most three; one that would carry a credential is
// dropped rather than stored.
func TestTidyLessons(t *testing.T) {
	var r lessonsReply
	if err := json.Unmarshal([]byte(`{"lessons":[
		{"kind":"improvement","text":"  Run the focused test\nfirst. ","same_as":" lesson-a "},
		{"kind":"improvement","text":"run the focused test first"},
		{"kind":"rule","text":"Not a kind."},
		{"kind":"missing_context","text":"Use API_KEY=sk-abcdefghijklmnopqrstuvwx for staging."},
		{"kind":"wasted_step","text":""},
		{"kind":"wasted_step","text":"Skip the full rebuild."},
		{"kind":"repeated_mistake","text":"Check the landscape layout."},
		{"kind":"missing_context","text":"A fourth lesson."}]}`), &r); err != nil {
		t.Fatal(err)
	}
	got := tidyLessons(r)
	if len(got) != 3 || got[0] != (foundLesson{Kind: "improvement", Text: "Run the focused test first.", SameAs: "lesson-a"}) ||
		got[1].Text != "Skip the full rebuild." || got[2].Text != "Check the landscape layout." {
		t.Fatalf("tidy = %+v", got)
	}
}

// The lessons of a continued to-do's stretch of work start after its earlier acceptance; friction is read from them.
func TestLessonReviewsAndFriction(t *testing.T) {
	t0 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	v := Supervision{Reviews: []TaskReview{
		{At: t0, Verdict: reviewRework, Round: 1},
		{At: t0.Add(time.Hour), Verdict: reviewAccepted},
		{At: t0.Add(2 * time.Hour), Verdict: reviewAccepted},
	}}
	list, since := lessonReviews(v)
	if len(list) != 1 || !since.Equal(t0.Add(time.Hour)) {
		t.Fatalf("reviews = %+v since %v", list, since)
	}
	if lessonFriction(list, nil, since, 0) {
		t.Fatal("a rework before the earlier acceptance counted for this stretch")
	}
	brief := &TaskBrief{ExpectNow: 30, ActualMinutes: 59}
	if lessonFriction(list, brief, since, 0) {
		t.Fatal("under twice the estimate is not friction")
	}
	brief.ActualMinutes = 60
	if !lessonFriction(list, brief, since, 0) {
		t.Fatal("twice the estimate is friction")
	}
	scope := &TaskBrief{Log: []BriefEvent{{At: t0.Add(90 * time.Minute), Kind: briefEventScope, Change: "drop iPad"}}}
	if !lessonFriction(list, scope, since, 0) || lessonFriction(list, scope, t0.Add(100*time.Minute), 0) {
		t.Fatal("a scope change counts only within the stretch")
	}
	if !lessonFriction(list, nil, since, 1) {
		t.Fatal("an answer of the owner is friction")
	}
	if !lessonFriction([]TaskReview{{Verdict: reviewAccepted, Checks: []ReviewCheck{{Criterion: "x", Met: false}}}}, nil, time.Time{}, 0) {
		t.Fatal("a criterion not met is friction")
	}
}

// The project page's lessons: listed for the owner's apps only, corrected and deleted there. A correction keeps the
// lesson's count and sources and marks it edited; a deletion is not learned again; a stale page's change is refused
// with the lessons as they are now.
func TestProjectLessonRoutes(t *testing.T) {
	s, _, _ := lessonsEnv(t, true)
	e := kbEng(t, s)
	appendTestLesson(t, e, "天气-app", "lesson-a", kb.LessonMissingContext, "Ask which account to use.", time.Now())
	appendTestLesson(t, e, "天气-app", "lesson-b", kb.LessonWastedStep, "Skip the full rebuild.", time.Now().Add(-100*24*time.Hour))
	path := "/api/v1/kb/projects/" + "天气-app" + "/lessons"
	if w := serveAs(s, "read", http.MethodGet, path, ""); w.Code != http.StatusForbidden {
		t.Fatalf("a read-only device got %d", w.Code)
	}
	var view struct {
		Enabled bool         `json:"enabled"`
		Lessons []lessonView `json:"lessons"`
		Code    string       `json:"code"`
	}
	read := func(code int, method, path, body string) {
		t.Helper()
		w := serve(s, method, path, body)
		if w.Code != code {
			t.Fatalf("%s %s = %d %s, want %d", method, path, w.Code, w.Body, code)
		}
		view.Lessons, view.Code = nil, ""
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, w.Body)
		}
	}
	read(http.StatusOK, http.MethodGet, path, "")
	if !view.Enabled || len(view.Lessons) != 2 || view.Lessons[0].ID != "lesson-a" || view.Lessons[0].Stale ||
		!view.Lessons[1].Stale || view.Lessons[0].Sources[0].TodoID != "todo-old" {
		t.Fatalf("list = %+v", view)
	}
	read(http.StatusOK, http.MethodPut, path+"/lesson-a", `{"text":"Ask the owner which account to use first."}`)
	edited := view.Lessons[0]
	if edited.ID == "lesson-a" || edited.Text != "Ask the owner which account to use first." || !edited.Edited || edited.Seen != 1 ||
		edited.Kind != kb.LessonMissingContext {
		t.Fatalf("edited = %+v", edited)
	}
	read(http.StatusConflict, http.MethodPut, path+"/lesson-a", `{"text":"An edit from an old page."}`)
	if view.Code != "lesson_changed" || len(view.Lessons) != 2 || view.Lessons[0].ID != edited.ID {
		t.Fatalf("stale edit = %+v", view)
	}
	if w := serve(s, http.MethodPut, path+"/"+edited.ID, `{"text":"  "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("an empty edit got %d", w.Code)
	}
	if w := serve(s, http.MethodDelete, path+"/lesson-none", ""); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown lesson got %d", w.Code)
	}
	read(http.StatusOK, http.MethodDelete, path+"/"+edited.ID, "")
	if len(view.Lessons) != 1 || view.Lessons[0].ID != "lesson-b" {
		t.Fatalf("after delete = %+v", view)
	}
	// A deleted lesson is not learned again, even said another way.
	now := time.Now()
	if again := planLessons(e.ProjectLessons("天气-app"), "todo-new", lessonKey("todo-new", now), now,
		[]foundLesson{{Kind: kb.LessonMissingContext, Text: "ask the owner, which account to use first"}}); again != nil {
		t.Fatalf("a deleted lesson came back: %+v", again)
	}
	if w := serve(s, http.MethodGet, "/api/v1/kb/projects/no-such-project/lessons", ""); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown project got %d", w.Code)
	}
}
