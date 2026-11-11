package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

func TestMCQNumberKeyAnswersAndSaves(t *testing.T) {
	a, st, clock := studyApp(t, "git.commit.mcq-what", 120)
	clock.advance(5 * time.Second)

	a, _ = press(t, a, "2")

	got := lastAnswer(t, st)
	if got.Correct || got.Misconception != "commit-takes-worktree" || got.Grade != domain.Again {
		t.Errorf("answer = %+v", got)
	}
	if got.Duration != 5*time.Second || !got.At.Equal(clock.t) {
		t.Errorf("duration %v at %v, want 5s at %v", got.Duration, got.At, clock.t)
	}
	mustContain(t, screen(a), "✗ Not quite", "The staged changes", "Only staged changes are committed",
		"commit-takes-worktree · 1st time", "A commit is a snapshot of the index.")
}

func TestMCQCursorAndEnter(t *testing.T) {
	a, st, clock := studyApp(t, "git.commit.mcq-what", 120)
	clock.advance(3 * time.Second)

	a, _ = press(t, a, "j", "k", "enter")

	got := lastAnswer(t, st)
	if !got.Correct || got.Grade != domain.Easy {
		t.Errorf("fast correct answer = %+v, want Easy", got)
	}
	mustContain(t, screen(a), "✓ Correct", "graded easy")
}

func TestOptionNumberPastTheEndDoesNothing(t *testing.T) {
	a, st, _ := studyApp(t, "git.commit.mcq-what", 120)

	a, _ = press(t, a, "9")

	if a.session.q.answered || a.Screen() != Study {
		t.Error("9 must neither answer a two-option item nor switch screens")
	}
	if got, _ := st.RecentAnswers(t.Context(), 1); len(got) != 0 {
		t.Errorf("saved %v", got)
	}
}

func TestOutputChoiceUsesOptions(t *testing.T) {
	a, st, _ := studyApp(t, "go.slices.output-len", 120)
	mustContain(t, screen(a), "make([]int, 2, 5)", "1   2 5")

	press(t, a, "1")

	if !lastAnswer(t, st).Correct {
		t.Error("option 1 is correct")
	}
}

func TestMultiSpaceTogglesEnterSubmits(t *testing.T) {
	a, st, _ := studyApp(t, "logic.recursion.multi-needs", 120)

	a, _ = press(t, a, "enter") // nothing chosen: ignored
	if a.session.q.answered {
		t.Fatal("enter with nothing selected must not submit")
	}
	a, _ = press(t, a, "space", "j", "space", "j", "space", "space")
	mustContain(t, screen(a), "[x] A base case", "[x] Progress toward", "[ ] A loop")

	a, _ = press(t, a, "enter")

	if !lastAnswer(t, st).Correct {
		t.Errorf("a base case + progress is correct; screen:\n%s", screen(a))
	}
}

func TestMultiWrongShowsTheMisconception(t *testing.T) {
	a, st, _ := studyApp(t, "logic.recursion.multi-needs", 120)

	a, _ = press(t, a, "1", "3", "enter")

	got := lastAnswer(t, st)
	if got.Correct || got.Misconception != "recursion-needs-loop" {
		t.Errorf("answer = %+v", got)
	}
	mustContain(t, screen(a), "Recursion replaces the loop.", "recursion-needs-loop · 1st time")
}

func TestCommandIsTypedWithoutTriggeringKeys(t *testing.T) {
	a, st, _ := studyApp(t, "git.commit.cmd-message", 120)

	// q, h, e, 1 and ? are letters of the answer here, not commands.
	a = typeText(t, a, `git commit -m "init" q1?`)
	if a.Screen() != Study || a.help || a.session.q.hint {
		t.Fatal("typed letters triggered global keys")
	}
	a, _ = press(t, a, "backspace", "backspace", "backspace", "backspace", "enter")

	if got := lastAnswer(t, st); !got.Correct {
		t.Errorf("answer = %+v; screen:\n%s", got, screen(a))
	}
	mustContain(t, screen(a), "✓ Correct")
}

func TestTypedEnterWithoutTextDoesNothing(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.cmd-message", 120)
	a, _ = press(t, a, "enter")
	if a.session.q.answered {
		t.Error("an empty answer must not be submitted")
	}
}

func TestOutputTypedAcceptsNewLines(t *testing.T) {
	a, st, _ := studyApp(t, "logic.loops.output-sum", 120)

	a = typeText(t, a, "6")
	a, _ = press(t, a, "alt+enter")
	if a.session.q.text != "6\n" {
		t.Fatalf("text = %q", a.session.q.text)
	}
	a, _ = press(t, a, "enter")

	if !lastAnswer(t, st).Correct {
		t.Error("trailing new lines are ignored by the grader")
	}
}

