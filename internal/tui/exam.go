package tui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// examRun is a section exam taking over Study (spec S4). Its answers are
// recorded and scheduled like any other; today's session and its lesson
// wait here until the exam ends.
type examRun struct {
	section domain.Section
	saved   session
	lesson  lessonState
	result  *domain.ExamResult // set once the last answer is in
}

// startExam starts the exam of sec on Study, or leaves a note on Tracks
// saying why it cannot start.
func (a App) startExam(sec domain.Section) App {
	if a.exam != nil && a.exam.result == nil {
		a.note = "The exam of " + a.exam.section.Title + " is still open: finish it in Study first."
		return a
	}
	if a.exam != nil {
		a = a.endExam()
	}
	if !srs.ExamAvailable(sec, a.states) {
		a.note = "The exam of " + sec.Title + " is locked: " + a.examLock(sec) + "."
		return a
	}
	picks := srs.PlanExam(a.servable(), sec, a.schedules)
	if len(picks) == 0 {
		a.note = "The exam of " + sec.Title + " has no question to serve (see the missing tools on Home)."
		return a
	}
	saved := a.session
	saved.q.running = false // a run or explanation in flight is dropped, not applied to the exam
	if saved.q.explain == explainLoading {
		saved.q.explain = explainIdle
	}
	ex := &examRun{section: sec, saved: saved}
	if a.lesson.on && a.lesson.screen == Study {
		ex.lesson, a.lesson = a.lesson, lessonState{}
	}
	a.exam, a.note = ex, ""
	a.session = newSession(picks)
	return a.show(Study)
}

// endExam gives Study back to today's session.
func (a App) endExam() App {
	if a.exam == nil {
		return a
	}
	a.session, a.lesson, a.exam = a.exam.saved, a.exam.lesson, nil
	return a
}

// finishExam scores the exam once its last answer is recorded and saves it.
func (a *App) finishExam(now time.Time) error {
	ex := *a.exam
	r := domain.NewExamResult(ex.section.ID, a.session.correct, len(a.session.picks), now)
	if err := a.progress.RecordExam(context.Background(), r); err != nil {
		return err
	}
	ex.result = &r
	a.exam = &ex
	a.exams = cloneMap(a.exams)
	a.exams[r.SectionID] = append(slices.Clone(a.exams[r.SectionID]), r)
	return nil
}

// examLock says what keeps the exam of sec locked: the concepts not yet
// learning, by title.
func (a App) examLock(sec domain.Section) string {
	missing := srs.ExamMissing(sec, a.states)
	names := make([]string, len(missing))
	for i, id := range missing {
		names[i] = a.conceptTitle(id)
	}
	return "answer " + joinAnd(names) + " correctly once, so every concept of the section is at least learning"
}

// bestPass is the best passed attempt at the exam of sec.
func (a App) bestPass(sec domain.Section) (domain.ExamResult, bool) {
	var passed []domain.ExamResult
	for _, r := range a.exams[sec.ID] {
		if r.Passed {
			passed = append(passed, r)
		}
	}
	return domain.BestExam(passed)
}

// conceptGroup is a run of concepts on the Tracks screen: a section, or
// (sec nil) the concepts in no section.
type conceptGroup struct {
	sec      *domain.Section
	concepts []domain.Concept
}

// trackGroups are the track's sections in order, then the concepts in no
// section. A track without sections is one group, in declaration order.
func (a App) trackGroups(track string) []conceptGroup {
	all := a.cat.ConceptsIn(track)
	secs := a.cat.SectionsIn(track)
	if len(secs) == 0 {
		return []conceptGroup{{concepts: all}}
	}
	var out []conceptGroup
	used := map[string]bool{}
	for i := range secs {
		g := conceptGroup{sec: &secs[i]}
		for _, id := range secs[i].Concepts {
			if c, ok := a.cat.Concept(id); ok {
				g.concepts = append(g.concepts, c)
				used[id] = true
			}
		}
		out = append(out, g)
	}
	var rest []domain.Concept
	for _, c := range all {
		if !used[c.ID] {
			rest = append(rest, c)
		}
	}
	if len(rest) > 0 {
		out = append(out, conceptGroup{concepts: rest})
	}
	return out
}

// trackConcepts are the track's concepts in the order Tracks lists them.
func (a App) trackConcepts(track string) []domain.Concept {
	var out []domain.Concept
	for _, g := range a.trackGroups(track) {
		out = append(out, g.concepts...)
	}
	return out
}

