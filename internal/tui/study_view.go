package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// shortDate is how the scheduler's dates read in the panes.
const shortDate = "Mon 2 Jan"

func (a App) studyPage(L layout) page {
	s := a.session
	switch {
	case len(s.picks) == 0:
		return a.caughtUpPage(L, "nothing due")
	case s.done() && a.exam != nil:
		return a.examSummary(L)
	case s.done():
		return a.summaryPage(L)
	}
	q := s.q
	p := page{side: sideOpts{expand: q.pick.Concept.Track, at: q.pick.Concept.ID}}
	p.ctx = q.pick.Item.ID + " · " + string(q.pick.Item.Type)
	if a.exam != nil {
		p.ctx = fmt.Sprintf("exam · %s · question %d of %d · ", a.exam.section.Title, s.pos+1, len(s.picks)) + p.ctx
	}
	if q.answered {
		p.ctx += " · answered in " + clock(q.took)
		p.main = a.feedbackView(L)
	} else {
		p.main = a.questionView(L)
	}
	if L.three {
		p.right = a.studyContext(L.ri)
	}
	return p
}

// caughtUpPage is Study (or Home) when nothing is left today.
func (a App) caughtUpPage(L layout, ctx string) page {
	p := page{main: a.caughtUp(L), ctx: ctx}
	if L.three {
		p.right = a.homeContext(L.ri)
	}
	return p
}

// clock formats a duration as m:ss.
func clock(d time.Duration) string {
	sec := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// cardTitle is "Go › Slices" for the card's top border, with the state on
// the right; the state word goes first when the title is too long.
func (a App) cardTitle(w int) (title, right string) {
	r := a.st.main
	q := a.session.q
	c := q.pick.Concept
	title = r.secondary.Render(a.shortTrack(c.Track)+" ›") + " " + r.strong.Render(c.Title)
	g := a.mark(r, c)
	right = g + " " + r.secondary.Render(a.states[c.ID].String())
	if ansi.StringWidth(title)+ansi.StringWidth(right)+8 > w {
		right = g
	}
	return title, right
}

// questionView is the focus card of an unanswered question.
func (a App) questionView(L layout) []string {
	r := a.st.main
	q := a.session.q
	it := q.pick.Item
	w := L.mi
	ci := w - 4

	inner := []string{""}
	blank := 0
	prompt := it.Prompt
	if q.kind == blanks {
		prompt, blank = a.fillBlanks(prompt, 0, r)
	}
	inner = append(inner, wrapLines(strings.TrimSpace(prompt), ci, r.strong)...)
	inner = append(inner, "")
	if it.Code != "" && q.kind != pickLine {
		inner = append(inner, a.codeBlock(q, blank, ci)...)
		inner = append(inner, "")
	}
	if q.kind == editor {
		top := 1
		if L.three {
			top = 2
		}
		// the card's borders and gaps, the hint line under it
		rows := L.body - top - len(inner) - 9
		if !L.three {
			rows -= 2
		}
		inner = append(inner, a.editorView(ci, rows)...)
	} else {
		inner = append(inner, a.answerView(ci, L.three)...)
	}
	inner = append(inner, "")

	title, right := a.cardTitle(w)
	var out []string
	if L.three {
		out = append(out, "")
	}
	out = append(out, "")
	out = append(out, box(inner, w, title, right, r.rule, r.base)...)
	if !L.three {
		out = append(out, "", " "+a.duePreviewRow(w-2))
	}
	out = append(out, "")
	switch {
	case q.hint:
		lines := strings.Split(wrap("Hint "+it.Hint, w), "\n")
		out = append(out, r.warning.Render("Hint")+r.secondary.Render(strings.TrimPrefix(lines[0], "Hint")))
		for _, l := range lines[1:] {
			out = append(out, r.secondary.Render(l))
		}
	case q.noHint:
		out = append(out, r.label.Render("No hint for this one."))
	case it.Hint != "":
		k := "h"
		if q.kind == typed || q.kind == blanks || q.kind == editor {
			k = "ctrl+t"
		}
		out = append(out, centered(r.label.Render("Stuck? ")+r.accent.Render(k)+
			r.label.Render(" shows a hint; a hinted answer grades Hard."), w))
	}
	return out
}

// codeBlock is the item's code on the raised surface with line numbers;
// cloze blanks inside it continue the numbering of the prompt's blanks.
func (a App) codeBlock(q question, blank, w int) []string {
	rr := a.st.raised
	code := strings.TrimRight(q.pick.Item.Code, "\n")
	if q.kind == blanks {
		code, _ = a.fillBlanks(code, blank, rr)
	}
	lines := strings.Split(code, "\n")
	numW := len(fmt.Sprint(len(lines)))
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = paint(rr.base.Render(" ")+rr.label.Render(fmt.Sprintf("%*d", numW, i+1))+rr.base.Render("  ")+rr.code.Render(l), w, rr)
	}
	return out
}