func TestClozeShowsBlanksAndMovesBetweenThem(t *testing.T) {
	a, st, _ := studyApp(t, "go.slices.cloze-append", 120)
	mustContain(t, screen(a), "s = [▏](s, [2___])", "Blank 1 of 2")

	a = typeText(t, a, "append")
	a, _ = press(t, a, "enter") // to the next blank, not submit
	if a.session.q.answered || a.session.q.blank != 1 {
		t.Fatalf("enter on the first blank must move on: %+v", a.session.q)
	}
	a = typeText(t, a, "5")
	a, _ = press(t, a, "shift+tab", "tab", "backspace")
	a = typeText(t, a, "4")
	mustContain(t, screen(a), "s = [append](s, [4▏])")

	a, _ = press(t, a, "enter")

	if got := lastAnswer(t, st); !got.Correct {
		t.Errorf("answer = %+v", got)
	}
}

func TestParsonsIsShuffledDeterministically(t *testing.T) {
	it := onlyItem(t, "logic.recursion.parsons-fact").Items[0]
	first := shuffled(it.ID, it.Lines)
	if slices.Equal(first, it.Lines) {
		t.Error("the starting order must not be the solution")
	}
	if !slices.Equal(first, shuffled(it.ID, it.Lines)) {
		t.Error("the same item must always start in the same order")
	}
	if slices.Equal(first, shuffled("other.id", it.Lines)) && slices.Equal(first, shuffled("another.id", it.Lines)) {
		t.Error("the order should depend on the item ID")
	}
	two := []string{"a", "b"}
	for _, id := range []string{"x", "y", "z", "w"} {
		if slices.Equal(shuffled(id, two), two) {
			t.Errorf("%s: two lines came back solved", id)
		}
	}
}

// solveParsons moves the lines into the correct order with J/K only.
func solveParsons(t *testing.T, a App) App {
	t.Helper()
	want := a.session.q.pick.Item.Lines
	for target, line := range want {
		at := slices.Index(a.session.q.order[target:], line) + target
		for a.session.q.cursor < at {
			a, _ = press(t, a, "j")
		}
		for a.session.q.cursor > at {
			a, _ = press(t, a, "k")
		}
		for a.session.q.cursor > target {
			a, _ = press(t, a, "K")
		}
	}
	return a
}

func TestParsonsMoveLinesAndSubmit(t *testing.T) {
	a, st, _ := studyApp(t, "git.branch.parsons-feature", 120)
	before := slices.Clone(a.session.q.order)

	a, _ = press(t, a, "J") // move the first line down
	if a.session.q.order[1] != before[0] || a.session.q.cursor != 1 {
		t.Fatalf("J must move the line and the cursor: %q cursor %d", a.session.q.order, a.session.q.cursor)
	}
	a, _ = press(t, a, "shift+up")
	if !slices.Equal(a.session.q.order, before) || a.session.q.cursor != 0 {
		t.Fatalf("shift+up must undo it: %q", a.session.q.order)
	}

	a = solveParsons(t, a)
	a, _ = press(t, a, "enter")

	if got := lastAnswer(t, st); !got.Correct {
		t.Errorf("answer = %+v; order %q", got, a.session.q.resp.Order)
	}
}

func TestBugSelectsALine(t *testing.T) {
	a, st, _ := studyApp(t, "logic.loops.bug-bounds", 120)
	mustContain(t, screen(a), "▸ 1 │ xs := []int{1, 2, 3}")

	a, _ = press(t, a, "j", "j", "k", "enter")

	if got := lastAnswer(t, st); !got.Correct {
		t.Errorf("line 2 is the bug: %+v", got)
	}
	mustContain(t, screen(a), "line 2: for i := 0; i <= len(xs); i++ {")
}

func TestHintGradesACorrectAnswerHard(t *testing.T) {
	a, st, _ := studyApp(t, "git.commit.mcq-what", 120)

	a, _ = press(t, a, "h")
	mustContain(t, screen(a), "Hint Think of git add.")
	a, _ = press(t, a, "1")

	if got := lastAnswer(t, st); !got.Correct || !got.HintUsed || got.Grade != domain.Hard {
		t.Errorf("answer = %+v, want correct, hint used, Hard", got)
	}
}

func TestMissingHintIsNotCounted(t *testing.T) {
	a, st, _ := studyApp(t, "go.slices.output-len", 120)

	a, _ = press(t, a, "h")
	mustContain(t, screen(a), "No hint for this one.")
	press(t, a, "1")

	if lastAnswer(t, st).HintUsed {
		t.Error("an item without a hint cannot mark the hint as used")
	}
}

