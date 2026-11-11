package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The page is a body above a two-row bottom bar (no top bar). At
// paneBreak columns or more the body has three panes: the track explorer
// on the left, the focus card in the center and the context pane on the
// right. Below it only the center remains, two cells in from each side.
const (
	paneBreak = 100

	// Pane widths at the 120-column reference; wider terminals give the
	// extra columns to the center and, a quarter of them, to the right pane.
	sideWidth    = 32
	contextWidth = 30
	contextMax   = 40
	sideMin      = 30 // at paneBreak columns
	contextMin   = 26

	narrowMargin = 2
	fullMax      = 100 // widest column for help and errors

	// Used until the terminal reports its size.
	defaultWidth  = 80
	defaultHeight = 24
)

func (a App) size() (w, h int) {
	if a.width == 0 || a.height == 0 {
		return defaultWidth, defaultHeight
	}
	return a.width, a.height
}

// layout is the geometry of one frame.
type layout struct {
	w, h  int
	body  int  // rows above the bottom bar
	three bool // side panes on screen
	sb    int  // explorer width
	mw    int  // center pane width, separators excluded
	rw    int  // context pane width
	mi    int  // usable width inside the center pane
	ri    int  // usable width inside the context pane
}

func (a App) layout() layout {
	w, h := a.size()
	L := layout{w: w, h: h, body: max(h-2, 0)}
	if w < paneBreak {
		L.mi = max(w-2*narrowMargin, 10)
		return L
	}
	L.three = true
	if w < 120 {
		// Between the break and the reference the side panes give up a
		// few columns (the legend still fits in 30), the card the rest.
		L.sb, L.rw = sideMin+(w-paneBreak)/10, contextMin+(w-paneBreak)/5
	} else {
		L.sb, L.rw = sideWidth, min(contextWidth+(w-120)/4, contextMax)
	}
	L.mw = w - L.sb - L.rw - 2
	L.mi, L.ri = L.mw-2, L.rw-2
	return L
}

// page is what a screen puts in the frame.
type page struct {
	main  []string // the center pane (the only one below paneBreak)
	right []string // the context pane
	side  sideOpts // how the explorer shows
	full  bool     // one centered column at every width (help, errors)
	ctx   string   // bottom bar, row 2, left: where you are
}

// render paints the whole terminal.
func (a App) render() string {
	L := a.layout()
	var p page
	switch {
	case a.err != nil:
		p = a.errorPage(L)
	case a.help:
		p = a.helpPage(L)
	default:
		p = a.screenPage(L)
	}
	rows := append(a.frame(L, p), a.bottomBar(L, p)...)
	return strings.Join(rows[:min(len(rows), max(L.h, 1))], "\n")
}

func (a App) screenPage(L layout) page {
	if a.lessonOn() {
		return a.lessonPage(L)
	}
	switch a.screen {
	case Study:
		return a.studyPage(L)
	case Tracks:
		return a.tracksPage(L)
	case Stats:
		return a.statsPage(L)
	default:
		return a.homePage(L)
	}
}

// frame lays the panes out into exactly L.body rows of exactly L.w columns.
func (a App) frame(L layout, p page) []string {
	base := a.st.main.base
	out := make([]string, L.body)
	if p.full || !L.three {
		cw := L.mi
		left := narrowMargin
		if p.full && L.w > fullMax+2*narrowMargin {
			cw, left = fullMax, (L.w-fullMax)/2
		} else if p.full {
			cw = max(L.w-2*narrowMargin, 10)
		}
		margin := strings.Repeat(" ", left)
		for i, l := range blockLines(p.main, cw, L.body, base) {
			out[i] = fill(margin+l, L.w, base)
		}
		return out
	}
	sep := a.st.main.rule.Render("│")
	side := blockLines(a.sidebar(L, p.side), L.sb, L.body, base)
	main := blockLines(p.main, L.mi, L.body, base)
	right := blockLines(p.right, L.ri, L.body, base)
	for i := range out {
		out[i] = side[i] + sep + " " + main[i] + " " + sep + " " + right[i] + " "
	}
	return out
}

func (a App) errorPage(L layout) page {
	r := a.st.main
	what := a.errWhat
	if what == "" {
		what = "your answer"
	}
	main := append([]string{""}, wrapLines("✗ Could not save "+what+": "+a.err.Error(), L.mi, r.danger)...)
	main = append(main, "", r.label.Render("Your earlier answers are saved. Press any key to quit."))
	return page{main: main, full: true}
}