// fillBlanks replaces each ___ of s with the learner's fill for that
// blank, numbering blanks from first. It returns the next blank number.
func (a App) fillBlanks(s string, first int, r roles) (string, int) {
	q := a.session.q
	parts := strings.Split(s, check.BlankMarker)
	var b strings.Builder
	n := first
	for i, p := range parts {
		b.WriteString(r.code.Render(p))
		if i == len(parts)-1 {
			break
		}
		fill := ""
		if n < len(q.fills) {
			fill = q.fills[n]
		}
		switch {
		case q.answered:
			b.WriteString(r.input.Render("[" + fill + "]"))
		case n == q.blank:
			b.WriteString(r.selected.Render("[" + fill + "▏]"))
		case fill == "":
			b.WriteString(r.label.Render(fmt.Sprintf("[%d___]", n+1)))
		default:
			b.WriteString(r.accent.Render("[" + fill + "]"))
		}
		n++
	}
	return b.String(), n
}

// answerView is how the learner answers the current item, w columns
// wide. Roomy puts a blank line between options.
func (a App) answerView(w int, roomy bool) []string {
	r, sr, rr := a.st.main, a.st.sel, a.st.raised
	q := a.session.q
	it := q.pick.Item
	var out []string

	// option rows: "   1   text", focused " ▸ 1   text" on the selection.
	option := func(i int, key, text string) {
		lines := strings.Split(wrap(text, w-7), "\n")
		for j, l := range lines {
			if i == q.cursor {
				head := sr.base.Render("       ")
				if j == 0 {
					head = sr.base.Render(" ") + sr.selected.Render("▸ "+key) + sr.base.Render(strings.Repeat(" ", max(4-ansi.StringWidth(key), 1)))
				}
				out = append(out, paint(head+sr.strong.Render(l), w, sr))
				continue
			}
			head := "       "
			if j == 0 {
				head = "   " + r.label.Render(key) + strings.Repeat(" ", max(4-ansi.StringWidth(key), 1))
			}
			out = append(out, head+r.value.Render(l))
		}
	}
	switch q.kind {
	case pickOne:
		for i, o := range it.Options {
			if i > 0 && roomy {
				out = append(out, "")
			}
			option(i, fmt.Sprint(i+1), o.Text)
		}
	case pickMany:
		out = append(out, r.label.Render("Select every correct answer."))
		for i, o := range it.Options {
			box := "[ ]"
			if q.chosen[i] {
				box = "[x]"
			}
			lines := strings.Split(wrap(o.Text, w-7), "\n")
			for j, l := range lines {
				head := "       "
				if j == 0 {
					head = "   " + box + " "
				}
				if i == q.cursor {
					if j == 0 {
						head = " ▸ " + box + " "
					}
					out = append(out, paint(sr.selected.Render(head)+sr.strong.Render(l), w, sr))
					continue
				}
				out = append(out, r.accent.Render(head)+r.value.Render(l))
			}
		}
	case typed:
		label := "Your answer"
		if it.Type == domain.Output {
			label = "Output"
		}
		out = append(out, r.label.Render(label))
		for i, l := range strings.Split(q.text+"▏", "\n") {
			prefix := "  "
			if i == 0 {
				prefix = r.accent.Render("› ")
			}
			out = append(out, prefix+r.input.Render(l))
		}
	case blanks:
		out = append(out, r.label.Render(fmt.Sprintf("Blank %d of %d", q.blank+1, len(q.fills))))
	case reorder:
		out = append(out, r.label.Render(fmt.Sprintf("Selected line %d of %d · %d %s moved so far",
			q.cursor+1, len(q.order), q.moved(), plural(q.moved(), "line", "lines"))))
		for i, l := range q.order {
			was := ""
			if q.from[i] != i {
				was = fmt.Sprintf("was %d", q.from[i]+1)
			}
			if i == q.cursor {
				left := sr.selected.Render(fmt.Sprint(i+1)) + sr.base.Render("  ") + sr.selected.Render(l)
				row := sr.base.Render("  ") + sr.selected.Render("▸") + sr.base.Render(" ") +
					lr(left, wasNote(sr, was), w-6, sr.base) + sr.base.Render(" ")
				out = append(out, paint(row, w, sr))
				continue
			}
			left := rr.label.Render(fmt.Sprint(i+1)) + rr.base.Render("  ") + rr.code.Render(l)
			row := rr.base.Render(" ") + lr(left, wasNote(rr, was), w-4, rr.base) + rr.base.Render(" ")
			out = append(out, paint(row, w-2, rr))
		}
		out = append(out, "", r.label.Render("J/K moves the lifted line · enter checks the order"))
	case pickLine:
		out = append(out, r.label.Render("Select the faulty line."))
		lines := codeLines(it)
		numW := len(fmt.Sprint(len(lines)))
		for i, l := range lines {
			num := fmt.Sprintf("%*d │ ", numW, i+1)
			if i == q.cursor {
				out = append(out, paint(sr.selected.Render("▸ ")+sr.label.Render(num)+sr.selected.Render(l), w, sr))
				continue
			}
			out = append(out, paint(rr.base.Render("  ")+rr.label.Render(num)+rr.code.Render(l), w, rr))
		}
	}
	return out
}

