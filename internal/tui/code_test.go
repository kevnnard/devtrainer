package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
	"github.com/kevnnard/devtrainer/internal/srs"
)

const codeTrackYAML = `track:
  id: py
  title: Python
  order: 10
concepts:
  - id: py.loops
    title: Loops
    summary: range(a, b) stops before b.
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
        explanation: range(1, 4) yields 1, 2 and 3.
        hint: range takes a start.
  - id: py.rust
    title: Rust corner
    level: beginner
    items:
      - id: py.rust.code-hello
        type: code
        language: rust
        prompt: Print hi.
        starter: |
          fn main() {}
        solution: |
          fn main() { println!("hi"); }
        expected: hi
`

// fakeRunner records the programs it is asked to run and answers with
// run/err; languages in missing have no toolchain.
type fakeRunner struct {
	missing map[string]bool
	run     check.CodeRun
	err     error
	got     []runner.Program
}

func (f *fakeRunner) Toolchain(lang string) (runner.Toolchain, error) {
	if f.missing[lang] {
		tool := map[string]string{"rust": "rustc", "python": "python3"}[lang]
		return runner.Toolchain{}, &runner.MissingError{Language: lang, Tool: tool, Detail: "not on PATH"}
	}
	return runner.Toolchain{Language: lang, Tool: lang}, nil
}

func (f *fakeRunner) Run(_ context.Context, p runner.Program) (check.CodeRun, error) {
	f.got = append(f.got, p)
	return f.run, f.err
}

func codeCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	cat, err := content.Load(fstest.MapFS{"py/py.yaml": {Data: []byte(codeTrackYAML)}})
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// codeApp opens Study on the python code item, with rust missing.
func codeApp(t *testing.T, fr *fakeRunner) (App, *testClock, Progress) {
	t.Helper()
	st := openStore(t)
	clock := &testClock{t: fixedNow}
	if fr.missing == nil {
		fr.missing = map[string]bool{"rust": true}
	}
	a, err := NewApp(Config{Catalog: codeCatalog(t), Progress: st, Now: clock.now, MaxNew: srs.DefaultMaxNew,
		Start: Study, Runner: fr})
	if err != nil {
		t.Fatal(err)
	}
	a, _ = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 36})
	return a, clock, st
}

// runCmd executes the command the app returned and feeds its message back.
func runCmd(t *testing.T, a App, cmd tea.Cmd) App {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run the code")
	}
	a, _ = send(t, a, cmd())
	return a
}

