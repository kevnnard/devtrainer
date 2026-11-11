package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// glyph marks a concept state.
func glyph(s domain.State) string {
	return [...]string{"○", "◔", "◑", "●"}[s]
}

// locked reports whether c was never answered correctly and still waits
// for a prerequisite.
func (a App) locked(c domain.Concept) bool {
	return a.states[c.ID] == domain.StateNew && !srs.Unlocked(c, a.states)
}

// mark is a concept's state glyph in its color; ⊘ when it is locked.
func (a App) mark(r roles, c domain.Concept) string {
	if a.locked(c) {
		return r.label.Render("⊘")
	}
	return stateStyle(r, a.states[c.ID]).Render(glyph(a.states[c.ID]))
}

// stateStyle is the color of a state: mastered green, review indigo,
// learning amber, new secondary.
func stateStyle(r roles, s domain.State) lipgloss.Style {
	switch s {
	case domain.StateMastered:
		return r.success
	case domain.StateReview:
		return r.info
	case domain.StateLearning:
		return r.warning
	}
	return r.secondary
}

func (a App) stateLabel(s domain.State) string {
	return stateStyle(a.st.main, s).Render(s.String())
}

// stateWord is "◑ review" in the state's color.
func (a App) stateWord(r roles, c domain.Concept) string {
	if a.locked(c) {
		return r.label.Render("⊘ locked")
	}
	s := a.states[c.ID]
	return stateStyle(r, s).Render(glyph(s)) + r.base.Render(" ") + r.secondary.Render(s.String())
}

// shortTrack is a track's title short enough for a table column: the part
// before " & ", else the first word.
func (a App) shortTrack(id string) string {
	t, ok := a.cat.Track(id)
	if !ok {
		return id
	}
	if before, _, ok := strings.Cut(t.Title, " & "); ok {
		return before
	}
	first, _, _ := strings.Cut(t.Title, " ")
	return first
}

// missingPrereqs are the titles of the prerequisites of c not yet learning.
func (a App) missingPrereqs(c domain.Concept) []string {
	var out []string
	for _, id := range c.Prereqs {
		if a.states[id] >= domain.StateLearning {
			continue
		}
		out = append(out, a.conceptTitle(id))
	}
	return out
}

func (a App) conceptTitle(id string) string {
	if c, ok := a.cat.Concept(id); ok {
		return c.Title
	}
	return id
}

// prereqName names a prerequisite of a concept in track: its title, with
// "Track › " in front when it lives in another track.
func (a App) prereqName(id, track string) string {
	c, ok := a.cat.Concept(id)
	if !ok {
		return id
	}
	if c.Track != track {
		return a.shortTrack(c.Track) + " › " + c.Title
	}
	return c.Title
}

// ---- the track explorer (left pane) ----

// sideOpts is how the explorer shows on a screen.
type sideOpts struct {
	cur    string // highlighted track ID
	expand string // track whose concepts are listed under it
	at     string // concept on screen, marked ▸
	focus  bool   // the explorer is the picker (Tracks)
}

// meterWidth is the width of the per-track stacked meter.
const meterWidth = 8

func (a App) sidebar(L layout, o sideOpts) []string {
	r := a.st.main
	w := L.sb
	mastered, total := 0, 0
	for _, p := range a.stats.Tracks {
		mastered += p.Mastered
		total += p.Total
	}
	head := lr(r.label.Render("▾")+" "+r.strong.Render("TRACKS"),
		r.label.Render(fmt.Sprintf("%d/%d mastered", mastered, total)), w-2, r.base)
	if o.focus {
		head = lr(r.accent.Render("▾")+" "+r.title.Render("TRACKS"),
			r.accent.Render("j/k")+r.label.Render(" to choose"), w-2, r.base)
	}
	lines := []string{" " + head}
	nameW := max(w-16, 6)
	for _, p := range a.stats.Tracks {
		sel := p.Track.ID == o.cur
		rr := r
		mark, name := rr.base.Render(" "), rr.value
		if sel {
			rr = a.st.sel
			mark, name = rr.selected.Render("▸"), rr.strong
		}
		row := mark + name.Render(pad(a.sideName(p.Track, nameW), nameW)) + rr.base.Render(" ") +
			a.trackMeter(rr, p, meterWidth) + rr.value.Render(fmt.Sprintf(" %3.0f%%", p.MasteryPercent()))
		lines = append(lines, fill(row, w, rr.base))
		if p.Track.ID != o.expand {
			continue
		}
		for _, c := range a.cat.ConceptsIn(p.Track.ID) {
			here := c.ID == o.at
			cur, title := " ", r.secondary.Render(cut(c.Title, w-7))
			if here {
				cur, title = r.accent.Render("▸"), r.accent.Render(cut(c.Title, w-7))
			}
			lines = append(lines, "  "+cur+" "+a.mark(r, c)+" "+title)
		}
	}
	legend := []string{
		" " + r.rule.Render(strings.Repeat("─", max(w-2, 0))),
		" " + r.success.Render("● █") + " " + r.secondary.Render("mastered") + "  " +
			r.info.Render("◑ ▓") + " " + r.secondary.Render("review"),
		" " + r.warning.Render("◔ ░") + " " + r.secondary.Render("learning") + "  " +
			r.secondary.Render("○") + " " + r.secondary.Render("new") + "  " +
			r.label.Render("⊘") + " " + r.secondary.Render("locked"),
	}
	top := max(L.body-len(legend), 0)
	lines = lines[:min(len(lines), top)]
	for len(lines) < top {
		lines = append(lines, "")
	}
	return append(lines, legend...)
}

