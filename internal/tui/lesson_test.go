package tui

import (
	"context"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/store"
)

// loopsLesson is a three-step lesson; step 2 is a prediction.
var loopsLesson = domain.Lesson{
	Intro: "A for loop repeats its body while its condition holds.",
	Example: domain.Example{Lang: "go", Code: `for i := 0; i < 3; i++ {
    fmt.Println(i)
}`},
	Steps: []domain.LessonStep{
		{Text: "i := 0 runs once, before the first check of i < 3."},
		{
			Text: "After 2 is printed, i++ makes i 3; 3 < 3 is false, so the loop ends.",
			Predict: &domain.Predict{
				Prompt: "After the loop prints 2, what happens next?",
				Options: []domain.PredictOption{
					{Text: "i becomes 3 and the loop stops", Correct: true},
					{Text: "It prints 3 and then stops", Explanation: "The check runs before the body: 3 < 3 is false."},
				},
			},
		},
		{Text: "The body ran for i = 0, 1 and 2: three times in all."},
	},
	Takeaway: "for init; cond; post runs",
}

// withLesson attaches loopsLesson to concept id of cat.
func withLesson(cat domain.Catalog, id string) domain.Catalog {
	out := cat
	out.Concepts = make([]domain.Concept, len(cat.Concepts))
	copy(out.Concepts, cat.Concepts)
	for i := range out.Concepts {
		if out.Concepts[i].ID == id {
			l := loopsLesson
			out.Concepts[i].Lesson = &l
		}
	}
	return out
}

// lessonApp opens Study over the test catalog with a lesson on
// logic.loops, the first new concept of the session.
func lessonApp(t *testing.T, w int) (App, *store.Store, *testClock) {
	t.Helper()
	st := openStore(t)
	clock := &testClock{t: fixedNow}
	return newApp(t, withLesson(testCatalog(t), "logic.loops"), st, clock, Study, w, 36), st, clock
}

const loopsQuestion = "How many times does the body"

