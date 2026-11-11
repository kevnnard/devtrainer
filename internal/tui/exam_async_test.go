package tui

import (
	"context"
	"fmt"
	"testing"
	"testing/fstest"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
	"github.com/kevnnard/devtrainer/internal/store"
)

// seqRunner fails its first run and passes every later one, so the order
// in which commands are executed decides each reply.
type seqRunner struct{ calls int }

func (r *seqRunner) Toolchain(lang string) (runner.Toolchain, error) {
	return runner.Toolchain{Language: lang, Tool: lang}, nil
}

func (r *seqRunner) Run(context.Context, runner.Program) (check.CodeRun, error) {
	r.calls++
	if r.calls == 1 {
		return check.CodeRun{Stdout: "wrong\n"}, nil
	}
	return check.CodeRun{Stdout: "1\n2\n3\n"}, nil
}

// seqTutor numbers its explanations in call order.
type seqTutor struct{ calls int }

func (t *seqTutor) Explain(context.Context, domain.Item, domain.Concept, check.Response, check.Result) (string, error) {
	t.calls++
	return fmt.Sprintf("explanation number %d", t.calls), nil
}

// asyncApp opens Study over a store where py.loops (one code item, in
// section py/all) is learning and due, so today's session and the exam
// both start with that item at position 0.
func asyncApp(t *testing.T, rn CodeRunner, tutor Explainer, yaml string) (App, *store.Store) {
	t.Helper()
	cat, err := content.Load(fstest.MapFS{"py/py.yaml": {Data: []byte(yaml)}})
	if err != nil {
		t.Fatal(err)
	}
	st := openStore(t)
	review(t, st, cat, cat.Items[0].ID, true, "", daysAgo(30))
	a, err := NewApp(Config{Catalog: cat, Progress: st, Now: (&testClock{t: fixedNow}).now, MaxNew: 3,
		Start: Study, Runner: rn, Tutor: tutor})
	if err != nil {
		t.Fatal(err)
	}
	a, _ = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	if len(a.session.picks) == 0 || a.session.picks[0].Item.ID != cat.Items[0].ID {
		t.Fatalf("today's session should start with %s, got %+v", cat.Items[0].ID, a.session.picks)
	}
	return a, st
}

const asyncCodeYAML = `track:
  id: py
  title: Python
  order: 1
concepts:
  - id: py.loops
    title: Loops
    level: beginner
    items:
      - id: py.loops.code-count
        type: code
        language: python
        prompt: Print the numbers 1 to 3, one per line.
        starter: |
          for i in range(3):
              print(i)
        solution: |
          for i in range(1, 4):
              print(i)
        expected: |
          1
          2
          3
sections:
  - id: py/all
    title: Python all
    concepts: [py.loops]
`

const asyncMCQYAML = `track:
  id: py
  title: Python
  order: 1
concepts:
  - id: py.loops
    title: Loops
    level: beginner
    items:
      - id: py.loops.mcq
        type: mcq
        prompt: How many times does range(3) loop?
        options:
          - {text: "3", correct: true}
          - {text: "4", misconception: range-inclusive, explanation: range stops before its end.}
sections:
  - id: py/all
    title: Python all
    concepts: [py.loops]
`

// snapshot is what a stale reply must not change.
type snapshot struct {
	answers int
	reps    uint64
	exams   int
}

func snap(t *testing.T, st *store.Store) snapshot {
	t.Helper()
	ctx := context.Background()
	sc, err := st.Schedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := st.Exams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot{answers: countAnswers(t, st), reps: sc["py.loops"].Reps, exams: len(ex["py/all"])}
}

