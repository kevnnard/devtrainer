package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
	"github.com/kevnnard/devtrainer/internal/srs"
	"github.com/kevnnard/devtrainer/internal/store"
)

// fixedNow is a Tuesday evening.
var fixedNow = time.Date(2026, 11, 10, 18, 30, 0, 0, time.UTC)

// testClock is an injectable clock tests can move forward.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

var specialKeys = map[string]tea.KeyPressMsg{
	"space":      {Code: tea.KeySpace, Text: " "},
	"enter":      {Code: tea.KeyEnter},
	"esc":        {Code: tea.KeyEscape},
	"tab":        {Code: tea.KeyTab},
	"shift+tab":  {Code: tea.KeyTab, Mod: tea.ModShift},
	"backspace":  {Code: tea.KeyBackspace},
	"up":         {Code: tea.KeyUp},
	"down":       {Code: tea.KeyDown},
	"shift+up":   {Code: tea.KeyUp, Mod: tea.ModShift},
	"shift+down": {Code: tea.KeyDown, Mod: tea.ModShift},
	"alt+enter":  {Code: tea.KeyEnter, Mod: tea.ModAlt},
	"ctrl+c":     {Code: 'c', Mod: tea.ModCtrl},
	"ctrl+t":     {Code: 't', Mod: tea.ModCtrl},
	"ctrl+u":     {Code: 'u', Mod: tea.ModCtrl},
}

