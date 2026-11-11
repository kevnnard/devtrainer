package tui

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/store"
)

// examCatalog is testContent with the go track grouped into one section.
func examCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, f := range testContent {
		fsys[name] = &fstest.MapFile{Data: f.Data}
	}
	fsys["go/go.yaml"] = &fstest.MapFile{Data: append([]byte(string(testContent["go/go.yaml"].Data)),
		"sections:\n  - id: go/basics\n    title: Go basics\n    concepts: [go.slices, go.maps]\n"...)}
	cat, err := content.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// goTracks opens Tracks on the go track (second by order).
func goTracks(t *testing.T, cat domain.Catalog, st Progress) App {
	t.Helper()
	a := newApp(t, cat, st, &testClock{t: fixedNow}, Tracks, 120, 40)
	a, _ = press(t, a, "j")
	mustContain(t, screen(a), "Go basics")
	return a
}

// learnGo brings both concepts of the go section to learning.
func learnGo(t *testing.T, st *store.Store, cat domain.Catalog) {
	t.Helper()
	review(t, st, cat, "go.slices.output-len", true, "", daysAgo(2))
	review(t, st, cat, "go.maps.mcq-missing", true, "", daysAgo(2))
}

// answerRight answers the current exam question correctly.
func answerRight(t *testing.T, a App) App {
	t.Helper()
	switch a.session.q.pick.Item.ID {
	case "go.slices.cloze-append":
		a = typeText(t, a, "append")
		a, _ = press(t, a, "enter")
		a = typeText(t, a, "4")
		a, _ = press(t, a, "enter")
	case "go.slices.output-len", "go.maps.mcq-missing":
		a, _ = press(t, a, "1")
	default:
		t.Fatalf("unexpected exam item %s", a.session.q.pick.Item.ID)
	}
	if !a.session.q.answered || !a.session.q.res.Correct {
		t.Fatalf("%s was not answered correctly:\n%s", a.session.q.pick.Item.ID, screen(a))
	}
	return a
}

func TestExamIsLockedUntilEveryConceptIsLearning(t *testing.T) {
	cat := examCatalog(t)
	st := openStore(t)
	review(t, st, cat, "go.slices.output-len", true, "", daysAgo(2)) // maps still new
	a := goTracks(t, cat, st)
	mustContain(t, screen(a), "exam locked")

	a, _ = press(t, a, "x")
	if a.Screen() != Tracks || a.exam != nil {
		t.Fatalf("a locked exam must not start (screen %v)", a.Screen())
	}
	s := screen(a)
	mustContain(t, s, "Go basics", "Maps")
	if !strings.Contains(s, "learning") {
		t.Errorf("the lock must say what to learn:\n%s", s)
	}
	mustNotContain(t, s, "x starts the exam")
}

func TestExamFlowPassFeedsFSRSAndShowsInTracksAndStats(t *testing.T) {
	cat := examCatalog(t)
	st := openStore(t)
	learnGo(t, st, cat)
	ctx := context.Background()
	before, _ := st.Schedules(ctx)
	a := goTracks(t, cat, st)
	today := len(a.session.picks)
	mustContain(t, screen(a), "x starts the exam")
	mustContain(t, strings.Join(a.hints(), " "), "x exam")

	a, _ = press(t, a, "x")
	if a.Screen() != Study || a.exam == nil {
		t.Fatalf("x must start the exam on Study, got %v", a.Screen())
	}
	mustContain(t, screen(a), "exam")
	picks := a.session.picks
	concepts := map[string]bool{}
	for _, p := range picks {
		concepts[p.Concept.ID] = true
	}
	if len(picks) != 3 || !concepts["go.slices"] || !concepts["go.maps"] {
		t.Fatalf("exam picks %d over %v, want 3 over both concepts", len(picks), concepts)
	}

	for i := range picks {
		a = answerRight(t, a)
		if i < len(picks)-1 {
			a, _ = press(t, a, "enter")
		}
	}
	// Every exam answer was recorded and rescheduled its concept.
	if n := countAnswers(t, st); n != 2+3 {
		t.Errorf("answers stored = %d, want 5", n)
	}
	after, _ := st.Schedules(ctx)
	for _, id := range []string{"go.slices", "go.maps"} {
		if after[id].Reps <= before[id].Reps {
			t.Errorf("%s reps %d → %d: the exam must feed FSRS", id, before[id].Reps, after[id].Reps)
		}
	}
	// The result is stored as soon as the last answer is in.
	exams, _ := st.Exams(ctx)
	if r := exams["go/basics"]; len(r) != 1 || !r[0].Passed || r[0].Correct != 3 || r[0].Total != 3 {
		t.Fatalf("stored exam = %+v", r)
	}

	a, _ = press(t, a, "enter")
	s := screen(a)
	mustContain(t, s, "Exam passed", "3 of 3", "100%", "80%")

	a, _ = press(t, a, "enter")
	if a.Screen() != Tracks || a.exam != nil {
		t.Fatalf("enter after the result goes back to Tracks, got %v", a.Screen())
	}
	if len(a.session.picks) != today {
		t.Errorf("today's session was not restored: %d picks, want %d", len(a.session.picks), today)
	}
	mustContain(t, screen(a), "exam passed")

	a, _ = press(t, a, "4")
	mustContain(t, screen(a), "Exams passed", "Go basics", "1/1")
}

