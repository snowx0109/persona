package assistant

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/kb"
)

// weatherLessons is the lessons route of the project lessonsEnv sets up.
const weatherLessons = "/api/v1/kb/projects/天气-app/lessons"

// deleteLesson deletes lesson id from the project's page, as the owner does, and answers its tombstone.
func deleteLesson(t *testing.T, s *Service, id string) kb.ProjectMemoryRecord {
	t.Helper()
	if w := serve(s, http.MethodDelete, weatherLessons+"/"+id, ""); w.Code != http.StatusOK {
		t.Fatalf("delete %s = %d %s", id, w.Code, w.Body)
	}
	l := kbEng(t, s).ProjectLessons("天气-app")
	if len(l.Retired) == 0 || l.Retired[0].SupersedesID != id {
		t.Fatalf("no tombstone for %s: %+v", id, l.Retired)
	}
	return l.Retired[0]
}

// editLesson corrects lesson id's text from the project's page, as the owner does, and answers the new version's ID.
func editLesson(t *testing.T, s *Service, id, text string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		t.Fatal(err)
	}
	if w := serve(s, http.MethodPut, weatherLessons+"/"+id, string(body)); w.Code != http.StatusOK {
		t.Fatalf("edit %s = %d %s", id, w.Code, w.Body)
	}
	for _, r := range kbEng(t, s).ProjectLessons("天气-app").Live {
		if r.SupersedesID == id {
			return r.ID
		}
	}
	t.Fatalf("no new version of %s", id)
	return ""
}

// answerLessons sets what the fake lessons model answers next.
func answerLessons(f *fakeHub, reply string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lessons = reply
}

// A deleted lesson taught again in other words stays deleted. Exact text cannot catch a rewording, so the model is shown
// the deleted lessons with their IDs and told to leave out one that repeats them, and an answer that still names a
// deleted lesson in same_as is dropped.
func TestDeletedLessonIsNotLearnedAgainInOtherWords(t *testing.T) {
	s, f, todo := lessonsEnv(t, true)
	e := kbEng(t, s)
	appendTestLesson(t, e, "天气-app", "lesson-a", kb.LessonMissingContext, "登录页要在 iPad 原版模拟器上复现。", time.Now())
	tomb := deleteLesson(t, s, "lesson-a")
	answerLessons(f, `{"lessons":[{"kind":"missing_context","text":"复现登录页的问题要用原版的 iPad 模拟器。","same_as":"`+tomb.ID+`"}]}`)
	acceptAfterRework(t, s, todo)
	asked := lessonsAsked(f)
	if len(asked) != 1 {
		t.Fatalf("the lessons model was asked %d times, want once", len(asked))
	}
	if want := "和用户删掉的某条经验说的是同一件事（哪怕说法不同），就不要写这条"; !strings.Contains(asked[0].System, want) {
		t.Errorf("the system prompt does not ask to leave out deleted lessons (%q):\n%s", want, asked[0].System)
	}
	if want := "用户删掉的经验（不要再学）：\n- [" + tomb.ID + "] missing_context：登录页要在 iPad 原版模拟器上复现。\n"; !strings.Contains(asked[0].User, want) {
		t.Errorf("the model's input lacks the deleted lesson %q:\n%s", want, asked[0].User)
	}
	if l := e.ProjectLessons("天气-app"); len(l.Live) != 0 {
		t.Errorf("a deleted lesson came back in other words: %+v", l.Live)
	}
}

// A lesson the model says repeats a deleted one is dropped, whichever ID of it same_as names: the tombstone, the version
// the owner deleted, or an earlier version of it. The same lesson not tied to a deleted one is still learned.
func TestPlanLessonsDropsOneNamingADeletedLesson(t *testing.T) {
	s, _, _ := lessonsEnv(t, true)
	e := kbEng(t, s)
	appendTestLesson(t, e, "天气-app", "lesson-a", kb.LessonMissingContext, "Ask which account to use.", time.Now())
	edited := editLesson(t, s, "lesson-a", "Ask the owner which account to use first.")
	tomb := deleteLesson(t, s, edited)
	cur := e.ProjectLessons("天气-app")
	now := time.Now()
	key := lessonKey("todo-new", now)
	text := "Check with the owner which login the work runs under."
	for _, id := range []string{tomb.ID, edited, "lesson-a"} {
		found := []foundLesson{{Kind: kb.LessonMissingContext, Text: text, SameAs: id}}
		if got := planLessons(cur, "todo-new", key, now, found); got != nil {
			t.Errorf("same_as %s names a deleted lesson, yet the plan is %+v", id, got)
		}
	}
	if got := planLessons(cur, "todo-new", key, now, []foundLesson{{Kind: kb.LessonMissingContext, Text: text}}); len(got) != 1 ||
		got[0].Conclusion != text || got[0].SupersedesID != "" {
		t.Fatalf("a lesson tied to no deleted one = %+v, want it learned", got)
	}
}

// A lesson the owner corrected and then deleted is not learned again in the words a task first taught it in: its
// tombstone keeps only the corrected text, and every version of a deleted lesson is blocked.
func TestEditedThenDeletedLessonIsNotLearnedInItsFirstWords(t *testing.T) {
	s, f, todo := lessonsEnv(t, true)
	e := kbEng(t, s)
	appendTestLesson(t, e, "天气-app", "lesson-a", kb.LessonMissingContext, "登录页要在 iPad 原版模拟器上复现。", time.Now())
	edited := editLesson(t, s, "lesson-a", "复现登录页的问题前，先问用户用哪台 iPad。")
	deleteLesson(t, s, edited)
	answerLessons(f, `{"lessons":[{"kind":"missing_context","text":"登录页要在 iPad 原版模拟器上复现！","same_as":""}]}`)
	acceptAfterRework(t, s, todo)
	if n := len(lessonsAsked(f)); n != 1 {
		t.Fatalf("the lessons model was asked %d times, want once", n)
	}
	if l := e.ProjectLessons("天气-app"); len(l.Live) != 0 {
		t.Fatalf("the first words of a corrected, then deleted lesson came back: %+v", l.Live)
	}
}