func TestCtrlTShowsTheHintWhileTyping(t *testing.T) {
	a, st, _ := studyApp(t, "git.commit.cmd-message", 120)
	a.session.q.pick.Item.Hint = "Use -m."

	a, _ = press(t, a, "ctrl+t")
	a = typeText(t, a, `git commit -m "init"`)
	press(t, a, "enter")

	if !lastAnswer(t, st).HintUsed {
		t.Error("ctrl+t must show the hint while typing")
	}
}

func TestSlowCorrectAnswerIsHard(t *testing.T) {
	a, st, clock := studyApp(t, "git.commit.mcq-what", 120)
	clock.advance(2 * time.Minute)
	press(t, a, "1")
	if g := lastAnswer(t, st).Grade; g != domain.Hard {
		t.Errorf("grade = %v, want hard", g)
	}
}

func TestClockStartsWhenTheQuestionIsShown(t *testing.T) {
	st := openStore(t)
	clock := &testClock{t: fixedNow}
	a := newApp(t, onlyItem(t, "git.commit.mcq-what"), st, clock, Home, 120, 24)
	clock.advance(time.Hour) // time on Home does not count

	a, _ = press(t, a, "enter")
	clock.advance(4 * time.Second)
	press(t, a, "1")

	if d := lastAnswer(t, st).Duration; d != 4*time.Second {
		t.Errorf("duration = %v, want 4s", d)
	}
}

func TestExplainIsAsyncAndShowsTheAnswer(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.mcq-what", 120)
	tutor := &fakeTutor{text: "The index is the draft of your next commit."}
	a.tutor = tutor
	a, _ = press(t, a, "2")

	a, cmd := press(t, a, "e")
	if cmd == nil || tutor.calls != 0 {
		t.Fatal("e must return a command and not block")
	}
	mustContain(t, screen(a), "Tutor is thinking…")
	a, _ = press(t, a, "e") // a second e while loading does nothing
	a, _ = send(t, a, cmd())

	if tutor.calls != 1 {
		t.Errorf("tutor calls = %d", tutor.calls)
	}
	mustContain(t, screen(a), "Tutor", "The index is the draft of your next commit.")
	mustNotContain(t, screen(a), "thinking")
}

func TestExplainErrorIsInline(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.mcq-what", 120)
	a.tutor = &fakeTutor{err: errors.New("pi not found (pi): install Gentle Shell")}
	a, _ = press(t, a, "2")

	a, cmd := press(t, a, "e")
	a, _ = send(t, a, cmd())

	mustContain(t, screen(a), "Tutor unavailable:", "install Gentle Shell")
	if a.Err() != nil {
		t.Error("a tutor failure must not stop the app")
	}
}

func TestExplainWithoutTutor(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.mcq-what", 120)
	a.tutor = nil
	a, _ = press(t, a, "1")
	a, cmd := press(t, a, "e")
	a, _ = send(t, a, cmd())
	mustContain(t, screen(a), "Tutor unavailable", "not configured")
}

func TestStaleExplanationIsDropped(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.mcq-what", 120)
	a, _ = press(t, a, "1")
	a, cmd := press(t, a, "e")
	a, _ = press(t, a, "enter") // move on before the answer arrives
	a, _ = send(t, a, cmd())
	mustNotContain(t, screen(a), "Think of the index")
}

func TestSessionLoopAndSummary(t *testing.T) {
	cat := testCatalog(t)
	st := seeded(t, cat)
	clock := &testClock{t: fixedNow}
	a := newApp(t, cat, st, clock, Study, 120, 30)
	n := len(a.session.picks)
	if n == 0 {
		t.Fatal("the seeded store should have a session")
	}
	for i := 0; i < n; i++ {
		if a.session.q.answered {
			t.Fatal("a new question must start unanswered")
		}
		a = answerAnything(t, a)
		a, _ = press(t, a, "enter")
	}
	if !a.session.done() {
		t.Fatal("the session should be done")
	}
	s := screen(a)
	mustContain(t, s, "Session complete", "Concepts touched", "Next review")
	a, _ = press(t, a, "enter")
	if a.Screen() != Home {
		t.Errorf("enter after the summary goes Home, got %v", a.Screen())
	}
	mustContain(t, screen(a), "All caught up for today.")
}

