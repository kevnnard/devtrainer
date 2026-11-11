package srs

import (
	"slices"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// ExamItemsPerConcept is how many items of each concept a section exam asks.
const ExamItemsPerConcept = 2

// ExamAvailable reports whether the exam of sec can be taken: every
// concept of the section is at least learning (spec S4).
func ExamAvailable(sec domain.Section, states map[string]domain.State) bool {
	return len(sec.Concepts) > 0 && len(ExamMissing(sec, states)) == 0
}

// ExamMissing lists, in section order, the concepts of sec that are not
// learning yet and so keep its exam locked.
func ExamMissing(sec domain.Section, states map[string]domain.State) []string {
	var out []string
	for _, id := range sec.Concepts {
		if states[id] < domain.StateLearning {
			out = append(out, id)
		}
	}
	return out
}

// PlanExam picks the questions of the exam of sec from cat: up to
// ExamItemsPerConcept items of every concept of the section, each concept
// starting after the item it showed last, taken round robin so the exam
// mixes the concepts. Concepts of sec without items in cat (such as code
// items whose toolchain is missing, already removed from cat) are skipped.
func PlanExam(cat domain.Catalog, sec domain.Section, schedules map[string]domain.Schedule) []Pick {
	items := cat.ItemsByConcept()
	var queues [][]Pick
	for _, id := range sec.Concepts {
		c, ok := cat.Concept(id)
		if !ok {
			continue
		}
		var q []Pick
		last := schedules[id].LastItemID
		for range min(ExamItemsPerConcept, len(items[id])) {
			it, _ := NextItem(items[id], last)
			q = append(q, Pick{Concept: c, Item: it})
			last = it.ID
		}
		if len(q) > 0 {
			queues = append(queues, q)
		}
	}
	var out []Pick
	for len(queues) > 0 {
		for i := range queues {
			out = append(out, queues[i][0])
			queues[i] = queues[i][1:]
		}
		queues = slices.DeleteFunc(queues, func(q []Pick) bool { return len(q) == 0 })
	}
	return out
}
