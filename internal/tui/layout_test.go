package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// TestPanesCollapseBelow100Columns: at 100 columns the track sidebar and
// the context pane frame the card; at 99 only the card is left.
func TestPanesCollapseBelow100Columns(t *testing.T) {
	cat := testCatalog(t)
	for _, tc := range []struct {
		w     int
		three bool
	}{{99, false}, {100, true}, {120, true}} {
		for _, start := range []Screen{Home, Study, Tracks, Stats} {
			a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, start, tc.w, 36)
			s := screen(a)
			checkLines(t, s, tc.w, 36)
			hasSide := strings.Contains(s, "▾ TRACKS") && strings.Contains(s, "● █ mastered")
			if hasSide != tc.three {
				t.Errorf("w=%d %v: sidebar shown = %v, want %v\n%s", tc.w, start, hasSide, tc.three, s)
			}
			body := strings.Split(s, "\n")[:34]
			col := strings.Index(body[0], "│") // the sidebar separator, if any
			seps := 0
			for _, l := range body {
				if r := []rune(l); col > 0 && len(r) > col && r[len([]rune(body[0][:col]))] == '│' {
					seps++
				}
			}
			if got := seps == len(body); got != tc.three {
				t.Errorf("w=%d %v: sidebar separator on %d of %d rows", tc.w, start, seps, len(body))
			}
		}
		a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Study, tc.w, 36)
		if got := strings.Contains(screen(a), "IF YOU ANSWER"); got != tc.three {
			t.Errorf("w=%d: context pane shown = %v, want %v", tc.w, got, tc.three)
		}
		if got := strings.Contains(screen(a), "next due if"); got == tc.three {
			t.Errorf("w=%d: narrow grade row shown = %v, want %v", tc.w, got, !tc.three)
		}
	}
}

// TestDuePreviewDoesNotTouchSchedules renders the per-grade next-due
// preview (pane and narrow row) and checks that neither the store nor the
// app's schedules changed.
func TestDuePreviewDoesNotTouchSchedules(t *testing.T) {
	cat := testCatalog(t)
	st := seeded(t, cat)
	before, err := st.Schedules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []int{80, 120} {
		a := newApp(t, cat, st, &testClock{t: fixedNow}, Study, w, 36)
		kept := make(map[string]domain.Schedule, len(a.schedules))
		for k, v := range a.schedules {
			kept[k] = v
		}
		s := screen(a)
		mustContain(t, s, "Fri 13") // the Good preview of the first pick (Commits)
		if w == 120 {
			mustContain(t, s, "IF YOU ANSWER", "✗ Again", "◆ Hard", "✓ Good", "▲ Easy")
		} else {
			mustContain(t, s, "next due if")
		}
		_ = a.duePreview()
		if !reflect.DeepEqual(a.schedules, kept) {
			t.Errorf("w=%d: the preview changed the app's schedules", w)
		}
	}
	after, err := st.Schedules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("the preview changed the stored schedules:\nbefore %+v\nafter  %+v", before, after)
	}
}

// TestParsonsMarksOnlyMovedLines: "was N" names the original slot of a
// line, and only lines away from it carry one.
func TestParsonsMarksOnlyMovedLines(t *testing.T) {
	a, _, _ := studyApp(t, "logic.recursion.parsons-fact", 120)
	wasLines := func(a App) []string {
		var out []string
		for _, l := range strings.Split(screen(a), "\n") {
			if strings.Contains(l, " was ") {
				out = append(out, l)
			}
		}
		return out
	}
	if got := wasLines(a); len(got) != 0 {
		t.Fatalf("nothing moved yet, but: %q", got)
	}
	start := append([]string(nil), a.session.q.order...)

	a, _ = press(t, a, "J") // line 1 down to 2
	got := wasLines(a)
	if len(got) != 2 {
		t.Fatalf("two lines moved, %d marked: %q", len(got), got)
	}
	if !strings.Contains(got[0], start[1]) || !strings.Contains(got[0], "was 2") ||
		!strings.Contains(got[1], start[0]) || !strings.Contains(got[1], "was 1") {
		t.Errorf("wrong marks: %q", got)
	}
	mustContain(t, screen(a), "2 lines moved so far")

	a, _ = press(t, a, "K") // and back
	if got := wasLines(a); len(got) != 0 {
		t.Errorf("back in place, but: %q", got)
	}
}