func lastOf(t *testing.T, p Progress) domain.Answer {
	t.Helper()
	h, err := p.History(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var last domain.Answer
	n := 0
	for _, as := range h {
		for _, x := range as {
			n++
			if x.At.After(last.At) || last.ItemID == "" {
				last = x
			}
		}
	}
	if n == 0 {
		t.Fatal("no answer saved")
	}
	return last
}

func countAnswers(t *testing.T, p Progress) int {
	t.Helper()
	h, _ := p.History(context.Background())
	n := 0
	for _, as := range h {
		n += len(as)
	}
	return n
}

func TestCodeEditorOpensWithTheStarter(t *testing.T) {
	a, _, _ := codeApp(t, &fakeRunner{})
	s := screen(a)
	mustContain(t, s, "Print the numbers 1 to 3", "for i in range(3):", "    print(i)", "ctrl+r", "python")
	if !a.typing() {
		t.Error("the editor must own the keys")
	}
	// Letters are typed, not read as commands.
	a, _ = press(t, a, "q")
	if a.screen != Study || !strings.Contains(screen(a), "q") {
		t.Error("q must be typed into the editor")
	}
}

func TestCodeEditorEdits(t *testing.T) {
	a, _, _ := codeApp(t, &fakeRunner{})
	// Cursor starts at the end of the starter; go to line 1 and fix range(3).
	a, _ = press(t, a, "up", "up", "home")
	for range len("for i in range(") {
		a, _ = press(t, a, "right")
	}
	a = typeText(t, a, "1, ")
	a, _ = press(t, a, "delete")
	a = typeText(t, a, "4")
	if got := a.session.q.source(); !strings.HasPrefix(got, "for i in range(1, 4):\n    print(i)") {
		t.Errorf("source = %q", got)
	}
	// enter keeps the indentation of the line.
	a, _ = press(t, a, "down", "end", "enter")
	a = typeText(t, a, "pass")
	if got := a.session.q.source(); !strings.Contains(got, "    print(i)\n    pass") {
		t.Errorf("auto-indent: %q", got)
	}
	// backspace at the start of a line joins it with the line above.
	a, _ = press(t, a, "home", "backspace")
	if got := a.session.q.source(); !strings.Contains(got, "print(i)    pass") {
		t.Errorf("join: %q", got)
	}
	// A paste goes in as text, new lines included.
	a, _ = send(t, a, tea.PasteMsg{Content: "x\ny"})
	if got := a.session.q.source(); !strings.Contains(got, "x\ny") {
		t.Errorf("paste: %q", got)
	}
}

func TestCodePassIsGoodOrEasy(t *testing.T) {
	fr := &fakeRunner{run: check.CodeRun{Stdout: "1\n2\n3\n"}}
	a, clock, st := codeApp(t, fr)
	clock.advance(3 * time.Minute)
	a, cmd := press(t, a, "ctrl+r")
	mustContain(t, screen(a), "Running")
	if countAnswers(t, st) != 0 {
		t.Fatal("graded before the program ran")
	}
	clock.advance(10 * time.Second) // run time does not count
	a = runCmd(t, a, cmd)

	if len(fr.got) != 1 || fr.got[0].Language != "python" || !strings.HasPrefix(fr.got[0].Code, "for i in range(3):") {
		t.Errorf("ran %+v", fr.got)
	}
	got := lastOf(t, st)
	if !got.Correct || got.Grade != domain.Good || got.Duration != 3*time.Minute {
		t.Errorf("answer = %+v, want correct Good after 3m", got)
	}
	mustContain(t, screen(a), "✓ Correct", "range(1, 4) yields")

	fr2 := &fakeRunner{run: check.CodeRun{Stdout: "1\n2\n3\n"}}
	a, clock, st = codeApp(t, fr2)
	clock.advance(20 * time.Second)
	a, cmd = press(t, a, "ctrl+r")
	runCmd(t, a, cmd)
	if got := lastOf(t, st); got.Grade != domain.Easy {
		t.Errorf("fast pass graded %v, want Easy", got.Grade)
	}
}

func TestCodeFailIsAgainAndShowsTheRealError(t *testing.T) {
	fr := &fakeRunner{run: check.CodeRun{Stdout: "", Error: "Traceback (most recent call last):\n  File \"main.py\", line 2\nNameError: name 'prnt' is not defined"}}
	a, clock, st := codeApp(t, fr)
	clock.advance(time.Minute)
	a, cmd := press(t, a, "ctrl+r")
	a = runCmd(t, a, cmd)
	got := lastOf(t, st)
	if got.Correct || got.Grade != domain.Again {
		t.Errorf("answer = %+v, want wrong Again", got)
	}
	mustContain(t, screen(a), "✗ Not quite", "NameError: name 'prnt' is not defined", "1", "range(1, 4)")
}

// A long compiler or runtime error on a small terminal is previewed from
// its first line, says how much is hidden and leaves the next action on
// screen.
func TestCodeLongErrorKeepsTheNextActionAt80x24(t *testing.T) {
	var trace []string
	for i := range 30 {
		trace = append(trace, fmt.Sprintf("    at frame%d (main.py:%d)", i, i))
	}
	fr := &fakeRunner{run: check.CodeRun{Error: "FirstLineOfTheError: boom\n" + strings.Join(trace, "\n")}}
	a, _, _ := codeApp(t, fr)
	a, _ = send(t, a, tea.WindowSizeMsg{Width: 80, Height: 24})
	a, cmd := press(t, a, "ctrl+r")
	a = runCmd(t, a, cmd)
	s := screen(a)
	mustContain(t, s, "✗ Not quite", "FirstLineOfTheError: boom", "more lines", "+ 1", "enter  ", "┘")
	checkLines(t, s, 80, 24)
}

func TestCodeWrongOutputShowsBoth(t *testing.T) {
	fr := &fakeRunner{run: check.CodeRun{Stdout: "0\n1\n2\n"}}
	a, _, st := codeApp(t, fr)
	a, cmd := press(t, a, "ctrl+r")
	a = runCmd(t, a, cmd)
	if lastOf(t, st).Grade != domain.Again {
		t.Error("wrong output must be Again")
	}
	s := screen(a)
	mustContain(t, s, "- 0", "+ 1", "for i in range(1, 4):")
}

func TestCodeTimeoutIsSaidPlainly(t *testing.T) {
	fr := &fakeRunner{run: check.CodeRun{TimedOut: true}}
	a, _, _ := codeApp(t, fr)
	a, cmd := press(t, a, "ctrl+r")
	a = runCmd(t, a, cmd)
	mustContain(t, screen(a), "time limit")
}

func TestCodeHintPassIsHard(t *testing.T) {
	fr := &fakeRunner{run: check.CodeRun{Stdout: "1\n2\n3"}}
	a, _, st := codeApp(t, fr)
	a, _ = press(t, a, "ctrl+t")
	mustContain(t, screen(a), "range takes a start.")
	a, cmd := press(t, a, "ctrl+r")
	runCmd(t, a, cmd)
	if got := lastOf(t, st); !got.Correct || !got.HintUsed || got.Grade != domain.Hard {
		t.Errorf("answer = %+v, want correct, hinted, Hard", got)
	}
}

func TestCodeMissingToolchainAtRunIsNotGraded(t *testing.T) {
	fr := &fakeRunner{err: &runner.MissingError{Language: "python", Tool: "python3", Detail: "not on PATH"}}
	a, _, st := codeApp(t, fr)
	a, cmd := press(t, a, "ctrl+r")
	a = runCmd(t, a, cmd)
	if n := countAnswers(t, st); n != 0 {
		t.Errorf("%d answers saved without a toolchain", n)
	}
	mustContain(t, screen(a), "python3 not", "not graded")
	if !a.typing() {
		t.Error("the editor must stay open")
	}
}

func TestCodeItemsOfAMissingToolchainAreNotServed(t *testing.T) {
	fr := &fakeRunner{}
	a, _, _ := codeApp(t, fr) // rust is missing
	for _, p := range a.session.picks {
		if p.Item.Language == "rust" {
			t.Errorf("served %s although rustc is missing", p.Item.ID)
		}
	}
	a, _ = press(t, a, "esc")
	mustContain(t, screen(a), "rust", "rustc not found")
}

func TestNoRunnerServesNoCodeItems(t *testing.T) {
	st := openStore(t)
	a, err := NewApp(Config{Catalog: codeCatalog(t), Progress: st, Now: (&testClock{t: fixedNow}).now, MaxNew: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.session.picks) != 0 {
		t.Errorf("served %d code picks without a runner", len(a.session.picks))
	}
}

func TestCodeSQLShowsRows(t *testing.T) {
	cat := codeCatalog(t)
	it := domain.Item{ID: "py.loops.code-sql", ConceptID: "py.loops", Type: domain.Code, Language: "sql",
		Prompt: "Names.", Starter: "SELECT", Solution: "SELECT name FROM u;", Rows: [][]string{{"Ana"}, {"Bo"}}}
	cat.Items = []domain.Item{it}
	cat.Concepts = cat.Concepts[:1]
	fr := &fakeRunner{run: check.CodeRun{Columns: []string{"name"}, Rows: [][]string{{"Ana"}}}}
	st := openStore(t)
	a, err := NewApp(Config{Catalog: cat, Progress: st, Now: (&testClock{t: fixedNow}).now, MaxNew: 3, Start: Study, Runner: fr})
	if err != nil {
		t.Fatal(err)
	}
	a, _ = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 36})
	a, cmd := press(t, a, "ctrl+r")
	a = runCmd(t, a, cmd)
	mustContain(t, screen(a), "- Ana", "+ Ana", "Bo")
}

