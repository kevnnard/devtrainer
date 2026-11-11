package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestLessonsSeenRoundTrip(t *testing.T) {
	s := openTemp(t)
	seen, err := s.LessonsSeen(ctx)
	if err != nil || len(seen) != 0 {
		t.Fatalf("LessonsSeen on a new store = %v, %v", seen, err)
	}
	at := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
	if err := s.MarkLessonSeen(ctx, "go.basics", at); err != nil {
		t.Fatal(err)
	}
	// Marking again is a no-op, not an error.
	if err := s.MarkLessonSeen(ctx, "go.basics", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkLessonSeen(ctx, "git.staging", at); err != nil {
		t.Fatal(err)
	}
	seen, err = s.LessonsSeen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !seen["go.basics"] || !seen["git.staging"] || seen["sql.joins"] {
		t.Errorf("LessonsSeen = %v", seen)
	}
	if err := s.MarkLessonSeen(ctx, "", at); err == nil {
		t.Error("MarkLessonSeen with an empty concept ID must fail")
	}
}

// TestMigrateFromVersion1 opens a database written before lessons existed:
// the answers stay and the lessons table is added.
func TestMigrateFromVersion1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{migrations[0], `PRAGMA user_version = 1`,
		`INSERT INTO answers (concept_id, item_id, correct, grade, answered_at) VALUES ('git.staging', 'i', 1, 3, '2026-11-09T10:00:00Z')`} {
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
	if v, _ := s.SchemaVersion(ctx); v != len(migrations) || v < 2 {
		t.Errorf("version = %d, want %d (≥ 2)", v, len(migrations))
	}
	h, err := s.History(ctx)
	if err != nil || len(h["git.staging"]) != 1 {
		t.Errorf("History after migrating = %v, %v", h, err)
	}
	if err := s.MarkLessonSeen(ctx, "git.staging", time.Now()); err != nil {
		t.Fatal(fmt.Errorf("lessons table missing after migrating: %w", err))
	}
}
