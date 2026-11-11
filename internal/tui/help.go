package tui

import (
	"strings"
)

// helpSection is one titled group of keys in the `?` overlay.
type helpSection struct {
	title string
	keys  [][2]string
}

// helpSections is the text of the `?` overlay. The README keys table
// mirrors it; change both together.
var helpSections = []helpSection{
	{"Everywhere", [][2]string{
		{"tab / shift+tab", "next / previous screen"},
		{"1 2 3 4", "Home, Study, Tracks, Stats (in Study, numbers answer first)"},
		{"?", "show / hide this help"},
		{"esc", "back to Home"},
		{"q  ·  ctrl+c", "quit"},
	}},
	{"Study", [][2]string{
		{"1–9 · j/k enter", "choose an option (mcq, output)"},
		{"space · enter", "toggle an option · submit (multi)"},
		{"type · enter", "write the answer (output, command)"},
		{"tab · enter", "next blank · submit on the last (cloze)"},
		{"j/k · J/K", "select a line · move it up/down (parsons)"},
		{"j/k · enter", "select the faulty line (bug)"},
		{"h  ·  ctrl+t", "show the hint (ctrl+t while typing); a hint grades Hard"},
		{"e", "after answering: ask the AI tutor to explain more"},
		{"enter", "after answering: next question"},
	}},
	{"Lesson", [][2]string{
		{"enter", "reveal the next step · after the takeaway, start the questions"},
		{"1–9 · j/k enter", "predict the next step (never graded or saved)"},
		{"s", "skip the lesson (it is not forced again)"},
		{"esc", "in a lesson opened from Tracks: back to Tracks"},
	}},
	{"Tracks", [][2]string{
		{"j/k", "choose a track"},
		{"J/K", "choose a concept"},
		{"l  ·  enter", "open the concept's lesson (a review, not graded)"},
		{"x", "start the exam of the concept's section, once every concept of it is learning; it passes at 80% and its answers are scheduled like any other"},
		{"enter", "after an exam's result: back to Tracks"},
	}},
}

// helpPage is the `?` overlay: one centered column of keys.
func (a App) helpPage(L layout) page {
	r := a.st.main
	w := min(max(L.w-2*narrowMargin, 10), fullMax)
	out := []string{"", r.title.Render("Keys")}
	keyW := 18
	for _, s := range helpSections {
		out = append(out, "", r.strong.Render(s.title))
		for _, k := range s.keys {
			lines := strings.Split(wrap(k[1], max(w-keyW-2, 10)), "\n")
			for i, l := range lines {
				head := strings.Repeat(" ", keyW+2)
				if i == 0 {
					head = "  " + r.accent.Render(pad(k[0], keyW))
				}
				out = append(out, head+r.value.Render(l))
			}
		}
	}
	return page{main: out, full: true, ctx: "keys"}
}
