package srs

import (
	"cmp"
	"slices"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// DefaultMaxNew is how many new concepts a session introduces by default.
const DefaultMaxNew = 3

// PlanInput is everything the planner needs.
type PlanInput struct {
	Catalog   domain.Catalog
	Schedules map[string]domain.Schedule // by concept ID; missing = never reviewed
	States    map[string]domain.State    // by concept ID; missing = new
	MaxNew    int
}

// Pick is one question of a session.
type Pick struct {
	Concept domain.Concept
	Item    domain.Item
	New     bool // first time this concept is studied
}

// PlanSession plans one study session (spec S8):
//
//  1. every reviewed concept due at now, most overdue first;
//  2. then up to MaxNew never-reviewed concepts whose prerequisites are at
//     least learning, taken round robin across tracks in catalog order.
//
// Tracks are interleaved: no two consecutive picks share a track when
// another order avoids it. Each pick shows the concept's next item after the
// one shown last time.
func PlanSession(in PlanInput, now time.Time) []Pick {
	items := in.Catalog.ItemsByConcept()
	pos := map[string]int{}
	for i, c := range in.Catalog.Concepts {
		pos[c.ID] = i
	}

	var due []domain.Concept
	for _, c := range in.Catalog.Concepts {
		if sc, ok := in.Schedules[c.ID]; ok && sc.Reviewed() && !sc.Due.After(now) {
			due = append(due, c)
		}
	}
	slices.SortStableFunc(due, func(a, b domain.Concept) int {
		return cmp.Or(in.Schedules[a.ID].Due.Compare(in.Schedules[b.ID].Due), cmp.Compare(pos[a.ID], pos[b.ID]))
	})

	fresh := newConcepts(in)

	var picks []Pick
	last := ""
	add := func(cs []domain.Concept, isNew bool) {
		for _, c := range interleave(cs, last) {
			it, ok := NextItem(items[c.ID], in.Schedules[c.ID].LastItemID)
			if !ok {
				continue
			}
			picks = append(picks, Pick{Concept: c, Item: it, New: isNew})
			last = c.Track
		}
	}
	add(due, false)
	add(fresh, true)
	return picks
}

// newConcepts picks up to in.MaxNew unlocked, never-reviewed concepts,
// round robin across tracks so a session does not drain one track first.
func newConcepts(in PlanInput) []domain.Concept {
	if in.MaxNew <= 0 {
		return nil
	}
	queues := map[string][]domain.Concept{}
	for _, c := range in.Catalog.Concepts {
		if sc, ok := in.Schedules[c.ID]; ok && sc.Reviewed() {
			continue
		}
		if Unlocked(c, in.States) {
			queues[c.Track] = append(queues[c.Track], c)
		}
	}
	var out []domain.Concept
	for len(out) < in.MaxNew {
		took := false
		for _, t := range in.Catalog.Tracks {
			if q := queues[t.ID]; len(q) > 0 && len(out) < in.MaxNew {
				out = append(out, q[0])
				queues[t.ID] = q[1:]
				took = true
			}
		}
		if !took {
			break
		}
	}
	return out
}

// interleave reorders cs so that no two neighbours share a track whenever
// that is possible, otherwise keeping the given priority order. last is the
// track of the pick before cs.
func interleave(cs []domain.Concept, last string) []domain.Concept {
	rest := slices.Clone(cs)
	out := make([]domain.Concept, 0, len(cs))
	for len(rest) > 0 {
		i := choose(rest, last)
		out = append(out, rest[i])
		last = rest[i].Track
		rest = slices.Delete(rest, i, i+1)
	}
	return out
}

// choose returns the index of the next concept: the first one off the last
// track, unless one track has so many left that it must go now to avoid a
// forced repeat later.
func choose(rest []domain.Concept, last string) int {
	count := map[string]int{}
	for _, c := range rest {
		count[c.Track]++
	}
	n := len(rest)
	// The most crowded track (first in priority order on ties).
	crowded, most := "", 0
	for _, c := range rest {
		if count[c.Track] > most {
			crowded, most = c.Track, count[c.Track]
		}
	}
	// After this pick, n-1 remain; a track with more than ceil((n-1)/2)
	// of them would be forced to repeat, so it has to go now.
	if crowded != last && most > n/2 {
		return slices.IndexFunc(rest, func(c domain.Concept) bool { return c.Track == crowded })
	}
	if i := slices.IndexFunc(rest, func(c domain.Concept) bool { return c.Track != last }); i >= 0 {
		return i
	}
	return 0
}

// NextItem returns the item after lastID in items (wrapping around), so
// consecutive reviews of a concept show different items when there are
// several. Without a known lastID it returns the first item.
func NextItem(items []domain.Item, lastID string) (domain.Item, bool) {
	if len(items) == 0 {
		return domain.Item{}, false
	}
	i := slices.IndexFunc(items, func(it domain.Item) bool { return it.ID == lastID })
	return items[(i+1)%len(items)], true
}