// sectionHeader is the line above a section's concepts on Tracks: its
// title and where its exam stands.
func (a App) sectionHeader(sec domain.Section, w int) string {
	r := a.st.main
	var status string
	switch best, ok := a.bestPass(sec); {
	case ok:
		status = r.success.Render(fmt.Sprintf("✓ exam passed %d%%", best.Percent()))
	case srs.ExamAvailable(sec, a.states):
		status = r.accent.Render("exam ready") + r.label.Render(" · x")
	default:
		n := len(srs.ExamMissing(sec, a.states))
		status = r.label.Render(fmt.Sprintf("⊘ exam locked · %d to learn", n))
	}
	return lr(r.strong.Render("§ "+cut(sec.Title, max(w-30, 6))), status, w, r.base)
}

// examLine is the line under the Tracks card about the exam of the
// selected concept's section: how to start it, or why it is locked.
func (a App) examLine(c domain.Concept, w int) string {
	r := a.st.main
	sec, ok := a.cat.SectionOf(c.ID)
	if !ok {
		return ""
	}
	if !srs.ExamAvailable(sec, a.states) {
		return " " + r.label.Render(cut("Exam of "+sec.Title+" locked: "+a.examLock(sec)+".", w-2))
	}
	n := len(srs.PlanExam(a.servable(), sec, a.schedules))
	rest := fmt.Sprintf(" starts the exam of %s · %d %s, pass at %d%%", sec.Title, n, plural(n, "question", "questions"), domain.ExamPassMark)
	if best, ok := a.bestPass(sec); ok {
		rest = fmt.Sprintf(" retakes the exam of %s · best %d%%", sec.Title, best.Percent())
	}
	return " " + r.accent.Render("x") + r.label.Render(cut(rest, w-4))
}

// examSummary is the end of an exam: its score against the pass mark and
// each question's verdict.
func (a App) examSummary(L layout) page {
	r := a.st.main
	s := a.session
	w := L.mi
	ci := w - 4
	ex := a.exam
	res := domain.NewExamResult(ex.section.ID, s.correct, len(s.picks), a.now())
	if ex.result != nil {
		res = *ex.result
	}
	title := r.success.Bold(true).Render("✓ Exam passed")
	if !res.Passed {
		title = r.danger.Bold(true).Render("✗ Exam not passed")
	}
	inner := []string{"",
		r.label.Render(pad("Section", 13)) + r.value.Render(cut(ex.section.Title, ci-13)),
		r.label.Render(pad("Score", 13)) + r.value.Render(fmt.Sprintf("%d of %d (%d%%)", res.Correct, res.Total, res.Percent())) +
			r.label.Render(fmt.Sprintf(" · pass mark %d%%", domain.ExamPassMark)),
	}
	if !res.Passed {
		inner = append(inner, "", r.secondary.Render(cut("Review the concepts below and take it again from Tracks.", ci)))
	}
	inner = append(inner, "", a.heading("Questions", "", ci))
	for i, p := range s.picks[:len(s.results)] {
		mark := r.success.Render("✓")
		if !s.results[i] {
			mark = r.danger.Render("✗")
		}
		inner = append(inner, "  "+mark+" "+r.value.Render(pad(cut(p.Concept.Title, max(ci-24, 10)), max(ci-24, 10)))+" "+
			a.stateWord(r, p.Concept))
	}
	inner = append(inner, "", r.reverse.Render("  enter  back to Tracks  "), "")
	main := append([]string{""}, box(inner, w, title, "", r.rule, r.base)...)
	p := page{main: main, ctx: "exam · " + ex.section.Title + " · finished"}
	if L.three {
		p.right = a.homeContext(L.ri)
	}
	return p
}

// examStats are the Stats lines of the passed exams: section, track and
// best score.
func (a App) examStats(w int) []string {
	r := a.st.main
	st := a.stats
	total := 0
	for _, p := range st.Tracks {
		total += p.Sections
	}
	if total == 0 {
		return nil
	}
	out := []string{"", a.heading("Exams passed", fmt.Sprintf("%d of %d sections", len(st.PassedSections), total), w)}
	if len(st.PassedSections) == 0 {
		return append(out, "  "+r.label.Render(fmt.Sprintf("none yet · an exam passes at %d%%", domain.ExamPassMark)))
	}
	for _, res := range st.PassedSections {
		sec, ok := a.cat.Section(res.SectionID)
		if !ok {
			continue
		}
		left := r.success.Render("✓") + " " + r.value.Render(cut(sec.Title, max(w-30, 6))) + " " + r.label.Render(a.shortTrack(sec.Track))
		out = append(out, lr(left, r.strong.Render(fmt.Sprintf("%d/%d %3d%%", res.Correct, res.Total, res.Percent())), w, r.base))
	}
	return out
}