// wasNote is the origin slot of a moved Parsons line: "was 4".
func wasNote(r roles, was string) string {
	if was == "" {
		return ""
	}
	n, _ := strings.CutPrefix(was, "was ")
	return r.label.Render("was ") + r.secondary.Render(n)
}

// duePreview is the next due date of the current concept for each grade,
// computed on a copy of its schedule; nothing is saved.
func (a App) duePreview() [4]time.Time {
	q := a.session.q
	sc := a.schedules[q.pick.Concept.ID] // a copy: Schedule is a value
	sc.ConceptID = q.pick.Concept.ID
	now := a.now()
	var out [4]time.Time
	for i, g := range []domain.Grade{domain.Again, domain.Hard, domain.Good, domain.Easy} {
		out[i] = a.sched.Review(sc, g, q.pick.Item.ID, now).Due
	}
	return out
}

// duePreviewRow is the narrow form of the grade preview, under the card.
func (a App) duePreviewRow(w int) string {
	r := a.st.main
	d := a.duePreview()
	loc := a.now().Location()
	day := func(t time.Time) string { return t.In(loc).Format("Mon 2") }
	right := r.danger.Render("✗") + " " + day(d[0]) + "  " + r.warning.Render("◆") + " " + day(d[1]) + "  " +
		r.success.Render("✓") + " " + day(d[2]) + "  " + r.accent.Render("▲") + " " + day(d[3])
	return lr(r.label.Render("next due if"), right, w, r.base)
}

