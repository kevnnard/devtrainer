package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// CodeRunner runs code exercises for real; *runner.Runner implements it.
type CodeRunner interface {
	Toolchain(lang string) (runner.Toolchain, error)
	Run(ctx context.Context, p runner.Program) (check.CodeRun, error)
}

// errNoRunner marks every language missing when the app has no runner.
var errNoRunner = errors.New("no code runner")

// tabWidth is how many spaces tab inserts and a tab in starter code shows as.
const tabWidth = 4

// codeEditor is the multi-line editor of a code item.
type codeEditor struct {
	lines    []string // runes per line are what the cursor counts
	row, col int      // col is a rune index into lines[row]
}

func newEditor(starter string) codeEditor {
	src := strings.ReplaceAll(strings.TrimRight(starter, "\n"), "\t", strings.Repeat(" ", tabWidth))
	e := codeEditor{lines: strings.Split(src, "\n")}
	e.row = len(e.lines) - 1
	e.col = len([]rune(e.lines[e.row]))
	return e
}

func (e codeEditor) source() string { return strings.Join(e.lines, "\n") + "\n" }

func (e *codeEditor) runes() []rune { return []rune(e.lines[e.row]) }

func (e *codeEditor) clampCol() { e.col = min(e.col, len(e.runes())) }

// insert types s at the cursor; new lines in s split the line.
func (e *codeEditor) insert(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
	for i, part := range strings.Split(s, "\n") {
		if i > 0 {
			e.split("")
		}
		r := e.runes()
		e.lines[e.row] = string(r[:e.col]) + part + string(r[e.col:])
		e.col += len([]rune(part))
	}
}

// split breaks the line at the cursor; the new line starts with indent.
func (e *codeEditor) split(indent string) {
	r := e.runes()
	head, tail := string(r[:e.col]), indent+string(r[e.col:])
	e.lines = slices.Insert(e.lines, e.row+1, tail)
	e.lines[e.row] = head
	e.row++
	e.col = len([]rune(indent))
}

// newline splits the line keeping its indentation.
func (e *codeEditor) newline() {
	line := e.lines[e.row]
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	if n := len([]rune(indent)); n > e.col {
		indent = indent[:e.col]
	}
	e.split(indent)
}

func (e *codeEditor) backspace() {
	switch {
	case e.col > 0:
		r := e.runes()
		e.lines[e.row] = string(r[:e.col-1]) + string(r[e.col:])
		e.col--
	case e.row > 0:
		prev := len([]rune(e.lines[e.row-1]))
		e.lines[e.row-1] += e.lines[e.row]
		e.lines = slices.Delete(e.lines, e.row, e.row+1)
		e.row--
		e.col = prev
	}
}

func (e *codeEditor) delete() {
	r := e.runes()
	switch {
	case e.col < len(r):
		e.lines[e.row] = string(r[:e.col]) + string(r[e.col+1:])
	case e.row < len(e.lines)-1:
		e.lines[e.row] += e.lines[e.row+1]
		e.lines = slices.Delete(e.lines, e.row+1, e.row+2)
	}
}

// dedent removes up to tabWidth leading spaces from the line.
func (e *codeEditor) dedent() {
	line := e.lines[e.row]
	n := min(len(line)-len(strings.TrimLeft(line, " ")), tabWidth)
	e.lines[e.row] = line[n:]
	e.col = max(e.col-n, 0)
}

// key applies an editing key; ok is false when the key is not an edit.
func (e *codeEditor) key(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "enter":
		e.newline()
	case "tab":
		e.insert(strings.Repeat(" ", tabWidth))
	case "shift+tab":
		e.dedent()
	case "backspace":
		e.backspace()
	case "delete", "ctrl+d":
		e.delete()
	case "left":
		if e.col > 0 {
			e.col--
		} else if e.row > 0 {
			e.row--
			e.col = len(e.runes())
		}
	case "right":
		if e.col < len(e.runes()) {
			e.col++
		} else if e.row < len(e.lines)-1 {
			e.row, e.col = e.row+1, 0
		}
	case "up":
		if e.row > 0 {
			e.row--
			e.clampCol()
		}
	case "down":
		if e.row < len(e.lines)-1 {
			e.row++
			e.clampCol()
		}
	case "home", "ctrl+a":
		e.col = 0
	case "end", "ctrl+e":
		e.col = len(e.runes())
	default:
		if msg.Text == "" {
			return false
		}
		e.insert(msg.Text)
	}
	return true
}

