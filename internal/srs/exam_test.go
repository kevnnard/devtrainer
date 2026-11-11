package srs

import (
	"testing"

	"github.com/kevnnard/devtrainer/internal/domain"
)

func TestExamAvailableWhenEveryConceptIsLearning(t *testing.T) {
	sec := domain.Section{ID: "a/s", Track: "a", Concepts: []string{"a.1", "a.2", "a.3"}}
	cases := []struct {
		name    string
		states  map[string]domain.State
		ok      bool
		missing []string
	}{
		{"all new", map[string]domain.State{}, false, []string{"a.1", "a.2", "a.3"}},
		{"one still new",
			map[string]domain.State{"a.1": domain.StateLearning, "a.2": domain.StateMastered, "a.3": domain.StateNew},
			false, []string{"a.3"}},
		{"all at least learning",
			map[string]domain.State{"a.1": domain.StateLearning, "a.2": domain.StateReview, "a.3": domain.StateMastered},
			true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExamAvailable(sec, c.states); got != c.ok {
				t.Errorf("ExamAvailable = %v, want %v", got, c.ok)
			}
			got := ExamMissing(sec, c.states)
			if len(got) != len(c.missing) {
				t.Fatalf("ExamMissing = %v, want %v", got, c.missing)
			}
			for i := range got {
				if got[i] != c.missing[i] {
					t.Errorf("ExamMissing = %v, want %v", got, c.missing)
				}
			}
		})
	}
}

func TestPlanExamMixesEveryConcept(t *testing.T) {
	cat := catalog(true, tr{"a", 3}, tr{"b", 1})
	// a.3 has a third item; a.2 only one.
	cat.Items = append(cat.Items, domain.Item{ID: "a.3.c", ConceptID: "a.3"})
	var items []domain.Item
	for _, it := range cat.Items {
		if it.ID != "a.2.b" {
			items = append(items, it)
		}
	}
	cat.Items = items
	sec := domain.Section{ID: "a/s", Track: "a", Concepts: []string{"a.1", "a.2", "a.3"}}
	scheds := map[string]domain.Schedule{"a.1": {ConceptID: "a.1", Reps: 1, LastItemID: "a.1.a"}}

	picks := PlanExam(cat, sec, scheds)
	var got []string
	seen := map[string]int{}
	for _, p := range picks {
		got = append(got, p.Item.ID)
		seen[p.Concept.ID]++
		if p.New {
			t.Errorf("exam pick %s is marked new", p.Item.ID)
		}
		if p.Item.ConceptID != p.Concept.ID {
			t.Errorf("pick %s belongs to %s", p.Item.ID, p.Concept.ID)
		}
	}
	// Round robin over the concepts, up to ExamItemsPerConcept each, each
	// concept starting after the item it showed last.
	want := []string{"a.1.b", "a.2.a", "a.3.a", "a.1.a", "a.3.b"}
	if ExamItemsPerConcept != 2 || len(got) != len(want) {
		t.Fatalf("picks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("picks = %v, want %v", got, want)
		}
	}
	if seen["b.1"] != 0 {
		t.Error("an exam never asks about concepts outside its section")
	}
	// No two consecutive picks share a concept when another order exists.
	for i := 1; i < len(picks)-1; i++ {
		if picks[i].Concept.ID == picks[i-1].Concept.ID {
			t.Errorf("picks %d and %d share concept %s", i-1, i, picks[i].Concept.ID)
		}
	}
}

func TestPlanExamSkipsConceptsWithoutItems(t *testing.T) {
	cat := catalog(false, tr{"a", 2})
	cat.Items = cat.Items[:2] // a.2 has no servable item left
	sec := domain.Section{ID: "a/s", Track: "a", Concepts: []string{"a.1", "a.2"}}
	picks := PlanExam(cat, sec, nil)
	if len(picks) != 2 || picks[0].Concept.ID != "a.1" || picks[1].Concept.ID != "a.1" {
		t.Errorf("picks = %+v", picks)
	}
}
