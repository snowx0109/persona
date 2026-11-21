package kb

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func lessonRecord(id, kind, text string) ProjectMemoryRecord {
	return ProjectMemoryRecord{ID: id, Type: ProjectMemoryLesson, Kind: kind, Conclusion: text, TodoID: "todo-1",
		Verification: ProjectMemoryAccepted, Seen: 1, RecordedAt: "2026-11-21T01:00:00Z",
		FirstSeenAt: "2026-11-21T01:00:00Z", LastSeenAt: "2026-11-21T01:00:00Z", Sources: []string{"todo-1"}}
}

func appendLessons(t *testing.T, e *Engine, slug string, records ...ProjectMemoryRecord) {
	t.Helper()
	if _, err := e.UpdateProjectLessons(slug, func(ProjectLessons) ([]ProjectMemoryRecord, error) { return records, nil }); err != nil {
		t.Fatal(err)
	}
}

func liveIDs(l ProjectLessons) []string {
	var out []string
	for _, r := range l.Live {
		out = append(out, r.ID)
	}
	return out
}

// Only the newest version of each lesson is live: a lesson taught again supersedes the one before, and a tombstone
// ends a chain and keeps the deleted lesson's kind and text.
func TestProjectLessonsKeepTheNewestVersion(t *testing.T) {
	e := projectMemoryEngine(t, t.TempDir())
	putMemoryProject(t, e, "persona")
	appendLessons(t, e, "persona",
		lessonRecord("lesson-a", LessonMissingContext, "The staging hub needs its own token."),
		lessonRecord("lesson-b", LessonWastedStep, "Rebuilding the whole app took longer than the test."))
	again := lessonRecord("lesson-a2", LessonMissingContext, "The staging hub needs its own token.")
	again.SupersedesID, again.Seen = "lesson-a", 2
	appendLessons(t, e, "persona", again)
	appendLessons(t, e, "persona", ProjectMemoryRecord{ID: "lesson-b-retired", Type: ProjectMemoryLessonRetired,
		SupersedesID: "lesson-b", Kind: LessonWastedStep, Conclusion: "Rebuilding the whole app took longer than the test."})

	got := e.ProjectLessons("persona")
	if ids := liveIDs(got); len(ids) != 1 || ids[0] != "lesson-a2" {
		t.Fatalf("live = %v, want only the newest version of lesson a", ids)
	}
	if r, ok := got.Find("lesson-a2"); !ok || r.Seen != 2 {
		t.Fatalf("find = %+v %v", r, ok)
	}
	if _, ok := got.Find("lesson-a"); ok {
		t.Fatal("an older version was found as live")
	}
	if len(got.Retired) != 1 || got.Retired[0].Conclusion == "" || got.Retired[0].Kind != LessonWastedStep {
		t.Fatalf("retired = %+v", got.Retired)
	}
	for _, id := range []string{"lesson-a", "lesson-a2", "lesson-b", "lesson-b-retired"} {
		if !got.IDs[id] {
			t.Fatalf("ids = %v, missing %s", got.IDs, id)
		}
	}
	if none := e.ProjectLessons("other"); len(none.Live) != 0 || len(none.IDs) != 0 {
		t.Fatalf("a project without memory.jsonl has lessons: %+v", none)
	}
}