// source is the learner's current program.
func (q question) source() string { return q.ed.source() }

// updateEditor handles a key while the code editor has focus.
func (a App) updateEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	q := &a.session.q
	switch msg.String() {
	case "esc":
		return a.show(Home), nil
	case "ctrl+t":
		a.showHint()
		return a, nil
	case "ctrl+r", "ctrl+s":
		if q.running || strings.TrimSpace(q.source()) == "" {
			return a, nil
		}
		return a.runCode()
	}
	if q.running {
		return a, nil
	}
	q.ed.lines = slices.Clone(q.ed.lines)
	if q.ed.key(msg) {
		q.runErr = nil
	}
	return a, nil
}

// paste inserts pasted text into the code editor.
func (a App) paste(msg tea.PasteMsg) App {
	q := &a.session.q
	if a.screen != Study || a.lessonOn() || a.session.done() || q.kind != editor || q.answered || q.running {
		return a
	}
	q.ed.lines = slices.Clone(q.ed.lines)
	q.ed.insert(msg.Content)
	return a
}

// codeRunMsg is what running the code of the pick at pos, started by
// request req, produced.
type codeRunMsg struct {
	pos int
	req int
	run check.CodeRun
	err error
}

// runCode starts the learner's program; the answer is graded when it ends.
// The answer time stops now, so a slow compiler never costs a grade.
func (a App) runCode() (App, tea.Cmd) {
	q := &a.session.q
	now := a.now()
	if q.shown.IsZero() {
		q.shown = now
	}
	a.reqs++
	q.running, q.runErr, q.took, q.runReq = true, nil, now.Sub(q.shown), a.reqs
	it, code, pos, rn, req := q.pick.Item, q.source(), a.session.pos, a.runner, a.reqs
	return a, func() tea.Msg {
		if rn == nil {
			return codeRunMsg{pos: pos, req: req, err: errNoRunner}
		}
		run, err := rn.Run(context.Background(), runner.Program{Language: it.Language, Code: code, Setup: it.Setup})
		return codeRunMsg{pos: pos, req: req, run: run, err: err}
	}
}

// gotRun grades the finished run. A missing toolchain or a runner
// failure grades nothing: the editor stays open with the reason.
func (a App) gotRun(msg codeRunMsg) App {
	s := &a.session
	if s.done() || msg.pos != s.pos || msg.req != s.q.runReq || !s.q.running {
		return a
	}
	q := &s.q
	q.running = false
	if msg.err != nil {
		q.runErr = msg.err
		return a
	}
	run := msg.run
	resp := check.Response{Code: q.source(), Run: &run}
	res := check.Grade(q.pick.Item, resp)
	grade := srs.DeriveCodeGrade(res.Correct, q.hint, q.took, srs.CodeThresholds)
	return a.record(resp, res, grade, q.took)
}

// codeLanguages are the languages of the catalog's code items, in
// domain.Languages order.
func codeLanguages(cat domain.Catalog) []string {
	used := map[string]bool{}
	for _, it := range cat.Items {
		if it.Type == domain.Code {
			used[it.Language] = true
		}
	}
	var out []string
	for _, l := range domain.Languages {
		if used[l] {
			out = append(out, l)
		}
	}
	return out
}

// checkToolchains finds the toolchain of every language used by code
// items: the missing ones and the notes worth showing.
func (a *App) checkToolchains() {
	a.missing = map[string]error{}
	for _, l := range codeLanguages(a.cat) {
		if a.runner == nil {
			a.missing[l] = errNoRunner
			continue
		}
		tc, err := a.runner.Toolchain(l)
		if err != nil {
			a.missing[l] = err
			continue
		}
		if tc.Note != "" {
			a.toolNotes = append(a.toolNotes, tc.Note)
		}
	}
}

// servable is the catalog without the code items whose toolchain is
// missing: those are never planned, so nothing about them is graded.
func (a App) servable() domain.Catalog {
	if len(a.missing) == 0 {
		return a.cat
	}
	out := a.cat
	out.Items = slices.DeleteFunc(slices.Clone(a.cat.Items), func(it domain.Item) bool {
		return it.Type == domain.Code && a.missing[it.Language] != nil
	})
	return out
}