func seenLessons(t *testing.T, p *store.Store) map[string]bool {
	t.Helper()
	seen, err := p.LessonsSeen(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return seen
}

func answerCount(t *testing.T, p *store.Store) int {
	t.Helper()
	h, err := p.History(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, as := range h {
		n += len(as)
	}
	return n
}

func TestLessonRevealsStepsBeforeTheFirstQuestion(t *testing.T) {
	a, st, clock := lessonApp(t, 120)

	s := screen(a)
	mustContain(t, s, "A for loop repeats its body", "fmt.Println(i)", "lesson", "enter", "skip lesson")
	mustNotContain(t, s, loopsQuestion, "runs once", "After the loop prints 2")

	a, _ = press(t, a, "enter")
	s = screen(a)
	mustContain(t, s, "runs once", "step 1/3")
	mustNotContain(t, s, "After the loop prints 2", loopsQuestion)

	a, _ = press(t, a, "enter")
	s = screen(a)
	mustContain(t, s, "Predict: After the loop prints 2", "i becomes 3 and the loop stops", "It prints 3", "step 2/3")
	mustNotContain(t, s, "so the loop ends", "three times")

	// enter needs a prediction first: j/k + enter picks.
	a, _ = press(t, a, "enter")
	s = screen(a)
	mustContain(t, s, "✓ Right", "so the loop ends")
	mustNotContain(t, s, "three times")

	a, _ = press(t, a, "enter")
	mustContain(t, screen(a), "three times", "step 3/3")
	mustNotContain(t, screen(a), "for init; cond; post")

	a, _ = press(t, a, "enter")
	mustContain(t, screen(a), "Takeaway", "for init; cond; post runs")
	if seenLessons(t, st)["logic.loops"] {
		t.Error("the lesson is marked seen before it ends")
	}

	clock.advance(time.Minute) // time spent on the lesson must not count
	a, _ = press(t, a, "enter")
	mustContain(t, screen(a), loopsQuestion)
	if !seenLessons(t, st)["logic.loops"] {
		t.Error("finishing the lesson must mark it seen")
	}
	if n := answerCount(t, st); n != 0 {
		t.Errorf("the lesson saved %d answers", n)
	}

	clock.advance(4 * time.Second)
	a, _ = press(t, a, "1")
	if got := lastAnswer(t, st); got.Duration != 4*time.Second {
		t.Errorf("answer took %v, want 4s (lesson time excluded)", got.Duration)
	}
	_ = a
}

func TestLessonPredictWrongShowsTheAnswerAndSavesNothing(t *testing.T) {
	a, st, _ := lessonApp(t, 120)
	a, _ = press(t, a, "enter", "enter")

	a, _ = press(t, a, "2")
	s := screen(a)
	mustContain(t, s, "✗ Not quite. It is: i becomes 3", "The check runs before",
		"so the loop ends")
	if a.Screen() != Study {
		t.Errorf("2 must predict, not switch screens; on %v", a.Screen())
	}
	if n := answerCount(t, st); n != 0 {
		t.Errorf("a prediction saved %d answers", n)
	}
	sc, _ := st.Schedules(context.Background())
	if _, ok := sc["logic.loops"]; ok {
		t.Error("a prediction touched the FSRS schedule")
	}
}

func TestLessonPredictWithCursor(t *testing.T) {
	a, _, _ := lessonApp(t, 120)
	a, _ = press(t, a, "enter", "enter", "j", "k", "j")
	mustNotContain(t, screen(a), "Not quite", "Right")
	a, _ = press(t, a, "enter")
	mustContain(t, screen(a), "✗ Not quite")

	// An option number past the last one does nothing (and does not switch screens).
	a, _, _ = lessonApp(t, 120)
	a, _ = press(t, a, "enter", "enter", "3")
	if a.Screen() != Study {
		t.Fatalf("3 switched to %v during a prediction", a.Screen())
	}
	mustContain(t, screen(a), "After the loop prints 2")
}

func TestLessonSkip(t *testing.T) {
	a, st, _ := lessonApp(t, 120)
	a, _ = press(t, a, "enter", "s")
	mustContain(t, screen(a), loopsQuestion)
	if !seenLessons(t, st)["logic.loops"] {
		t.Error("a skipped lesson is not forced again")
	}
}

func TestLessonNotShownAgain(t *testing.T) {
	cat := withLesson(testCatalog(t), "logic.loops")

	// Seen in an earlier run.
	st := openStore(t)
	if err := st.MarkLessonSeen(context.Background(), "logic.loops", fixedNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, cat, st, &testClock{t: fixedNow}, Study, 120, 36)
	mustContain(t, screen(a), loopsQuestion)
	mustNotContain(t, screen(a), "A for loop repeats its body")

	// Answered before lessons existed: not new, so no lesson.
	st = openStore(t)
	review(t, st, cat, "logic.loops.mcq-count", false, "off-by-one-inclusive", daysAgo(2))
	a = newApp(t, cat, st, &testClock{t: fixedNow}, Study, 120, 36)
	mustNotContain(t, screen(a), "A for loop repeats its body")
}

func TestLessonSurvivesScreenSwitch(t *testing.T) {
	a, _, _ := lessonApp(t, 120)
	a, _ = press(t, a, "enter", "1", "2")
	if a.Screen() != Study {
		t.Fatalf("on %v", a.Screen())
	}
	mustContain(t, screen(a), "runs once")
	mustNotContain(t, screen(a), loopsQuestion)
}

func TestConceptWithoutLessonGoesStraightToTheQuestion(t *testing.T) {
	a, st, _ := studyApp(t, "git.commit.mcq-what", 120)
	mustContain(t, screen(a), "What does git commit record?")
	mustNotContain(t, screen(a), "skip lesson")
	press(t, a, "1")
	if !lastAnswer(t, st).Correct {
		t.Error("1 answers the question directly")
	}

	// After the lesson concept, the next concept (no lesson) starts at once.
	b, _, _ := lessonApp(t, 120)
	b, _ = press(t, b, "s", "1", "enter")
	mustNotContain(t, screen(b), "skip lesson")
	if b.session.pos != 1 || b.session.q.shown.IsZero() {
		t.Errorf("second pick not started: pos %d", b.session.pos)
	}
}

func TestTracksOpensALessonForReview(t *testing.T) {
	cat := withLesson(testCatalog(t), "logic.loops")
	st := seeded(t, cat)
	a := newApp(t, cat, st, &testClock{t: fixedNow}, Tracks, 120, 36)
	mustContain(t, screen(a), "Loops · lesson", "l opens the lesson")

	a, _ = press(t, a, "l")
	if a.Screen() != Tracks {
		t.Fatalf("l switched to %v", a.Screen())
	}
	mustContain(t, screen(a), "A for loop repeats its body", "back to Tracks")

	a, _ = press(t, a, "enter", "enter", "1", "enter", "enter")
	mustContain(t, screen(a), "Takeaway")
	a, _ = press(t, a, "enter")
	s := screen(a)
	mustContain(t, s, "Logic & Algorithms", "Fastest unlock")
	mustNotContain(t, s, "A for loop repeats its body")
	if n := answerCount(t, st); n != 10 {
		t.Errorf("a review lesson changed the answers: %d", n)
	}

	// enter on the concept opens it too; esc closes back to Tracks.
	a, _ = press(t, a, "enter")
	mustContain(t, screen(a), "A for loop repeats its body")
	a, _ = press(t, a, "esc")
	if a.Screen() != Tracks {
		t.Fatalf("esc in a review lesson goes back to Tracks, got %v", a.Screen())
	}
	mustNotContain(t, screen(a), "A for loop repeats its body")

	// J moves to a concept without a lesson: l does nothing.
	a, _ = press(t, a, "J")
	mustContain(t, screen(a), "Recursion has no lesson yet")
	a, _ = press(t, a, "l")
	mustNotContain(t, screen(a), "A for loop repeats its body")
	if a.Screen() != Tracks {
		t.Errorf("on %v", a.Screen())
	}
}

func TestLessonHelpAndHints(t *testing.T) {
	a, _, _ := lessonApp(t, 120)
	mustContain(t, screen(a), "enter next", "s skip lesson")
	a, _ = press(t, a, "enter", "enter")
	mustContain(t, screen(a), "1-9 predict", "j/k move")
	a, _ = press(t, a, "?")
	mustContain(t, screen(a), "Lesson", "predict the next step", "skip the lesson", "J/K", "open the concept's lesson")
}

func TestLessonFitsSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 24}, {120, 36}, {160, 36}, {60, 20}} {
		st := openStore(t)
		a := newApp(t, withLesson(testCatalog(t), "logic.loops"), st, &testClock{t: fixedNow}, Study, size[0], size[1])
		for _, k := range []string{"", "enter", "enter", "2", "enter", "enter"} {
			if k != "" {
				a, _ = press(t, a, k)
			}
			checkLines(t, screen(a), size[0], size[1])
		}
	}
}