// sideName is the track title, or its short form when it does not fit.
func (a App) sideName(t domain.Track, w int) string {
	if len([]rune(t.Title)) <= w {
		return t.Title
	}
	return a.shortTrack(t.ID)
}

// trackMeter is the stacked meter of a track: █ mastered, ▓ review,
// ░ learning, ─ the rest, so glyph density carries the meaning.
func (a App) trackMeter(r roles, p domain.TrackProgress, n int) string {
	if p.Total == 0 {
		return r.faint.Render(strings.Repeat("─", n))
	}
	cells := func(k int) int { return int(float64(k)/float64(p.Total)*float64(n) + 0.5) }
	m := cells(p.Mastered)
	v := cells(p.Mastered+p.Review) - m
	l := cells(p.Mastered+p.Review+p.Learning) - m - v
	return r.success.Render(strings.Repeat("█", m)) + r.info.Render(strings.Repeat("▓", v)) +
		r.warning.Render(strings.Repeat("░", l)) + r.faint.Render(strings.Repeat("─", max(n-m-v-l, 0)))
}

// ---- the Tracks screen ----

func (a App) tracksPage(L layout) page {
	r := a.st.main
	if len(a.cat.Tracks) == 0 {
		return page{main: []string{"", r.secondary.Render("No tracks loaded.")}}
	}
	sel := min(a.track, len(a.cat.Tracks)-1)
	t := a.cat.Tracks[sel]
	concepts := a.trackConcepts(t.ID)
	ci := L.mi - 4

	var inner []string
	if t.Description != "" {
		inner = append(inner, wrapLines(t.Description, ci, r.secondary)...)
		inner = append(inner, "")
	}
	levelW := 13
	nLocked, nMastered := 0, 0
	at, _ := a.selectedConcept()
	for gi, g := range a.trackGroups(t.ID) {
		if g.sec != nil {
			if gi > 0 {
				inner = append(inner, "")
			}
			inner = append(inner, a.sectionHeader(*g.sec, ci))
		} else if gi > 0 {
			inner = append(inner, "", r.label.Render("not in a section"))
		}
		for _, c := range g.concepts {
			inner = a.conceptRow(inner, c, at, ci, levelW, &nLocked, &nMastered)
		}
	}
	note := fmt.Sprintf("%d/%d mastered", nMastered, len(concepts))
	if nLocked > 0 {
		note += fmt.Sprintf(" · %d locked", nLocked)
	}
	main := append([]string{""}, box(inner, L.mi, r.strong.Render(t.Title), r.label.Render(note), r.rule, r.base)...)
	if at.Lesson != nil {
		main = append(main, " "+r.accent.Render("l")+r.label.Render(" opens the lesson of ")+r.strong.Render(cut(at.Title, L.mi-24)))
	} else if at.ID != "" {
		main = append(main, " "+r.label.Render(cut(at.Title+" has no lesson yet.", L.mi-2)))
	}
	if line := a.examLine(at, L.mi); line != "" {
		main = append(main, line)
	}
	if a.note != "" {
		main = append(main, wrapLines(a.note, L.mi-2, r.warning)...)
	}
	main = append(main, "", a.heading("Fastest unlock", "", L.mi))
	main = append(main, a.fastestUnlock(concepts, L.mi)...)

	p := page{
		main: main,
		side: sideOpts{cur: t.ID, focus: true},
		ctx:  fmt.Sprintf("track %d of %d · %s · %d %s", sel+1, len(a.cat.Tracks), t.ID, len(concepts), plural(len(concepts), "concept", "concepts")),
	}
	if L.three {
		p.right = a.prereqPath(t, concepts, L.ri)
	}
	return p
}

