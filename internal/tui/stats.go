package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// heatWeeks is how far back the Stats heatmap goes.
const heatWeeks = 26

// activity counts the answers of every study day (YYYY-MM-DD in now's
// time zone), from the stored answer history.
func (a App) activity() map[string]int {
	loc := a.now().Location()
	out := map[string]int{}
	for _, answers := range a.history {
		for _, ans := range answers {
			out[domain.DateOf(ans.At, loc)]++
		}
	}
	return out
}

// bestStreak is the longest run of consecutive study days.
func (a App) bestStreak() int {
	loc := a.now().Location()
	var days []time.Time
	for d := range a.activity() {
		t, err := time.ParseInLocation(time.DateOnly, d, loc)
		if err == nil {
			days = append(days, t)
		}
	}
	slices.SortFunc(days, func(x, y time.Time) int { return x.Compare(y) })
	best, run := 0, 0
	for i, d := range days {
		if i > 0 && domain.DateOf(days[i-1].AddDate(0, 0, 1), loc) == domain.DateOf(d, loc) {
			run++
		} else {
			run = 1
		}
		best = max(best, run)
	}
	return best
}

// todayIndex is today's row in a Monday-first week.
func (a App) todayIndex() int {
	return (int(a.now().Weekday()) + 6) % 7
}

// weekGrid is n Monday-first weeks of answer counts ending with this
// week, oldest first; days after today are -1. starts are the Mondays.
func (a App) weekGrid(n int) (grid [][7]int, starts []time.Time) {
	now := a.now()
	loc := now.Location()
	y, m, d := now.In(loc).Date()
	monday := time.Date(y, m, d, 0, 0, 0, 0, loc).AddDate(0, 0, -a.todayIndex())
	today := domain.DateOf(now, loc)
	act := a.activity()
	grid = make([][7]int, n)
	starts = make([]time.Time, n)
	for i := range grid {
		starts[i] = monday.AddDate(0, 0, -7*(n-1-i))
		for day := range 7 {
			date := domain.DateOf(starts[i].AddDate(0, 0, day), loc)
			if date > today {
				grid[i][day] = -1
				continue
			}
			grid[i][day] = act[date]
		}
	}
	return grid, starts
}

// heatCell is one day: · none, ░ 1–3, ▒ 4–6, ▓ 7–9, █ 10 or more
// answers; blank for days to come. Today is in reverse video.
func (a App) heatCell(v int, today bool) string {
	r := a.st.main
	g := " "
	switch {
	case v < 0:
	case v == 0:
		g = "·"
	case v <= 3:
		g = "░"
	case v <= 6:
		g = "▒"
	case v <= 9:
		g = "▓"
	default:
		g = "█"
	}
	switch {
	case today:
		return r.reverse.Render(g)
	case v == 0:
		return r.label.Render(g)
	}
	return r.success.Render(g)
}

func (a App) heatLegend() string {
	r := a.st.main
	return r.label.Render("less · ") + r.success.Render("░▒▓█") + r.label.Render(" more")
}

// trackAnswers counts answers and correct ones per track ID.
func (a App) trackAnswers() (answers, correct map[string]int) {
	answers, correct = map[string]int{}, map[string]int{}
	for id, hist := range a.history {
		c, ok := a.cat.Concept(id)
		if !ok {
			continue
		}
		for _, ans := range hist {
			answers[c.Track]++
			if ans.Correct {
				correct[c.Track]++
			}
		}
	}
	return answers, correct
}

