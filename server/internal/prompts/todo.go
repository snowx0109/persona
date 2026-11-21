package prompts

import "strings"

// TodoDecisions is the context a handoff to an agent carries for a todo's decisions in effect (todo/decisions,
// #1618, docs/assistant/todos.md "Decision records"). The caller leaves out the closing tags
// ("</persona-context>") from the texts.
type TodoDecisions struct {
	Language  string
	Title     string
	Decisions []TodoDecision
}

// TodoDecision is one decision: N counts from 1; When is its time as the user reads it. Source is where it came from
// (TodoDecisionOutcome, …), "" for one written in the todo; Asker the title of the agent whose question it answered
// (TodoDecisionCard), "" for an unnamed one; Ancestor the title of the earlier todo it was carried from, "" for this
// todo's own. The template words them.
type TodoDecision struct {
	N                        int
	Question, Choice, Reason string
	When                     string
	Source, Asker, Ancestor  string
}

// Where a decision came from (TodoDecision.Source).
const (
	TodoDecisionOutcome        = "outcome"         // pulled out of a finished task's reply
	TodoDecisionEarlierOutcome = "earlier-outcome" // from an earlier phase of the same task
	TodoDecisionAgent          = "agent"           // todo_decision_add, the agent's own call
	TodoDecisionThread         = "thread"          // the user's reply in the to-do's chat thread
	TodoDecisionCard           = "card"            // the user's answer to an agent's question
)

// RenderTodoDecisions is the context block of a todo's decisions; empty when it has none.
func RenderTodoDecisions(d TodoDecisions) string {
	if len(d.Decisions) == 0 {
		return ""
	}
	return render("todo/decisions", d)
}

// TodoThreadReply is what the user said in the chat thread a todo is bound to, as it goes to the todo's agent (todo/thread_reply,
// #5279, docs/assistant/todos.md "Chat thread"): one line saying whose decision it is, then the words as they were said. Title
// is the todo's, on one line, with nothing in it that closes a context block; Recorded: the words were written into
// the todo's decisions (it is not said when they were not: a message of only a picture, no database, a failed write).
type TodoThreadReply struct {
	Language    string
	Title, Text string
	Recorded    bool
}

func RenderTodoThreadReply(r TodoThreadReply) string { return render("todo/thread_reply", r) }

// TodoThreadOpen is what the main conversation gets when the owner opens a new topic in a registered group chat and the
// hub opens a to-do for it (todo/thread_open, #5279, docs/assistant/todos.md "Opening a to-do from a topic"): whose to-do it is,
// the first words as said, and that task_plan takes the to-do's id. Title and Text carry nothing that closes a context
// block.
type TodoThreadOpen struct {
	Language            string
	Title, TodoID, Text string
	// Quote is the text of a message the owner's request replied to in another chat (#6146): someone else's words, shown
	// apart from Text, as background and escaped (ctx), never as the user's own. "" for none.
	Quote string
	// NoTopic: the to-do is bound to no topic (no home topic group, so the request was taken in the owner's private chat
	// with the bot): the hand-over says answers and later words go through that chat, not a topic.
	NoTopic bool
}

// RenderTodoThreadOpen is the hand-over; the words go in as a quotation, each line after "> ", so that a many-line
// message cannot pass for the list of instructions below it.
func RenderTodoThreadOpen(o TodoThreadOpen) string {
	text := strings.ReplaceAll(strings.TrimSpace(o.Text), "\r\n", "\n")
	o.Text = "> " + strings.ReplaceAll(text, "\n", "\n> ")
	return render("todo/thread_open", o)
}

// TodoTitle is the system prompt naming a todo a task opened after what the agent is asked to do (todo/title, #2775,
// docs/assistant/todos.md "To-do titles").
type TodoTitle struct {
	// Language is the title's language by its own name ("English", "Japanese" written in Japanese); "" is Chinese.
	Language string
	Max      int // characters
}

func RenderTodoTitle(t TodoTitle) string { return render("todo/title", t) }

// RenderTodoTitleInput is what the title model reads: the task's prompt (the todo's notes).
func RenderTodoTitleInput(prompt, language string) string {
	return render("todo/title_input", struct{ Prompt, Language string }{prompt, language})
}

// TodoOutcome is the system prompt pulling what a finished task came to out of its agents' last replies: a one-line
// outcome, links, decisions, pitfalls (todo/outcome, docs/assistant/todos.md "Record conclusions on completion", #2820).
type TodoOutcome struct {
	Language string
	Owner    string
}

func RenderTodoOutcome(d TodoOutcome) string { return render("todo/outcome", d) }

// TodoOutcomeInput is the task as it was handed out and what its agents said when they stopped (todo/outcome_input).
type TodoOutcomeInput struct {
	Language      string
	Title, Prompt string
	Replies       []TodoOutcomeReply
}

// TodoOutcomeReply is one agent's last reply: the machine's name and the directory it worked in.
type TodoOutcomeReply struct {
	Machine, Cwd, Text string
}

func RenderTodoOutcomeInput(in TodoOutcomeInput) string { return render("todo/outcome_input", in) }

// TodoLessons is the system prompt asking what a task the supervisor accepted taught: lessons of four kinds for the
// next tasks of its project (todo/lessons, #12295, docs/assistant/todos.md "Lessons from accepted tasks").
type TodoLessons struct {
	Language string
	Owner    string
}