// conceptRow appends the Tracks rows of concept c (its line and what it
// needs) to inner, counting it as locked or mastered.
func (a App) conceptRow(inner []string, c domain.Concept, at domain.Concept, ci, levelW int, nLocked, nMastered *int) []string {
	r := a.st.main
	title := r.strong
	if a.locked(c) {
		title = r.secondary
		*nLocked++
	}
	if a.states[c.ID] == domain.StateMastered {
		*nMastered++
	}
	cur := "  "
	if c.ID == at.ID {
		cur = r.accent.Render("▸") + " "
	}
	tag, tagW := "", 0
	if c.Lesson != nil {
		tag, tagW = r.label.Render(" · lesson"), 9
	}
	nameW := ci - 4 - levelW
	inner = append(inner, cur+a.mark(r, c)+" "+fill(title.Render(cut(c.Title, max(nameW-tagW, 4)))+tag, nameW, r.base)+
		r.label.Render(string(c.Level)))
	return append(inner, indent(a.needsLines(c, ci-2))...)
}

// selectedConcept is the concept under the cursor of the Tracks screen.
func (a App) selectedConcept() (domain.Concept, bool) {
	if len(a.cat.Tracks) == 0 {
		return domain.Concept{}, false
	}
	cs := a.trackConcepts(a.cat.Tracks[min(a.track, len(a.cat.Tracks)-1)].ID)
	if len(cs) == 0 {
		return domain.Concept{}, false
	}
	return cs[min(a.concept, len(cs)-1)], true
}

// tracksKey handles the Tracks keys: j/k the track, J/K the concept,
// l or enter the concept's lesson, x the exam of the concept's section.
func (a App) tracksKey(msg tea.KeyPressMsg) App {
	a.note = ""
	n := 0
	if len(a.cat.Tracks) > 0 {
		n = len(a.trackConcepts(a.cat.Tracks[min(a.track, len(a.cat.Tracks)-1)].ID))
	}
	switch msg.String() {
	case "j", "down":
		a.track = min(a.track+1, max(len(a.cat.Tracks)-1, 0))
		a.concept = 0
	case "k", "up":
		a.track = max(a.track-1, 0)
		a.concept = 0
	case "J", "shift+j", "shift+down":
		a.concept = min(a.concept+1, max(n-1, 0))
	case "K", "shift+k", "shift+up":
		a.concept = max(a.concept-1, 0)
	case "l", "enter":
		if c, ok := a.selectedConcept(); ok && c.Lesson != nil {
			a.lesson = newLesson(c, Tracks)
		}
	case "x":
		if c, ok := a.selectedConcept(); ok {
			if sec, ok := a.cat.SectionOf(c.ID); ok {
				return a.startExam(sec)
			}
			a.note = c.Title + " is in no section, so it has no exam."
		}
	}
	return a
}

func (a App) tracksHints() []string {
	hints := []string{"j/k track", "J/K concept"}
	if c, ok := a.selectedConcept(); ok && c.Lesson != nil {
		hints = append(hints, "l lesson")
	}
	if c, ok := a.selectedConcept(); ok {
		if _, ok := a.cat.SectionOf(c.ID); ok {
			hints = append(hints, "x exam")
		}
	}
	return append(hints, "tab screens", "? help", "q quit")
}

// needsLines name every prerequisite of c with ✓ met or ✗ missing, so a
// lock always shows its way out.
func (a App) needsLines(c domain.Concept, w int) []string {
	r := a.st.main
	if len(c.Prereqs) == 0 {
		return nil
	}
	var out []string
	line, n := "  "+r.label.Render("needs")+" ", 0
	indent := "        "
	for _, id := range c.Prereqs {
		ok := a.states[id] >= domain.StateLearning
		g := r.danger.Render("✗")
		if ok {
			g = r.success.Render("✓")
		}
		part := g + " " + r.value.Render(a.prereqName(id, c.Track))
		if n > 0 && widthOf(line)+2+widthOf(part) > w {
			out = append(out, line)
			line, n = indent, 0
		}
		if n > 0 {
			line += "  "
		}
		line += part
		n++
	}
	return append(out, line)
}