// TestStaleCodeRunIsNotGradedAsAnExamAnswer: a run started in today's
// session must not grade the exam question that took its position.
func TestStaleCodeRunIsNotGradedAsAnExamAnswer(t *testing.T) {
	rn := &seqRunner{}
	a, st := asyncApp(t, rn, &seqTutor{}, asyncCodeYAML)

	a, cmdA := press(t, a, "ctrl+r") // today's run A, pending
	if cmdA == nil || !a.session.q.running {
		t.Fatal("ctrl+r should start run A")
	}
	a, _ = press(t, a, "esc", "3", "x")
	if a.exam == nil || a.Screen() != Study || a.session.pos != 0 {
		t.Fatalf("the exam should be open at question 1:\n%s", screen(a))
	}
	a, cmdB := press(t, a, "ctrl+r") // the exam's run B, pending
	if cmdB == nil {
		t.Fatal("ctrl+r should start run B")
	}
	msgA, msgB := cmdA(), cmdB() // A runs first: it fails, B passes

	before := snap(t, st)
	a, _ = send(t, a, msgA)
	if got := snap(t, st); got != before {
		t.Errorf("stale run A changed the store: %+v → %+v", before, got)
	}
	if a.session.q.answered || !a.session.q.running || a.exam.result != nil {
		t.Errorf("stale run A changed the exam question: answered=%v running=%v result=%v",
			a.session.q.answered, a.session.q.running, a.exam.result)
	}

	a, _ = send(t, a, msgB)
	if !a.session.q.answered || !a.session.q.res.Correct {
		t.Fatalf("run B should grade the exam question as passed:\n%s", screen(a))
	}
	got := snap(t, st)
	if got.answers != before.answers+1 || got.reps != before.reps+1 || got.exams != 1 {
		t.Errorf("after run B: %+v, want one more answer and rep and one exam (from %+v)", got, before)
	}
	exams, _ := st.Exams(context.Background())
	if r := exams["py/all"]; len(r) != 1 || !r[0].Passed || r[0].Correct != 1 {
		t.Errorf("stored exam = %+v, want 1 of 1 passed", r)
	}

	// Back in today's session, the old reply still grades nothing.
	a, _ = press(t, a, "enter", "enter")
	if a.exam != nil || a.Screen() != Tracks {
		t.Fatalf("enter twice should leave the exam for Tracks, got %v", a.Screen())
	}
	a, _ = press(t, a, "2")
	before = snap(t, st)
	a, _ = send(t, a, msgA)
	if got := snap(t, st); got != before || a.session.q.answered {
		t.Errorf("stale run A graded today's session after the exam: %+v → %+v", before, got)
	}
}

// TestStaleExplanationDoesNotReplaceTheExamOne: the tutor's reply for
// today's question must not show on the exam question at its position.
func TestStaleExplanationDoesNotReplaceTheExamOne(t *testing.T) {
	tutor := &seqTutor{}
	a, _ := asyncApp(t, nil, tutor, asyncMCQYAML)

	a, _ = press(t, a, "1")
	a, cmdA := press(t, a, "e") // today's explanation A, pending
	if cmdA == nil {
		t.Fatal("e should ask the tutor")
	}
	a, _ = press(t, a, "3", "x")
	if a.exam == nil || a.session.pos != 0 {
		t.Fatalf("the exam should be open at question 1:\n%s", screen(a))
	}
	a, _ = press(t, a, "1")
	a, cmdB := press(t, a, "e") // the exam's explanation B, pending
	if cmdB == nil {
		t.Fatal("e should ask the tutor for the exam question")
	}
	msgA, msgB := cmdA(), cmdB() // A is "number 1", B "number 2"

	a, _ = send(t, a, msgA)
	if a.session.q.explain != explainLoading {
		t.Errorf("stale explanation A was applied to the exam question: %q", a.session.q.explanation)
	}
	mustNotContain(t, screen(a), "explanation number 1")

	a, _ = send(t, a, msgB)
	if a.session.q.explain != explainDone || a.session.q.explanation != "explanation number 2" {
		t.Errorf("explanation B = %v %q, want done with number 2", a.session.q.explain, a.session.q.explanation)
	}
	mustContain(t, screen(a), "explanation number 2")

	// Back in today's session, neither old reply lands.
	a, _ = press(t, a, "enter", "enter", "2")
	if a.exam != nil {
		t.Fatal("the exam should be over")
	}
	a, _ = send(t, a, msgA)
	a, _ = send(t, a, msgB)
	if a.session.q.explain == explainDone {
		t.Errorf("a stale explanation landed on today's question: %q", a.session.q.explanation)
	}
}
