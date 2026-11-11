package tui

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// homeAction is the single preselected action on Home.
type homeAction struct {
	verb   string // the hint: "Start session"
	label  string // "Start session"
	target Screen
}

// secondsPerQuestion is the time estimate behind "about N minutes" on Home.
const secondsPerQuestion = 45

func (a App) homeAction() homeAction {
	s := a.session
	switch {
	case a.exam != nil && !s.done():
		return homeAction{verb: "Continue exam", label: "Continue the exam of " + a.exam.section.Title, target: Study}
	case len(s.picks) > 0 && !s.done():
		verb := "Start session"
		if s.pos > 0 || s.q.answered {
			verb = "Continue session"
		}
		return homeAction{verb: verb, label: verb, target: Study}
	case s.done() && len(s.picks) > 0:
		return homeAction{verb: "Browse tracks", label: "Session complete — browse tracks", target: Tracks}
	}
	return homeAction{verb: "Browse tracks", label: "Nothing due — browse tracks", target: Tracks}
}

func (a App) homePage(L layout) page {
	p := page{ctx: fmt.Sprintf("%d concepts · %d tracks · max new/day %d (--new)",
		len(a.cat.Concepts), len(a.cat.Tracks), a.maxNew)}
	if L.three {
		p.right = a.homeContext(L.ri)
	}
	s := a.session
	if len(s.picks) == 0 || s.done() {
		p.main = a.caughtUp(L)
	} else {
		p.main = a.todayCard(L)
	}
	if tools := a.toolchainLines(L.mi); len(tools) > 0 && len(p.main) > 0 {
		// right under the first card, so a short terminal still shows it
		at := 1
		for i, l := range p.main {
			if strings.HasPrefix(ansi.Strip(l), "└") {
				at = i + 1
				break
			}
		}
		p.main = slices.Concat(p.main[:at], append([]string{""}, tools...), p.main[at:])
	}
	return p
}

// ahead are the picks not answered yet.
func (a App) ahead() []srs.Pick {
	s := a.session
	from := s.pos
	if s.q.answered {
		from++
	}
	return s.picks[min(from, len(s.picks)):]
}

// todayCard is Home with a session to do: the Today card and its one
// action, the queue in order, and what today's answers unlock.
func (a App) todayCard(L layout) []string {
	r := a.st.main
	w := L.mi
	ci := w - 4
	act := a.homeAction()
	left := a.ahead()
	n := len(left)
	mins := int(math.Ceil(float64(n*secondsPerQuestion) / 60))

	var reviews, fresh []string
	tracks := map[string]bool{}
	for _, p := range left {
		if p.New {
			fresh = append(fresh, p.Concept.Title)
		} else {
			reviews = append(reviews, p.Concept.Title)
			tracks[p.Concept.Track] = true
		}
	}
	var plan string
	rv := fmt.Sprintf("%d %s due across %d %s", len(reviews), plural(len(reviews), "review", "reviews"),
		len(tracks), plural(len(tracks), "track", "tracks"))
	switch {
	case len(reviews) > 0 && len(fresh) > 0:
		plan = fmt.Sprintf("%s, then %d new: %s.", rv, len(fresh), joinAnd(fresh))
	case len(reviews) > 0:
		plan = rv + "."
	default:
		plan = fmt.Sprintf("%d new %s: %s.", len(fresh), plural(len(fresh), "concept", "concepts"), joinAnd(fresh))
	}
	count := fmt.Sprintf("%d %s", n, plural(n, "question", "questions"))
	if act.verb == "Continue session" {
		count += " left"
	}
	inner := []string{"", r.strong.Render(count) + r.secondary.Render(fmt.Sprintf(" · about %d %s", mins, plural(mins, "minute", "minutes")))}
	inner = append(inner, wrapLines(plan, ci, r.secondary)...)
	inner = append(inner, "", r.reverse.Render("  ▸ "+act.label+"  ")+"  "+r.label.Render("enter"), "")

	out := append([]string{""}, box(inner, w, r.strong.Render("Today"), r.label.Render(a.now().Format("Monday 2 Jan")), r.rule, r.base)...)

	unlocks := a.unlockLines(w)
	room := L.body - len(out) - 3 - len(unlocks) - 2
	out = append(out, "", a.heading("Up next", "in session order", w))
	nameW := max(w-26, 10)
	out = append(out, r.label.Render("   #  "+pad("concept", nameW)+" "+pad("track", 8)+" state"))
	for i, p := range left {
		if i >= max(room, 1) && len(left) > room {
			out = append(out, r.label.Render(fmt.Sprintf("  …  %d more", len(left)-i)))
			break
		}
		out = append(out, "  "+r.label.Render(fmt.Sprintf("%2d", i+1))+"  "+r.value.Render(pad(p.Concept.Title, nameW))+" "+
			r.secondary.Render(pad(a.shortTrack(p.Concept.Track), 8))+" "+a.stateWord(r, p.Concept))
	}
	out = append(out, "", a.heading("Unlocks after today", "", w))
	return append(out, unlocks...)
}

