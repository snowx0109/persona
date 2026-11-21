package assistant

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/ai"
	"github.com/nwverse/persona/server/internal/db"
	"github.com/nwverse/persona/server/internal/kb"
	"github.com/nwverse/persona/server/internal/prompts"
	"github.com/nwverse/persona/server/internal/transcript"
)

// Lessons learned from accepted tasks (#12295, docs/assistant/todos.md "Lessons from accepted tasks"). With
// assistant.task_lessons on (off by default), a supervised task the supervisor accepts after friction gets a short
// retrospective in the background. Friction is a rework round, a criterion found not met, the owner's answer to one of
// its questions, a scope change, or far more time than estimated. The Information processing model (kb.project, the
// purpose that writes task outcomes) reads the task's story and answers up to three lessons of four kinds
// (kb.LessonKinds). They go into the task's project memory.jsonl, under the ownership worker outcomes use
// (projectsForTask). Every new Agent of the project gets the most repeated fresh ones after the project's own block
// (renderProjectLessons). A smooth acceptance makes no model call. A lesson the project already has is taught again (its
// count goes up) instead of being stored twice, one the owner deleted is never learned again, and one not taught again
// for lessonFresh is no longer handed out. Text alone cannot tell a deleted lesson said in other words, so the model is
// shown the deleted ones to leave out, and what it ties to one is dropped. Nothing here changes the supervisor's
// instructions or task_review.

const (
	lessonsReadTimeout  = time.Minute      // reading the Agent's last reply
	lessonsModelTimeout = 90 * time.Second // the model's answer
	lessonsModelTokens  = 1024
	lessonsPerTask      = 3    // lessons one acceptance may teach
	lessonTextMax       = 200  // runes of one lesson
	lessonsKnownMost    = 12   // the project's lessons the model is shown, so it can say a new one repeats one
	lessonsDeletedMost  = 12   // the newest deleted lessons the model is shown, so it leaves out one that repeats them
	lessonsReviewsMost  = 8    // the latest reviews of the stretch of work read
	lessonsCriteriaMost = 10   // criteria, scope changes and answers read, each
	lessonsReplyMax     = 2000 // runes of the Agent's last reply
	lessonsRequestMax   = 1500 // runes of the owner's request
	lessonFieldMax      = 300  // runes of a note, an evidence or a scope change
	lessonSourcesMost   = 5    // to-dos a lesson keeps as the ones that taught it
	lessonOverrun       = 2    // taking this many times the estimate is friction
	lessonsShownMost    = 5    // lessons a new Agent gets
	lessonsShownPerKind = 2
	lessonsShownRunes   = 1200
	lessonFresh         = 90 * 24 * time.Hour // a lesson not taught again for this long is no longer handed out
)

// lessonsReply is what the model answers (todo/lessons).
type lessonsReply struct {
	Lessons []struct {
		Kind   string `json:"kind"`
		Text   string `json:"text"`
		SameAs string `json:"same_as"`
	} `json:"lessons"`
}

// foundLesson is one lesson the model answered, tidied: a known kind, one line within lessonTextMax, and the ID the
// model gave of a project lesson it repeats ("" none), which may name a deleted one.
type foundLesson struct {
	Kind, Text, SameAs string
}

// taskLessons is assistant.task_lessons in effect: off unless the owner turned it on.
func (s *Service) taskLessons(ctx context.Context) bool {
	v := s.settings(ctx).TaskLessons
	return v != nil && *v
}

// goLearnTaskLessons learns what task t, accepted at doneAt, taught, in the background (counted in tasks.outcomeRuns,
// as its outcome is): the acceptance's answer never waits for it. t is the to-do as it was when it closed, with its
// agents and ledger. Nothing while the switch is off or the task had no ledger.
func (s *Service) goLearnTaskLessons(ctx context.Context, t Todo, doneAt time.Time) {
	if t.Supervision == nil || !s.taskLessons(ctx) {
		return
	}
	ctx = context.WithoutCancel(ctx)
	s.tasks.outcomeRuns.Add(1)
	go func() {
		defer s.tasks.outcomeRuns.Done()
		s.learnTaskLessons(ctx, t, doneAt)
	}()
}

