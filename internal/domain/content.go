// Package domain holds devtrainer's core types: the content model
// (Track → Concept → Item), learner answers, per-concept schedules and
// progress states. It is pure: no I/O, no UI, no clock.
package domain

import "slices"

// Track is a subject area such as Git, Go or AWS.
type Track struct {
	ID          string
	Title       string
	Description string
	Order       int // display and new-concept order; lower comes first
}

// Level is how advanced a concept is.
type Level string

const (
	Beginner     Level = "beginner"
	Intermediate Level = "intermediate"
	Advanced     Level = "advanced"
)

// Valid reports whether l is one of the known levels.
func (l Level) Valid() bool {
	switch l {
	case Beginner, Intermediate, Advanced:
		return true
	}
	return false
}

// Concept is the unit that is scheduled and mastered. Items are the
// different questions that exercise it.
type Concept struct {
	ID      string
	Track   string
	Title   string
	Summary string
	Prereqs []string // concept IDs that must be at least "learning" first
	Level   Level
	Lesson  *Lesson // optional; shown before the concept's first question
}

// Lesson limits enforced when content loads.
const (
	MaxLessonCodeLines = 15 // lines of a lesson's worked example
	MaxPredictOptions  = 9  // options of a predict step: keys 1–9
)

// Lesson is a short worked example taught before a concept's first
// question: an intro, an example, steps revealed one at a time and a
// one-line takeaway. Nothing in a lesson is graded or scheduled.
type Lesson struct {
	Intro    string
	Example  Example // Code is empty when the lesson has no example
	Steps    []LessonStep
	Takeaway string
}

// Example is a lesson's worked example.
type Example struct {
	Code string
	Lang string // optional: go, python, sql, shell…
}

// LessonStep explains one step of the example. With Predict set, the
// learner first guesses what comes next and only then sees Text.
//
// Error, when set, is a real error message captured by running the
// example: the learner sees it, predicts why the example fails (Predict)
// and only then reads the explanation in Text.
type LessonStep struct {
	Text    string
	Error   string
	Predict *Predict
}

// Predict is a tiny closed check before a step: exactly one option is
// correct. It is never graded or saved.
type Predict struct {
	Prompt  string
	Options []PredictOption
}

// PredictOption is one choice of a predict step; Explanation is optional.
type PredictOption struct {
	Text        string
	Correct     bool
	Explanation string
}

// Answer is the index of the correct option, or -1 when there is none.
func (p Predict) Answer() int {
	for i, o := range p.Options {
		if o.Correct {
			return i
		}
	}
	return -1
}

// ItemType is the kind of closed, auto-graded question.
type ItemType string

const (
	MCQ     ItemType = "mcq"     // single correct option
	Multi   ItemType = "multi"   // one or more correct options; exact set required
	Output  ItemType = "output"  // predict what Code prints: options or typed answer
	Cloze   ItemType = "cloze"   // fill the ___ blanks
	Parsons ItemType = "parsons" // put shuffled lines back in order
	Bug     ItemType = "bug"     // pick the faulty line of Code
	Command ItemType = "command" // type a shell/git/docker/aws command
	Code    ItemType = "code"    // write code that runs; graded by its output
)

// ItemTypes lists every item type in a stable order.
var ItemTypes = []ItemType{MCQ, Multi, Output, Cloze, Parsons, Bug, Command, Code}

// Languages a code item can be written in, in a stable order.
var Languages = []string{"sql", "python", "go", "javascript", "typescript", "java", "csharp", "rust"}

// ValidLanguage reports whether lang is one of Languages.
func ValidLanguage(lang string) bool {
	return slices.Contains(Languages, lang)
}

// SQL is the language whose code items compare result rows instead of
// standard output.
const SQL = "sql"

// Valid reports whether t is a known item type.
func (t ItemType) Valid() bool {
	return slices.Contains(ItemTypes, t)
}

