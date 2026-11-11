package domain

import (
	"testing"
	"time"
)

func TestExamPassMarkBoundary(t *testing.T) {
	at := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		correct, total int
		pass           bool
	}{
		{79, 100, false}, // 79% fails
		{80, 100, true},  // 80% passes
		{4, 5, true},     // exactly 80%
		{3, 4, false},    // 75%
		{7, 9, false},    // 77.8%: never rounded up
		{10, 10, true},
		{0, 0, false}, // an empty exam is never passed
	}
	for _, c := range cases {
		r := NewExamResult("go/basics", c.correct, c.total, at)
		if r.Passed != c.pass || ExamPassed(c.correct, c.total) != c.pass {
			t.Errorf("%d/%d: Passed = %v, want %v", c.correct, c.total, r.Passed, c.pass)
		}
		if r.SectionID != "go/basics" || r.Correct != c.correct || r.Total != c.total || !r.At.Equal(at) {
			t.Errorf("NewExamResult = %+v", r)
		}
	}
	if got := NewExamResult("s", 79, 100, at).Percent(); got != 79 {
		t.Errorf("Percent = %d, want 79", got)
	}
	if ExamPassMark != 80 {
		t.Errorf("ExamPassMark = %d, want 80", ExamPassMark)
	}
}

func TestBestExam(t *testing.T) {
	at := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	if _, ok := BestExam(nil); ok {
		t.Error("no attempts, no best")
	}
	tries := []ExamResult{
		NewExamResult("s", 3, 5, at),
		NewExamResult("s", 5, 5, at.Add(time.Hour)),
		NewExamResult("s", 4, 5, at.Add(2*time.Hour)),
	}
	best, ok := BestExam(tries)
	if !ok || best.Correct != 5 || !best.Passed {
		t.Errorf("BestExam = %+v, %v", best, ok)
	}
}