// learnTaskLessons is goLearnTaskLessons' work: friction first (no model call without it), then the health space's
// check as for the outcome line, the projects, the model, and one locked write per project.
func (s *Service) learnTaskLessons(ctx context.Context, t Todo, doneAt time.Time) {
	reviews, since := lessonReviews(*t.Supervision)
	brief, hasBrief := s.briefOfTodo(t.ID)
	answers := s.lessonAnswers(ctx, t.ID, since)
	var briefRef *TaskBrief
	if hasBrief {
		briefRef = &brief
	}
	if !lessonFriction(reviews, briefRef, since, len(answers)) || !s.kbWritable(ctx) {
		return
	}
	// The health space's words never become lessons, and neither do those of a to-do whose conversations cannot be
	// told now (todo_scope.go): a lesson is never held back to be written later.
	if s.todoVerdicts(ctx, []string{t.ID}, map[string]Todo{t.ID: t}, nil)[t.ID] != todoShown {
		return
	}
	e, err := s.kbGet(ctx)
	if err != nil || e == nil {
		s.logf("task %s: lessons: knowledge base unavailable: %v", t.ID, err)
		return
	}
	a := lastAgent(t)
	projects := s.projectsForTask(ctx, e, t, a.MachineID, a.Cwd)
	if len(projects) == 0 {
		return // no project owns the task: nothing to learn for
	}
	key := lessonKey(t.ID, doneAt)
	known := e.ProjectLessons(projects[0].Slug)
	if lessonsLearned(known, key) {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, lessonsReadTimeout)
	agents := s.outcomeAgents(rctx, t)
	cancel()
	language := answerLanguage(s.hello(ctx).Locale)
	in := lessonsInput(language, t, briefRef, reviews, since, answers, agents, known)
	mctx, mcancel := context.WithTimeout(ctx, lessonsModelTimeout)
	defer mcancel()
	var out lessonsReply
	err = s.purposeModel("kb.project").JSONMax(mctx, prompts.RenderTodoLessons(prompts.TodoLessons{Language: language, Owner: s.owner(ctx)}),
		prompts.RenderTodoLessonsInput(in), lessonsModelTokens, &out)
	if err != nil {
		if !ai.IsPaused(err) {
			s.logf("task %s: lessons: %v (nothing learned)", t.ID, err)
		}
		return
	}
	found := tidyLessons(out)
	if len(found) == 0 {
		return
	}
	now := time.Now()
	for _, p := range projects {
		if _, err := e.UpdateProjectLessons(p.Slug, func(cur kb.ProjectLessons) ([]kb.ProjectMemoryRecord, error) {
			return planLessons(cur, t.ID, key, now, found), nil
		}); err != nil {
			s.logf("task %s: lessons for project %s: %v", t.ID, p.Slug, err)
		}
	}
}

// lessonReviews are the reviews of the stretch of work an acceptance closed, oldest first, the acceptance last: those
// after the acceptance or cancellation before it (a continued to-do keeps its earlier reviews), at most
// lessonsReviewsMost. since is when that earlier close was (zero: none).
func lessonReviews(v Supervision) (list []TaskReview, since time.Time) {
	start := 0
	for i := len(v.Reviews) - 2; i >= 0; i-- { // the last one is the acceptance being recorded
		if r := v.Reviews[i]; r.Verdict == reviewAccepted || r.Verdict == reviewCancelled {
			start, since = i+1, r.At
			break
		}
	}
	list = v.Reviews[start:]
	if len(list) > lessonsReviewsMost {
		list = list[len(list)-lessonsReviewsMost:]
	}
	return list, since
}

// lessonFriction says, without a model, whether an accepted stretch of work has anything to learn from: a rework or a
// question for the owner, a criterion a review found not met, an answer the owner gave, a scope change, or taking
// lessonOverrun times the estimate. A task accepted at once and in time has none.
func lessonFriction(reviews []TaskReview, brief *TaskBrief, since time.Time, answers int) bool {
	if answers > 0 {
		return true
	}
	for _, r := range reviews {
		if r.Verdict == reviewRework || r.Verdict == reviewNeedsUser {
			return true
		}
		if slices.ContainsFunc(r.Checks, func(c ReviewCheck) bool { return !c.Met }) {
			return true
		}
	}
	if brief == nil {
		return false
	}
	if slices.ContainsFunc(brief.Log, func(ev BriefEvent) bool { return ev.Kind == briefEventScope && !ev.At.Before(since) }) {
		return true
	}
	return brief.ExpectNow > 0 && brief.ActualMinutes >= lessonOverrun*brief.ExpectNow
}

