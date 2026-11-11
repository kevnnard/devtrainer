package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// lessonState is a concept's lesson on screen: the intro and example,
// then one step at a time, then the takeaway. Predictions are never
// graded or saved; only "seen" is, when the lesson ends.
type lessonState struct {
	on      bool
	concept domain.Concept
	screen  Screen // Study: before the concept's first question; Tracks: a review
	pos     int    // -1 the intro; 0…n-1 the step on screen; n the takeaway
	cursor  int    // option of the current prediction
	picks   []int  // per step, the predicted option; -1 not predicted yet
}

func newLesson(c domain.Concept, s Screen) lessonState {
	picks := make([]int, len(c.Lesson.Steps))
	for i := range picks {
		picks[i] = -1
	}
	return lessonState{on: true, concept: c, screen: s, pos: -1, picks: picks}
}

func (l lessonState) steps() []domain.LessonStep { return l.concept.Lesson.Steps }

func (l lessonState) review() bool { return l.screen == Tracks }

func (l lessonState) atTakeaway() bool { return l.pos >= len(l.steps()) }

// predicting reports whether the current step waits for a prediction.
func (l lessonState) predicting() bool {
	s := l.steps()
	return l.pos >= 0 && l.pos < len(s) && s[l.pos].Predict != nil && l.picks[l.pos] < 0
}

// progress is "intro", "step 2/4" or "takeaway".
func (l lessonState) progress() string {
	switch {
	case l.pos < 0:
		return "intro"
	case l.atTakeaway():
		return "takeaway"
	}
	return fmt.Sprintf("step %d/%d", l.pos+1, len(l.steps()))
}

// lessonOn reports whether a lesson owns the current screen.
func (a App) lessonOn() bool {
	l := a.lesson
	if !l.on || a.screen != l.screen {
		return false
	}
	return l.screen != Study || !a.session.done()
}

// lessonDue reports whether c's lesson must come before its first
// question: c has a lesson, it was never answered and the lesson was
// never gone through.
func (a App) lessonDue(c domain.Concept) bool {
	return c.Lesson != nil && !a.seen[c.ID] && len(a.history[c.ID]) == 0
}

// lessonKey handles the keys of a lesson; ok is false for the global ones.
func (a App) lessonKey(msg tea.KeyPressMsg) (App, tea.Cmd, bool) {
	k := msg.String()
	l := a.lesson
	l.picks = slices.Clone(l.picks)
	switch {
	case k == "s":
		return a.endLesson(true), nil, true
	case k == "esc" && l.review():
		a.lesson = lessonState{}
		return a, nil, true
	}
	if l.predicting() {
		n := len(l.steps()[l.pos].Predict.Options)
		switch {
		case isDigit(k):
			if i := int(k[0] - '1'); i < n {
				l.cursor, l.picks[l.pos] = i, i
			}
		case k == "j" || k == "down":
			l.cursor = min(l.cursor+1, n-1)
		case k == "k" || k == "up":
			l.cursor = max(l.cursor-1, 0)
		case k == "enter":
			l.picks[l.pos] = l.cursor
		default:
			return a, nil, false
		}
		a.lesson = l
		return a, nil, true
	}
	if k != "enter" && k != "space" {
		return a, nil, false
	}
	if l.atTakeaway() {
		return a.endLesson(false), nil, true
	}
	l.pos++
	l.cursor = 0
	a.lesson = l
	return a, nil, true
}

// endLesson closes the lesson. Before a question it is marked seen
// (finished or skipped) and the question starts; a review from Tracks
// is marked seen only when finished.
func (a App) endLesson(skipped bool) App {
	l := a.lesson
	a.lesson = lessonState{}
	if !l.review() || !skipped {
		a.markSeen(l.concept.ID)
	}
	if !l.review() {
		a.startQuestion()
	}
	return a
}

func (a *App) markSeen(id string) {
	if a.seen[id] {
		return
	}
	a.seen = cloneMap(a.seen)
	a.seen[id] = true
	if err := a.progress.MarkLessonSeen(context.Background(), id, a.now()); err != nil {
		a.err, a.errWhat = err, "the lesson as seen"
	}
}

