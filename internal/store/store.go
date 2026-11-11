// Package store persists schedules, answers and study days in SQLite using
// the pure-Go modernc.org/sqlite driver.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// timeLayout is how timestamps are stored as TEXT. The offset is kept so
// study days stay in the learner's time zone.
const timeLayout = time.RFC3339Nano

// migrations run in order; the database remembers how many it has applied
// in PRAGMA user_version. Only ever append to this list.
var migrations = []string{
	`CREATE TABLE concept_schedules (
		concept_id      TEXT    PRIMARY KEY,
		stability       REAL    NOT NULL DEFAULT 0,
		difficulty      REAL    NOT NULL DEFAULT 0,
		elapsed_days    INTEGER NOT NULL DEFAULT 0,
		scheduled_days  INTEGER NOT NULL DEFAULT 0,
		reps            INTEGER NOT NULL DEFAULT 0,
		lapses          INTEGER NOT NULL DEFAULT 0,
		fsrs_state      INTEGER NOT NULL DEFAULT 0,
		due             TEXT    NOT NULL DEFAULT '',
		last_review     TEXT    NOT NULL DEFAULT '',
		last_item_id    TEXT    NOT NULL DEFAULT ''
	);
	CREATE TABLE answers (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		concept_id     TEXT    NOT NULL,
		item_id        TEXT    NOT NULL,
		correct        INTEGER NOT NULL,
		grade          INTEGER NOT NULL,
		misconception  TEXT    NOT NULL DEFAULT '',
		duration_ms    INTEGER NOT NULL DEFAULT 0,
		hint_used      INTEGER NOT NULL DEFAULT 0,
		answered_at    TEXT    NOT NULL
	);
	CREATE INDEX answers_concept ON answers (concept_id, answered_at);
	CREATE TABLE study_days (
		date     TEXT    PRIMARY KEY,
		answers  INTEGER NOT NULL DEFAULT 0,
		correct  INTEGER NOT NULL DEFAULT 0
	);`,
	// 2: lessons the learner has gone through, so they are not forced again.
	`CREATE TABLE lessons_seen (
		concept_id  TEXT PRIMARY KEY,
		seen_at     TEXT NOT NULL
	);`,
	// 3: section exam attempts (spec S4); the answers themselves live in answers.
	`CREATE TABLE exam_results (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		section_id  TEXT    NOT NULL,
		correct     INTEGER NOT NULL,
		total       INTEGER NOT NULL,
		passed      INTEGER NOT NULL,
		taken_at    TEXT    NOT NULL
	);
	CREATE INDEX exam_results_section ON exam_results (section_id, taken_at);`,
}

// Store is the SQLite-backed progress repository.
type Store struct {
	db *sql.DB
}

// Day is one calendar day with study activity.
type Day struct {
	Date    string // YYYY-MM-DD in the learner's time zone
	Answers int
	Correct int
}

// DefaultPath is $XDG_DATA_HOME/devtrainer/devtrainer.db, falling back to
// ~/.local/share/devtrainer/devtrainer.db.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find data directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "devtrainer", "devtrainer.db"), nil
}

// Open opens (or creates, with its directory) the database at path and
// applies pending migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection keeps SQLite simple: no "database is locked" between our own queries.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// SchemaVersion is the number of migrations applied.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v)
	return v, err
}