// feedbackView is shown after an answer: the question dimmed above, then
// the verdict card with the -/+ answer diff, why not, why this, the
// takeaway and the next action.
func (a App) feedbackView(L layout) []string {
	r := a.st.main
	q := a.session.q
	it, c := q.pick.Item, q.pick.Concept
	w := L.mi
	ci := w - 4
	loc := a.now().Location()

	out := []string{""}
	out = append(out, lr(r.secondary.Render(a.shortTrack(c.Track)+" › ")+r.strong.Render(c.Title),
		a.mark(r, c)+" "+r.secondary.Render(a.states[c.ID].String()), w, r.base))
	prompt := strings.TrimSpace(it.Prompt)
	blank := 0
	if q.kind == blanks {
		prompt, blank = a.fillBlanks(prompt, 0, r)
	}
	out = append(out, wrapLines(prompt, w, r.secondary)...)
	if it.Code != "" {
		out = append(out, a.codeBlock(q, blank, w)...)
	}
	out = append(out, "")

	var body []string
	switch q.explain {
	case explainLoading:
		body = append(body, "", r.accent.Render("⠋ ")+r.meta.Render("Tutor is thinking…"))
	case explainDone:
		body = append(body, "", r.title.Render("Tutor"))
		body = append(body, indent(wrapLines(q.explanation, ci-2, r.value))...)
	case explainFailed:
		body = append(body, "", r.danger.Render("Tutor unavailable:"))
		body = append(body, indent(wrapLines(q.explainErr.Error(), ci-2, r.secondary))...)
	}
	section := func(title, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		body = append(body, "", r.strong.Render(title))
		body = append(body, indent(wrapLines(strings.TrimSpace(text), ci-2, r.value))...)
	}
	for _, o := range a.wrongChosen() {
		section("Why not", o.Explanation)
	}
	section("Why this", it.Explanation)
	if q.kind == editor && !q.res.Correct && strings.TrimSpace(it.Solution) != "" {
		body = append(body, "", r.strong.Render("A solution"))
		body = append(body, indent(a.solutionLines(it, ci-2))...)
	}
	section("Takeaway", c.Summary)

	next := "  enter  next question  "
	if a.session.pos == len(a.session.picks)-1 {
		next = "  enter  finish session  "
	}
	action := r.reverse.Render(next)
	if q.explain == explainIdle || q.explain == explainFailed {
		action += "   " + r.accent.Render("e") + "  " + r.secondary.Render("ask the tutor")
	}

	title, border := r.danger.Bold(true).Render("✗ Not quite"), r.danger
	if q.res.Correct {
		title, border = r.success.Bold(true).Render("✓ Correct"), r.success
	}
	right := r.secondary.Render("graded ") + r.strong.Render(q.grade.String()) +
		r.secondary.Render(" · due "+q.next.Due.In(loc).Format(shortDate))

	// On a short terminal the verdict and its next action win: the dimmed
	// question goes first (keeping its title line), then the blank lines
	// between sections, then the diff shrinks to the differing lines (a
	// code diff to fewer lines a side), and as a last resort the
	// explanation is clipped.
	diff := a.answerDiff(ci, false, maxDiffLines)
	tall := func() bool { return len(out)+len(diff)+len(body)+6 > L.body }
	if tall() {
		out = out[:2]
	}
	for i := len(body) - 1; i >= 0 && tall(); i-- {
		if body[i] == "" {
			body = slices.Delete(body, i, i+1)
		}
	}
	if tall() {
		diff = a.answerDiff(ci, true, maxDiffLines)
	}
	for n := maxDiffLines - 1; q.kind == editor && n >= minDiffLines && tall(); n-- {
		diff = a.answerDiff(ci, true, n)
	}
	if n := len(out) + len(diff) + len(body) + 6 - L.body; n > 0 && len(body) > n {
		body = append(body[:len(body)-n-1], indent([]string{r.label.Render("…")})...)
	}
	inner := append([]string{""}, diff...)
	inner = append(inner, body...)
	inner = append(inner, "", action, "")
	return append(out, box(inner, w, title, right, border, r.base)...)
}

// indent prefixes every line with two spaces.
func indent(lines []string) []string {
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return lines
}