// TestWrongAnswerShowsDiff: a wrong answer opens the card with "- your
// answer" (and its misconception) and "+ expected"; a right one has no
// "-" line.
func TestWrongAnswerShowsDiff(t *testing.T) {
	a, _, _ := studyApp(t, "git.commit.mcq-what", 80)
	a, _ = press(t, a, "2")
	s := screen(a)
	mustContain(t, s, "│ - Every change in the working tree  your answer",
		"commit-takes-worktree · 1st time", "│ + The staged changes  expected")
	if strings.Index(s, "│ - Every") > strings.Index(s, "Why not") {
		t.Error("the diff must open the card, before Why not")
	}

	a, _, _ = studyApp(t, "git.commit.mcq-what", 80)
	a, _ = press(t, a, "1")
	s = screen(a)
	mustContain(t, s, "✓ Correct", "│ + The staged changes  your answer")
	mustNotContain(t, s, "│ - ", "  expected")

	// A Parsons order is diffed per line: lines already in place show once.
	a = newApp(t, onlyItem(t, "logic.recursion.parsons-fact"), openStore(t), &testClock{t: fixedNow}, Study, 120, 36)
	a, _ = press(t, a, "enter")
	s = screen(a)
	for i, want := range strings.Split(a.session.q.res.Expected, "\n") {
		mine := a.session.q.resp.Order[i]
		if mine == want {
			mustNotContain(t, s, "- "+mine+" ")
			continue
		}
		mustContain(t, s, "- "+mine, "+ "+want)
	}
}

// TestHeatmapCountsRealStudyDays checks the activity grid against the
// seeded answer log: one cell per study day, nothing invented.
func TestHeatmapCountsRealStudyDays(t *testing.T) {
	cat := testCatalog(t)
	a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Stats, 80, 36)

	grid, starts := a.weekGrid(heatWeeks)
	if got := starts[len(starts)-1].Format("Mon 2 Jan"); got != "Mon 9 Nov" {
		t.Fatalf("last week starts %s, want Mon 9 Nov", got)
	}
	week, prev := grid[len(grid)-1], grid[len(grid)-2]
	// fixedNow is Tue 10 Nov: Mon 9 has 3 answers, today none, then the future.
	if want := [7]int{3, 0, -1, -1, -1, -1, -1}; week != want {
		t.Errorf("this week = %v, want %v", week, want)
	}
	// Week of 2 Nov: Wed 4 (1), Thu 5 (1), Sat 7 (2), Sun 8 (2).
	if want := [7]int{0, 0, 1, 1, 0, 2, 2}; prev != want {
		t.Errorf("last week = %v, want %v", prev, want)
	}
	total := 0
	for _, w := range grid {
		for _, v := range w {
			total += max(v, 0)
		}
	}
	if total != 10 {
		t.Errorf("grid holds %d answers, the store 10", total)
	}

	// On screen: six study days, each one light cell (1–3 answers), plus
	// the legend's sample; no other days are lit.
	s := screen(a)
	if n := strings.Count(s, "░"); n != 6+1 {
		t.Errorf("%d light cells, want 6 study days + legend:\n%s", n, s)
	}
	mustNotContain(t, strings.ReplaceAll(s, "░▒▓█", ""), "▒", "▓", "█")
	mustContain(t, s, "26 weeks", "today, not studied yet")
}

// TestBottomBarHasTwoRows: no top bar; row one is tabs, the session meter
// and the counters, row two the screen context and the key hints.
func TestBottomBarHasTwoRows(t *testing.T) {
	cat := testCatalog(t)
	a := newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Home, 120, 36)
	lines := strings.Split(screen(a), "\n")
	if strings.Contains(lines[0], "devtrainer") {
		t.Errorf("there is no top bar any more: %q", lines[0])
	}
	row1, row2 := lines[len(lines)-2], lines[len(lines)-1]
	for _, want := range []string{"devtrainer", "1 Home", "2 Study", "3 Tracks", "4 Stats",
		"1 due · 3 new", "streak 3d · best 3", "Tue 10 Nov 2026"} {
		mustContain(t, row1, want)
	}
	for _, want := range []string{"6 concepts · 3 tracks · max new/day 3 (--new)", "enter start session", "? help", "q quit"} {
		mustContain(t, row2, want)
	}

	a, _ = press(t, a, "enter") // into the session
	a = typeText(t, a, "x")     // wrong on the first question
	a, _ = press(t, a, "enter")
	lines = strings.Split(screen(a), "\n")
	row1, row2 = lines[len(lines)-2], lines[len(lines)-1]
	mustContain(t, row1, "session [✗···] 1/4 ✓0 ✗1", "streak")
	mustContain(t, row2, "git.commit.cmd-message · command · answered in", "enter next", "e ask the tutor")

	a, _ = press(t, a, "enter")
	row1 = strings.Split(screen(a), "\n")[34]
	mustContain(t, row1, "session [✗●··] 2/4 ✓0 ✗1")

	// 80 columns: compact tabs and meter, keys only.
	a = newApp(t, cat, seeded(t, cat), &testClock{t: fixedNow}, Study, 80, 24)
	lines = strings.Split(screen(a), "\n")
	row1, row2 = lines[22], lines[23]
	mustContain(t, row1, "devtrainer", "2 Study", "[●···] 1/4 ✓0 ✗0")
	mustContain(t, row2, "enter submit", "ctrl+t hint")
	mustNotContain(t, row1, "session [")
	mustNotContain(t, row2, "git.commit")
}