// unlockLines name the locked concepts that open if today's remaining
// concepts are answered correctly.
func (a App) unlockLines(w int) []string {
	r := a.st.main
	after := maps.Clone(a.states)
	ahead := map[string]bool{}
	for _, p := range a.ahead() {
		ahead[p.Concept.ID] = true
		after[p.Concept.ID] = max(after[p.Concept.ID], domain.StateLearning)
	}
	var out []string
	shown, more := 0, 0
	for _, c := range a.cat.Concepts {
		if !a.locked(c) || !srs.Unlocked(c, after) {
			continue
		}
		if shown == 2 {
			more++
			continue
		}
		var need []string
		for _, id := range c.Prereqs {
			if a.states[id] < domain.StateLearning && ahead[id] {
				need = append(need, a.conceptTitle(id))
			}
		}
		out = append(out,
			"  "+r.label.Render("answer ")+r.value.Render(cut(joinAnd(need), w-20))+r.label.Render(" correctly"),
			"  "+r.label.Render("→ ")+r.strong.Render(cut(a.shortTrack(c.Track)+" › "+c.Title, w-24))+r.label.Render(" becomes available"))
		shown++
	}
	if more > 0 {
		out = append(out, "  "+r.label.Render(fmt.Sprintf("and %d more", more)))
	}
	if len(out) == 0 {
		out = append(out, "  "+r.label.Render("Nothing new unlocks from today's session."))
	}
	return out
}

// caughtUp is Home (and Study) when nothing is left today: when the next
// review is, what is coming up, and where to go meanwhile.
func (a App) caughtUp(L layout) []string {
	r := a.st.main
	w := L.mi
	now := a.now()
	loc := now.Location()
	today := domain.DateOf(now, loc)
	answered, correct := 0, 0
	for _, answers := range a.history {
		for _, ans := range answers {
			if domain.DateOf(ans.At, loc) == today {
				answered++
				if ans.Correct {
					correct++
				}
			}
		}
	}
	inner := []string{"", r.success.Render("✓") + r.strong.Render(" All caught up for today.")}
	streak := fmt.Sprintf("streak %d %s", a.stats.Streak, plural(a.stats.Streak, "day", "days"))
	if answered > 0 {
		inner = append(inner, r.secondary.Render(fmt.Sprintf("%d answered · %d correct · %s", answered, correct, streak)))
	} else {
		inner = append(inner, r.secondary.Render("Nothing was due today · "+streak))
	}
	inner = append(inner, "")
	coming := a.comingUp()
	if len(coming) > 0 {
		first := coming[0].Due
		n := 0
		for _, sc := range coming {
			if domain.DateOf(sc.Due, loc) == domain.DateOf(first, loc) {
				n++
			}
		}
		inner = append(inner, r.label.Render("Next review  ")+r.strong.Render(first.In(loc).Format(shortDate))+
			r.label.Render(fmt.Sprintf(" · %d %s due", n, plural(n, "concept", "concepts"))))
	}
	why := ""
	switch unlocked := a.unlockedNew(); {
	case a.newToday(now) >= a.maxNew:
		why = fmt.Sprintf(" · daily limit %d reached (--new)", a.maxNew)
	case unlocked == 0:
		why = " · none unlocked yet"
	default:
		why = fmt.Sprintf(" · %d unlocked for your next session", unlocked)
	}
	if len("New concepts 0 left")+widthOf(why) > w-4 {
		why = strings.TrimSuffix(why, " (--new)")
	}
	newLine := r.label.Render("New concepts ") + r.strong.Render("0 left") + r.label.Render(why)
	inner = append(inner, newLine, "")
	out := append([]string{""}, box(inner, w, r.strong.Render("Today"), r.label.Render(now.Format("Monday 2 Jan")), r.rule, r.base)...)

	if len(coming) > 0 {
		out = append(out, "", a.heading("Coming up", "", w))
		nameW := max(min(w-34, 30), 10)
		for i, sc := range coming[:min(len(coming), 4)] {
			c, _ := a.cat.Concept(sc.ConceptID)
			date := r.secondary.Render(sc.Due.In(loc).Format(shortDate))
			if domain.DateOf(sc.Due, loc) == domain.DateOf(coming[0].Due, loc) || i == 0 {
				date = r.strong.Render(sc.Due.In(loc).Format(shortDate))
			}
			out = append(out, "  "+fill(date, 10, r.base)+"  "+r.value.Render(pad(c.Title, nameW))+
				r.secondary.Render(pad(a.shortTrack(c.Track), 8))+a.stateWord(r, c))
		}
	}
	return append(out, "", r.secondary.Render("While you wait:"),
		"  "+r.accent.Render("3")+"  "+r.secondary.Render("Tracks · see what today's answers unlocked"),
		"  "+r.accent.Render("4")+"  "+r.secondary.Render("Stats · review your top misconceptions"))
}

