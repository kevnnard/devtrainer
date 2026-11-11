package store

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// Stats limits for the progress screen.
const (
	TopMisconceptionsLimit = 5
	WeakestLimit           = 5
)

// TopMisconceptions returns the misconceptions behind the most wrong
// answers, most frequent first (ties by name).
func (s *Store) TopMisconceptions(ctx context.Context, limit int) ([]domain.MisconceptionCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT misconception, COUNT(*) AS n FROM answers
		WHERE correct = 0 AND misconception <> ''
		GROUP BY misconception ORDER BY n DESC, misconception LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MisconceptionCount
	for rows.Next() {
		var m domain.MisconceptionCount
		if err := rows.Scan(&m.Misconception, &m.Count); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Weakest returns the concepts with at least one wrong answer, lowest
// accuracy first (then most answered, then ID).
func (s *Store) Weakest(ctx context.Context, limit int) ([]domain.ConceptAccuracy, error) {
	all, err := s.accuracy(ctx)
	if err != nil {
		return nil, err
	}
	return weakest(all, limit), nil
}

func (s *Store) accuracy(ctx context.Context) ([]domain.ConceptAccuracy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT concept_id, COUNT(*), SUM(correct) FROM answers GROUP BY concept_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ConceptAccuracy
	for rows.Next() {
		var a domain.ConceptAccuracy
		if err := rows.Scan(&a.ConceptID, &a.Answers, &a.Correct); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func weakest(all []domain.ConceptAccuracy, limit int) []domain.ConceptAccuracy {
	var out []domain.ConceptAccuracy
	for _, a := range all {
		if a.Correct < a.Answers {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b domain.ConceptAccuracy) int {
		return cmp.Or(
			cmp.Compare(a.Accuracy(), b.Accuracy()),
			cmp.Compare(b.Answers, a.Answers),
			cmp.Compare(a.ConceptID, b.ConceptID))
	})
	return out[:min(limit, len(out))]
}

// Stats computes the learner's progress over the concepts of cat (spec
// S9). Days, the streak and states use now's time zone. Answers for
// concepts no longer in the content are left out of every figure except
// the study days behind the streak and the misconception counts.
func (s *Store) Stats(ctx context.Context, cat domain.Catalog, now time.Time) (domain.Stats, error) {
	var st domain.Stats
	loc := now.Location()

	history, err := s.History(ctx)
	if err != nil {
		return st, err
	}
	ids := make([]string, len(cat.Concepts))
	inCatalog := map[string]bool{}
	for i, c := range cat.Concepts {
		ids[i] = c.ID
		inCatalog[c.ID] = true
	}
	st.States = srs.States(ids, history, loc)

	byTrack := map[string]*domain.TrackProgress{}
	for _, t := range cat.Tracks {
		st.Tracks = append(st.Tracks, domain.TrackProgress{Track: t})
	}
	for i := range st.Tracks {
		byTrack[st.Tracks[i].Track.ID] = &st.Tracks[i]
	}
	for _, c := range cat.Concepts {
		p := byTrack[c.Track]
		if p == nil {
			continue
		}
		p.Total++
		switch st.States[c.ID] {
		case domain.StateNew:
			p.New++
		case domain.StateLearning:
			p.Learning++
		case domain.StateReview:
			p.Review++
		case domain.StateMastered:
			p.Mastered++
		}
	}

	for id, answers := range history {
		if !inCatalog[id] {
			continue
		}
		for _, a := range answers {
			st.TotalAnswers++
			if a.Correct {
				st.TotalCorrect++
			}
		}
	}

	days, err := s.StudyDays(ctx)
	if err != nil {
		return st, err
	}
	dates := make([]string, len(days))
	for i, d := range days {
		dates[i] = d.Date
	}
	today := domain.DateOf(now, loc)
	st.Streak = srs.Streak(dates, today)
	st.StudiedToday = slices.Contains(dates, today)

	if st.TopMisconceptions, err = s.TopMisconceptions(ctx, TopMisconceptionsLimit); err != nil {
		return st, err
	}
	all, err := s.accuracy(ctx)
	if err != nil {
		return st, err
	}
	all = slices.DeleteFunc(all, func(a domain.ConceptAccuracy) bool { return !inCatalog[a.ConceptID] })
	st.Weakest = weakest(all, WeakestLimit)
	if err := s.passedSections(ctx, cat, &st); err != nil {
		return st, err
	}
	return st, nil
}

func sortByTime(answers []domain.Answer) {
	slices.SortStableFunc(answers, func(a, b domain.Answer) int { return a.At.Compare(b.At) })
}
