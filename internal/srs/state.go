package srs

import (
	"slices"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// Mastery rule (spec S8): MasteryCorrect correct answers among the last
// MasteryWindow, given on at least MasteryDays different days.
const (
	MasteryWindow  = 5
	MasteryCorrect = 4
	MasteryDays    = 2
)

// ConceptState derives a concept's state from its answers, oldest first.
// Days are calendar days in loc.
//
//   - new: never answered correctly
//   - mastered: the mastery rule holds
//   - review: last answer correct, and correct on two or more days
//   - learning: anything else that was answered correctly at least once
func ConceptState(history []domain.Answer, loc *time.Location) domain.State {
	if !slices.ContainsFunc(history, func(a domain.Answer) bool { return a.Correct }) {
		return domain.StateNew
	}
	if Mastered(history, loc) {
		return domain.StateMastered
	}
	if history[len(history)-1].Correct && correctDays(history, loc) >= MasteryDays {
		return domain.StateReview
	}
	return domain.StateLearning
}

// Mastered reports whether the last MasteryWindow answers hold at least
// MasteryCorrect correct ones, spread over at least MasteryDays days.
func Mastered(history []domain.Answer, loc *time.Location) bool {
	window := history[max(0, len(history)-MasteryWindow):]
	correct := 0
	for _, a := range window {
		if a.Correct {
			correct++
		}
	}
	return correct >= MasteryCorrect && correctDays(window, loc) >= MasteryDays
}

// correctDays counts the distinct days with a correct answer.
func correctDays(answers []domain.Answer, loc *time.Location) int {
	days := map[string]bool{}
	for _, a := range answers {
		if a.Correct {
			days[domain.DateOf(a.At, loc)] = true
		}
	}
	return len(days)
}

// States derives the state of every concept in ids from history (answers
// by concept, oldest first). Concepts without answers are new.
func States(ids []string, history map[string][]domain.Answer, loc *time.Location) map[string]domain.State {
	out := make(map[string]domain.State, len(ids))
	for _, id := range ids {
		out[id] = ConceptState(history[id], loc)
	}
	return out
}

// Unlocked reports whether every prerequisite of c is at least learning.
// Unknown prerequisites count as locked.
func Unlocked(c domain.Concept, states map[string]domain.State) bool {
	for _, pre := range c.Prereqs {
		if s, ok := states[pre]; !ok || s < domain.StateLearning {
			return false
		}
	}
	return true
}

// Streak counts consecutive study days (YYYY-MM-DD) ending today, or
// ending yesterday when today has no study yet.
func Streak(days []string, today string) int {
	set := map[string]bool{}
	for _, d := range days {
		set[d] = true
	}
	cur, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return 0
	}
	if !set[today] {
		cur = cur.AddDate(0, 0, -1)
	}
	n := 0
	for set[cur.Format(time.DateOnly)] {
		n++
		cur = cur.AddDate(0, 0, -1)
	}
	return n
}