// A deleted lesson keeps every version it had, the newest first: the one its tombstone retired, the version before the
// owner's correction and the task's first words. Earlier versions of a lesson still live are not among them.
func TestProjectLessonsKeepEveryVersionOfADeletedLesson(t *testing.T) {
	e := projectMemoryEngine(t, t.TempDir())
	putMemoryProject(t, e, "persona")
	appendLessons(t, e, "persona",
		lessonRecord("lesson-a", LessonMissingContext, "Ask which account to use."),
		lessonRecord("lesson-b", LessonWastedStep, "Skip the full rebuild."))
	again := lessonRecord("lesson-a2", LessonMissingContext, "Ask which account to use.")
	again.SupersedesID, again.Seen = "lesson-a", 2
	appendLessons(t, e, "persona", again)
	edited := lessonRecord("lesson-a3", LessonMissingContext, "Ask the owner which account to use first.")
	edited.SupersedesID, edited.Verification = "lesson-a2", ProjectMemoryOwner
	b2 := lessonRecord("lesson-b2", LessonWastedStep, "Build only the target under test.")
	b2.SupersedesID = "lesson-b"
	appendLessons(t, e, "persona", edited, b2)
	appendLessons(t, e, "persona", ProjectMemoryRecord{ID: "lesson-a-retired", Type: ProjectMemoryLessonRetired,
		SupersedesID: "lesson-a3", Kind: LessonMissingContext, Conclusion: edited.Conclusion})

	got := e.ProjectLessons("persona")
	var versions []string
	for _, r := range got.RetiredVersions {
		versions = append(versions, r.ID+": "+r.Conclusion)
	}
	want := []string{"lesson-a3: Ask the owner which account to use first.", "lesson-a2: Ask which account to use.",
		"lesson-a: Ask which account to use."}
	if strings.Join(versions, "\n") != strings.Join(want, "\n") {
		t.Fatalf("versions of the deleted lesson = %q, want %q", versions, want)
	}
	if ids := liveIDs(got); len(ids) != 1 || ids[0] != "lesson-b2" || len(got.Retired) != 1 {
		t.Fatalf("live = %v, retired = %+v", ids, got.Retired)
	}
}