// toolchainLines tell Home which code exercises are off and why.
func (a App) toolchainLines(w int) []string {
	r := a.st.main
	var out []string
	for _, l := range domain.Languages {
		err, ok := a.missing[l]
		if !ok {
			continue
		}
		why := err.Error()
		var miss *runner.MissingError
		if errors.As(err, &miss) {
			why = strings.TrimPrefix(why, l+": ")
		}
		out = append(out, wrapLines(fmt.Sprintf("%s code exercises are off: %s.", l, why), w, r.warning)...)
	}
	for _, n := range a.toolNotes {
		out = append(out, wrapLines(n+".", w, r.label)...)
	}
	return out
}

// ---- view ----

// editorView is the code editor: line numbers, the code on the raised
// surface and the cursor, scrolled to keep the cursor in rows lines.
func (a App) editorView(w, rows int) []string {
	r, rr := a.st.main, a.st.raised
	q := a.session.q
	e := q.ed
	rows = max(rows, 3)
	top := 0
	if len(e.lines) > rows {
		top = min(max(e.row-rows/2, 0), len(e.lines)-rows)
	}
	numW := len(fmt.Sprint(len(e.lines)))
	head := r.label.Render(q.pick.Item.Language + " · editing")
	if q.running {
		head = r.accent.Render("⠋ Running your code…")
	}
	out := []string{lr(head, r.label.Render(fmt.Sprintf("line %d/%d", e.row+1, len(e.lines))), w, r.base)}
	if top > 0 {
		out = append(out, r.label.Render(fmt.Sprintf("  ↑ %d more", top)))
	}
	for i := top; i < min(top+rows, len(e.lines)); i++ {
		text := cut(e.lines[i], w-numW-4)
		if i == e.row && !q.running {
			rs := []rune(e.lines[i])
			col := min(e.col, len(rs))
			text = rr.code.Render(string(rs[:col])) + rr.selected.Render("▏") + rr.code.Render(string(rs[col:]))
			out = append(out, paint(rr.base.Render(" ")+rr.accent.Render(fmt.Sprintf("%*d", numW, i+1))+rr.base.Render("  ")+text, w, rr))
			continue
		}
		out = append(out, paint(rr.base.Render(" ")+rr.label.Render(fmt.Sprintf("%*d", numW, i+1))+rr.base.Render("  ")+rr.code.Render(text), w, rr))
	}
	if rest := len(e.lines) - top - rows; rest > 0 {
		out = append(out, r.label.Render(fmt.Sprintf("  ↓ %d more", rest)))
	}
	if q.runErr != nil {
		out = append(out, "")
		out = append(out, wrapLines("Did not run, so not graded: "+q.runErr.Error(), w, r.danger)...)
	}
	return out
}

// codeVerdict is what the run produced, for the "-" side of the diff.
func codeVerdict(it domain.Item, run *check.CodeRun) string {
	if run == nil {
		return "(did not run)"
	}
	var parts []string
	switch {
	case run.TimedOut:
		parts = append(parts, "Stopped at the time limit: the program ran too long (an endless loop?).")
	case run.Truncated:
		parts = append(parts, "Stopped at the output limit: the program printed too much.")
	}
	if run.Error != "" {
		parts = append(parts, run.Error)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	if it.Language == domain.SQL {
		if len(run.Rows) == 0 {
			return "(no rows)"
		}
		return check.FormatRows(run.Rows)
	}
	if strings.TrimSpace(run.Stdout) == "" {
		return "(printed nothing)"
	}
	return strings.TrimRight(run.Stdout, "\n")
}

// maxDiffLines caps each side of a code diff so the verdict stays on screen.
// On a short terminal each side shrinks down to minDiffLines (its first
// lines and a "… n more lines" note) before the explanation is clipped.
const (
	maxDiffLines = 12
	minDiffLines = 4
)

func clipLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n-1], "\n") + fmt.Sprintf("\n… %d more lines", len(lines)-n+1)
}

// solutionLines are the item's reference solution on the raised surface.
func (a App) solutionLines(it domain.Item, w int) []string {
	rr := a.st.raised
	src := strings.ReplaceAll(strings.TrimRight(it.Solution, "\n"), "\t", strings.Repeat(" ", tabWidth))
	var out []string
	for _, l := range strings.Split(src, "\n") {
		out = append(out, paint(rr.base.Render(" ")+rr.code.Render(cut(l, w-2)), w, rr))
	}
	return out
}