// errorLesson is a lesson whose second step shows a real error.
var errorLesson = domain.Lesson{
	Intro:   "Python reads names when the line runs.",
	Example: domain.Example{Lang: "python", Code: "print(totl)\n"},
	Steps: []domain.LessonStep{
		{Text: "The program starts on line 1."},
		{
			Text:  "totl was never assigned: Python only knows total.",
			Error: "NameError: name 'totl' is not defined",
			Predict: &domain.Predict{
				Prompt: "Why does it fail?",
				Options: []domain.PredictOption{
					{Text: "The name is misspelled", Correct: true},
					{Text: "print needs two arguments"},
				},
			},
		},
	},
	Takeaway: "A NameError names the unknown name.",
}

func TestLessonErrorStepShowsTheErrorBeforeTheWhy(t *testing.T) {
	cat := testCatalog(t)
	cat.Concepts = append([]domain.Concept(nil), cat.Concepts...)
	for i := range cat.Concepts {
		if cat.Concepts[i].ID == "logic.loops" {
			l := errorLesson
			cat.Concepts[i].Lesson = &l
		}
	}
	a := newApp(t, cat, openStore(t), &testClock{t: fixedNow}, Study, 120, 36)
	a, _ = press(t, a, "enter")
	mustNotContain(t, screen(a), "NameError")
	a, _ = press(t, a, "enter")
	s := screen(a)
	mustContain(t, s, "NameError: name 'totl' is not defined", "Predict: Why does it fail?", "The name is misspelled")
	mustNotContain(t, s, "Python only knows")
	a, _ = press(t, a, "1")
	mustContain(t, screen(a), "✓ Right", "Python only knows", "NameError")
}