// ---- view ----

func (a App) lessonPage(L layout) page {
	l := a.lesson
	c := l.concept
	p := page{main: a.lessonCard(L), side: sideOpts{expand: c.Track, at: c.ID}}
	p.ctx = "lesson · " + c.ID + " · not graded"
	if l.review() {
		p.side.cur, p.side.focus = c.Track, true
		p.ctx = "lesson review · " + c.ID + " · not graded"
	}
	if L.three {
		p.right = a.lessonContext(L.ri)
	}
	return p
}

// lessonCard is the focus card of a lesson. On a short terminal the
// current step and the next action win: the oldest revealed steps go
// first, then the intro, then the blank lines, then the end of the code.
func (a App) lessonCard(L layout) []string {
	r := a.st.main
	l := a.lesson
	les := *l.concept.Lesson
	n := len(les.Steps)
	w := L.mi
	ci := w - 4

	intro := wrapLines(strings.TrimSpace(les.Intro), ci, r.value)
	code := a.lessonCode(les.Example, ci)
	var done [][]string
	for i := 0; i < min(l.pos, n); i++ {
		done = append(done, a.stepLines(i, ci, false))
	}
	var cur []string
	switch {
	case l.atTakeaway():
		cur = append([]string{r.success.Bold(true).Render("Takeaway")},
			indent(wrapLines(strings.TrimSpace(les.Takeaway), ci-2, r.strong))...)
	case l.pos >= 0:
		cur = a.stepLines(l.pos, ci, true)
	}
	action := a.lessonAction()

	top := 1
	if L.three {
		top = 2
	}
	budget := L.body - top - 2
	showIntro, gaps, codeRows := true, true, len(code)
	build := func() []string {
		var sections [][]string
		if showIntro {
			sections = append(sections, intro)
		}
		if len(code) > 0 {
			sections = append(sections, clipCode(a, code, codeRows))
		}
		if len(done) > 0 {
			sections = append(sections, slices.Concat(done...))
		}
		if len(cur) > 0 {
			sections = append(sections, cur)
		}
		sections = append(sections, []string{action})
		var inner []string
		if gaps {
			inner = append(inner, "")
		}
		for i, s := range sections {
			if i > 0 && gaps {
				inner = append(inner, "")
			}
			inner = append(inner, s...)
		}
		if gaps {
			inner = append(inner, "")
		}
		return inner
	}
	inner := build()
	for len(inner) > budget {
		switch {
		case len(done) > 0:
			done = done[1:]
		case showIntro && l.pos >= 0:
			showIntro = false
		case gaps:
			gaps = false
		case codeRows > 2:
			codeRows--
		default: // last resort: keep the end, where the next action is
			inner = inner[len(inner)-max(budget, 0):]
			continue
		}
		inner = build()
	}

	title := r.secondary.Render(a.shortTrack(l.concept.Track)+" ›") + " " + r.strong.Render(l.concept.Title)
	right := r.accent.Render("lesson") + r.label.Render(" · "+l.progress())
	if ansi.StringWidth(title)+ansi.StringWidth(right)+8 > w {
		right = r.accent.Render("lesson")
	}
	out := make([]string, top)
	return append(out, box(inner, w, title, right, r.rule, r.base)...)
}

// lessonCode is the worked example on the raised surface with line
// numbers and its language on the first line.
func (a App) lessonCode(ex domain.Example, w int) []string {
	if strings.TrimSpace(ex.Code) == "" {
		return nil
	}
	rr := a.st.raised
	lines := strings.Split(strings.TrimRight(ex.Code, "\n"), "\n")
	numW := len(fmt.Sprint(len(lines)))
	out := make([]string, len(lines))
	for i, l := range lines {
		left := rr.base.Render(" ") + rr.label.Render(fmt.Sprintf("%*d", numW, i+1)) + rr.base.Render("  ") + rr.code.Render(l)
		if i == 0 && ex.Lang != "" && ansi.StringWidth(left)+ansi.StringWidth(ex.Lang)+3 <= w {
			out[i] = lr(left, rr.label.Render(ex.Lang)+rr.base.Render(" "), w, rr.base)
			continue
		}
		out[i] = paint(left, w, rr)
	}
	return out
}