func RenderTodoLessons(d TodoLessons) string { return render("todo/lessons", d) }

// TodoLessonsInput is the story of an accepted task (todo/lessons_input): the owner's request, goal and criteria, the
// scope changes, every review (rework rounds with the criteria not met), the owner's answers, how each criterion ended,
// the estimate against the time it took, the Agent's last reply, the lessons its project already has, and the ones the
// owner deleted, which the model leaves out even said in other words.
type TodoLessonsInput struct {
	Language                     string
	Title, Request, Goal         string
	Criteria                     []string
	Changes                      []TodoLessonsChange
	Reviews                      []TodoLessonsReview
	Answers                      []TodoLessonsAnswer
	Result                       []TodoLessonsCriterion
	ExpectMinutes, ActualMinutes int
	Reply                        string
	Known, Deleted               []TodoLessonsKnown
}

// TodoLessonsChange is one scope change of the task's brief: what changed and why.
type TodoLessonsChange struct {
	Change, Why string
}

// TodoLessonsReview is one review: its verdict, the rework round it started, who gave it (the reviewer, the done
// judge; "" the supervisor), its note and the criteria it found not met.
type TodoLessonsReview struct {
	Verdict, By, Note string
	Round             int
	Unmet             []TodoLessonsGap
}

// TodoLessonsGap is a criterion a review found not met, with what it found.
type TodoLessonsGap struct {
	Criterion, Evidence string
}

// TodoLessonsAnswer is a question of the task the owner answered: the question and their choice.
type TodoLessonsAnswer struct {
	Question, Choice string
}

// TodoLessonsCriterion is how one criterion ended: met, not_met, unchecked or dropped, with its evidence.
type TodoLessonsCriterion struct {
	Criterion, State, Evidence string
}

// TodoLessonsKnown is a lesson of the project: one it has, for the model to say a new one repeats it, or one the owner
// deleted (its tombstone's ID), for the model to leave out.
type TodoLessonsKnown struct {
	ID, Kind, Text string
}

func RenderTodoLessonsInput(in TodoLessonsInput) string { return render("todo/lessons_input", in) }

// TodoProjectLessons is the <persona-context> of a project's lessons a new Agent gets before its first prompt, after
// the project's own block (todo/project_lessons, #12295): the most repeated first. The caller cleans the texts.
type TodoProjectLessons struct {
	Language, Name string
	Lessons        []TodoProjectLesson
}

// TodoProjectLesson is one lesson: its kind, text, how many accepted tasks taught it, and when it was last taught.
type TodoProjectLesson struct {
	Kind, Text, When string
	Seen             int
}

// RenderTodoProjectLessons is the block; empty without lessons.
func RenderTodoProjectLessons(p TodoProjectLessons) string {
	if len(p.Lessons) == 0 {
		return ""
	}
	return render("todo/project_lessons", p)
}

// TodoProject is the <persona-context> a new agent gets before its first prompt when its directory belongs to a
// project (todo/project, #2820): the project's one line and the outcomes of its latest finished todos, newest first.
// The caller leaves out "</persona-context>" from the texts.
type TodoProject struct {
	Language      string
	Name, OneLine string
	Outcomes      []TodoProjectOutcome
	// Extensions are what extensions wrote for this project (contributes.prompts project_context), each under its
	// extension's name.
	Extensions []ExtensionContext
}

// ExtensionContext is one extension's text for a prompt: Name is the extension's, Text what it wrote (cleaned and
// within its limit by the extension manager).
type ExtensionContext struct {
	Name, Text string
}

// TodoProjectOutcome is one finished todo's outcome as the project's timeline keeps it.
type TodoProjectOutcome struct {
	When, Title, Text string
}

// TodoProjectMemory is the memory-aware project background for a dispatch. Its sections always render in this order:
// bounded summary, outcomes, effective decisions with reasons, pitfalls, then extension-provided context.
type TodoProjectMemory struct {
	Language, Name, Summary string
	Outcomes                []TodoProjectMemoryOutcome
	Decisions               []TodoProjectMemoryDecision
	Pitfalls                []TodoProjectMemoryPitfall
	Extensions              []ExtensionContext
}

type TodoProjectMemoryOutcome struct {
	When, Text, Verification string
	Links                    []string
	Unaccepted               bool
}

type TodoProjectMemoryDecision struct {
	ID, Question, Choice, Reason string
	When, Verification           string
	Unaccepted                   bool
}

type TodoProjectMemoryPitfall struct {
	Text, When, Verification string
	Unaccepted               bool
}

// RenderTodoProjectMemory renders only projects with usable memory.jsonl records. RenderTodoProject remains the
// byte-compatible path for projects without them.
func RenderTodoProjectMemory(p TodoProjectMemory) string {
	if p.Summary == "" && len(p.Outcomes) == 0 && len(p.Decisions) == 0 && len(p.Pitfalls) == 0 && len(p.Extensions) == 0 {
		return ""
	}
	return render("todo/project_memory", p)
}

// RenderTodoProject is the block; empty when there is nothing to say (no one line, no outcomes, nothing from an
// extension).
func RenderTodoProject(p TodoProject) string {
	if p.OneLine == "" && len(p.Outcomes) == 0 && len(p.Extensions) == 0 {
		return ""
	}
	return render("todo/project", p)
}
