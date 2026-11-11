package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/devtrainer/content"
	contentload "github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
)

// TestEveryEmbeddedItemRendersAndAnswers opens each shipped item alone in
// the Study screen at every reference size, answers it and checks that the
// whole feedback card (down to its next action) appears and that no line
// overflows.
func TestEveryEmbeddedItemRendersAndAnswers(t *testing.T) {
	cat, err := contentload.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{80, 24}, {120, 36}, {160, 36}} {
		w, h := size[0], size[1]
		for _, it := range cat.Items {
			t.Run(fmt.Sprintf("%dx%d/%s", w, h, it.ID), func(t *testing.T) {
				c, _ := cat.Concept(it.ConceptID)
				c.Prereqs = nil
				c.Lesson = nil // lessons have their own walk-through below
				tr, _ := cat.Track(c.Track)
				one := domain.Catalog{Tracks: []domain.Track{tr}, Concepts: []domain.Concept{c}, Items: []domain.Item{it}}
				a := newApp(t, one, openStore(t), &testClock{t: fixedNow}, Study, w, h)
				checkLines(t, screen(a), w, h)
				if os.Getenv("DEVTRAINER_SHOW") == "1" {
					t.Log("\n" + screen(a)) // eyeball the real content: go test -v -run Embedded
				}

				a = answerAnything(t, a)

				s := screen(a)
				if os.Getenv("DEVTRAINER_SHOW") == "1" {
					t.Log("\n" + s)
				}
				if !strings.Contains(s, "Correct") && !strings.Contains(s, "Not quite") {
					t.Errorf("no feedback:\n%s", s)
				}
				if !strings.Contains(s, "finish session") || !strings.Contains(s, "┘") {
					t.Errorf("the card's next action is cut off:\n%s", s)
				}
				checkLines(t, s, w, h)
			})
		}
	}
}

// TestEveryEmbeddedLessonRenders walks each shipped lesson, before its
// concept's first question and as a review from Tracks, at every
// reference size: every frame must fit, every prediction must show its
// verdict, and the card's next action must stay visible.
func TestEveryEmbeddedLessonRenders(t *testing.T) {
	cat, err := contentload.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, size := range [][2]int{{80, 24}, {120, 36}, {160, 36}} {
		w, h := size[0], size[1]
		for _, c := range cat.Concepts {
			if c.Lesson == nil {
				continue
			}
			n++
			for _, from := range []Screen{Study, Tracks} {
				t.Run(fmt.Sprintf("%dx%d/%s/%v", w, h, c.ID, from), func(t *testing.T) {
					c.Prereqs = nil
					tr, _ := cat.Track(c.Track)
					one := domain.Catalog{Tracks: []domain.Track{tr}, Concepts: []domain.Concept{c}, Items: cat.ItemsFor(c.ID)}
					a := newApp(t, one, openStore(t), &testClock{t: fixedNow}, from, w, h)
					if from == Tracks {
						a, _ = press(t, a, "l")
					}
					frame := func(want string) {
						t.Helper()
						s := screen(a)
						if os.Getenv("DEVTRAINER_SHOW") == "1" {
							t.Log("\n" + s)
						}
						checkLines(t, s, w, h)
						if !strings.Contains(s, want) || !strings.Contains(s, "┘") {
							t.Errorf("missing %q or the card's end:\n%s", want, s)
						}
					}
					frame("enter  first step")
					for i, st := range c.Lesson.Steps {
						a, _ = press(t, a, "enter")
						if st.Predict != nil {
							frame("predicts")
							// alternate wrong and right predictions to show both verdicts
							pick := st.Predict.Answer()
							if i%2 == 0 {
								pick = (pick + 1) % len(st.Predict.Options)
							}
							a, _ = press(t, a, fmt.Sprint(pick+1))
							if pick == st.Predict.Answer() {
								frame("✓ Right")
							} else {
								frame("✗ Not quite")
							}
							continue
						}
						frame("enter")
					}
					a, _ = press(t, a, "enter")
					frame("Takeaway")
					a, _ = press(t, a, "enter")
					if a.lessonOn() {
						t.Fatalf("the lesson did not end:\n%s", screen(a))
					}
					checkLines(t, screen(a), w, h)
				})
			}
		}
	}
	if n == 0 {
		t.Fatal("no embedded lessons")
	}
}

// checkLines fails when the screen is not exactly h lines of at most w cells.
func checkLines(t *testing.T, s string, w, h int) {
	t.Helper()
	lines := strings.Split(s, "\n")
	if len(lines) != h {
		t.Errorf("%d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if n := ansi.StringWidth(l); n > w {
			t.Errorf("line %d is %d wide: %q", i, n, l)
		}
	}
}