// comingUp are the reviewed concepts of the catalog due after now, soonest first.
func (a App) comingUp() []domain.Schedule {
	now := a.now()
	var out []domain.Schedule
	for _, c := range a.cat.Concepts {
		if sc, ok := a.schedules[c.ID]; ok && sc.Reviewed() && sc.Due.After(now) {
			out = append(out, sc)
		}
	}
	slices.SortStableFunc(out, func(x, y domain.Schedule) int {
		return cmp.Or(x.Due.Compare(y.Due), cmp.Compare(x.ConceptID, y.ConceptID))
	})
	return out
}

// homeContext is the right pane of Home: the last five weeks, the
// misconceptions to watch and the overall totals.
func (a App) homeContext(w int) []string {
	r := a.st.main
	out := []string{a.heading("LAST 5 WEEKS", "", w), r.label.Render("        M  T  W  T  F  S  S")}
	grid, starts := a.weekGrid(5)
	today := a.todayIndex()
	for i, col := range grid {
		row := r.label.Render(pad(starts[i].Format("Jan 2"), 7))
		for d, v := range col {
			if d > 0 {
				row += " "
			}
			row += " " + a.heatCell(v, i == len(grid)-1 && d == today)
		}
		out = append(out, row)
	}
	out = append(out, lr("", r.label.Render("today ")+a.heatCell(grid[len(grid)-1][today], true), w, r.base), a.heatLegend(), "")

	out = append(out, a.heading("WATCH", "misconceptions", w))
	if len(a.stats.TopMisconceptions) == 0 {
		out = append(out, r.label.Render("none yet"))
	}
	for _, m := range a.stats.TopMisconceptions[:min(3, len(a.stats.TopMisconceptions))] {
		out = append(out, lr(r.warning.Render(cut(m.Misconception, w-5)), r.secondary.Render(fmt.Sprintf("%d×", m.Count)), w, r.base))
	}
	mastered, total := 0, 0
	for _, p := range a.stats.Tracks {
		mastered += p.Mastered
		total += p.Total
	}
	return append(out, "", a.heading("OVERALL", "", w),
		lr(r.label.Render("mastered"), r.strong.Render(fmt.Sprint(mastered))+r.label.Render(fmt.Sprintf("/%d concepts", total)), w, r.base),
		lr(r.label.Render("answers"), r.strong.Render(fmt.Sprint(a.stats.TotalAnswers)), w, r.base),
		lr(r.label.Render("accuracy"), r.strong.Render(fmt.Sprintf("%d%%", a.accuracy())), w, r.base))
}

// accuracy is the share of correct answers, 0–100.
func (a App) accuracy() int {
	if a.stats.TotalAnswers == 0 {
		return 0
	}
	return 100 * a.stats.TotalCorrect / a.stats.TotalAnswers
}

// unlockedNew counts the never-studied concepts whose prerequisites are met.
func (a App) unlockedNew() int {
	n := 0
	for _, c := range a.cat.Concepts {
		if a.schedules[c.ID].Reviewed() {
			continue
		}
		if srs.Unlocked(c, a.states) {
			n++
		}
	}
	return n
}

// nextDue is the earliest due date of a reviewed concept after now.
func (a App) nextDue() (time.Time, bool) {
	if c := a.comingUp(); len(c) > 0 {
		return c[0].Due, true
	}
	return time.Time{}, false
}

// daysUntil counts calendar days from now to t in now's time zone.
func (a App) daysUntil(t time.Time) int {
	now := a.now()
	loc := now.Location()
	d0, _ := time.ParseInLocation(time.DateOnly, domain.DateOf(now, loc), loc)
	d1, _ := time.ParseInLocation(time.DateOnly, domain.DateOf(t, loc), loc)
	return int(math.Round(d1.Sub(d0).Hours() / 24))
}

// relDay names a day relative to today: "today", "tomorrow", "Fri 13 Nov (in 3 days)".
func (a App) relDay(t time.Time) string {
	n := a.daysUntil(t)
	date := t.In(a.now().Location()).Format(shortDate)
	switch {
	case n <= 0:
		return "today"
	case n == 1:
		return "tomorrow, " + date
	}
	return fmt.Sprintf("%s (in %d days)", date, n)
}
