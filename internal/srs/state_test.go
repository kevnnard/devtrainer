package srs

import (
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// day returns 10:00 UTC on day n after t0's date.
func day(n int) time.Time {
	return time.Date(2026, 11, 10+n, 10, 0, 0, 0, time.UTC)
}

func ans(correct bool, at time.Time) domain.Answer {
	return domain.Answer{ConceptID: "c", Correct: correct, At: at}
}

func TestConceptState(t *testing.T) {
	utc := time.UTC
	cases := []struct {
		name    string
		history []domain.Answer
		want    domain.State
	}{
		{"never answered", nil, domain.StateNew},
		{"only wrong", []domain.Answer{ans(false, day(0)), ans(false, day(1))}, domain.StateNew},
		{"one correct", []domain.Answer{ans(true, day(0))}, domain.StateLearning},
		{"correct twice same day", []domain.Answer{ans(true, day(0)), ans(true, day(0).Add(time.Hour))}, domain.StateLearning},
		{"correct on two days", []domain.Answer{ans(true, day(0)), ans(true, day(1))}, domain.StateReview},
		{"lapse after review", []domain.Answer{ans(true, day(0)), ans(true, day(1)), ans(false, day(3))}, domain.StateLearning},
		{"4 correct over 2 days", []domain.Answer{ans(true, day(0)), ans(true, day(0)), ans(true, day(1)), ans(true, day(1))}, domain.StateMastered},
		{"4 of last 5 over 3 days", []domain.Answer{
			ans(false, day(0)), ans(false, day(0)),
			ans(true, day(1)), ans(true, day(2)), ans(false, day(3)), ans(true, day(4)), ans(true, day(5)),
		}, domain.StateMastered},
		{"3 of last 5", []domain.Answer{
			ans(true, day(0)), ans(true, day(1)), ans(true, day(2)), ans(true, day(3)),
			ans(false, day(4)), ans(false, day(5)), ans(true, day(6)),
		}, domain.StateReview},
		{"5 correct in one day", []domain.Answer{
			ans(true, day(0)), ans(true, day(0)), ans(true, day(0)), ans(true, day(0)), ans(true, day(0)),
		}, domain.StateLearning},
		{"mastered even if the last one is wrong", []domain.Answer{
			ans(true, day(0)), ans(true, day(1)), ans(true, day(2)), ans(true, day(3)), ans(false, day(4)),
		}, domain.StateMastered},
	}
	for _, c := range cases {
		if got := ConceptState(c.history, utc); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDaysUseTheLearnersTimeZone(t *testing.T) {
	bogota := time.FixedZone("COT", -5*3600)
	// 22:00 and 23:30 in Bogotá on the same day are on two different UTC days.
	h := []domain.Answer{
		ans(true, time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)),
		ans(true, time.Date(2026, 11, 11, 4, 30, 0, 0, time.UTC)),
		ans(true, time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)),
		ans(true, time.Date(2026, 11, 10, 16, 0, 0, 0, time.UTC)),
	}
	// Sort chronologically as the store does.
	h = []domain.Answer{h[2], h[3], h[0], h[1]}
	if got := ConceptState(h, bogota); got != domain.StateLearning {
		t.Errorf("Bogotá: %v, want learning (all on Nov 10 local)", got)
	}
	if got := ConceptState(h, time.UTC); got != domain.StateMastered {
		t.Errorf("UTC: %v, want mastered (Nov 10 and Nov 11)", got)
	}
}

func TestUnlocked(t *testing.T) {
	states := map[string]domain.State{"a": domain.StateLearning, "b": domain.StateNew, "m": domain.StateMastered}
	cases := []struct {
		prereqs []string
		want    bool
	}{
		{nil, true},
		{[]string{"a"}, true},
		{[]string{"a", "m"}, true},
		{[]string{"b"}, false},
		{[]string{"a", "b"}, false},
		{[]string{"missing"}, false},
	}
	for _, c := range cases {
		if got := Unlocked(domain.Concept{ID: "x", Prereqs: c.prereqs}, states); got != c.want {
			t.Errorf("Unlocked(%v) = %v, want %v", c.prereqs, got, c.want)
		}
	}
}

func TestStreak(t *testing.T) {
	cases := []struct {
		name  string
		days  []string
		today string
		want  int
	}{
		{"none", nil, "2026-11-10", 0},
		{"today only", []string{"2026-11-10"}, "2026-11-10", 1},
		{"through today", []string{"2026-11-07", "2026-11-08", "2026-11-09", "2026-11-10"}, "2026-11-10", 4},
		{"ends yesterday still counts", []string{"2026-11-08", "2026-11-09"}, "2026-11-10", 2},
		{"broken", []string{"2026-11-06", "2026-11-08"}, "2026-11-10", 0},
		{"gap inside", []string{"2026-11-05", "2026-11-06", "2026-11-09", "2026-11-10"}, "2026-11-10", 2},
		{"unsorted and month boundary", []string{"2026-11-01", "2026-10-31", "2026-10-30"}, "2026-11-01", 3},
	}
	for _, c := range cases {
		if got := Streak(c.days, c.today); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
