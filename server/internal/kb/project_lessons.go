package kb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// Lessons learned from accepted tasks (#12295; docs/assistant/todos.md "Lessons from accepted tasks"). They are typed
// records in the project's memory.jsonl, beside the worker outcomes, decisions and pitfalls (M1), and are never changed
// in place. A lesson taught again, or corrected by the owner, is appended as a record that supersedes it. A deleted
// lesson gets a lesson_retired tombstone that keeps its kind and text, so the same lesson is not learned again (an
// undo sticks, docs/design/PRINCIPLES.md 3). Readers keep the newest record of each chain, and every version of a
// deleted one: the tombstone keeps only the last text, and the words a task taught before the owner's correction must
// stay deleted too. A manager from before lessons skips these types (ProjectMemoryForDispatch reads only the types it
// knows).

// The lesson kinds.
const (
	LessonRepeatedMistake = "repeated_mistake" // a mistake the work made and had to redo
	LessonMissingContext  = "missing_context"  // background the owner or the supervisor had to add
	LessonWastedStep      = "wasted_step"      // a step that cost time and did not help
	LessonImprovement     = "improvement"      // a better way to run this kind of work
)

// LessonKinds are the kinds in the order the apps list them.
var LessonKinds = []string{LessonRepeatedMistake, LessonMissingContext, LessonWastedStep, LessonImprovement}

// projectLessonsScanMost bounds how much of memory.jsonl's tail a lessons read goes through, in bytes. Lessons are a few
// lines per accepted task, and a lesson that was taught again or edited has a newer record anyway. A variable so a test
// can make it small.
var projectLessonsScanMost int64 = 8 << 20

// ErrLessonChanged: the lesson a write supersedes is not its newest version any more. It was taught again, edited or
// deleted after it was read.
var ErrLessonChanged = errors.New("the lesson changed since it was read")

// ProjectLessons is a project's lessons as memory.jsonl has them now.
type ProjectLessons struct {
	// Live is the newest version of every lesson not deleted, newest first.
	Live []ProjectMemoryRecord
	// Retired are the tombstones of deleted lessons, each with the deleted lesson's kind and text, newest first.
	Retired []ProjectMemoryRecord
	// RetiredVersions are the versions of the deleted lessons, newest first: the one a tombstone retired and every one
	// before it, such as a task's words the owner corrected before deleting the lesson.
	RetiredVersions []ProjectMemoryRecord
	// IDs are every lesson record and tombstone read, older versions included.
	IDs map[string]bool
}

// ValidLessonKind says whether kind is one of LessonKinds.
func ValidLessonKind(kind string) bool {
	for _, k := range LessonKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Find is the live lesson with this ID (only a newest version is live).
func (l ProjectLessons) Find(id string) (ProjectMemoryRecord, bool) {
	for _, r := range l.Live {
		if r.ID == id {
			return r, true
		}
	}
	return ProjectMemoryRecord{}, false
}

// LessonKey is a lesson's text reduced for comparing two lessons: its letters and digits in lower case, so spacing,
// punctuation and case never make two copies of one lesson.
func LessonKey(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ProjectLessons reads a project's lessons. A missing or unreadable file has none.
func (e *Engine) ProjectLessons(slug string) ProjectLessons {
	s := e.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	l, _ := s.projectLessonsLocked(slug)
	return l
}

// projectLessonsLocked reads the lesson records of memory.jsonl's tail, newest first. A record seen later in the read
// (older) that a newer one supersedes or retires is an earlier version and not live; one in a chain a tombstone ends is
// a version of a deleted lesson. A record only ever supersedes an older one, so a chain is read from its newest record
// back. Callers hold s.mu.
func (s *Store) projectLessonsLocked(slug string) (ProjectLessons, error) {
	out := ProjectLessons{IDs: map[string]bool{}}
	f, err := os.Open(s.path(KindProject, slug, "memory.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, err
	}
	size, start := info.Size(), int64(0)
	if size > projectLessonsScanMost {
		start = size - projectLessonsScanMost // the cut line is not JSON and is skipped
	}
	superseded, retired := map[string]bool{}, map[string]bool{}
	err = linesBack(io.NewSectionReader(f, start, size-start), size-start, logChunk, func(line []byte) bool {
		var r ProjectMemoryRecord
		if len(bytes.TrimSpace(line)) == 0 || json.Unmarshal(line, &r) != nil || r.ID == "" {
			return true
		}
		if r.Type != ProjectMemoryLesson && r.Type != ProjectMemoryLessonRetired {
			return true
		}
		out.IDs[r.ID] = true
		if r.SupersedesID != "" {
			superseded[r.SupersedesID] = true
		}
		switch {
		case r.Type == ProjectMemoryLessonRetired:
			out.Retired = append(out.Retired, r)
			retired[r.SupersedesID] = true
		case retired[r.ID]:
			out.RetiredVersions = append(out.RetiredVersions, r)
			retired[r.SupersedesID] = true
		case !superseded[r.ID] && strings.TrimSpace(r.Conclusion) != "":
			out.Live = append(out.Live, r)
		}
		return true
	})
	return out, err
}

// UpdateProjectLessons appends what plan decides from the project's lessons as they are now, under the store's lock,
// so a lesson taught again, the owner's edit and a deletion can never supersede the same version twice. Every record
// is a lesson or a tombstone with an ID no lesson record has. One that supersedes names the newest version of a live
// lesson (else ErrLessonChanged), and a tombstone always names the lesson it retires. Nothing is written when plan
// returns no records or an error. Answers the records appended.
func (e *Engine) UpdateProjectLessons(slug string, plan func(ProjectLessons) ([]ProjectMemoryRecord, error)) ([]ProjectMemoryRecord, error) {
	s := e.Store
	if s.readOnly {
		return nil, ErrReadOnly
	}
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if err := s.checkProjectDetailWrite(slug, "project lessons"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.projectLessonsLocked(slug)
	if err != nil {
		return nil, err
	}
	records, err := plan(cur)
	if err != nil || len(records) == 0 {
		return nil, err
	}
	live := make(map[string]bool, len(cur.Live))
	for _, r := range cur.Live {
		live[r.ID] = true
	}
	taken := make(map[string]bool, len(cur.IDs)+len(records))
	for id := range cur.IDs {
		taken[id] = true
	}
	var payload bytes.Buffer
	for _, r := range records {
		switch {
		case r.ID == "" || taken[r.ID]:
			return nil, fmt.Errorf("lesson record id %q is empty or taken", r.ID)
		case r.Type != ProjectMemoryLesson && r.Type != ProjectMemoryLessonRetired:
			return nil, fmt.Errorf("lesson record %q has type %q", r.ID, r.Type)
		case r.Type == ProjectMemoryLessonRetired && r.SupersedesID == "":
			return nil, fmt.Errorf("lesson tombstone %q names no lesson", r.ID)
		case r.Type == ProjectMemoryLesson && strings.TrimSpace(r.Conclusion) == "":
			return nil, fmt.Errorf("lesson %q has no text", r.ID)
		}
		if r.SupersedesID != "" {
			if !live[r.SupersedesID] {
				return nil, ErrLessonChanged
			}
			delete(live, r.SupersedesID) // one successor per version
		}
		taken[r.ID] = true
		if r.Type == ProjectMemoryLesson {
			live[r.ID] = true
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		payload.Write(encoded)
		payload.WriteByte('\n')
	}
	if err := appendProjectMemoryFile(s.path(KindProject, slug, "memory.jsonl"), payload.Bytes()); err != nil {
		return nil, err
	}
	return records, nil
}
