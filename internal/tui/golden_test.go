package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/")

// TestGolden keeps a plain-text snapshot of each key screen at the three
// reference widths, so layout changes show up in review as a text diff.
// Run `go test ./internal/tui -run Golden -update` after an intended change.
func TestGolden(t *testing.T) {
	cases := []struct {
		name string
		app  func(t *testing.T, w int) App
		keys []string
	}{
		{name: "home", app: seededApp(Home)},
		{name: "tracks", app: seededApp(Tracks), keys: []string{"j", "j"}},
		{name: "stats", app: seededApp(Stats)},
		{name: "home-empty", app: caughtUpApp},
		{name: "study-session", app: seededApp(Study)},
		{name: "study-mcq", app: itemApp("git.commit.mcq-what")},
		{name: "study-mcq-code", app: itemApp("go.slices.output-len")},
		{name: "study-feedback", app: itemApp("git.commit.mcq-what"), keys: []string{"2"}},
		{name: "study-feedback-code", app: itemApp("go.slices.output-len"), keys: []string{"2"}},
		{name: "study-parsons", app: itemApp("logic.recursion.parsons-fact"), keys: []string{"J", "J", "j"}},
		{name: "study-parsons-feedback", app: itemApp("logic.recursion.parsons-fact"), keys: []string{"J", "enter"}},
		{name: "study-cloze", app: itemApp("go.slices.cloze-append"), keys: []string{"a", "p", "p", "e", "n", "d", "enter"}},
		{name: "study-bug", app: itemApp("logic.loops.bug-bounds"), keys: []string{"j"}},
		{name: "lesson-step", app: lessonItemApp, keys: []string{"enter"}},
		{name: "lesson-predict", app: lessonItemApp, keys: []string{"enter", "enter", "j"}},
	}
	for _, c := range cases {
		for _, width := range []int{80, 120, 160} {
			name := fmt.Sprintf("%s-%d", c.name, width)
			t.Run(name, func(t *testing.T) {
				a, _ := press(t, c.app(t, width), c.keys...)
				got := trimRight(screen(a))

				path := filepath.Join("testdata", name+".golden")
				if *update {
					if err := os.MkdirAll("testdata", 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run with -update to create it)", err)
				}
				if got != string(want) {
					t.Errorf("view differs from %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
				}
			})
		}
	}
}

// heightFor is the reference height of each golden width: the artboard
// draws 80×24 and 120×36.
func heightFor(w int) int {
	if w < paneBreak {
		return 24
	}
	return 36
}

func seededApp(start Screen) func(t *testing.T, w int) App {
	return func(t *testing.T, w int) App {
		cat := testCatalog(t)
		return newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, start, w, heightFor(w))
	}
}

func itemApp(id string) func(t *testing.T, w int) App {
	return func(t *testing.T, w int) App {
		return newApp(t, onlyItem(t, id), openStore(t), &testClock{t: fixedNow}, Study, w, heightFor(w))
	}
}

// lessonItemApp is Study on logic.loops alone with loopsLesson attached.
func lessonItemApp(t *testing.T, w int) App {
	cat := withLesson(onlyItem(t, "logic.loops.mcq-count"), "logic.loops")
	return newApp(t, cat, openStore(t), &testClock{t: fixedNow}, Study, w, heightFor(w))
}

// caughtUpApp is Home after answering the whole seeded session.
func caughtUpApp(t *testing.T, w int) App {
	a := seededApp(Study)(t, w)
	for !a.session.done() {
		a = answerAnything(t, a)
		a, _ = press(t, a, "enter")
	}
	a, _ = press(t, a, "enter")
	return a
}

// trimRight drops trailing spaces so golden files stay readable.
func trimRight(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
