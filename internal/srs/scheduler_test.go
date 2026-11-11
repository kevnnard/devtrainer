package srs

import (
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

var t0 = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func TestDeriveGrade(t *testing.T) {
	th := DefaultThresholds
	if th.Fast != 8*time.Second || th.Slow != 45*time.Second {
		t.Fatalf("DefaultThresholds = %+v, want fast 8s, slow 45s", th)
	}
	cases := []struct {
		name    string
		correct bool
		hint    bool
		d       time.Duration
		want    domain.Grade
	}{
		{"wrong", false, false, 20 * time.Second, domain.Again},
		{"wrong fast", false, false, time.Second, domain.Again},
		{"wrong with hint", false, true, time.Second, domain.Again},
		{"correct", true, false, 20 * time.Second, domain.Good},
		{"correct at fast threshold", true, false, 8 * time.Second, domain.Good},
		{"correct fast", true, false, 5 * time.Second, domain.Easy},
		{"correct at slow threshold", true, false, 45 * time.Second, domain.Good},
		{"correct slow", true, false, 46 * time.Second, domain.Hard},
		{"correct with hint", true, true, 20 * time.Second, domain.Hard},
		{"correct fast with hint", true, true, 2 * time.Second, domain.Hard},
	}
	for _, c := range cases {
		if got := DeriveGrade(c.correct, c.hint, c.d, th); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	custom := Thresholds{Fast: 2 * time.Second, Slow: 10 * time.Second}
	if got := DeriveGrade(true, false, 5*time.Second, custom); got != domain.Good {
		t.Errorf("custom thresholds: 5s = %v, want good", got)
	}
	if got := DeriveGrade(true, false, 11*time.Second, custom); got != domain.Hard {
		t.Errorf("custom thresholds: 11s = %v, want hard", got)
	}
}

func review(s Scheduler, sc domain.Schedule, g domain.Grade, now time.Time) domain.Schedule {
	return s.Review(sc, g, "item", now)
}

func TestFirstReviewSchedulesWholeDays(t *testing.T) {
	s := NewScheduler()
	fresh := domain.Schedule{ConceptID: "git.staging"}
	var prev time.Duration
	for _, g := range []domain.Grade{domain.Again, domain.Hard, domain.Good, domain.Easy} {
		got := review(s, fresh, g, t0)
		if got.ConceptID != "git.staging" || got.Reps != 1 || !got.LastReview.Equal(t0) || got.LastItemID != "item" {
			t.Errorf("%v: schedule = %+v", g, got)
		}
		gap := got.Due.Sub(t0)
		if gap < 24*time.Hour {
			t.Errorf("%v: due in %v, want at least a day (concepts are studied daily)", g, gap)
		}
		if gap <= prev {
			t.Errorf("%v: due in %v, want longer than the lower grade's %v", g, gap, prev)
		}
		prev = gap
	}
}

func TestGoodReviewsGrowIntervalsAndStability(t *testing.T) {
	s := NewScheduler()
	sc := domain.Schedule{ConceptID: "c"}
	now := t0
	var lastGap time.Duration
	var lastStab float64
	for i := range 6 {
		sc = review(s, sc, domain.Good, now)
		gap := sc.Due.Sub(now)
		if i > 0 && gap <= lastGap {
			t.Fatalf("review %d: interval %v did not grow from %v", i+1, gap, lastGap)
		}
		if sc.Stability <= lastStab {
			t.Fatalf("review %d: stability %v did not grow from %v", i+1, sc.Stability, lastStab)
		}
		lastGap, lastStab = gap, sc.Stability
		now = sc.Due // review exactly when due
	}
	if sc.Reps != 6 || sc.Lapses != 0 {
		t.Errorf("reps/lapses = %d/%d, want 6/0", sc.Reps, sc.Lapses)
	}
}

func TestAgainAfterReviewIsALapse(t *testing.T) {
	s := NewScheduler()
	sc := review(s, domain.Schedule{ConceptID: "c"}, domain.Good, t0)
	sc = review(s, sc, domain.Good, sc.Due)
	before := sc

	again := review(s, before, domain.Again, before.Due)
	good := review(s, before, domain.Good, before.Due)
	if again.Lapses != before.Lapses+1 {
		t.Errorf("lapses = %d, want %d", again.Lapses, before.Lapses+1)
	}
	if again.Stability >= before.Stability {
		t.Errorf("Again stability %v should drop below %v", again.Stability, before.Stability)
	}
	if again.Difficulty <= good.Difficulty {
		t.Errorf("Again difficulty %v should exceed Good's %v", again.Difficulty, good.Difficulty)
	}
	if !again.Due.Before(good.Due) {
		t.Errorf("Again due %v should come before Good due %v", again.Due, good.Due)
	}
}

func TestDifficultyMonotonicInGrade(t *testing.T) {
	s := NewScheduler()
	base := review(s, domain.Schedule{ConceptID: "c"}, domain.Good, t0)
	at := base.Due
	var prevD = 11.0
	var prevS = 0.0
	for _, g := range []domain.Grade{domain.Again, domain.Hard, domain.Good, domain.Easy} {
		next := review(s, base, g, at)
		if next.Difficulty >= prevD {
			t.Errorf("%v: difficulty %v should be below the lower grade's %v", g, next.Difficulty, prevD)
		}
		if next.Stability <= prevS {
			t.Errorf("%v: stability %v should exceed the lower grade's %v", g, next.Stability, prevS)
		}
		if next.Difficulty < 1 || next.Difficulty > 10 {
			t.Errorf("%v: difficulty %v outside [1,10]", g, next.Difficulty)
		}
		prevD, prevS = next.Difficulty, next.Stability
	}
}

func TestReviewIsDeterministic(t *testing.T) {
	s := NewScheduler()
	a := review(s, domain.Schedule{ConceptID: "c"}, domain.Good, t0)
	b := review(s, domain.Schedule{ConceptID: "c"}, domain.Good, t0)
	if a != b {
		t.Errorf("same input gave %+v and %+v", a, b)
	}
}

func TestRetrievability(t *testing.T) {
	s := NewScheduler()
	if r := s.Retrievability(domain.Schedule{}, t0); r != 0 {
		t.Errorf("never reviewed: %v, want 0", r)
	}
	sc := review(s, domain.Schedule{ConceptID: "c"}, domain.Good, t0)
	soon, later := s.Retrievability(sc, t0.Add(time.Hour)), s.Retrievability(sc, t0.Add(30*24*time.Hour))
	if !(soon > later && soon <= 1 && later > 0) {
		t.Errorf("retrievability soon=%v later=%v, want decreasing in (0,1]", soon, later)
	}
}

func TestDeriveCodeGrade(t *testing.T) {
	th := CodeThresholds
	cases := []struct {
		name   string
		passed bool
		hint   bool
		d      time.Duration
		want   domain.Grade
	}{
		{"fail", false, false, time.Minute, domain.Again},
		{"fail with hint", false, true, time.Minute, domain.Again},
		{"pass", true, false, th.Fast + time.Minute, domain.Good},
		{"pass slowly is still Good", true, false, 20 * time.Minute, domain.Good},
		{"pass fast", true, false, th.Fast - time.Second, domain.Easy},
		{"pass with hint", true, true, th.Fast + time.Minute, domain.Hard},
		{"pass fast with hint", true, true, time.Second, domain.Hard},
	}
	for _, c := range cases {
		if got := DeriveCodeGrade(c.passed, c.hint, c.d, th); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if th.Fast <= DefaultThresholds.Slow {
		t.Errorf("CodeThresholds.Fast = %v: writing code needs more time than a closed question", th.Fast)
	}
}