// answerAnything submits some answer to the current question.
func answerAnything(t *testing.T, a App) App {
	t.Helper()
	switch a.session.q.kind {
	case typed:
		a = typeText(t, a, "x")
		a, _ = press(t, a, "enter")
	case blanks:
		for range a.session.q.fills {
			a, _ = press(t, a, "enter")
		}
	case pickMany:
		a, _ = press(t, a, "space", "enter")
	case editor:
		b, cmd := press(t, a, "ctrl+r")
		a = runCmd(t, b, cmd)
	default:
		a, _ = press(t, a, "enter")
	}
	if !a.session.q.answered && !a.session.done() {
		t.Fatalf("could not answer %s:\n%s", a.session.q.pick.Item.ID, screen(a))
	}
	return a
}

func TestNewConceptsAreCappedPerDay(t *testing.T) {
	cat := testCatalog(t)
	st := openStore(t)
	// One concept was already started earlier today.
	review(t, st, cat, "git.commit.mcq-what", true, "", fixedNow.Add(-time.Hour))
	clock := &testClock{t: fixedNow}

	a := newApp(t, cat, st, clock, Home, 120, 24)

	fresh := 0
	for _, p := range a.session.picks {
		if p.New {
			fresh++
		}
	}
	if fresh != 2 {
		t.Errorf("%d new picks, want 3 per day minus the one started today", fresh)
	}
}

func TestSaveErrorStopsTheApp(t *testing.T) {
	a, st, _ := studyApp(t, "git.commit.mcq-what", 120)
	st.Close()

	a, _ = press(t, a, "1")

	if a.Err() == nil {
		t.Fatal("want the save error")
	}
	mustContain(t, screen(a), "Could not save your answer")
	if _, cmd := press(t, a, "x"); !isQuit(cmd) {
		t.Error("any key quits the error screen")
	}
}

func TestGlobalKeys(t *testing.T) {
	cat := testCatalog(t)
	a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Home, 120, 24)

	for _, c := range []struct {
		key  string
		want Screen
	}{{"tab", Study}, {"tab", Tracks}, {"tab", Stats}, {"tab", Home}, {"shift+tab", Stats}, {"3", Tracks}, {"esc", Home}, {"4", Stats}, {"1", Home}} {
		a, _ = press(t, a, c.key)
		if a.Screen() != c.want {
			t.Fatalf("%s: screen %v, want %v", c.key, a.Screen(), c.want)
		}
	}
	a, _ = press(t, a, "?")
	mustContain(t, screen(a), "Keys", "Everywhere", "Study", "ask the AI tutor")
	a, _ = press(t, a, "?")
	if a.help {
		t.Error("? closes the help")
	}
	if _, cmd := press(t, a, "q"); !isQuit(cmd) {
		t.Error("q quits")
	}
	if _, cmd := press(t, a, "ctrl+c"); !isQuit(cmd) {
		t.Error("ctrl+c quits")
	}
}

func TestStudyNumbersAnswerBeforeSwitchingScreens(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.mcq-what", 120)
	a, _ = press(t, a, "2")
	if a.Screen() != Study || !a.session.q.answered {
		t.Fatal("2 answers the question on Study")
	}
	a, _ = press(t, a, "3")
	if a.Screen() != Tracks {
		t.Error("after answering, numbers switch screens again")
	}
}

func TestTracksShowStatesAndLocks(t *testing.T) {
	cat := testCatalog(t)
	a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Tracks, 120, 24)

	s := screen(a)
	mustContain(t, s, "Logic & Algorithms", "Loops", "mastered", "Recursion")

	a, _ = press(t, a, "j", "j")
	s = screen(a)
	mustContain(t, s, "Commits", "learning", "Branches")
	mustNotContain(t, s, "needs Commits") // commits is learning, so branches is open

	a, _ = press(t, a, "k")
	mustContain(t, screen(a), "Slices", "review")
}

func TestTracksLockedMarker(t *testing.T) {
	cat := testCatalog(t)
	a := newApp(t, cat, openStore(t), &testClock{t: fixedNow}, Tracks, 120, 24)
	mustContain(t, screen(a), "⊘ Recursion", "needs ✗ Loops")
}

func TestLayoutNeverOverflows(t *testing.T) {
	cat := testCatalog(t)
	for _, w := range []int{40, 80, 120, 160} {
		a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Home, w, 24)
		for _, k := range []string{"", "2", "3", "4", "?"} {
			if k != "" {
				a, _ = press(t, a, k)
			}
			lines := strings.Split(screen(a), "\n")
			if len(lines) != 24 {
				t.Errorf("w=%d key %q: %d lines", w, k, len(lines))
			}
			for i, l := range lines {
				if n := len([]rune(l)); n > w {
					t.Errorf("w=%d key %q line %d is %d wide: %q", w, k, i, n, l)
				}
			}
		}
	}
}
