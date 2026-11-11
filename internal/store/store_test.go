package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

var ctx = context.Background()

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "nested", "devtrainer.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	got, err := DefaultPath()
	if err != nil || got != "/xdg/data/devtrainer/devtrainer.db" {
		t.Errorf("DefaultPath with XDG_DATA_HOME = %q, %v", got, err)
	}

	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/someone")
	got, err = DefaultPath()
	if err != nil || got != "/home/someone/.local/share/devtrainer/devtrainer.db" {
		t.Errorf("DefaultPath fallback = %q, %v", got, err)
	}
}

func TestOpenCreatesAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "devtrainer.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	v, err := s.SchemaVersion(ctx)
	if err != nil || v != len(migrations) || v == 0 {
		t.Errorf("SchemaVersion = %d, %v; want %d", v, err, len(migrations))
	}
	if err := s.RecordReview(ctx, answer("git.a", "i1", true, time.Now()), domain.Schedule{ConceptID: "git.a", Reps: 1}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file not created: %v", err)
	}

	// Reopening applies nothing new and keeps the data.
	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	if v2, _ := s.SchemaVersion(ctx); v2 != v {
		t.Errorf("version after reopen = %d, want %d", v2, v)
	}
	scheds, err := s.Schedules(ctx)
	if err != nil || len(scheds) != 1 {
		t.Errorf("Schedules after reopen = %v, %v", scheds, err)
	}
}

func answer(concept, item string, correct bool, at time.Time) domain.Answer {
	a := domain.Answer{ConceptID: concept, ItemID: item, Correct: correct, Grade: domain.Good, Duration: 12500 * time.Millisecond, At: at}
	if !correct {
		a.Grade = domain.Again
	}
	return a
}