// bottomBar is the two-row bar: name, tabs and today's facts (or the
// session meter) on row 1; where you are and the keys on row 2.
func (a App) bottomBar(L layout, p page) []string {
	r := a.st.chrome
	narrow := !L.three
	sep := r.rule.Render(" │ ")

	tabs := make([]string, len(screens))
	for i, s := range screens {
		label := fmt.Sprintf("%d %s", i+1, s)
		switch {
		case s == a.screen && !a.help:
			tabs[i] = r.reverse.Render(" " + label + " ")
		case narrow:
			tabs[i] = r.label.Render(fmt.Sprint(i+1)) + r.base.Render(" ") + r.secondary.Render(s.String())
		default:
			tabs[i] = r.base.Render(" ") + r.label.Render(fmt.Sprint(i+1)) + r.base.Render(" ") +
				r.secondary.Render(s.String()) + r.base.Render(" ")
		}
	}
	gap := " "
	if narrow {
		gap = "  "
	}
	left1 := r.base.Render(" ") + r.title.Render("devtrainer") + sep + strings.Join(tabs, r.base.Render(gap))
	room := L.w - ansi.StringWidth(left1) - 2
	right1 := fitParts(a.barFacts(narrow, room), sep, room)
	row1 := lr(left1, right1+r.base.Render(" "), L.w, r.base)

	left2 := r.base.Render(" ")
	if !narrow && p.ctx != "" {
		left2 += r.label.Render(p.ctx)
	}
	hints := a.hints()
	right2 := ""
	for n := len(hints); n > 0; n-- {
		right2 = keyHints(r, hints[:n]) + r.base.Render(" ")
		if ansi.StringWidth(left2)+ansi.StringWidth(right2)+3 <= L.w {
			break
		}
		right2 = ""
	}
	if ansi.StringWidth(left2)+ansi.StringWidth(right2)+3 > L.w {
		left2 = r.base.Render(" ")
	}
	return []string{row1, lr(left2, right2, L.w, r.base)}
}

// barFacts are the parts of row 1's right side, most important first:
// the session meter while studying, today's counters elsewhere.
func (a App) barFacts(narrow bool, room int) []string {
	r := a.st.chrome
	now := a.now()
	streak := r.label.Render("streak ") + r.strong.Render(fmt.Sprintf("%dd", a.stats.Streak))
	s := a.session
	if a.lessonOn() && !a.help {
		parts := []string{a.lessonMeter(r)}
		if a.screen == Study && len(s.picks) > 0 {
			parts = append(parts, a.sessionMeter(r, !narrow, 10))
		}
		return append(parts, streak)
	}
	if a.screen == Study && len(s.picks) > 0 && !a.help {
		meter := a.sessionMeter(r, !narrow, min(room-20, 30))
		if narrow {
			return []string{meter}
		}
		return []string{meter, streak, r.label.Render(now.Format("Mon 2 Jan"))}
	}
	reviews, fresh := a.sessionCounts()
	counts := r.strong.Render(fmt.Sprint(reviews)) + r.label.Render(" due · ") +
		r.strong.Render(fmt.Sprint(fresh)) + r.label.Render(" new")
	if narrow {
		return []string{counts, streak}
	}
	if best := a.bestStreak(); best > 0 {
		streak += r.label.Render(fmt.Sprintf(" · best %d", best))
	}
	return []string{counts, streak, r.label.Render(now.Format("Mon 2 Jan 2006"))}
}

// fitParts joins parts with sep, dropping parts from the end until the
// result fits in w columns.
func fitParts(parts []string, sep string, w int) string {
	for n := len(parts); n > 0; n-- {
		s := strings.Join(parts[:n], sep)
		if ansi.StringWidth(s) <= w {
			return s
		}
	}
	return ""
}

// sessionMeter is "[✓✓●·······] 3/10 ✓2 ✗0": one mark per pick (answered,
// current, to come), the position and the score. At most maxMarks marks
// show, in a window around the current pick.
func (a App) sessionMeter(r roles, label bool, maxMarks int) string {
	s := a.session
	n := len(s.picks)
	maxMarks = max(maxMarks, 5)
	from, to := 0, n
	if n > maxMarks {
		from = min(max(s.pos-maxMarks/2, 0), n-maxMarks)
		to = from + maxMarks
	}
	var marks strings.Builder
	for i := from; i < to; i++ {
		switch {
		case i < len(s.results) && s.results[i]:
			marks.WriteString(r.success.Render("✓"))
		case i < len(s.results):
			marks.WriteString(r.danger.Render("✗"))
		case i == s.pos:
			marks.WriteString(r.selected.Render("●"))
		default:
			marks.WriteString(r.label.Render("·"))
		}
	}
	out := ""
	if label {
		out = r.accent.Render("session") + r.base.Render(" ")
	}
	wrong := len(s.results) - s.correct
	return out + r.label.Render("[") + marks.String() + r.label.Render("]") + r.base.Render(" ") +
		r.strong.Render(fmt.Sprintf("%d/%d", min(s.pos+1, n), n)) + r.base.Render(" ") +
		r.success.Render(fmt.Sprintf("✓%d", s.correct)) + r.base.Render(" ") +
		r.danger.Render(fmt.Sprintf("✗%d", wrong))
}

