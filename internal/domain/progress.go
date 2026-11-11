package domain

import "time"

// Grade is the FSRS rating derived from an answer.
type Grade int

const (
	Again Grade = iota + 1
	Hard
	Good
	Easy
)

func (g Grade) String() string {
	switch g {
	case Again:
		return "again"
	case Hard:
		return "hard"
	case Good:
		return "good"
	case Easy:
		return "easy"
	}
	return "unknown"
}

// State is where a concept stands in the learner's study. States are ordered:
// a later state means more progress.
type State int

const (
	StateNew      State = iota // never answered correctly
	StateLearning              // answered correctly, not yet on two different days
	StateReview                // last answer correct, correct on two or more days
	StateMastered              // 4 of the last 5 answers correct, over two or more days
)

func (s State) String() string {
	switch s {
	case StateNew:
		return "new"
	case StateLearning:
		return "learning"
	case StateReview:
		return "review"
	case StateMastered:
		return "mastered"
	}
	return "unknown"
}

// Answer is one graded attempt at an item.
type Answer struct {
	ConceptID     string
	ItemID        string
	Correct       bool
	Grade         Grade
	Misconception string // of the wrong option chosen, if any
	Duration      time.Duration
	HintUsed      bool
	At            time.Time
}

// Schedule is the FSRS memory state of one concept. The zero value (apart
// from ConceptID) is a concept that was never reviewed.
type Schedule struct {
	ConceptID     string
	Stability     float64
	Difficulty    float64
	ElapsedDays   uint64
	ScheduledDays uint64
	Reps          uint64
	Lapses        uint64
	FSRSState     int // FSRS card state: 0 new, 1 learning, 2 review, 3 relearning
	Due           time.Time
	LastReview    time.Time
	LastItemID    string // item shown last, so the next review can pick another
}

// Reviewed reports whether the concept has been reviewed at least once.
func (s Schedule) Reviewed() bool {
	return s.Reps > 0
}

// dateLayout is how calendar days are written.
const dateLayout = "2006-01-02"

// DateOf returns the calendar day of t in loc as YYYY-MM-DD.
func DateOf(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(dateLayout)
}

// TrackProgress summarizes the concept states of one track.
type TrackProgress struct {
	Track    Track
	Total    int
	New      int
	Learning int
	Review   int
	Mastered int

	Sections       int // sections of the track
	SectionsPassed int // of them, with a passed exam
}

// MasteryPercent is the share of the track's concepts that are mastered, 0–100.
func (p TrackProgress) MasteryPercent() float64 {
	if p.Total == 0 {
		return 0
	}
	return 100 * float64(p.Mastered) / float64(p.Total)
}

// MisconceptionCount is how often a misconception led to a wrong answer.
// Misconception keys are shared across concepts and tracks.
type MisconceptionCount struct {
	Misconception string
	Count         int
}

// ConceptAccuracy is a concept's answer record.
type ConceptAccuracy struct {
	ConceptID string
	Answers   int
	Correct   int
}

// Accuracy is the share of correct answers, 0–1.
func (a ConceptAccuracy) Accuracy() float64 {
	if a.Answers == 0 {
		return 0
	}
	return float64(a.Correct) / float64(a.Answers)
}

// Stats is the learner's overall progress.
type Stats struct {
	Tracks            []TrackProgress
	States            map[string]State // by concept ID
	Streak            int              // consecutive study days ending today or yesterday
	StudiedToday      bool
	TotalAnswers      int
	TotalCorrect      int
	TopMisconceptions []MisconceptionCount
	Weakest           []ConceptAccuracy
	PassedSections    []ExamResult // best passed attempt per section, in catalog order
}

// ExamPassMark is the share of correct answers, in percent, that passes a
// section exam.
const ExamPassMark = 80

// ExamPassed reports whether correct of total reaches ExamPassMark. It
// compares integers, so 79.9% never rounds up to a pass; an empty exam
// never passes.
func ExamPassed(correct, total int) bool {
	return total > 0 && correct >= 0 && 100*correct >= ExamPassMark*total
}

// ExamResult is one finished attempt at a section exam.
type ExamResult struct {
	SectionID string
	Correct   int
	Total     int
	Passed    bool
	At        time.Time
}

// NewExamResult scores an attempt, deriving Passed from the pass mark.
func NewExamResult(sectionID string, correct, total int, at time.Time) ExamResult {
	return ExamResult{SectionID: sectionID, Correct: correct, Total: total, Passed: ExamPassed(correct, total), At: at}
}

// Percent is the score rounded down to a whole percent.
func (r ExamResult) Percent() int {
	if r.Total <= 0 {
		return 0
	}
	return 100 * r.Correct / r.Total
}

// BestExam is the attempt with the highest score (the earliest on ties);
// ok is false when there are none.
func BestExam(attempts []ExamResult) (best ExamResult, ok bool) {
	for _, r := range attempts {
		if !ok || r.Correct*max(best.Total, 1) > best.Correct*max(r.Total, 1) {
			best, ok = r, true
		}
	}
	return best, ok
}