// answerDiff is the top of the verdict card: "- your answer" with its
// misconception on the error surface, "+ expected" on the success one.
// A Parsons order is diffed line by line: lines in their right place show
// once, unmarked; short keeps only the misplaced lines' expected text.
// A code diff shows at most codeCap lines a side, the last one saying how
// many more there are.
func (a App) answerDiff(w int, short bool, codeCap int) []string {
	q := a.session.q
	r, er, ok := a.st.main, a.st.errRow, a.st.okRow
	var out []string
	// row paints one sign-prefixed line; note says what it is and extra,
	// when it fits, sits on the right.
	row := func(rr roles, st func(...string) string, text, note, extra string) {
		left := st(text)
		if note != "" && ansi.StringWidth(text)+2+ansi.StringWidth(note) <= w {
			left += rr.base.Render("  ") + rr.label.Render(note)
		}
		switch {
		case extra == "":
			out = append(out, paint(left, w, rr))
		case ansi.StringWidth(left)+2+ansi.StringWidth(extra) <= w:
			out = append(out, lr(left, extra, w, rr.base))
		default:
			out = append(out, paint(left, w, rr), paint(rr.base.Render("  ")+extra, w, rr))
		}
	}
	rows := func(rr roles, st func(...string) string, sign, text, note, extra string) {
		for i, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			if i == 0 {
				row(rr, st, sign+" "+l, note, extra)
			} else {
				row(rr, st, "  "+l, "", "")
			}
		}
	}
	if q.kind == editor {
		exp := clipLines(q.res.Expected, codeCap)
		if q.res.Correct {
			rows(ok, ok.success.Render, "+", exp, "your output", "")
			return out
		}
		rows(er, er.danger.Render, "-", clipLines(codeVerdict(q.pick.Item, q.resp.Run), codeCap), "your output", "")
		rows(ok, ok.success.Render, "+", exp, "expected", "")
		return out
	}
	if q.res.Correct {
		rows(ok, ok.success.Render, "+", q.res.Expected, "your answer", "")
		return out
	}
	extra := ""
	if m := q.res.ChosenMisconception; m != "" {
		n := a.misconceptionCount(m, time.Time{})
		extra = er.warning.Render(m) + er.label.Render(" · "+ordinal(n)+" time")
	}
	if q.kind != reorder {
		rows(er, er.danger.Render, "-", describe(q.pick.Item, q.resp), "your answer", extra)
		rows(ok, ok.success.Render, "+", q.res.Expected, "expected", "")
		return out
	}
	mine, want := q.resp.Order, strings.Split(q.res.Expected, "\n")
	var wrong []int
	for i := range want {
		if i >= len(mine) || mine[i] != want[i] {
			wrong = append(wrong, i)
		}
	}
	if short {
		row(er, er.danger.Render, fmt.Sprintf("- %d of %d lines out of place", len(wrong), len(want)), "your answer", extra)
		for j, i := range wrong {
			note := ""
			if j == 0 {
				note = "expected"
			}
			row(ok, ok.success.Render, fmt.Sprintf("+ %d  %s", i+1, want[i]), note, "")
		}
		return out
	}
	first := true
	for i := range want {
		if !slices.Contains(wrong, i) {
			out = append(out, r.secondary.Render("  "+want[i]))
			continue
		}
		note, note2, ex := "", "", ""
		if first {
			note, note2, ex, first = "your answer", "expected", extra, false
		}
		if i < len(mine) {
			row(er, er.danger.Render, "- "+mine[i], note, ex)
		}
		row(ok, ok.success.Render, "+ "+want[i], note2, "")
	}
	return out
}

// misconceptionCount counts the answers with misconception m at or after
// since (zero: all time).
func (a App) misconceptionCount(m string, since time.Time) int {
	n := 0
	for _, answers := range a.history {
		for _, ans := range answers {
			if ans.Misconception == m && !ans.At.Before(since) {
				n++
			}
		}
	}
	return n
}

// ordinal is "1st", "2nd", "3rd", "4th"…
func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// wrongChosen are the wrong options the learner picked.
func (a App) wrongChosen() []domain.Option {
	q := a.session.q
	it := q.pick.Item
	var idx []int
	switch q.kind {
	case pickOne:
		idx = []int{q.resp.Choice}
	case pickMany:
		idx = q.resp.Choices
	}
	var out []domain.Option
	for _, i := range idx {
		if i >= 0 && i < len(it.Options) && !it.Options[i].Correct {
			out = append(out, it.Options[i])
		}
	}
	return out
}

// describe renders the learner's response for the feedback.
func describe(it domain.Item, r check.Response) string {
	switch kindOf(it) {
	case pickOne:
		if r.Choice >= 0 && r.Choice < len(it.Options) {
			return it.Options[r.Choice].Text
		}
	case pickMany:
		var t []string
		for _, i := range r.Choices {
			t = append(t, it.Options[i].Text)
		}
		return strings.Join(t, "; ")
	case blanks:
		return strings.Join(r.Blanks, " | ")
	case reorder:
		return strings.Join(r.Order, "\n")
	case pickLine:
		lines := codeLines(it)
		if r.Line >= 1 && r.Line <= len(lines) {
			return fmt.Sprintf("line %d: %s", r.Line, strings.TrimSpace(lines[r.Line-1]))
		}
	case editor:
		return codeVerdict(it, r.Run)
	}
	return r.Text
}