// fastestUnlock names the locked concept of the track with the fewest
// missing prerequisites and what to answer to open it.
func (a App) fastestUnlock(concepts []domain.Concept, w int) []string {
	r := a.st.main
	var best domain.Concept
	var missing []string
	for _, c := range concepts {
		if !a.locked(c) {
			continue
		}
		var m []string
		for _, id := range c.Prereqs {
			if a.states[id] < domain.StateLearning {
				m = append(m, id)
			}
		}
		if best.ID == "" || len(m) < len(missing) {
			best, missing = c, m
		}
	}
	if best.ID == "" {
		return []string{"  " + r.label.Render("Nothing is locked in this track.")}
	}
	names := make([]string, len(missing))
	var inSession []string
	for i, id := range missing {
		names[i] = a.prereqName(id, best.Track)
		if a.inSession(id) {
			inSession = append(inSession, a.conceptTitle(id))
		}
	}
	out := []string{
		"  " + r.label.Render("answer ") + r.strong.Render(joinAnd(names)) + r.label.Render(" correctly"),
		"  " + r.label.Render("→ ") + r.strong.Render(best.Title) + r.label.Render(" opens"),
	}
	if len(inSession) > 0 {
		out = append(out, "  "+r.secondary.Render(cut(joinAnd(inSession)+" "+plural(len(inSession), "is", "are")+" in today's session", w-2)))
	}
	return out
}

// inSession reports whether concept id is still ahead in today's session.
func (a App) inSession(id string) bool {
	s := a.session
	for i := s.pos; i < len(s.picks); i++ {
		if i == s.pos && s.q.answered {
			continue
		}
		if s.picks[i].Concept.ID == id {
			return true
		}
	}
	return false
}

// prereqPath is the context pane of Tracks: each prerequisite the track
// depends on and the concepts it opens, then the track's state counts.
func (a App) prereqPath(t domain.Track, concepts []domain.Concept, w int) []string {
	r := a.st.main
	out := []string{a.heading("PREREQUISITE PATH", "", w)}
	var roots []string
	for _, c := range concepts {
		for _, id := range c.Prereqs {
			if !slices.Contains(roots, id) {
				roots = append(roots, id)
			}
		}
	}
	if len(roots) == 0 {
		out = append(out, r.label.Render("No prerequisites in this track."))
	}
	for i, id := range roots {
		if i > 0 {
			out = append(out, "")
		}
		root, _ := a.cat.Concept(id)
		name := root.Title
		if root.Track != t.ID {
			name = r.secondary.Render(a.shortTrack(root.Track)+" ›") + " " + r.value.Render(root.Title)
		}
		out = append(out, cut(name, w-2)+" "+a.mark(r, root))
		var kids []domain.Concept
		for _, c := range concepts {
			if slices.Contains(c.Prereqs, id) {
				kids = append(kids, c)
			}
		}
		for j, c := range kids {
			branch := "├─▶"
			if j == len(kids)-1 {
				branch = "└─▶"
			}
			out = append(out, r.rule.Render(branch)+" "+cut(c.Title, w-6)+" "+a.mark(r, c))
		}
	}
	counts := map[string]int{}
	for _, c := range concepts {
		switch {
		case a.locked(c):
			counts["locked"]++
		default:
			counts[a.states[c.ID].String()]++
		}
	}
	out = append(out, "", a.heading("THIS TRACK", fmt.Sprintf("%d concepts", len(concepts)), w))
	for _, s := range []domain.State{domain.StateMastered, domain.StateReview, domain.StateLearning, domain.StateNew} {
		out = append(out, lr(stateStyle(r, s).Render(glyph(s))+" "+r.secondary.Render(s.String()),
			r.strong.Render(fmt.Sprint(counts[s.String()])), w, r.base))
	}
	out = append(out, lr(r.label.Render("⊘")+" "+r.secondary.Render("locked"), r.strong.Render(fmt.Sprint(counts["locked"])), w, r.base))
	return out
}

// joinAnd joins names as "a, b and c".
func joinAnd(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