// lessonAnswers are what the owner answered on to-do id since (its question cards, what they wrote on it or in its
// chat thread), oldest first: background the work did not have. None without a store.
func (s *Service) lessonAnswers(ctx context.Context, id string, since time.Time) []prompts.TodoLessonsAnswer {
	st := s.todoDB()
	if st == nil {
		return nil
	}
	list, err := st.ListTodoDecisions(ctx, id, false)
	if err != nil {
		return nil
	}
	var out []prompts.TodoLessonsAnswer
	for _, d := range list {
		switch d.Source {
		case db.DecisionCard, db.DecisionManual, db.DecisionThread:
		default:
			continue
		}
		if d.CreatedAt.Before(since) || strings.TrimSpace(d.Choice) == "" {
			continue
		}
		out = append(out, prompts.TodoLessonsAnswer{Question: clip(oneLine(d.Question), lessonFieldMax), Choice: clip(oneLine(d.Choice), lessonFieldMax)})
	}
	if len(out) > lessonsCriteriaMost {
		out = out[len(out)-lessonsCriteriaMost:]
	}
	return out
}

// lessonsInput is the task's story as the model reads it (todo/lessons_input): the brief's request, goal, criteria,
// scope changes and result when it has one (else the to-do's notes and done-when), the reviews with the criteria not
// met, the owner's answers, the time, the newest Agent's last reply, the project's lessons, and the newest deleted ones
// with their tombstones' IDs, each text once. Only the model can tell a deleted lesson said in other words.
func lessonsInput(language string, t Todo, brief *TaskBrief, reviews []TaskReview, since time.Time,
	answers []prompts.TodoLessonsAnswer, agents []outcomeAgent, known kb.ProjectLessons) prompts.TodoLessonsInput {
	in := prompts.TodoLessonsInput{Language: language, Title: clip(oneLine(t.Title), lessonFieldMax), Answers: answers}
	criteria := t.Supervision.DoneWhen
	if brief != nil {
		if brief.Request != nil {
			in.Request = clipMiddle(strings.TrimSpace(brief.Request.Text), lessonsRequestMax)
		}
		if goal := clip(oneLine(brief.Goal), lessonFieldMax); goal != in.Title {
			in.Goal = goal
		}
		if len(brief.Criteria) > 0 {
			criteria = brief.Criteria
		}
		for _, ev := range brief.Log {
			if ev.Kind == briefEventScope && !ev.At.Before(since) && len(in.Changes) < lessonsCriteriaMost {
				in.Changes = append(in.Changes, prompts.TodoLessonsChange{Change: clip(oneLine(ev.Change), lessonFieldMax), Why: clip(oneLine(ev.Why), lessonFieldMax)})
			}
		}
		if brief.Result != nil {
			for _, c := range brief.Result.Criteria[:min(len(brief.Result.Criteria), lessonsCriteriaMost)] {
				in.Result = append(in.Result, prompts.TodoLessonsCriterion{Criterion: clip(oneLine(c.Criterion), lessonFieldMax),
					State: c.State, Evidence: clip(oneLine(c.Evidence), lessonFieldMax)})
			}
		}
		in.ExpectMinutes, in.ActualMinutes = brief.ExpectNow, brief.ActualMinutes
	}
	if in.Request == "" {
		in.Request = clipMiddle(strings.TrimSpace(t.Notes), lessonsRequestMax)
	}
	for _, c := range criteria[:min(len(criteria), lessonsCriteriaMost)] {
		in.Criteria = append(in.Criteria, clip(oneLine(c), lessonFieldMax))
	}
	for _, r := range reviews {
		lr := prompts.TodoLessonsReview{Verdict: r.Verdict, By: r.By, Note: clip(oneLine(r.Note), lessonFieldMax), Round: r.Round}
		for _, c := range r.Checks {
			if !c.Met {
				lr.Unmet = append(lr.Unmet, prompts.TodoLessonsGap{Criterion: clip(oneLine(c.Criterion), lessonFieldMax), Evidence: clip(oneLine(c.Evidence), lessonFieldMax)})
			}
		}
		in.Reviews = append(in.Reviews, lr)
	}
	if len(agents) > 0 {
		in.Reply = clipMiddle(agents[0].Text, lessonsReplyMax)
	}
	for _, k := range known.Live[:min(len(known.Live), lessonsKnownMost)] {
		in.Known = append(in.Known, prompts.TodoLessonsKnown{ID: k.ID, Kind: k.Kind, Text: clip(oneLine(k.Conclusion), lessonTextMax)})
	}
	shown := map[string]bool{}
	for _, r := range known.Retired {
		if len(in.Deleted) == lessonsDeletedMost {
			break
		}
		if key := kb.LessonKey(r.Conclusion); key != "" && !shown[key] {
			shown[key] = true
			in.Deleted = append(in.Deleted, prompts.TodoLessonsKnown{ID: r.ID, Kind: r.Kind, Text: clip(oneLine(r.Conclusion), lessonTextMax)})
		}
	}
	return in
}