// ---- the context pane of Study ----

func (a App) studyContext(w int) []string {
	r := a.st.main
	s := a.session
	q := s.q
	c := q.pick.Concept
	loc := a.now().Location()

	note := fmt.Sprintf("%d/%d", s.pos+1, len(s.picks))
	if q.answered {
		note += fmt.Sprintf(" · ✓%d ✗%d", s.correct, len(s.results)-s.correct)
	}
	out := []string{a.heading("SESSION", note, w)}
	out = append(out, a.queueRows(w, 10)...)
	out = append(out, "")

	if !q.answered {
		out = append(out, a.heading("CONCEPT", string(c.Level), w))
		if c.Summary != "" {
			out = append(out, wrapLines(c.Summary, w, r.secondary)...)
		}
		d := a.duePreview()
		day := func(t time.Time) string { return t.In(loc).Format(shortDate) }
		wrong, hard, good := "wrong", "hint/slow", "correct"
		if q.kind == editor {
			wrong, hard, good = "fails", "hint", "passes"
		}
		return append(out, "",
			a.heading("IF YOU ANSWER", "next due", w),
			lr(r.danger.Render("✗ Again")+" "+r.label.Render(wrong), day(d[0]), w, r.base),
			lr(r.warning.Render("◆ Hard")+"  "+r.label.Render(hard), day(d[1]), w, r.base),
			lr(r.success.Render("✓ Good")+"  "+r.label.Render(good), day(d[2]), w, r.base),
			lr(r.accent.Render("▲ Easy")+"  "+r.label.Render("fast"), day(d[3]), w, r.base))
	}

	// After answering: the last five against the mastery rule, the
	// schedule before and after, the tutor.
	hist := a.history[c.ID]
	last := hist[max(len(hist)-srs.MasteryWindow, 0):]
	marks := make([]string, len(last))
	right := 0
	for i, ans := range last {
		if ans.Correct {
			marks[i] = r.success.Render("✓")
			right++
		} else {
			marks[i] = r.danger.Render("✗")
		}
	}
	out = append(out, a.heading(fmt.Sprintf("LAST %d ANSWERS", srs.MasteryWindow), "", w),
		strings.Join(marks, " ")+"   "+r.secondary.Render(fmt.Sprintf("%d of %d", right, len(last))))
	out = append(out, wrapLines(fmt.Sprintf("Mastery needs %d of the last %d correct, over at least %d days.",
		srs.MasteryCorrect, srs.MasteryWindow, srs.MasteryDays), w, r.label)...)

	before := "new"
	if q.prev.Reviewed() {
		before = q.prev.Due.In(loc).Format("Mon 2")
	}
	lapse := r.strong
	if q.next.Lapses > q.prev.Lapses {
		lapse = r.danger
	}
	out = append(out, "", a.heading("SCHEDULE", "before → after", w),
		lr(r.label.Render("next due"), r.value.Render(before+" → ")+r.strong.Render(q.next.Due.In(loc).Format("Mon 2")), w, r.base),
		lr(r.label.Render("lapses"), r.value.Render(fmt.Sprintf("%d → ", q.prev.Lapses))+lapse.Render(fmt.Sprint(q.next.Lapses)), w, r.base))
	if m := q.res.ChosenMisconception; m != "" {
		n := a.misconceptionCount(m, a.now().AddDate(0, 0, -30))
		out = append(out, lr(r.warning.Render(cut(m, w-14)), r.strong.Render(fmt.Sprintf("%d×", n))+r.label.Render(" in 30 days"), w, r.base))
	}

	tutorNote := "via pi"
	if a.tutor == nil {
		tutorNote = "off"
	}
	out = append(out, "", a.heading("TUTOR", tutorNote, w))
	switch q.explain {
	case explainLoading:
		out = append(out, r.meta.Render("thinking…"))
	case explainDone:
		out = append(out, r.secondary.Render("answered in the card"))
	default:
		out = append(out, r.accent.Render("e")+"  "+r.secondary.Render("explain deeper"))
		out = append(out, wrapLines("Sends the question, your answer and the expected one. Everything else works without it.", w, r.label)...)
	}
	return out
}

