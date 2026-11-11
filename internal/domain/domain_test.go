package domain

import (
	"testing"
	"time"
)

func TestItemTypeValid(t *testing.T) {
	for _, typ := range ItemTypes {
		if !typ.Valid() {
			t.Errorf("%q should be valid", typ)
		}
	}
	if len(ItemTypes) != 8 {
		t.Errorf("got %d item types, want 8", len(ItemTypes))
	}
	for _, l := range Languages {
		if !ValidLanguage(l) {
			t.Errorf("language %q should be valid", l)
		}
	}
	if ValidLanguage("cobol") || ValidLanguage("") || ValidLanguage("Python") {
		t.Error("unknown languages should be invalid")
	}
	for _, bad := range []ItemType{"", "essay", "MCQ"} {
		if bad.Valid() {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestLevelValid(t *testing.T) {
	for _, l := range []Level{Beginner, Intermediate, Advanced} {
		if !l.Valid() {
			t.Errorf("%q should be valid", l)
		}
	}
	if Level("expert").Valid() || Level("").Valid() {
		t.Error("unknown levels must be invalid")
	}
}

func TestStateOrderAndString(t *testing.T) {
	if !(StateNew < StateLearning && StateLearning < StateReview && StateReview < StateMastered) {
		t.Error("states must be ordered new < learning < review < mastered")
	}
	want := map[State]string{StateNew: "new", StateLearning: "learning", StateReview: "review", StateMastered: "mastered"}
	for s, name := range want {
		if s.String() != name {
			t.Errorf("%d.String() = %q, want %q", s, s.String(), name)
		}
	}
}

func TestGradeString(t *testing.T) {
	want := map[Grade]string{Again: "again", Hard: "hard", Good: "good", Easy: "easy", Grade(0): "unknown"}
	for g, name := range want {
		if g.String() != name {
			t.Errorf("Grade(%d).String() = %q, want %q", g, g.String(), name)
		}
	}
}

func TestCatalogLookups(t *testing.T) {
	cat := Catalog{
		Tracks: []Track{{ID: "git", Title: "Git"}, {ID: "go", Title: "Go"}},
		Concepts: []Concept{
			{ID: "git.commit", Track: "git", Title: "Commits"},
			{ID: "go.vars", Track: "go", Title: "Variables"},
			{ID: "git.branch", Track: "git", Title: "Branches", Prereqs: []string{"git.commit"}},
		},
		Items: []Item{
			{ID: "git.commit.1", ConceptID: "git.commit", Type: MCQ},
			{ID: "go.vars.1", ConceptID: "go.vars", Type: Cloze},
			{ID: "git.commit.2", ConceptID: "git.commit", Type: Command},
		},
	}

	if c, ok := cat.Concept("go.vars"); !ok || c.Title != "Variables" {
		t.Errorf("Concept(go.vars) = %+v, %v", c, ok)
	}
	if _, ok := cat.Concept("nope"); ok {
		t.Error("Concept(nope) should not be found")
	}
	if tr, ok := cat.Track("go"); !ok || tr.Title != "Go" {
		t.Errorf("Track(go) = %+v, %v", tr, ok)
	}
	items := cat.ItemsFor("git.commit")
	if len(items) != 2 || items[0].ID != "git.commit.1" || items[1].ID != "git.commit.2" {
		t.Errorf("ItemsFor(git.commit) = %+v", items)
	}
	if got := cat.ConceptsIn("git"); len(got) != 2 || got[0].ID != "git.commit" || got[1].ID != "git.branch" {
		t.Errorf("ConceptsIn(git) = %+v", got)
	}
	tracks := cat.ConceptTracks()
	if tracks["git.branch"] != "git" || tracks["go.vars"] != "go" || len(tracks) != 3 {
		t.Errorf("ConceptTracks = %v", tracks)
	}
}

func TestItemCorrectOptions(t *testing.T) {
	it := Item{Options: []Option{{Text: "a"}, {Text: "b", Correct: true}, {Text: "c", Correct: true}}}
	got := it.CorrectOptions()
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("CorrectOptions = %v, want [1 2]", got)
	}
}

func TestDateOf(t *testing.T) {
	bogota := time.FixedZone("COT", -5*3600)
	// 02:00 UTC on the 11th is still the 10th in Bogotá.
	at := time.Date(2026, 11, 11, 2, 0, 0, 0, time.UTC)
	if got := DateOf(at, bogota); got != "2026-11-10" {
		t.Errorf("DateOf = %q, want 2026-11-10", got)
	}
	if got := DateOf(at, time.UTC); got != "2026-11-11" {
		t.Errorf("DateOf UTC = %q, want 2026-11-11", got)
	}
}