func key(s string) tea.KeyPressMsg {
	if k, ok := specialKeys[s]; ok {
		return k
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func TestKeyStrings(t *testing.T) {
	for name := range specialKeys {
		if got := key(name).String(); got != name {
			t.Errorf("key(%q).String() = %q", name, got)
		}
	}
	for _, s := range []string{"q", "?", "1", "J", "K", "h", "e"} {
		if got := key(s).String(); got != s {
			t.Errorf("key(%q).String() = %q", s, got)
		}
	}
}

func send(t *testing.T, a App, msgs ...tea.Msg) (App, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		next, c := a.Update(msg)
		a, cmd = next.(App), c
	}
	return a, cmd
}

func press(t *testing.T, a App, keys ...string) (App, tea.Cmd) {
	t.Helper()
	msgs := make([]tea.Msg, len(keys))
	for i, k := range keys {
		msgs[i] = key(k)
	}
	return send(t, a, msgs...)
}

// typeText presses one key per rune of s.
func typeText(t *testing.T, a App, s string) App {
	t.Helper()
	for _, r := range s {
		k := string(r)
		if r == ' ' {
			k = "space"
		}
		a, _ = press(t, a, k)
	}
	return a
}

func screen(a App) string { return ansi.Strip(a.View().Content) }

func mustContain(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func mustNotContain(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(got, w) {
			t.Errorf("unexpected %q in:\n%s", w, got)
		}
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// testContent is a small catalog with every item type over three tracks.
var testContent = fstest.MapFS{
	"logic/logic.yaml": {Data: []byte(`track:
  id: logic
  title: Logic & Algorithms
  description: Loops, recursion and how to reason about them.
  order: 10
concepts:
  - id: logic.loops
    title: Loops
    summary: Repeat a block while a condition holds.
    level: beginner
    items:
      - id: logic.loops.mcq-count
        type: mcq
        prompt: How many times does the body of "for i := 0; i < 3; i++" run?
        options:
          - text: "3"
            correct: true
          - text: "4"
            misconception: off-by-one-inclusive
            explanation: The loop stops when i reaches 3, so i takes 0, 1 and 2.
          - text: "2"
            misconception: off-by-one-exclusive
            explanation: i = 0 counts as a run too.
        explanation: i takes the values 0, 1 and 2.
        hint: Write down each value of i.
      - id: logic.loops.output-sum
        type: output
        prompt: What does this print?
        code: |
          s := 0
          for i := 1; i <= 3; i++ {
              s += i
          }
          fmt.Println(s)
        answers: ["6"]
        explanation: 1 + 2 + 3 = 6.
      - id: logic.loops.bug-bounds
        type: bug
        prompt: This should print every element. Which line is wrong?
        code: |
          xs := []int{1, 2, 3}
          for i := 0; i <= len(xs); i++ {
              fmt.Println(xs[i])
          }
        bug_line: 2
        explanation: i <= len(xs) reads one past the end.
  - id: logic.recursion
    title: Recursion
    summary: A function that calls itself on a smaller input.
    level: intermediate
    prereqs: [logic.loops]
    items:
      - id: logic.recursion.parsons-fact
        type: parsons
        prompt: Order the lines of a recursive factorial.
        lines:
          - "func fact(n int) int {"
          - "    if n <= 1 {"
          - "        return 1"
          - "    }"
          - "    return n * fact(n-1)"
          - "}"
        explanation: The base case comes before the recursive call.
      - id: logic.recursion.multi-needs
        type: multi
        prompt: What does every terminating recursion need?
        options:
          - text: A base case
            correct: true
          - text: Progress toward the base case
            correct: true
          - text: A loop
            misconception: recursion-needs-loop
            explanation: Recursion replaces the loop.
        explanation: Without both, the recursion never stops.
`)},
	"go/go.yaml": {Data: []byte(`track:
  id: go
  title: Go
  description: The Go language from slices to concurrency.
  order: 20
concepts:
  - id: go.slices
    title: Slices
    summary: A view over an array with length and capacity.
    level: beginner
    items:
      - id: go.slices.output-len
        type: output
        prompt: What does this print?
        code: |
          s := make([]int, 2, 5)
          fmt.Println(len(s), cap(s))
        options:
          - text: "2 5"
            correct: true
          - text: "5 5"
            misconception: make-len-is-cap
            explanation: The second argument is the length; the third is the capacity.
        explanation: make([]T, len, cap).
      - id: go.slices.cloze-append
        type: cloze
        prompt: Add 4 to the slice.
        code: |
          s = ___(s, ___)
        answers: ["append | 4"]
        explanation: append returns the new slice.
  - id: go.maps
    title: Maps
    summary: Hash tables keyed by comparable values.
    level: beginner
    prereqs: [go.slices]
    items:
      - id: go.maps.mcq-missing
        type: mcq
        prompt: What does m["x"] return for a missing key of a map[string]int?
        options:
          - text: "0"
            correct: true
          - text: A panic
            misconception: missing-key-panics
            explanation: Reading a missing key returns the zero value.
        explanation: Missing keys read as the zero value.
`)},
	"git/git.yaml": {Data: []byte(`track:
  id: git
  title: Git
  description: Version control from the first commit to branching.
  order: 50
concepts:
  - id: git.commit
    title: Commits
    summary: A commit records the staged snapshot.
    level: beginner
    items:
      - id: git.commit.mcq-what
        type: mcq
        prompt: What does git commit record?
        options:
          - text: The staged changes
            correct: true
          - text: Every change in the working tree
            misconception: commit-takes-worktree
            explanation: Only staged changes are committed; unstaged edits stay in the working tree.
        explanation: A commit is a snapshot of the index.
        hint: Think of git add.
      - id: git.commit.cmd-message
        type: command
        prompt: Commit the staged changes with the message "init".
        answers: ['git commit -m "init"']
        explanation: -m sets the message inline.
  - id: git.branch
    title: Branches
    summary: A branch is a movable pointer to a commit.
    level: intermediate
    prereqs: [git.commit]
    items:
      - id: git.branch.parsons-feature
        type: parsons
        prompt: Start a feature branch and commit to it.
        lines:
          - git switch -c feature
          - git add .
          - git commit -m "wip"
        explanation: Create the branch first, then stage and commit.
`)},
}

func testCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	cat, err := content.Load(testContent)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// openStore opens a fresh store in a temp directory.
func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// review records one answer to itemID at at, scheduled like the app does.
func review(t *testing.T, st *store.Store, cat domain.Catalog, itemID string, correct bool, misconception string, at time.Time) {
	t.Helper()
	var it domain.Item
	for _, x := range cat.Items {
		if x.ID == itemID {
			it = x
		}
	}
	if it.ID == "" {
		t.Fatalf("no item %s", itemID)
	}
	ctx := context.Background()
	scheds, err := st.Schedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sc := scheds[it.ConceptID]
	sc.ConceptID = it.ConceptID
	g := srs.DeriveGrade(correct, false, 20*time.Second, srs.DefaultThresholds)
	next := srs.NewScheduler().Review(sc, g, it.ID, at)
	a := domain.Answer{ConceptID: it.ConceptID, ItemID: it.ID, Correct: correct, Grade: g,
		Misconception: misconception, Duration: 20 * time.Second, At: at}
	if err := st.RecordReview(ctx, a, next); err != nil {
		t.Fatal(err)
	}
}

func daysAgo(n int) time.Time { return fixedNow.AddDate(0, 0, -n).Add(-8 * time.Hour) }

// seeded is a store with a week of history: loops mastered, slices in
// review, commits learning with a mistake, and a three-day streak.
func seeded(t *testing.T, cat domain.Catalog) *store.Store {
	st := openStore(t)
	review(t, st, cat, "logic.loops.mcq-count", true, "", daysAgo(9))
	review(t, st, cat, "logic.loops.output-sum", true, "", daysAgo(6))
	review(t, st, cat, "logic.loops.bug-bounds", true, "", daysAgo(3))
	review(t, st, cat, "logic.loops.mcq-count", true, "", daysAgo(2))
	review(t, st, cat, "go.slices.output-len", false, "make-len-is-cap", daysAgo(5))
	review(t, st, cat, "go.slices.cloze-append", true, "", daysAgo(3))
	review(t, st, cat, "go.slices.output-len", true, "", daysAgo(1))
	review(t, st, cat, "git.commit.mcq-what", true, "", daysAgo(2))
	review(t, st, cat, "git.commit.cmd-message", false, "", daysAgo(1))
	review(t, st, cat, "git.commit.mcq-what", false, "commit-takes-worktree", daysAgo(1))
	return st
}

// fakeTutor answers Explain with text or err.
type fakeTutor struct {
	text  string
	err   error
	calls int
}

func (f *fakeTutor) Explain(context.Context, domain.Item, domain.Concept, check.Response, check.Result) (string, error) {
	f.calls++
	return f.text, f.err
}

// newApp builds an app over cat and st at the clock, sized w×h.
func newApp(t *testing.T, cat domain.Catalog, st Progress, clock *testClock, start Screen, w, h int) App {
	t.Helper()
	a, err := NewApp(Config{Catalog: cat, Progress: st, Now: clock.now, MaxNew: srs.DefaultMaxNew, Start: start,
		Tutor: &fakeTutor{text: "Think of the index as the next commit."}, Runner: runner.New()})
	if err != nil {
		t.Fatal(err)
	}
	a, _ = send(t, a, tea.WindowSizeMsg{Width: w, Height: h})
	return a
}

// onlyItem is a catalog with one concept holding only the item with id,
// so the session is exactly that item.
func onlyItem(t *testing.T, id string) domain.Catalog {
	t.Helper()
	full := testCatalog(t)
	for _, it := range full.Items {
		if it.ID != id {
			continue
		}
		c, _ := full.Concept(it.ConceptID)
		c.Prereqs = nil
		tr, _ := full.Track(c.Track)
		return domain.Catalog{Tracks: []domain.Track{tr}, Concepts: []domain.Concept{c}, Items: []domain.Item{it}}
	}
	t.Fatalf("no item %s", id)
	return domain.Catalog{}
}

// studyApp opens the Study screen on the single item id over a fresh store.
func studyApp(t *testing.T, id string, w int) (App, *store.Store, *testClock) {
	t.Helper()
	st := openStore(t)
	clock := &testClock{t: fixedNow}
	return newApp(t, onlyItem(t, id), st, clock, Study, w, 24), st, clock
}

// lastAnswer is the newest answer in st.
func lastAnswer(t *testing.T, st *store.Store) domain.Answer {
	t.Helper()
	got, err := st.RecentAnswers(context.Background(), 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("answers %v, err %v", got, err)
	}
	return got[0]
}