// queueRows list the session's picks around the current one: ✓/✗ done,
// ▸ current, · to come, with the track on the right.
func (a App) queueRows(w, n int) []string {
	r := a.st.main
	s := a.session
	from := 0
	if len(s.picks) > n {
		from = min(max(s.pos-n/2, 0), len(s.picks)-n)
	}
	var out []string
	for i := from; i < min(from+n, len(s.picks)); i++ {
		p := s.picks[i]
		name := cut(p.Concept.Title, w-10)
		var mark string
		switch {
		case i < len(s.results) && i == s.pos:
			mark, name = r.danger.Render("✗"), r.strong.Render(name)
			if s.results[i] {
				mark = r.success.Render("✓")
			}
		case i < len(s.results):
			mark, name = r.danger.Render("✗"), r.label.Render(name)
			if s.results[i] {
				mark = r.success.Render("✓")
			}
		case i == s.pos:
			mark, name = r.accent.Render("▸"), r.accent.Render(name)
		default:
			mark, name = r.label.Render("·"), r.value.Render(name)
		}
		out = append(out, lr(mark+" "+name, r.label.Render(cut(a.shortTrack(p.Concept.Track), 8)), w, r.base))
	}
	return out
}

// ---- end of session ----

func (a App) summaryPage(L layout) page {
	r := a.st.main
	s := a.session
	w := L.mi
	ci := w - 4
	n := len(s.results)
	pct := 0
	if n > 0 {
		pct = 100 * s.correct / n
	}
	inner := []string{"",
		r.label.Render(pad("Correct", 13)) + r.value.Render(fmt.Sprintf("%d of %d (%d%%)", s.correct, n, pct))}
	if next, ok := a.nextDue(); ok {
		inner = append(inner, r.label.Render(pad("Next review", 13))+r.value.Render(a.relDay(next)))
	}
	inner = append(inner, "", a.heading("Concepts touched", "", ci))
	for i, p := range s.picks[:n] {
		mark := r.success.Render("✓")
		if !s.results[i] {
			mark = r.danger.Render("✗")
		}
		inner = append(inner, "  "+mark+" "+r.value.Render(pad(p.Concept.Title, max(ci-24, 10)))+" "+
			r.label.Render(pad(a.shortTrack(p.Concept.Track), 9))+" "+a.stateWord(r, p.Concept))
	}
	inner = append(inner, "", r.reverse.Render("  enter  back to Home  "), "")
	main := append([]string{""}, box(inner, w, r.success.Bold(true).Render("✓ Session complete"), "", r.rule, r.base)...)
	p := page{main: main, ctx: "session complete"}
	if L.three {
		p.right = a.homeContext(L.ri)
	}
	return p
}

func (a App) studyHints() []string {
	s := a.session
	switch {
	case s.done() && a.exam != nil:
		return []string{"enter tracks", "tab screens", "? help", "q quit"}
	case len(s.picks) == 0 || s.done():
		return []string{"enter home", "tab screens", "? help", "q quit"}
	case s.q.answered:
		hints := []string{"enter next"}
		if s.q.explain == explainIdle || s.q.explain == explainFailed {
			hints = append(hints, "e ask the tutor")
		}
		return append(hints, "1-4 screens", "? help", "q quit")
	}
	switch s.q.kind {
	case pickOne:
		return []string{"1-9 answer", "j/k move", "enter submit", "h hint", "? help"}
	case pickMany:
		return []string{"space toggle", "j/k move", "enter submit", "h hint", "? help"}
	case typed:
		if s.q.pick.Item.Type == domain.Output {
			return []string{"enter submit", "alt+enter new line", "ctrl+t hint", "esc home"}
		}
		return []string{"enter submit", "ctrl+t hint", "esc home"}
	case blanks:
		return []string{"tab next blank", "enter next/submit", "ctrl+t hint", "esc home"}
	case reorder:
		return []string{"j/k select", "J/K move line", "enter submit", "h hint", "? help"}
	case pickLine:
		return []string{"j/k select line", "enter submit", "h hint", "? help"}
	case editor:
		if s.q.running {
			return []string{"running…", "ctrl+c quit"}
		}
		return []string{"ctrl+r run & submit", "enter new line", "tab indent", "ctrl+t hint", "esc home"}
	}
	return nil
}