func (s *Store) migrate() error {
	version, err := s.SchemaVersion(context.Background())
	if err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept placeholders; i+1 is our own integer.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// RecordReview atomically logs an answer, replaces the concept's schedule
// and counts the answer on its study day (the date of a.At in a.At's zone).
func (s *Store) RecordReview(ctx context.Context, a domain.Answer, sc domain.Schedule) error {
	if a.ConceptID == "" || sc.ConceptID != a.ConceptID {
		return fmt.Errorf("record review: answer concept %q and schedule concept %q must match", a.ConceptID, sc.ConceptID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	_, err = tx.ExecContext(ctx,
		`INSERT INTO answers (concept_id, item_id, correct, grade, misconception, duration_ms, hint_used, answered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ConceptID, a.ItemID, boolInt(a.Correct), int(a.Grade), a.Misconception,
		a.Duration.Milliseconds(), boolInt(a.HintUsed), a.At.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("insert answer: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO concept_schedules (concept_id, stability, difficulty, elapsed_days, scheduled_days,
			reps, lapses, fsrs_state, due, last_review, last_item_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (concept_id) DO UPDATE SET
			stability = excluded.stability, difficulty = excluded.difficulty,
			elapsed_days = excluded.elapsed_days, scheduled_days = excluded.scheduled_days,
			reps = excluded.reps, lapses = excluded.lapses, fsrs_state = excluded.fsrs_state,
			due = excluded.due, last_review = excluded.last_review, last_item_id = excluded.last_item_id`,
		sc.ConceptID, sc.Stability, sc.Difficulty, int64(sc.ElapsedDays), int64(sc.ScheduledDays),
		int64(sc.Reps), int64(sc.Lapses), sc.FSRSState, formatTime(sc.Due), formatTime(sc.LastReview), sc.LastItemID)
	if err != nil {
		return fmt.Errorf("save schedule %s: %w", sc.ConceptID, err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO study_days (date, answers, correct) VALUES (?, 1, ?)
		ON CONFLICT (date) DO UPDATE SET answers = answers + 1, correct = correct + excluded.correct`,
		a.At.Format(time.DateOnly), boolInt(a.Correct))
	if err != nil {
		return fmt.Errorf("count study day: %w", err)
	}
	return tx.Commit()
}

// MarkLessonSeen remembers that the lesson of conceptID was gone through
// (finished or skipped) at at. Marking it again keeps the first time.
func (s *Store) MarkLessonSeen(ctx context.Context, conceptID string, at time.Time) error {
	if conceptID == "" {
		return fmt.Errorf("mark lesson seen: empty concept ID")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO lessons_seen (concept_id, seen_at) VALUES (?, ?) ON CONFLICT (concept_id) DO NOTHING`,
		conceptID, at.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("mark lesson %s seen: %w", conceptID, err)
	}
	return nil
}

// LessonsSeen returns the concept IDs whose lesson was gone through.
func (s *Store) LessonsSeen(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT concept_id FROM lessons_seen`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// Schedules returns every stored concept schedule by concept ID.
func (s *Store) Schedules(ctx context.Context) (map[string]domain.Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT concept_id, stability, difficulty, elapsed_days, scheduled_days, reps, lapses,
			fsrs_state, due, last_review, last_item_id
		FROM concept_schedules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]domain.Schedule{}
	for rows.Next() {
		var sc domain.Schedule
		var elapsed, scheduled, reps, lapses int64
		var due, last string
		if err := rows.Scan(&sc.ConceptID, &sc.Stability, &sc.Difficulty, &elapsed, &scheduled,
			&reps, &lapses, &sc.FSRSState, &due, &last, &sc.LastItemID); err != nil {
			return nil, err
		}
		sc.ElapsedDays, sc.ScheduledDays, sc.Reps, sc.Lapses = uint64(elapsed), uint64(scheduled), uint64(reps), uint64(lapses)
		if sc.Due, err = parseTime(due); err != nil {
			return nil, fmt.Errorf("schedule %s due: %w", sc.ConceptID, err)
		}
		if sc.LastReview, err = parseTime(last); err != nil {
			return nil, fmt.Errorf("schedule %s last review: %w", sc.ConceptID, err)
		}
		out[sc.ConceptID] = sc
	}
	return out, rows.Err()
}

const answerColumns = `concept_id, item_id, correct, grade, misconception, duration_ms, hint_used, answered_at`

// History returns every answer grouped by concept, oldest first.
func (s *Store) History(ctx context.Context) (map[string][]domain.Answer, error) {
	answers, err := s.queryAnswers(ctx, `SELECT `+answerColumns+` FROM answers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	// answered_at strings may carry different offsets, so order by instant here.
	sortByTime(answers)
	out := map[string][]domain.Answer{}
	for _, a := range answers {
		out[a.ConceptID] = append(out[a.ConceptID], a)
	}
	return out, nil
}

// RecentAnswers returns the latest limit answers, newest first.
func (s *Store) RecentAnswers(ctx context.Context, limit int) ([]domain.Answer, error) {
	answers, err := s.queryAnswers(ctx, `SELECT `+answerColumns+` FROM answers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	sortByTime(answers)
	out := make([]domain.Answer, 0, min(limit, len(answers)))
	for i := len(answers) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, answers[i])
	}
	return out, nil
}

func (s *Store) queryAnswers(ctx context.Context, query string, args ...any) ([]domain.Answer, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Answer
	for rows.Next() {
		var a domain.Answer
		var correct, hint int
		var grade int
		var ms int64
		var at string
		if err := rows.Scan(&a.ConceptID, &a.ItemID, &correct, &grade, &a.Misconception, &ms, &hint, &at); err != nil {
			return nil, err
		}
		a.Correct, a.HintUsed, a.Grade = correct != 0, hint != 0, domain.Grade(grade)
		a.Duration = time.Duration(ms) * time.Millisecond
		if a.At, err = time.Parse(timeLayout, at); err != nil {
			return nil, fmt.Errorf("answer time %q: %w", at, err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// StudyDays returns every day with answers, oldest first.
func (s *Store) StudyDays(ctx context.Context) ([]Day, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date, answers, correct FROM study_days ORDER BY date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Day
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.Date, &d.Answers, &d.Correct); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(timeLayout, s)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