// Option is one choice of an mcq, multi or choice-style output item. A wrong
// option names the misconception that leads to it and explains why it is wrong.
type Option struct {
	Text          string
	Correct       bool
	Misconception string
	Explanation   string
}

// Item is one closed question about a concept.
type Item struct {
	ID            string
	ConceptID     string
	Type          ItemType
	Prompt        string
	Code          string   // optional snippet shown with the prompt
	Options       []Option // mcq, multi, output (choice)
	Answers       []string // accepted variants: cloze, command, output (typed)
	Lines         []string // parsons: lines in the correct order
	BugLine       int      // bug: 1-based line of Code that is faulty
	CaseSensitive bool     // cloze: compare blanks case-sensitively
	Explanation   string
	Hint          string

	// code items
	Language string     // one of Languages
	Starter  string     // the code the editor opens with
	Solution string     // a reference answer; it must pass
	Expected string     // stdout the program must print (not sql)
	Setup    string     // sql: statements run before the learner's query
	Rows     [][]string // sql: the expected result rows, as text
	Ordered  bool       // sql: rows must come in this order
}

// CorrectOptions returns the indices of the correct options.
func (it Item) CorrectOptions() []int {
	var idx []int
	for i, o := range it.Options {
		if o.Correct {
			idx = append(idx, i)
		}
	}
	return idx
}

// Section groups some concepts of one track; it ends with an exam that
// mixes items from all of them (spec S4). A concept is in at most one
// section, and a track may have none.
type Section struct {
	ID       string // unique across all tracks, e.g. "go/basics"
	Track    string
	Title    string
	Concepts []string // concept IDs of Track, in study order
}

// Catalog is all loaded content, in file and declaration order.
type Catalog struct {
	Tracks   []Track
	Concepts []Concept
	Items    []Item
	Sections []Section
}

// Section returns the section with id.
func (c Catalog) Section(id string) (Section, bool) {
	for _, s := range c.Sections {
		if s.ID == id {
			return s, true
		}
	}
	return Section{}, false
}

// SectionsIn returns the sections of a track in declaration order.
func (c Catalog) SectionsIn(track string) []Section {
	var out []Section
	for _, s := range c.Sections {
		if s.Track == track {
			out = append(out, s)
		}
	}
	return out
}

// SectionOf returns the section that holds concept id, if any.
func (c Catalog) SectionOf(conceptID string) (Section, bool) {
	for _, s := range c.Sections {
		if slices.Contains(s.Concepts, conceptID) {
			return s, true
		}
	}
	return Section{}, false
}

// Track returns the track with id.
func (c Catalog) Track(id string) (Track, bool) {
	for _, t := range c.Tracks {
		if t.ID == id {
			return t, true
		}
	}
	return Track{}, false
}

// Concept returns the concept with id.
func (c Catalog) Concept(id string) (Concept, bool) {
	for _, k := range c.Concepts {
		if k.ID == id {
			return k, true
		}
	}
	return Concept{}, false
}

// ConceptsIn returns the concepts of a track in declaration order.
func (c Catalog) ConceptsIn(track string) []Concept {
	var out []Concept
	for _, k := range c.Concepts {
		if k.Track == track {
			out = append(out, k)
		}
	}
	return out
}

// ItemsFor returns the items of a concept in declaration order.
func (c Catalog) ItemsFor(conceptID string) []Item {
	var out []Item
	for _, it := range c.Items {
		if it.ConceptID == conceptID {
			out = append(out, it)
		}
	}
	return out
}

// ItemsByConcept groups all items by concept ID.
func (c Catalog) ItemsByConcept() map[string][]Item {
	out := make(map[string][]Item, len(c.Concepts))
	for _, it := range c.Items {
		out[it.ConceptID] = append(out[it.ConceptID], it)
	}
	return out
}

// ConceptTracks maps every concept ID to its track ID.
func (c Catalog) ConceptTracks() map[string]string {
	out := make(map[string]string, len(c.Concepts))
	for _, k := range c.Concepts {
		out[k.ID] = k.Track
	}
	return out
}