func (a App) statsPage(L layout) page {
	r := a.st.main
	st := a.stats
	w := L.mi

	main := []string{"",
		lr(r.strong.Render(fmt.Sprint(st.TotalAnswers))+r.label.Render(" answers · ")+r.strong.Render(fmt.Sprintf("%d%%", a.accuracy()))+r.label.Render(" correct"),
			r.label.Render("streak ")+r.strong.Render(fmt.Sprint(st.Streak))+r.label.Render(" days · best ")+r.strong.Render(fmt.Sprint(a.bestStreak())), w, r.base),
		r.rule.Render(strings.Repeat("─", w)), ""}

	weeks := min(heatWeeks, max((w-2+1)/2, 1))
	main = append(main, a.heading("Activity", fmt.Sprintf("%d weeks", weeks), w))
	main = append(main, a.heatmap(weeks)...)
	todayNote := r.reverse.Render("·") + r.label.Render(" today, not studied yet")
	if st.StudiedToday {
		todayNote = a.heatCell(1, true) + r.label.Render(" today")
	}
	main = append(main, lr(a.heatLegend(), todayNote, w, r.base), "")

	withExams := len(a.cat.Sections) > 0
	nameW, head := min(max(w-33, 8), 24), " ●  ◑  ◔  ○  ⊘   answers  correct"
	if withExams {
		nameW, head = min(max(w-40, 8), 24), head+"  exams"
	}
	main = append(main, a.heading("By track", "", w), r.label.Render(pad("track", nameW)+head))
	answers, correct := a.trackAnswers()
	for _, p := range st.Tracks {
		locked := 0
		for _, c := range a.cat.ConceptsIn(p.Track.ID) {
			if a.locked(c) {
				locked++
			}
		}
		counts := []struct {
			n   int
			fmt func(...string) string
		}{{p.Mastered, r.success.Render}, {p.Review, r.info.Render}, {p.Learning, r.warning.Render},
			{p.New - locked, r.secondary.Render}, {locked, r.label.Render}}
		cells := make([]string, len(counts))
		for i, c := range counts {
			cells[i] = r.label.Render(" ·")
			if c.n > 0 {
				cells[i] = c.fmt(fmt.Sprintf("%2d", c.n))
			}
		}
		n := answers[p.Track.ID]
		acc := r.label.Render(fmt.Sprintf("%7s", "—"))
		if n > 0 {
			pct := 100 * correct[p.Track.ID] / n
			st := r.secondary
			if pct < 70 {
				st = r.warning
			}
			acc = st.Render(fmt.Sprintf("%6d%%", pct))
		}
		exams := ""
		if withExams {
			exams = r.label.Render(fmt.Sprintf("%7s", "—"))
		}
		if withExams && p.Sections > 0 {
			st := r.secondary
			if p.SectionsPassed == p.Sections {
				st = r.success
			}
			exams = st.Render(fmt.Sprintf("%7s", fmt.Sprintf("%d/%d", p.SectionsPassed, p.Sections)))
		}
		main = append(main, r.value.Render(pad(a.sideName(p.Track, nameW), nameW))+strings.Join(cells, " ")+
			r.value.Render(fmt.Sprintf("  %8d", n))+"  "+acc+exams)
	}
	main = append(main, a.examStats(w)...)

	ctx := fmt.Sprintf("all time · %d %s", st.TotalAnswers, plural(st.TotalAnswers, "answer", "answers"))
	right := a.statsContext(L.three, L.ri, w)
	if !L.three {
		main = append(main, "")
		main = append(main, right...)
		return page{main: main, ctx: ctx}
	}
	return page{main: main, right: right, ctx: ctx}
}

// heatmap is the Stats activity grid: one column per week, one row per
// weekday (Monday first), months underneath.
func (a App) heatmap(weeks int) []string {
	r := a.st.main
	grid, starts := a.weekGrid(weeks)
	today := a.todayIndex()
	days := []string{"M", "", "W", "", "F", "", "S"}
	out := make([]string, 0, 8)
	for d := range 7 {
		cells := make([]string, weeks)
		for i := range grid {
			cells[i] = a.heatCell(grid[i][d], i == weeks-1 && d == today)
		}
		out = append(out, r.label.Render(pad(days[d], 2))+strings.Join(cells, " "))
	}
	// Month names under the week where each month starts, when they fit.
	row := []rune(strings.Repeat(" ", 2+2*weeks))
	end, prev := -1, time.Month(0)
	for i, s := range starts {
		m := s.Month()
		at := 2 + 2*i
		if m != prev && at > end {
			copy(row[at:], []rune(m.String()[:3]))
			end = at + 3
		}
		prev = m
	}
	return append(out, r.label.Render(strings.TrimRight(string(row[:min(len(row), 2+2*weeks)]), " ")))
}

// statsContext is the right pane of Stats (below the table when narrow):
// top misconceptions and weakest concepts.
func (a App) statsContext(three bool, rw, mw int) []string {
	r := a.st.main
	w := rw
	if !three {
		w = mw
	}
	st := a.stats
	out := []string{a.heading("TOP MISCONCEPTIONS", "", w)}
	if len(st.TopMisconceptions) == 0 {
		out = append(out, r.label.Render("none yet"))
	}
	for _, m := range st.TopMisconceptions {
		out = append(out, lr(r.warning.Render(cut(m.Misconception, w-6)), r.strong.Render(fmt.Sprintf("%d×", m.Count)), w, r.base))
	}
	out = append(out, "", a.heading("WEAKEST CONCEPTS", "correct", w))
	if len(st.Weakest) == 0 {
		out = append(out, r.label.Render("none yet"))
	}
	for _, c := range st.Weakest {
		name, track := c.ConceptID, ""
		if k, ok := a.cat.Concept(c.ConceptID); ok {
			name, track = k.Title, a.shortTrack(k.Track)
		}
		pctSt := r.secondary
		if c.Accuracy() < 0.5 {
			pctSt = r.warning
		}
		out = append(out,
			lr(r.value.Render(cut(name, w-6)), pctSt.Render(fmt.Sprintf("%3.0f%%", 100*c.Accuracy())), w, r.base),
			lr("  "+r.label.Render(track), r.label.Render(fmt.Sprintf("%d/%d", c.Correct, c.Answers)), w, r.base))
	}
	return out
}
