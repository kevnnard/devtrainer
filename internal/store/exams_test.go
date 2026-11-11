package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

func TestExamResultsRoundTrip(t *testing.T) {
	s := openTemp(t)
	if got, err := s.Exams(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Exams on a new store = %v, %v", got, err)
	}
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.FixedZone("COT", -5*3600))
	results := []domain.ExamResult{
		domain.NewExamResult("go/basics", 7, 9, at.Add(time.Hour)), // 77%: failed
		domain.NewExamResult("go/basics", 8, 10, at.Add(2*time.Hour)),
		domain.NewExamResult("sql/querying", 3, 6, at),
	}
	for _, r := range results {
		if err := s.RecordExam(ctx, r); err != nil {
			t.Fatalf("RecordExam(%+v): %v", r, err)
		}
	}
	got, err := s.Exams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	g := got["go/basics"]
	if len(g) != 2 || g[0].Passed || !g[1].Passed || g[1].Correct != 8 || g[1].Total != 10 || !g[1].At.Equal(at.Add(2*time.Hour)) {
		t.Errorf("go/basics attempts = %+v", g)
	}
	if q := got["sql/querying"]; len(q) != 1 || q[0].Passed || q[0].Correct != 3 {
		t.Errorf("sql/querying attempts = %+v", q)
	}
	if err := s.RecordExam(ctx, domain.ExamResult{Correct: 1, Total: 1}); err == nil {
		t.Error("an exam without a section ID must be rejected")
	}
	if err := s.RecordExam(ctx, domain.ExamResult{SectionID: "x", Correct: 2, Total: 1}); err == nil {
		t.Error("more correct than total must be rejected")
	}
}

// TestRecordExamStoresThePassMark keeps Passed derived from the score, so
// a caller cannot store a failed score as passed.
func TestRecordExamStoresThePassMark(t *testing.T) {
	s := openTemp(t)
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	forged := domain.ExamResult{SectionID: "s", Correct: 79, Total: 100, Passed: true, At: at}
	if err := s.RecordExam(ctx, forged); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExam(ctx, domain.ExamResult{SectionID: "t", Correct: 80, Total: 100, At: at}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Exams(ctx)
	if got["s"][0].Passed {
		t.Error("79/100 was stored as passed")
	}
	if !got["t"][0].Passed {
		t.Error("80/100 was stored as failed")
	}
}

// TestMigrateFromVersion2 opens a database written before exams existed:
// its answers, schedules and lessons stay, and exams can be recorded.
func TestMigrateFromVersion2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{migrations[0], migrations[1], `PRAGMA user_version = 2`,
		`INSERT INTO answers (concept_id, item_id, correct, grade, answered_at) VALUES ('go.basics', 'go.basics.mcq', 1, 3, '2026-11-09T10:00:00Z')`,
		`INSERT INTO answers (concept_id, item_id, correct, grade, answered_at) VALUES ('go.slices', 'go.slices.mcq', 0, 1, '2026-11-10T10:00:00Z')`,
		`INSERT INTO concept_schedules (concept_id, reps, due) VALUES ('go.basics', 1, '2026-11-12T10:00:00Z')`,
		`INSERT INTO lessons_seen (concept_id, seen_at) VALUES ('go.basics', '2026-11-09T09:00:00Z')`,
		`INSERT INTO study_days (date, answers, correct) VALUES ('2026-11-09', 1, 1)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.SchemaVersion(ctx); v != len(migrations) || v < 3 {
		t.Errorf("version = %d, want %d (≥ 3)", v, len(migrations))
	}
	h, err := s.History(ctx)
	if err != nil || len(h["go.basics"]) != 1 || len(h["go.slices"]) != 1 || !h["go.basics"][0].Correct {
		t.Errorf("History after migrating = %v, %v", h, err)
	}
	if sc, err := s.Schedules(ctx); err != nil || sc["go.basics"].Reps != 1 {
		t.Errorf("Schedules after migrating = %v, %v", sc, err)
	}
	if seen, err := s.LessonsSeen(ctx); err != nil || !seen["go.basics"] {
		t.Errorf("LessonsSeen after migrating = %v, %v", seen, err)
	}
	if days, err := s.StudyDays(ctx); err != nil || len(days) != 1 {
		t.Errorf("StudyDays after migrating = %v, %v", days, err)
	}
	if err := s.RecordExam(ctx, domain.NewExamResult("go/basics", 4, 5, time.Now())); err != nil {
		t.Fatalf("exam table missing after migrating: %v", err)
	}
}

func TestStatsCountsPassedSections(t *testing.T) {
	s := openTemp(t)
	cat := domain.Catalog{
		Tracks:   []domain.Track{{ID: "go", Title: "Go"}, {ID: "git", Title: "Git"}},
		Concepts: []domain.Concept{{ID: "go.a", Track: "go"}, {ID: "go.b", Track: "go"}, {ID: "git.a", Track: "git"}},
		Sections: []domain.Section{
			{ID: "go/one", Track: "go", Title: "One", Concepts: []string{"go.a"}},
			{ID: "go/two", Track: "go", Title: "Two", Concepts: []string{"go.b"}},
		},
	}
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	for _, r := range []domain.ExamResult{
		domain.NewExamResult("go/one", 1, 2, at),
		domain.NewExamResult("go/one", 2, 2, at.Add(time.Hour)), // passed on the second try
		domain.NewExamResult("go/two", 3, 4, at),                // 75%: not passed
		domain.NewExamResult("gone/old", 5, 5, at),              // section no longer in the content
	} {
		if err := s.RecordExam(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	st, err := s.Stats(ctx, cat, at.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	g := st.Tracks[0]
	if g.Track.ID != "go" || g.Sections != 2 || g.SectionsPassed != 1 {
		t.Errorf("go sections = %d passed of %d, want 1 of 2", g.SectionsPassed, g.Sections)
	}
	if o := st.Tracks[1]; o.Sections != 0 || o.SectionsPassed != 0 {
		t.Errorf("git has no sections, got %+v", o)
	}
	if len(st.PassedSections) != 1 || st.PassedSections[0].SectionID != "go/one" || st.PassedSections[0].Correct != 2 {
		t.Errorf("PassedSections = %+v, want go/one (best attempt)", st.PassedSections)
	}
}