// tidyLessons keeps what the model answered that can be a lesson: a known kind, text that is not empty, at most
// lessonsPerTask, each text once. One that looks like it holds a credential (a key, a token, a password) is dropped,
// not shortened: a lesson is never stored with a secret in it.
func tidyLessons(r lessonsReply) []foundLesson {
	var out []foundLesson
	seen := map[string]bool{}
	for _, l := range r.Lessons {
		kind, text := strings.TrimSpace(l.Kind), clip(oneLine(l.Text), lessonTextMax)
		key := kb.LessonKey(text)
		if !kb.ValidLessonKind(kind) || key == "" || seen[key] || lessonHasSecret(text) {
			continue
		}
		seen[key] = true
		out = append(out, foundLesson{Kind: kind, Text: text, SameAs: strings.TrimSpace(l.SameAs)})
		if len(out) == lessonsPerTask {
			break
		}
	}
	return out
}

// lessonHasSecret: what the shared-transcript scrub would redact in text (private keys, bearer tokens, credential
// assignments, known key formats, secrets in a link).
func lessonHasSecret(text string) bool {
	clean, err := transcript.SanitizeShareText(text)
	return err != nil || strings.Contains(clean, "[REDACTED")
}

// lessonKey names one completion of a to-do among lesson IDs: every record it writes starts with it and a dash, so a
// completion is learned from once (lessonsLearned).
func lessonKey(todoID string, doneAt time.Time) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(todoID + "@" + doneAt.UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("lesson-%016x", h.Sum64())
}

// lessonsLearned: the project already has a record of the completion key names.
func lessonsLearned(l kb.ProjectLessons, key string) bool {
	for id := range l.IDs {
		if strings.HasPrefix(id, key+"-") {
			return true
		}
	}
	return false
}

// planLessons is what one completion (key, of to-do todoID) writes to a project whose lessons are cur. A lesson found
// either teaches a live one again (the one the model named, else one with the same text: a record that supersedes it
// with its count up and the to-do among its sources) or is new. Nothing for a completion already learned from, for a
// lesson the to-do taught already, or for one the owner deleted (lessonsDeleted): one with the text of any version of
// a deleted lesson, or one the model says repeats a deleted lesson. The text of a lesson taught again is kept: the
// owner may have corrected it.
func planLessons(cur kb.ProjectLessons, todoID, key string, now time.Time, found []foundLesson) []kb.ProjectMemoryRecord {
	if lessonsLearned(cur, key) {
		return nil
	}
	deletedIDs, deletedTexts := lessonsDeleted(cur)
	live := slices.Clone(cur.Live)
	byKey := map[string]int{}
	for i := len(live) - 1; i >= 0; i-- { // the newest of two with the same text wins
		byKey[kb.LessonKey(live[i].Conclusion)] = i
	}
	at := now.UTC().Format(time.RFC3339Nano)
	var out []kb.ProjectMemoryRecord
	for i, l := range found {
		k := kb.LessonKey(l.Text)
		if deletedTexts[k] || deletedIDs[l.SameAs] {
			continue
		}
		target := -1
		if l.SameAs != "" {
			target = slices.IndexFunc(live, func(r kb.ProjectMemoryRecord) bool { return r.ID == l.SameAs })
		}
		if j, ok := byKey[k]; target < 0 && ok {
			target = j
		}
		id := fmt.Sprintf("%s-%d", key, i+1)
		if target >= 0 {
			old := live[target]
			if slices.Contains(old.Sources, todoID) {
				continue
			}
			r := old
			r.ID, r.SupersedesID, r.TodoID, r.RecordedAt, r.LastSeenAt = id, old.ID, todoID, at, at
			r.Seen = max(old.Seen, 1) + 1
			r.FirstSeenAt = cmp.Or(old.FirstSeenAt, old.RecordedAt, at)
			r.Sources = append(slices.Clone(old.Sources), todoID)
			if len(r.Sources) > lessonSourcesMost {
				r.Sources = r.Sources[len(r.Sources)-lessonSourcesMost:]
			}
			live[target] = r
			out = append(out, r)
			continue
		}
		r := kb.ProjectMemoryRecord{ID: id, Type: kb.ProjectMemoryLesson, Kind: l.Kind, Conclusion: l.Text, TodoID: todoID,
			Verification: kb.ProjectMemoryAccepted, RecordedAt: at, Seen: 1, FirstSeenAt: at, LastSeenAt: at, Sources: []string{todoID}}
		byKey[k] = len(live)
		live = append(live, r)
		out = append(out, r)
	}
	return out
}

