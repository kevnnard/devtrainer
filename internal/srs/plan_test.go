package srs

import (
	"fmt"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// catalog builds tracks with the given concept counts; every concept gets
// two items. Concept IDs are "<track>.<n>"; within a track concept n
// requires concept n-1 when chained is true.
func catalog(chained bool, tracks ...struct {
	id string
	n  int
}) domain.Catalog {
	var cat domain.Catalog
	for i, tr := range tracks {
		cat.Tracks = append(cat.Tracks, domain.Track{ID: tr.id, Order: i})
		for n := 1; n <= tr.n; n++ {
			id := fmt.Sprintf("%s.%d", tr.id, n)
			c := domain.Concept{ID: id, Track: tr.id, Level: domain.Beginner}
			if chained && n > 1 {
				c.Prereqs = []string{fmt.Sprintf("%s.%d", tr.id, n-1)}
			}
			cat.Concepts = append(cat.Concepts, c)
			cat.Items = append(cat.Items,
				domain.Item{ID: id + ".a", ConceptID: id},
				domain.Item{ID: id + ".b", ConceptID: id})
		}
	}
	return cat
}

type tr = struct {
	id string
	n  int
}

func reviewed(id string, due time.Time, lastItem string) domain.Schedule {
	return domain.Schedule{ConceptID: id, Reps: 1, Due: due, LastReview: due.Add(-24 * time.Hour), LastItemID: lastItem}
}

func ids(picks []Pick) []string {
	out := make([]string, len(picks))
	for i, p := range picks {
		out[i] = p.Concept.ID
	}
	return out
}

func TestPlanDueFirstThenNew(t *testing.T) {
	cat := catalog(false, tr{"go", 3}, tr{"git", 2})
	now := t0
	in := PlanInput{
		Catalog: cat,
		Schedules: map[string]domain.Schedule{
			"go.1":  reviewed("go.1", now.Add(-time.Hour), ""),
			"git.1": reviewed("git.1", now.Add(-2*time.Hour), ""),
			"go.2":  reviewed("go.2", now.Add(time.Hour), ""), // not due yet
		},
		States: map[string]domain.State{"go.1": domain.StateLearning, "git.1": domain.StateLearning, "go.2": domain.StateLearning},
		MaxNew: 3,
	}
	got := PlanSession(in, now)
	// git.1 is the most overdue; the new picks keep alternating tracks after go.1.
	want := []string{"git.1", "go.1", "git.2", "go.3"}
	if fmt.Sprint(ids(got)) != fmt.Sprint(want) {
		t.Fatalf("plan = %v, want %v", ids(got), want)
	}
	for i, p := range got {
		if p.New != (i >= 2) {
			t.Errorf("pick %d (%s): New = %v", i, p.Concept.ID, p.New)
		}
	}
}

func TestPlanLimitsNewConcepts(t *testing.T) {
	cat := catalog(false, tr{"go", 5}, tr{"git", 5})
	for _, maxNew := range []int{0, 1, 3, 20} {
		got := PlanSession(PlanInput{Catalog: cat, MaxNew: maxNew}, t0)
		want := min(maxNew, 10)
		if len(got) != want {
			t.Errorf("MaxNew %d: %d picks, want %d", maxNew, len(got), want)
		}
	}
	// New concepts alternate tracks instead of draining the first one.
	got := PlanSession(PlanInput{Catalog: cat, MaxNew: 4}, t0)
	if fmt.Sprint(ids(got)) != "[go.1 git.1 go.2 git.2]" {
		t.Errorf("new picks = %v, want round robin across tracks", ids(got))
	}
}

func TestPlanInterleavesTracks(t *testing.T) {
	cat := catalog(false, tr{"a", 4}, tr{"b", 2}, tr{"c", 1})
	sched := map[string]domain.Schedule{}
	states := map[string]domain.State{}
	// a.* are the most overdue, so a naive due-order plan would be a a a a b b c.
	for i, id := range []string{"a.1", "a.2", "a.3", "a.4", "b.1", "b.2", "c.1"} {
		sched[id] = reviewed(id, t0.Add(-time.Duration(10-i)*time.Hour), "")
		states[id] = domain.StateLearning
	}
	got := PlanSession(PlanInput{Catalog: cat, Schedules: sched, States: states}, t0)
	if len(got) != 7 {
		t.Fatalf("plan = %v, want all 7 due concepts", ids(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Concept.Track == got[i-1].Concept.Track {
			t.Errorf("plan %v repeats track %s at %d although it is avoidable", ids(got), got[i].Concept.Track, i)
		}
	}
	if got[0].Concept.ID != "a.1" {
		t.Errorf("first pick = %s, want the most overdue a.1", got[0].Concept.ID)
	}
}

func TestPlanUnavoidableRepeats(t *testing.T) {
	cat := catalog(false, tr{"a", 3}, tr{"b", 1})
	sched := map[string]domain.Schedule{}
	states := map[string]domain.State{}
	for _, id := range []string{"a.1", "a.2", "a.3", "b.1"} {
		sched[id] = reviewed(id, t0.Add(-time.Hour), "")
		states[id] = domain.StateLearning
	}
	got := PlanSession(PlanInput{Catalog: cat, Schedules: sched, States: states}, t0)
	if fmt.Sprint(ids(got)) != "[a.1 b.1 a.2 a.3]" {
		t.Errorf("plan = %v, want [a.1 b.1 a.2 a.3]", ids(got))
	}
}

func TestPlanOnlyUnlockedNewConcepts(t *testing.T) {
	cat := catalog(true, tr{"go", 3})
	// go.1 learning → go.2 unlocked; go.3 needs go.2, which is new.
	in := PlanInput{
		Catalog:   cat,
		Schedules: map[string]domain.Schedule{"go.1": reviewed("go.1", t0.Add(48*time.Hour), "")},
		States:    map[string]domain.State{"go.1": domain.StateLearning},
		MaxNew:    3,
	}
	if got := ids(PlanSession(in, t0)); fmt.Sprint(got) != "[go.2]" {
		t.Errorf("plan = %v, want only the unlocked go.2", got)
	}

	// A concept answered only wrongly stays "new" for unlocking but is a
	// scheduled review, never a new pick.
	in.States["go.1"] = domain.StateNew
	in.Schedules["go.1"] = reviewed("go.1", t0.Add(-time.Hour), "")
	if got := ids(PlanSession(in, t0)); fmt.Sprint(got) != "[go.1]" {
		t.Errorf("plan = %v, want only the due go.1", got)
	}
}

func TestPlanRotatesItems(t *testing.T) {
	cat := catalog(false, tr{"go", 2})
	in := PlanInput{
		Catalog: cat,
		Schedules: map[string]domain.Schedule{
			"go.1": reviewed("go.1", t0.Add(-time.Hour), "go.1.a"),
			"go.2": reviewed("go.2", t0.Add(-time.Hour), "go.2.b"),
		},
		States: map[string]domain.State{"go.1": domain.StateLearning, "go.2": domain.StateLearning},
	}
	got := PlanSession(in, t0)
	if len(got) != 2 || got[0].Item.ID != "go.1.b" || got[1].Item.ID != "go.2.a" {
		t.Errorf("items = %+v, want go.1.b and go.2.a (not the last shown)", got)
	}
}

func TestNextItem(t *testing.T) {
	items := []domain.Item{{ID: "x"}, {ID: "y"}, {ID: "z"}}
	cases := map[string]string{"": "x", "x": "y", "y": "z", "z": "x", "gone": "x"}
	for last, want := range cases {
		if got, _ := NextItem(items, last); got.ID != want {
			t.Errorf("NextItem(last=%q) = %s, want %s", last, got.ID, want)
		}
	}
	if got, _ := NextItem(items[:1], "x"); got.ID != "x" {
		t.Errorf("single item must repeat, got %s", got.ID)
	}
	if _, ok := NextItem(nil, ""); ok {
		t.Error("no items must report !ok")
	}
}
