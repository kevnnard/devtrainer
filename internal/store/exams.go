package store

import (
	"context"
	"fmt"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// RecordExam saves one finished section exam attempt. Passed is derived
// again from the score, so only a score at or above the pass mark is
// stored as passed.
func (s *Store) RecordExam(ctx context.Context, r domain.ExamResult) error {
	switch {
	case r.SectionID == "":
		return fmt.Errorf("record exam: empty section ID")
	case r.Total <= 0 || r.Correct < 0 || r.Correct > r.Total:
		return fmt.Errorf("record exam %s: score %d of %d is not valid", r.SectionID, r.Correct, r.Total)
	}
	r = domain.NewExamResult(r.SectionID, r.Correct, r.Total, r.At)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO exam_results (section_id, correct, total, passed, taken_at) VALUES (?, ?, ?, ?, ?)`,
		r.SectionID, r.Correct, r.Total, boolInt(r.Passed), r.At.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("record exam %s: %w", r.SectionID, err)
	}
	return nil
}

// Exams returns every exam attempt grouped by section ID, oldest first.
func (s *Store) Exams(ctx context.Context) (map[string][]domain.ExamResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT section_id, correct, total, passed, taken_at FROM exam_results ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.ExamResult{}
	for rows.Next() {
		var r domain.ExamResult
		var passed int
		var at string
		if err := rows.Scan(&r.SectionID, &r.Correct, &r.Total, &passed, &at); err != nil {
			return nil, err
		}
		r.Passed = passed != 0
		if r.At, err = time.Parse(timeLayout, at); err != nil {
			return nil, fmt.Errorf("exam time %q: %w", at, err)
		}
		out[r.SectionID] = append(out[r.SectionID], r)
	}
	return out, rows.Err()
}

// passedSections fills the section counts of every track of st and its
// best passed attempt per section of cat, in catalog order.
func (s *Store) passedSections(ctx context.Context, cat domain.Catalog, st *domain.Stats) error {
	exams, err := s.Exams(ctx)
	if err != nil {
		return err
	}
	byTrack := map[string]*domain.TrackProgress{}
	for i := range st.Tracks {
		byTrack[st.Tracks[i].Track.ID] = &st.Tracks[i]
	}
	for _, sec := range cat.Sections {
		p := byTrack[sec.Track]
		if p == nil {
			continue
		}
		p.Sections++
		var passed []domain.ExamResult
		for _, r := range exams[sec.ID] {
			if r.Passed {
				passed = append(passed, r)
			}
		}
		if best, ok := domain.BestExam(passed); ok {
			p.SectionsPassed++
			st.PassedSections = append(st.PassedSections, best)
		}
	}
	return nil
}