func TestExamFailBelowThePassMark(t *testing.T) {
	cat := examCatalog(t)
	st := openStore(t)
	learnGo(t, st, cat)
	a := goTracks(t, cat, st)
	a, _ = press(t, a, "x")
	n := len(a.session.picks)
	for i := 0; i < n; i++ {
		if i == 0 {
			a = answerAnything(t, a) // a wrong (or empty) answer
		} else {
			a = answerRight(t, a)
		}
		a, _ = press(t, a, "enter")
	}
	s := screen(a)
	mustContain(t, s, "Exam not passed", "2 of 3", "66%", "80%")
	exams, _ := st.Exams(context.Background())
	if r := exams["go/basics"]; len(r) != 1 || r[0].Passed {
		t.Fatalf("stored exam = %+v", r)
	}
	a, _ = press(t, a, "enter")
	mustNotContain(t, screen(a), "exam passed")
	mustContain(t, screen(a), "x starts the exam")
}

func TestExamSkipsCodeItemsWithoutToolchain(t *testing.T) {
	yaml := `track:
  id: rs
  title: Rust
  order: 1
concepts:
  - id: rs.vars
    title: Variables
    level: beginner
    items:
      - id: rs.vars.mcq
        type: mcq
        prompt: Which keyword makes a binding mutable?
        options:
          - {text: mut, correct: true}
          - {text: var, misconception: rust-var, explanation: Rust has no var.}
      - id: rs.vars.code
        type: code
        language: rust
        prompt: Print hi.
        starter: |
          fn main() {}
        solution: |
          fn main() { println!("hi"); }
        expected: hi
sections:
  - id: rs/basics
    title: Rust basics
    concepts: [rs.vars]
`
	cat, err := content.Load(fstest.MapFS{"rs/rs.yaml": {Data: []byte(yaml)}})
	if err != nil {
		t.Fatal(err)
	}
	st := openStore(t)
	review(t, st, cat, "rs.vars.mcq", true, "", daysAgo(2))
	a, err := NewApp(Config{Catalog: cat, Progress: st, Now: (&testClock{t: fixedNow}).now, MaxNew: 3,
		Start: Tracks, Runner: &fakeRunner{missing: map[string]bool{"rust": true}}})
	if err != nil {
		t.Fatal(err)
	}
	a, _ = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	a, _ = press(t, a, "x")
	if a.exam == nil {
		t.Fatalf("exam did not start:\n%s", screen(a))
	}
	for _, p := range a.session.picks {
		if p.Item.Type == domain.Code {
			t.Errorf("exam served %s although rustc is missing", p.Item.ID)
		}
	}
	if len(a.session.picks) != 1 {
		t.Errorf("picks = %d, want the one closed item", len(a.session.picks))
	}
}

func TestHelpDocumentsTheExamKey(t *testing.T) {
	cat := examCatalog(t)
	a := newApp(t, cat, openStore(t), &testClock{t: fixedNow}, Tracks, 120, 60)
	a, _ = press(t, a, "?")
	mustContain(t, screen(a), "exam of the concept's section")
}