// clipCode keeps the first k rows of code, the last one saying how many
// more there are.
func clipCode(a App, code []string, k int) []string {
	if k >= len(code) {
		return code
	}
	rr := a.st.raised
	more := len(code) - k + 1
	return append(slices.Clone(code[:k-1]), paint(rr.base.Render(" ")+rr.label.Render(fmt.Sprintf("… %d more lines", more)), ansi.StringWidth(code[0]), rr))
}

// stepLines is step i: "✓ 1  text" once revealed, "▸ 2  text" when it is
// the current step. A current step with a prediction shows the prompt
// and options first, then the verdict and the text.
func (a App) stepLines(i, w int, current bool) []string {
	r := a.st.main
	l := a.lesson
	st := l.steps()[i]
	num := fmt.Sprint(i + 1)
	hang := strings.Repeat(" ", 2+len(num)+2)
	head, text := r.label.Render("✓ "+num)+"  ", r.secondary
	if current {
		head, text = r.accent.Render("▸ "+num)+"  ", r.value
	}
	bw := w - len(hang)
	var body []string
	if st.Error != "" {
		body = append(body, a.errorLines(st.Error, bw)...)
		body = append(body, "")
	}
	if p := st.Predict; p != nil && current {
		pick := l.picks[i]
		if pick < 0 {
			body = append(body, wrapLines("Predict: "+strings.TrimSpace(p.Prompt), bw, r.strong)...)
			body = append(body, "")
			body = append(body, a.predictOptions(*p, l.cursor, bw)...)
			return hangLines(head, hang, body)
		}
		ans := p.Answer()
		if pick == ans {
			body = append(body, wrapLines("✓ Right: "+p.Options[ans].Text, bw, r.success)...)
		} else {
			body = append(body, wrapLines("✗ Not quite. It is: "+p.Options[ans].Text, bw, r.danger)...)
		}
		if e := strings.TrimSpace(p.Options[pick].Explanation); e != "" {
			body = append(body, wrapLines(e, bw, r.secondary)...)
		}
		body = append(body, "")
	}
	body = append(body, wrapLines(strings.TrimSpace(st.Text), bw, text)...)
	return hangLines(head, hang, body)
}

// maxErrorLines caps the error shown by an error-on-purpose step.
const maxErrorLines = 8

// errorLines are a step's real error message on the error surface.
func (a App) errorLines(msg string, w int) []string {
	er := a.st.errRow
	out := []string{paint(er.danger.Render(" ✗ the example fails with"), w, er)}
	for _, l := range strings.Split(clipLines(msg, maxErrorLines), "\n") {
		out = append(out, paint(er.base.Render(" ")+er.danger.Render(cut(l, w-2)), w, er))
	}
	return out
}

// hangLines puts head before the first line and hang before the others.
func hangLines(head, hang string, body []string) []string {
	out := make([]string, len(body))
	for i, l := range body {
		if i == 0 {
			out[i] = head + l
			continue
		}
		out[i] = hang + l
	}
	return out
}

// predictOptions are the options of a prediction: " ▸ 1  text" on the
// cursor, "   2  text" elsewhere.
func (a App) predictOptions(p domain.Predict, cursor, w int) []string {
	r, sr := a.st.main, a.st.sel
	var out []string
	for i, o := range p.Options {
		key := fmt.Sprint(i + 1)
		for j, l := range strings.Split(wrap(o.Text, w-7), "\n") {
			if i == cursor {
				head := sr.base.Render("       ")
				if j == 0 {
					head = sr.base.Render(" ") + sr.selected.Render("▸ "+key) + sr.base.Render("   ")
				}
				out = append(out, paint(head+sr.strong.Render(l), w, sr))
				continue
			}
			head := "       "
			if j == 0 {
				head = "   " + r.label.Render(key) + "   "
			}
			out = append(out, head+r.value.Render(l))
		}
	}
	return out
}

