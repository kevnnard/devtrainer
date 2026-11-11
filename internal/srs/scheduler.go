// Package srs schedules concepts with FSRS, derives grades from answers,
// tracks mastery and plans study sessions. It is pure: time is always
// passed in and nothing touches I/O.
package srs

import (
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// Thresholds turn answer time into Easy or Hard.
type Thresholds struct {
	Fast time.Duration // correct answers quicker than this are Easy
	Slow time.Duration // correct answers slower than this are Hard
}

// DefaultThresholds suit short closed questions.
var DefaultThresholds = Thresholds{Fast: 8 * time.Second, Slow: 45 * time.Second}

// CodeThresholds suit code exercises: only Fast is used, since writing a
// program takes as long as it takes.
var CodeThresholds = Thresholds{Fast: 90 * time.Second}

// DeriveCodeGrade grades a code exercise: a failing program is Again; a
// passing one is Hard with a hint, Easy when quicker than th.Fast and
// Good otherwise. Slowness alone never makes it Hard.
func DeriveCodeGrade(passed, hintUsed bool, d time.Duration, th Thresholds) domain.Grade {
	switch {
	case !passed:
		return domain.Again
	case hintUsed:
		return domain.Hard
	case d < th.Fast:
		return domain.Easy
	}
	return domain.Good
}

// DeriveGrade turns an answer into an FSRS grade: wrong is Again; correct
// with a hint or slower than th.Slow is Hard; correct quicker than th.Fast
// is Easy; any other correct answer is Good.
func DeriveGrade(correct, hintUsed bool, d time.Duration, th Thresholds) domain.Grade {
	switch {
	case !correct:
		return domain.Again
	case hintUsed || d > th.Slow:
		return domain.Hard
	case d < th.Fast:
		return domain.Easy
	default:
		return domain.Good
	}
}

// DefaultRetention is the recall probability FSRS aims for at each review.
const DefaultRetention = 0.9

// Scheduler applies FSRS (default weights) to concept schedules. It uses
// the long-term scheduler — intervals are whole days, matching one study
// session a day — and no fuzz, so results are deterministic.
type Scheduler struct {
	params fsrs.Parameters
}

// NewScheduler returns a scheduler with the default FSRS weights and
// DefaultRetention.
func NewScheduler() Scheduler {
	return NewSchedulerWithRetention(DefaultRetention)
}

// NewSchedulerWithRetention returns a scheduler aiming at the given recall
// probability (0 < r < 1).
func NewSchedulerWithRetention(r float64) Scheduler {
	p := fsrs.DefaultParam()
	p.RequestRetention = r
	p.EnableShortTerm = false
	p.EnableFuzz = false
	return Scheduler{params: p}
}

// Review returns the schedule after grading a review of itemID at now.
func (s Scheduler) Review(sc domain.Schedule, g domain.Grade, itemID string, now time.Time) domain.Schedule {
	f := fsrs.NewFSRS(s.params) // per call: the library keeps scratch state in its parameters
	next := f.Next(toCard(sc), now, fsrs.Rating(g)).Card
	out := fromCard(sc.ConceptID, next)
	out.LastItemID = itemID
	return out
}

// Retrievability is the predicted probability of recalling the concept at
// now; 0 for a concept never reviewed.
func (s Scheduler) Retrievability(sc domain.Schedule, now time.Time) float64 {
	if !sc.Reviewed() {
		return 0
	}
	return fsrs.NewFSRS(s.params).GetRetrievability(toCard(sc), now)
}

func toCard(sc domain.Schedule) fsrs.Card {
	return fsrs.Card{
		Due:           sc.Due,
		Stability:     sc.Stability,
		Difficulty:    sc.Difficulty,
		ElapsedDays:   sc.ElapsedDays,
		ScheduledDays: sc.ScheduledDays,
		Reps:          sc.Reps,
		Lapses:        sc.Lapses,
		State:         fsrs.State(sc.FSRSState),
		LastReview:    sc.LastReview,
	}
}

func fromCard(conceptID string, c fsrs.Card) domain.Schedule {
	return domain.Schedule{
		ConceptID:     conceptID,
		Stability:     c.Stability,
		Difficulty:    c.Difficulty,
		ElapsedDays:   c.ElapsedDays,
		ScheduledDays: c.ScheduledDays,
		Reps:          c.Reps,
		Lapses:        c.Lapses,
		FSRSState:     int(c.State),
		Due:           c.Due,
		LastReview:    c.LastReview,
	}
}