func TestRecordReviewRoundTrip(t *testing.T) {
	s := openTemp(t)
	bogota := time.FixedZone("COT", -5*3600)
	at := time.Date(2026, 11, 10, 21, 30, 15, 123456789, bogota)
	sc := domain.Schedule{
		ConceptID: "git.staging", Stability: 3.17, Difficulty: 5.28, ElapsedDays: 0, ScheduledDays: 3,
		Reps: 1, Lapses: 0, FSRSState: 2, Due: at.Add(72 * time.Hour), LastReview: at, LastItemID: "git.staging.1",
	}
	a := domain.Answer{
		ConceptID: "git.staging", ItemID: "git.staging.1", Correct: false, Grade: domain.Again,
		Misconception: "commit-takes-worktree", Duration: 9876 * time.Millisecond, HintUsed: true, At: at,
	}
	if err := s.RecordReview(ctx, a, sc); err != nil {
		t.Fatalf("RecordReview: %v", err)
	}

	scheds, err := s.Schedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := scheds["git.staging"]
	if !got.Due.Equal(sc.Due) || !got.LastReview.Equal(sc.LastReview) {
		t.Errorf("times = %v / %v, want %v / %v", got.Due, got.LastReview, sc.Due, sc.LastReview)
	}
	got.Due, got.LastReview = sc.Due, sc.LastReview
	if got != sc {
		t.Errorf("schedule = %+v\nwant       %+v", got, sc)
	}

	// A second review replaces the schedule.
	sc2 := sc
	sc2.Reps, sc2.Lapses, sc2.LastItemID = 2, 1, "git.staging.2"
	if err := s.RecordReview(ctx, answer("git.staging", "git.staging.2", true, at.Add(time.Hour)), sc2); err != nil {
		t.Fatal(err)
	}
	scheds, _ = s.Schedules(ctx)
	if g := scheds["git.staging"]; g.Reps != 2 || g.Lapses != 1 || g.LastItemID != "git.staging.2" || len(scheds) != 1 {
		t.Errorf("after second review: %+v (%d schedules)", g, len(scheds))
	}

	hist, err := s.History(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h := hist["git.staging"]
	if len(h) != 2 {
		t.Fatalf("history = %+v", h)
	}
	first := h[0]
	if !first.At.Equal(at) || first.Duration != a.Duration || !first.HintUsed || first.Correct ||
		first.Grade != domain.Again || first.Misconception != a.Misconception || first.ItemID != a.ItemID {
		t.Errorf("first answer = %+v, want %+v", first, a)
	}
	if !h[1].Correct || h[1].Grade != domain.Good {
		t.Errorf("second answer = %+v", h[1])
	}
}

func TestHistoryIsChronologicalPerConcept(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	// Recorded out of order on purpose.
	for _, a := range []domain.Answer{
		answer("go.a", "1", true, base.Add(2*time.Hour)),
		answer("go.b", "1", false, base.Add(time.Hour)),
		answer("go.a", "2", false, base),
	} {
		if err := s.RecordReview(ctx, a, domain.Schedule{ConceptID: a.ConceptID, Reps: 1}); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := s.History(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 || len(hist["go.a"]) != 2 || len(hist["go.b"]) != 1 {
		t.Fatalf("history = %+v", hist)
	}
	if hist["go.a"][0].ItemID != "2" || hist["go.a"][1].ItemID != "1" {
		t.Errorf("go.a not chronological: %+v", hist["go.a"])
	}

	recent, err := s.RecentAnswers(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || !recent[0].At.Equal(base.Add(2*time.Hour)) || !recent[1].At.Equal(base.Add(time.Hour)) {
		t.Errorf("RecentAnswers = %+v, want newest first", recent)
	}
}

func TestStudyDays(t *testing.T) {
	s := openTemp(t)
	bogota := time.FixedZone("COT", -5*3600)
	d1 := time.Date(2026, 11, 9, 23, 0, 0, 0, bogota) // 04:00 UTC on the 10th
	d2 := time.Date(2026, 11, 10, 8, 0, 0, 0, bogota)
	for _, a := range []domain.Answer{
		answer("c", "1", true, d1), answer("c", "2", false, d1.Add(time.Minute)), answer("c", "1", true, d2),
	} {
		if err := s.RecordReview(ctx, a, domain.Schedule{ConceptID: "c", Reps: 1}); err != nil {
			t.Fatal(err)
		}
	}
	days, err := s.StudyDays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Day{{Date: "2026-11-09", Answers: 2, Correct: 1}, {Date: "2026-11-10", Answers: 1, Correct: 1}}
	if len(days) != 2 || days[0] != want[0] || days[1] != want[1] {
		t.Errorf("StudyDays = %+v, want %+v (dates in the learner's zone)", days, want)
	}
}

func TestTopMisconceptionsAndWeakest(t *testing.T) {
	s := openTemp(t)
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	record := func(concept string, correct bool, misc string) {
		t.Helper()
		a := answer(concept, "i", correct, at)
		a.Misconception = misc
		at = at.Add(time.Minute)
		if err := s.RecordReview(ctx, a, domain.Schedule{ConceptID: concept, Reps: 1}); err != nil {
			t.Fatal(err)
		}
	}
	record("git.a", false, "commit-takes-worktree")
	record("git.a", false, "commit-takes-worktree")
	record("git.b", false, "commit-takes-worktree")
	record("git.b", false, "push-local")
	record("git.b", false, "")
	record("git.a", true, "")
	record("go.a", true, "")
	record("go.b", false, "zero-index")
	record("go.b", true, "")

	top, err := s.TopMisconceptions(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0] != (domain.MisconceptionCount{Misconception: "commit-takes-worktree", Count: 3}) || top[1].Count != 1 {
		t.Errorf("TopMisconceptions = %+v", top)
	}

	weak, err := s.Weakest(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	// git.b 0/3, git.a 1/3, go.b 1/2; go.a is never wrong and is left out.
	want := []domain.ConceptAccuracy{
		{ConceptID: "git.b", Answers: 3, Correct: 0},
		{ConceptID: "git.a", Answers: 3, Correct: 1},
		{ConceptID: "go.b", Answers: 2, Correct: 1},
	}
	if len(weak) != len(want) {
		t.Fatalf("Weakest = %+v, want %+v", weak, want)
	}
	for i := range want {
		if weak[i] != want[i] {
			t.Errorf("Weakest[%d] = %+v, want %+v", i, weak[i], want[i])
		}
	}
	if w, _ := s.Weakest(ctx, 1); len(w) != 1 || w[0].ConceptID != "git.b" {
		t.Errorf("Weakest limit 1 = %+v", w)
	}
}

func TestStats(t *testing.T) {
	s := openTemp(t)
	cat := domain.Catalog{
		Tracks: []domain.Track{{ID: "git", Title: "Git"}, {ID: "go", Title: "Go"}},
		Concepts: []domain.Concept{
			{ID: "git.a", Track: "git"}, {ID: "git.b", Track: "git"}, {ID: "git.c", Track: "git"}, {ID: "git.d", Track: "git"},
			{ID: "go.a", Track: "go"},
		},
	}
	loc := time.UTC
	day := func(n, h int) time.Time { return time.Date(2026, 11, 8+n, h, 0, 0, 0, loc) }
	record := func(concept string, correct bool, at time.Time) {
		t.Helper()
		a := answer(concept, "i", correct, at)
		if !correct {
			a.Misconception = "m-" + concept
		}
		if err := s.RecordReview(ctx, a, domain.Schedule{ConceptID: concept, Reps: 1}); err != nil {
			t.Fatal(err)
		}
	}
	// git.a mastered: 4 correct over days 0 and 1.
	record("git.a", true, day(0, 9))
	record("git.a", true, day(0, 10))
	record("git.a", true, day(1, 9))
	record("git.a", true, day(1, 10))
	// git.b review: correct on days 1 and 2.
	record("git.b", true, day(1, 11))
	record("git.b", true, day(2, 9))
	// git.c learning: one correct.
	record("git.c", false, day(2, 10))
	record("git.c", true, day(2, 11))
	// git.d new. go.a new with a wrong answer.
	record("go.a", false, day(2, 12))
	// An answer for a concept no longer in the content is ignored by Stats.
	record("old.x", false, day(2, 13))

	now := day(2, 20) // days 0,1,2 studied → streak 3
	st, err := s.Stats(ctx, cat, now)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(st.Tracks) != 2 {
		t.Fatalf("tracks = %+v", st.Tracks)
	}
	g := st.Tracks[0]
	if g.Track.ID != "git" || g.Total != 4 || g.Mastered != 1 || g.Review != 1 || g.Learning != 1 || g.New != 1 {
		t.Errorf("git progress = %+v", g)
	}
	if g.MasteryPercent() != 25 {
		t.Errorf("git mastery = %v%%, want 25%%", g.MasteryPercent())
	}
	if o := st.Tracks[1]; o.Track.ID != "go" || o.Total != 1 || o.New != 1 || o.MasteryPercent() != 0 {
		t.Errorf("go progress = %+v", o)
	}
	wantStates := map[string]domain.State{
		"git.a": domain.StateMastered, "git.b": domain.StateReview, "git.c": domain.StateLearning,
		"git.d": domain.StateNew, "go.a": domain.StateNew,
	}
	if len(st.States) != len(wantStates) {
		t.Errorf("states = %v", st.States)
	}
	for id, w := range wantStates {
		if st.States[id] != w {
			t.Errorf("state[%s] = %v, want %v", id, st.States[id], w)
		}
	}
	if st.Streak != 3 || !st.StudiedToday {
		t.Errorf("streak = %d, today = %v; want 3, true", st.Streak, st.StudiedToday)
	}
	if st.TotalAnswers != 9 || st.TotalCorrect != 7 {
		t.Errorf("totals = %d/%d, want 9 answers, 7 correct (catalog only)", st.TotalAnswers, st.TotalCorrect)
	}
	for _, w := range st.Weakest {
		if w.ConceptID == "old.x" {
			t.Error("weakest includes a concept outside the catalog")
		}
	}
	if len(st.Weakest) != 2 || st.Weakest[0].ConceptID != "go.a" {
		t.Errorf("weakest = %+v, want go.a then git.c", st.Weakest)
	}
	if len(st.TopMisconceptions) == 0 {
		t.Error("expected top misconceptions")
	}

	// Next day without study: the streak still counts until it is broken.
	st, _ = s.Stats(ctx, cat, day(3, 9))
	if st.Streak != 3 || st.StudiedToday {
		t.Errorf("next day: streak = %d, today = %v; want 3, false", st.Streak, st.StudiedToday)
	}
	st, _ = s.Stats(ctx, cat, day(5, 9))
	if st.Streak != 0 {
		t.Errorf("after a gap: streak = %d, want 0", st.Streak)
	}
}