// lessonAction is the card's last line: what enter does, and how to leave.
func (a App) lessonAction() string {
	r := a.st.main
	l := a.lesson
	n := len(l.steps())
	var main string
	switch {
	case l.predicting():
		main = r.accent.Render("1–9") + r.label.Render(" or ") + r.accent.Render("j/k enter") + r.label.Render(" predicts")
	case l.atTakeaway() && l.review():
		main = r.reverse.Render("  enter  back to Tracks  ")
	case l.atTakeaway():
		main = r.reverse.Render("  enter  start the questions  ")
	case l.pos < 0:
		main = r.reverse.Render("  enter  first step  ")
	case l.pos == n-1:
		main = r.reverse.Render("  enter  takeaway  ")
	default:
		main = r.reverse.Render("  enter  next step  ")
	}
	leave := r.accent.Render("s") + "  " + r.secondary.Render("skip lesson")
	if l.review() {
		leave = r.accent.Render("esc") + "  " + r.secondary.Render("back to Tracks")
	}
	return main + "   " + leave
}

// lessonMeter is "lesson [✓●··] step 2/4" for the bottom bar.
func (a App) lessonMeter(r roles) string {
	l := a.lesson
	var marks strings.Builder
	for i := range l.steps() {
		switch {
		case i < l.pos:
			marks.WriteString(r.success.Render("✓"))
		case i == l.pos:
			marks.WriteString(r.selected.Render("●"))
		default:
			marks.WriteString(r.label.Render("·"))
		}
	}
	return r.accent.Render("lesson") + r.base.Render(" ") + r.label.Render("[") + marks.String() + r.label.Render("]") +
		r.base.Render(" ") + r.strong.Render(l.progress())
}

func (a App) lessonHints() []string {
	l := a.lesson
	leave := "s skip lesson"
	if l.review() {
		leave = "esc back"
	}
	if l.predicting() {
		return []string{"1-9 predict", "j/k move", "enter choose", leave, "? help"}
	}
	next := "enter next"
	switch {
	case l.atTakeaway() && l.review():
		next = "enter back"
	case l.atTakeaway():
		next = "enter questions"
	}
	return []string{next, leave, "? help", "q quit"}
}

// lessonContext is the context pane of a lesson: the steps so far, the
// concept and its prerequisites.
func (a App) lessonContext(w int) []string {
	r := a.st.main
	l := a.lesson
	c := l.concept
	row := func(i int, label string) string {
		// i is the lesson position the row stands for.
		switch {
		case i < l.pos:
			return r.success.Render("✓") + " " + r.secondary.Render(label)
		case i == l.pos:
			return r.accent.Render("▸") + " " + r.accent.Render(label)
		}
		return r.label.Render("·") + " " + r.value.Render(label)
	}
	out := []string{a.heading("LESSON", "not graded", w), row(-1, "intro & example")}
	for i, st := range l.steps() {
		label := fmt.Sprintf("step %d", i+1)
		switch {
		case st.Error != "":
			label += " · error"
		case st.Predict != nil:
			label += " · predict"
		}
		line := row(i, label)
		if p := l.picks[i]; p >= 0 && i < l.pos {
			if p == st.Predict.Answer() {
				line += r.success.Render(" ✓")
			} else {
				line += r.danger.Render(" ✗")
			}
		}
		out = append(out, line)
	}
	out = append(out, row(len(l.steps()), "takeaway"), "")

	out = append(out, a.heading("CONCEPT", string(c.Level), w))
	if c.Summary != "" {
		out = append(out, wrapLines(strings.TrimSpace(c.Summary), w, r.secondary)...)
	}
	out = append(out, "", a.heading("PREREQUISITES", "", w))
	if len(c.Prereqs) == 0 {
		out = append(out, r.label.Render("None: a starting point."))
	}
	for _, id := range c.Prereqs {
		g := r.danger.Render("✗")
		if a.states[id] >= domain.StateLearning {
			g = r.success.Render("✓")
		}
		out = append(out, g+" "+r.value.Render(cut(a.prereqName(id, c.Track), w-2)))
	}
	note := "Predictions are only for you: nothing here is graded or scheduled. The questions start after the takeaway."
	if l.review() {
		note = "A review from Tracks: nothing here is graded or scheduled."
	}
	return append(append(out, ""), wrapLines(note, w, r.label)...)
}