// keyHints renders "1-9 answer · j/k move": the key in accent, what it
// does in the secondary color.
func keyHints(r roles, hints []string) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		k, what, _ := strings.Cut(h, " ")
		parts[i] = r.accent.Render(k) + r.base.Render(" ") + r.secondary.Render(what)
	}
	return strings.Join(parts, r.label.Render(" · "))
}

// sessionCounts are the reviews and new concepts left in the session.
func (a App) sessionCounts() (reviews, fresh int) {
	s := a.session
	for i := s.pos; i < len(s.picks); i++ {
		if i == s.pos && s.q.answered {
			continue
		}
		if s.picks[i].New {
			fresh++
		} else {
			reviews++
		}
	}
	return reviews, fresh
}

// hints are the keys worth showing for what is on screen now.
func (a App) hints() []string {
	switch {
	case a.err != nil:
		return []string{"any key quit"}
	case a.help:
		return []string{"? close", "esc close"}
	}
	if a.lessonOn() {
		return a.lessonHints()
	}
	switch a.screen {
	case Home:
		return []string{"enter " + strings.ToLower(a.homeAction().verb), "1-4 screens", "? help", "q quit"}
	case Study:
		return a.studyHints()
	case Tracks:
		return a.tracksHints()
	default:
		return []string{"tab screens", "esc home", "? help", "q quit"}
	}
}

// ---- cell helpers: every one returns lines of an exact width ----

// blockLines turns lines into exactly h lines of exactly w columns: longer
// lines are cut, shorter ones padded with pad's background. This
// guarantees that no line is ever wider than its pane.
func blockLines(lines []string, w, h int, pad lipgloss.Style) []string {
	out := make([]string, h)
	for i := range out {
		var l string
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = fill(l, w, pad)
	}
	return out
}

// fill cuts s to w columns (ending in …) or pads it with base's background.
func fill(s string, w int, base lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += base.Render(strings.Repeat(" ", gap))
	}
	return s
}

// lr puts l on the left and r on the right of w columns. When both do not
// fit, l is cut first; r is dropped when it alone is too wide.
func lr(l, r string, w int, base lipgloss.Style) string {
	lw, rw := ansi.StringWidth(l), ansi.StringWidth(r)
	if rw > w {
		return fill(l, w, base)
	}
	if lw+rw+1 > w && lw > 0 {
		l = fill(l, max(w-rw-1, 0), base)
		lw = ansi.StringWidth(l)
	}
	return l + base.Render(strings.Repeat(" ", max(w-lw-rw, 0))) + r
}

// centered centers s in w columns.
func centered(s string, w int) string {
	g := max(w-ansi.StringWidth(s), 0)
	return strings.Repeat(" ", g/2) + s
}

// paint fills s to w columns on the background of rr (a focused row, a
// code line).
func paint(s string, w int, rr roles) string {
	return fill(s, w, rr.base)
}

// box draws a card of w columns around lines: the title in the top
// border on the left, right on the right, border in the border style.
func box(lines []string, w int, title, right string, border, base lipgloss.Style) []string {
	if w < 6 {
		return lines
	}
	left := border.Render("┌─") + " " + title + " "
	rp := border.Render("┐")
	if right != "" {
		rp = " " + right + " " + border.Render("─┐")
	}
	if ansi.StringWidth(left)+ansi.StringWidth(rp) > w {
		rp = border.Render("─┐")
		left = fill(left, w-ansi.StringWidth(rp)-1, base) + " "
	}
	top := left + border.Render(strings.Repeat("─", max(w-ansi.StringWidth(left)-ansi.StringWidth(rp), 0))) + rp
	out := make([]string, 0, len(lines)+2)
	out = append(out, top)
	for _, l := range lines {
		out = append(out, border.Render("│")+" "+fill(l, w-4, base)+" "+border.Render("│"))
	}
	return append(out, border.Render("└"+strings.Repeat("─", w-2)+"┘"))
}

// heading is a section title: "▾ Title" on the left, a muted note on the right.
func (a App) heading(title, note string, w int) string {
	r := a.st.main
	right := ""
	if note != "" {
		right = r.label.Render(note)
	}
	return lr(r.label.Render("▾")+" "+r.strong.Render(title), right, w, r.base)
}

// wrapLines word-wraps s to w columns and styles each line.
func wrapLines(s string, w int, st lipgloss.Style) []string {
	ls := strings.Split(wrap(s, w), "\n")
	for i, l := range ls {
		ls[i] = st.Render(l)
	}
	return ls
}

// wrap word-wraps s to w columns.
func wrap(s string, w int) string {
	return ansi.Wordwrap(s, max(w, 10), "")
}

// plural picks the singular or plural word for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// pad right-pads s with spaces to w columns, cutting it when longer.
func pad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// widthOf is the width of s in cells.
func widthOf(s string) int { return ansi.StringWidth(s) }

// cut shortens s to w columns, ending in … when it was longer.
func cut(s string, w int) string {
	return ansi.Truncate(s, max(w, 1), "…")
}