// A write is checked against the lessons as they are under the lock: an ID that is taken, a supersede of a version that
// is no longer the newest, two successors of one version and a tombstone naming no lesson are refused, and a refused
// plan writes nothing.
func TestUpdateProjectLessonsRefusesStaleWrites(t *testing.T) {
	e := projectMemoryEngine(t, t.TempDir())
	putMemoryProject(t, e, "persona")
	appendLessons(t, e, "persona", lessonRecord("lesson-a", LessonImprovement, "Run the focused test first."))
	edited := lessonRecord("lesson-a2", LessonImprovement, "Run the focused test before the whole suite.")
	edited.SupersedesID = "lesson-a"
	appendLessons(t, e, "persona", edited)
	path := e.Store.Path(KindProject, "persona", "memory.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stale := lessonRecord("lesson-a3", LessonImprovement, "An edit from an old page.")
	stale.SupersedesID = "lesson-a"
	first, second := lessonRecord("lesson-x", LessonImprovement, "One."), lessonRecord("lesson-y", LessonImprovement, "Two.")
	first.SupersedesID, second.SupersedesID = "lesson-a2", "lesson-a2"
	for name, c := range map[string]struct {
		records []ProjectMemoryRecord
		stale   bool
	}{
		"taken id":         {records: []ProjectMemoryRecord{lessonRecord("lesson-a", LessonImprovement, "Again.")}},
		"stale supersede":  {records: []ProjectMemoryRecord{stale}, stale: true},
		"two successors":   {records: []ProjectMemoryRecord{first, second}, stale: true},
		"tombstone target": {records: []ProjectMemoryRecord{{ID: "lesson-t", Type: ProjectMemoryLessonRetired}}},
		"no text":          {records: []ProjectMemoryRecord{lessonRecord("lesson-e", LessonImprovement, " ")}},
		"not a lesson":     {records: []ProjectMemoryRecord{{ID: "lesson-o", Type: ProjectMemoryOutcome, Conclusion: "x"}}},
	} {
		_, err := e.UpdateProjectLessons("persona", func(ProjectLessons) ([]ProjectMemoryRecord, error) { return c.records, nil })
		if err == nil || c.stale != errors.Is(err, ErrLessonChanged) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("a refused write changed memory.jsonl:\n%s", after)
	}
	// The plan sees the lessons as they are now, and an error from it writes nothing either.
	_, err = e.UpdateProjectLessons("persona", func(cur ProjectLessons) ([]ProjectMemoryRecord, error) {
		if ids := liveIDs(cur); len(ids) != 1 || ids[0] != "lesson-a2" {
			t.Fatalf("plan saw %v", ids)
		}
		return nil, errors.New("nothing to learn")
	})
	if err == nil {
		t.Fatal("the plan's error was dropped")
	}
}

// Lessons sit in memory.jsonl beside M1's records without changing what dispatch reads of them: a file with only
// lessons still has no usable dispatch memory (the legacy prompt stays), and in a mixed file the outcomes, decisions
// and pitfalls come back as before.
func TestProjectLessonsLeaveDispatchMemoryAlone(t *testing.T) {
	e := projectMemoryEngine(t, t.TempDir())
	putMemoryProject(t, e, "lessons-only")
	appendLessons(t, e, "lessons-only", lessonRecord("lesson-a", LessonMissingContext, "Ask which account to use."))
	if _, ok := e.ProjectMemoryForDispatch("lessons-only", 5, 5, 3); ok {
		t.Fatal("a lessons-only file was taken as dispatch memory")
	}
	putMemoryProject(t, e, "mixed")
	outcome := ProjectMemoryRecord{ID: "todo-1/box/s1/1/0", TodoID: "todo-1", Type: ProjectMemoryOutcome,
		Conclusion: "Shipped.", Verification: ProjectMemoryAgentReported}
	pitfall := ProjectMemoryRecord{ID: "todo-1/box/s1/1/0/pitfall/1", TodoID: "todo-1", Type: ProjectMemoryPitfall,
		Conclusion: "The cache hid the change.", Verification: ProjectMemoryAgentReported}
	if err := e.AppendProjectMemoryBatch("mixed", []ProjectMemoryRecord{pitfall, outcome}); err != nil {
		t.Fatal(err)
	}
	appendLessons(t, e, "mixed", lessonRecord("lesson-a", LessonWastedStep, "Clear the cache before measuring."))
	got, ok := e.ProjectMemoryForDispatch("mixed", 5, 5, 3)
	if !ok || len(got.Outcomes) != 1 || len(got.Pitfalls) != 1 || len(got.Decisions) != 0 ||
		got.Outcomes[0].ID != outcome.ID || got.Pitfalls[0].ID != pitfall.ID {
		t.Fatalf("dispatch memory = %+v %v", got, ok)
	}
	if l := e.ProjectLessons("mixed"); len(l.Live) != 1 || len(l.IDs) != 1 {
		t.Fatalf("lessons read M1 records: %+v", l)
	}
}

// A lessons read goes through a bounded tail of memory.jsonl; a lesson cut off at the start is skipped, not misread.
func TestProjectLessonsReadABoundedTail(t *testing.T) {
	old := projectLessonsScanMost
	t.Cleanup(func() { projectLessonsScanMost = old })
	projectLessonsScanMost = 1 << 10
	e := projectMemoryEngine(t, t.TempDir())
	putMemoryProject(t, e, "persona")
	appendLessons(t, e, "persona", lessonRecord("lesson-old", LessonImprovement, "An old lesson."))
	filler := ProjectMemoryRecord{ID: "todo-2/box/s1/1/0", Type: ProjectMemoryOutcome, Conclusion: strings.Repeat("x", 2000)}
	if err := e.AppendProjectMemoryBatch("persona", []ProjectMemoryRecord{filler}); err != nil {
		t.Fatal(err)
	}
	appendLessons(t, e, "persona", lessonRecord("lesson-new", LessonImprovement, "A new lesson."))
	if ids := liveIDs(e.ProjectLessons("persona")); len(ids) != 1 || ids[0] != "lesson-new" {
		t.Fatalf("live = %v, want only the lesson within the bound", ids)
	}
}

func TestLessonKey(t *testing.T) {
	if a, b := LessonKey("Run the focused test first."), LessonKey("  run THE focused-test first "); a != b || a == "" {
		t.Fatalf("keys %q and %q differ", a, b)
	}
	if LessonKey("先跑单个测试。") != LessonKey("先跑 单个测试") {
		t.Fatal("CJK punctuation and spaces changed the key")
	}
	if !ValidLessonKind(LessonMissingContext) || ValidLessonKind("rule") {
		t.Fatal("kind check")
	}
}