// lessonsDeleted are what the owner's deletions block: the IDs of each tombstone and of every version of the lesson it
// retired, which the model may name in same_as, and those versions' texts (kb.LessonKey). A tombstone keeps only the
// last text, and the words a task taught before the owner's correction stay deleted too.
func lessonsDeleted(cur kb.ProjectLessons) (ids, texts map[string]bool) {
	ids, texts = map[string]bool{}, map[string]bool{}
	for _, r := range cur.Retired {
		ids[r.ID], ids[r.SupersedesID], texts[kb.LessonKey(r.Conclusion)] = true, true, true
	}
	for _, r := range cur.RetiredVersions {
		ids[r.ID], texts[kb.LessonKey(r.Conclusion)] = true, true
	}
	delete(ids, "")
	delete(texts, "")
	return ids, texts
}

// lessonLastSeen is when a lesson was last taught or corrected.
func lessonLastSeen(r kb.ProjectMemoryRecord) (time.Time, bool) {
	for _, v := range []string{r.LastSeenAt, r.RecordedAt} {
		if at, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

// lessonStale: not taught again for lessonFresh (or with no time at all), so no longer handed out.
func lessonStale(r kb.ProjectMemoryRecord, now time.Time) bool {
	at, ok := lessonLastSeen(r)
	return !ok || now.Sub(at) > lessonFresh
}

// lessonsInOrder are lessons the most repeated first, then the most recently taught.
func lessonsInOrder(list []kb.ProjectMemoryRecord) []kb.ProjectMemoryRecord {
	list = slices.Clone(list)
	sort.SliceStable(list, func(i, j int) bool {
		if a, b := max(list[i].Seen, 1), max(list[j].Seen, 1); a != b {
			return a > b
		}
		a, _ := lessonLastSeen(list[i])
		b, _ := lessonLastSeen(list[j])
		return a.After(b)
	})
	return list
}

// lessonsToShow is what a new Agent of a project gets of its live lessons: the fresh ones in lessonsInOrder, at most
// lessonsShownPerKind of one kind, lessonsShownMost in all and lessonsShownRunes of text.
func lessonsToShow(live []kb.ProjectMemoryRecord, now time.Time) []kb.ProjectMemoryRecord {
	var out []kb.ProjectMemoryRecord
	perKind := map[string]int{}
	runes := 0
	for _, r := range lessonsInOrder(live) {
		n := len([]rune(r.Conclusion))
		if lessonStale(r, now) || perKind[r.Kind] >= lessonsShownPerKind || runes+n > lessonsShownRunes {
			continue
		}
		perKind[r.Kind]++
		runes += n
		out = append(out, r)
		if len(out) == lessonsShownMost {
			break
		}
	}
	return out
}

// renderProjectLessons is the lessons block (todo/project_lessons) a new Agent of project p gets after the project's
// own block: "" while assistant.task_lessons is off or no lesson is fresh, so the prompt is as before.
func (s *Service) renderProjectLessons(ctx context.Context, e *kb.Engine, p *kb.Project, language string) string {
	if !s.taskLessons(ctx) {
		return ""
	}
	shown := lessonsToShow(e.ProjectLessons(p.Slug).Live, time.Now())
	if len(shown) == 0 {
		return ""
	}
	d := prompts.TodoProjectLessons{Language: language, Name: contextText(oneLine(p.Name))}
	loc := s.location(ctx)
	for _, r := range shown {
		when := ""
		if at, ok := lessonLastSeen(r); ok {
			when = at.In(loc).Format(time.DateOnly)
		}
		d.Lessons = append(d.Lessons, prompts.TodoProjectLesson{Kind: r.Kind, Text: contextText(r.Conclusion), When: when, Seen: max(r.Seen, 1)})
	}
	return prompts.RenderTodoProjectLessons(d)
}
