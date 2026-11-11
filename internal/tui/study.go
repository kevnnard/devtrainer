package tui

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// inputKind is how the learner answers an item.
type inputKind int

const (
	pickOne  inputKind = iota // mcq, output with options: 1–9 or j/k + enter
	pickMany                  // multi: space toggles, enter submits
	typed                     // output typed, command: a text input
	blanks                    // cloze: one text input per blank
	reorder                   // parsons: move lines with J/K
	pickLine                  // bug: j/k select a line of code
	editor                    // code: a multi-line editor, ctrl+r runs
)

func kindOf(it domain.Item) inputKind {
	switch it.Type {
	case domain.MCQ:
		return pickOne
	case domain.Output:
		if len(it.Options) > 0 {
			return pickOne
		}
		return typed
	case domain.Multi:
		return pickMany
	case domain.Cloze:
		return blanks
	case domain.Parsons:
		return reorder
	case domain.Bug:
		return pickLine
	case domain.Code:
		return editor
	}
	return typed
}

// explainState is where the tutor's deeper explanation stands.
type explainState int

const (
	explainIdle explainState = iota
	explainLoading
	explainDone
	explainFailed
)

// question is the item on screen and everything the learner did with it.
type question struct {
	pick   srs.Pick
	kind   inputKind
	shown  time.Time // when it first appeared; zero until then
	hint   bool      // the hint was shown, which grades a correct answer Hard
	noHint bool      // h was pressed but the item has no hint

	cursor int      // option, line or parsons row
	chosen []bool   // multi
	order  []string // parsons, the learner's current order
	from   []int    // parsons, the starting slot of each line of order
	text   string   // typed answer
	fills  []string // cloze, one per blank
	blank  int      // cloze, the blank being edited

	ed      codeEditor // code
	running bool       // code: the program is running
	runReq  int        // code: the request number of the run in flight
	runErr  error      // code: why the last run graded nothing

	answered bool
	resp     check.Response
	res      check.Result
	grade    domain.Grade
	prev     domain.Schedule // the concept's schedule before this answer
	next     domain.Schedule
	took     time.Duration

	explain     explainState
	explanation string
	explainErr  error
	explainReq  int // the request number of the explanation in flight
}

func newQuestion(p srs.Pick) question {
	q := question{pick: p, kind: kindOf(p.Item)}
	switch q.kind {
	case pickMany:
		q.chosen = make([]bool, len(p.Item.Options))
	case reorder:
		q.order = shuffled(p.Item.ID, p.Item.Lines)
		q.from = make([]int, len(q.order))
		for i := range q.from {
			q.from[i] = i
		}
	case blanks:
		q.fills = make([]string, max(check.Blanks(p.Item), 1))
	case editor:
		q.ed = newEditor(p.Item.Starter)
	}
	return q
}

// swap exchanges two Parsons lines, keeping track of where each started.
func (q *question) swap(i, j int) {
	q.order[i], q.order[j] = q.order[j], q.order[i]
	q.from[i], q.from[j] = q.from[j], q.from[i]
}

// moved counts the Parsons lines that are not in their starting slot.
func (q question) moved() int {
	n := 0
	for i, f := range q.from {
		if f != i {
			n++
		}
	}
	return n
}

// shuffled returns lines in an order derived from the item ID, so the same
// item always starts the same way. It never returns the solved order when
// another order exists.
func shuffled(id string, lines []string) []string {
	h := fnv.New64a()
	h.Write([]byte(id))
	seed := h.Sum64()
	out := slices.Clone(lines)
	rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)).Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	if slices.Equal(out, lines) && len(out) > 1 {
		for i := 1; i < len(out); i++ {
			if out[i] != out[0] {
				out[0], out[i] = out[i], out[0]
				break
			}
		}
	}
	return out
}

// codeLines are the lines of the item's code without the trailing newline.
func codeLines(it domain.Item) []string {
	return strings.Split(strings.TrimRight(it.Code, "\n"), "\n")
}

// session is today's study session.
type session struct {
	picks   []srs.Pick
	pos     int
	q       question
	correct int
	results []bool // per answered pick, in order
}

func newSession(picks []srs.Pick) session {
	s := session{picks: picks}
	if len(picks) > 0 {
		s.q = newQuestion(picks[0])
	}
	return s
}

func (s session) done() bool { return s.pos >= len(s.picks) }

// remaining counts the picks not answered yet.
func (s session) remaining() int {
	n := len(s.picks) - s.pos
	if !s.done() && s.q.answered {
		n--
	}
	return max(n, 0)
}

// typing reports whether a text input has focus, so letters are typed
// instead of being read as commands.
func (a App) typing() bool {
	s := a.session
	if a.lessonOn() {
		return false
	}
	return !s.done() && !s.q.answered && (s.q.kind == typed || s.q.kind == blanks || s.q.kind == editor)
}

// studyKey handles the Study keys that take priority over the global
// ones; ok is false when the key is not a Study key.
func (a App) studyKey(msg tea.KeyPressMsg) (App, tea.Cmd, bool) {
	k := msg.String()
	s := &a.session
	if s.done() {
		if k == "enter" && a.exam != nil {
			return a.show(Tracks), nil, true
		}
		if k == "enter" {
			return a.show(Home), nil, true
		}
		return a, nil, false
	}
	q := &s.q
	if q.answered {
		switch k {
		case "enter", "space", "n":
			a.advance()
			return a, nil, true
		case "e":
			if q.explain == explainLoading || q.explain == explainDone {
				return a, nil, true
			}
			a.reqs++
			q.explain, q.explainErr, q.explainReq = explainLoading, nil, a.reqs
			return a, a.explainCmd(), true
		}
		return a, nil, false
	}
	if k == "h" {
		a.showHint()
		return a, nil, true
	}
	switch q.kind {
	case pickOne:
		n := len(q.pick.Item.Options)
		switch {
		case isDigit(k) && int(k[0]-'1') < n:
			q.cursor = int(k[0] - '1')
			return a.submit(), nil, true
		case k == "j" || k == "down":
			q.cursor = min(q.cursor+1, n-1)
		case k == "k" || k == "up":
			q.cursor = max(q.cursor-1, 0)
		case k == "enter":
			return a.submit(), nil, true
		default:
			return a, nil, isDigit(k) // an option number past the last one does nothing
		}
	case pickMany:
		n := len(q.pick.Item.Options)
		switch {
		case isDigit(k) && int(k[0]-'1') < n:
			i := int(k[0] - '1')
			q.cursor, q.chosen[i] = i, !q.chosen[i]
		case k == "j" || k == "down":
			q.cursor = min(q.cursor+1, n-1)
		case k == "k" || k == "up":
			q.cursor = max(q.cursor-1, 0)
		case k == "space":
			q.chosen[q.cursor] = !q.chosen[q.cursor]
		case k == "enter":
			if slices.Contains(q.chosen, true) {
				return a.submit(), nil, true
			}
		default:
			return a, nil, isDigit(k)
		}
	case reorder:
		n := len(q.order)
		switch k {
		case "j", "down":
			q.cursor = min(q.cursor+1, n-1)
		case "k", "up":
			q.cursor = max(q.cursor-1, 0)
		case "J", "shift+j", "shift+down":
			if q.cursor < n-1 {
				q.swap(q.cursor, q.cursor+1)
				q.cursor++
			}
		case "K", "shift+k", "shift+up":
			if q.cursor > 0 {
				q.swap(q.cursor, q.cursor-1)
				q.cursor--
			}
		case "enter":
			return a.submit(), nil, true
		default:
			return a, nil, false
		}
	case pickLine:
		n := len(codeLines(q.pick.Item))
		switch k {
		case "j", "down":
			q.cursor = min(q.cursor+1, n-1)
		case "k", "up":
			q.cursor = max(q.cursor-1, 0)
		case "enter":
			return a.submit(), nil, true
		default:
			return a, nil, false
		}
	}
	return a, nil, true
}

// updateStudy handles a key while a text input has focus.
func (a App) updateStudy(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	q := &a.session.q
	if q.kind == editor {
		return a.updateEditor(msg)
	}
	k := msg.String()
	switch k {
	case "esc":
		return a.show(Home), nil
	case "ctrl+t":
		a.showHint()
		return a, nil
	}
	if q.kind == blanks {
		last := len(q.fills) - 1
		switch k {
		case "tab":
			q.blank = min(q.blank+1, last)
		case "shift+tab":
			q.blank = max(q.blank-1, 0)
		case "enter":
			if q.blank < last {
				q.blank++
				break
			}
			return a.submit(), nil
		default:
			q.fills[q.blank] = edit(q.fills[q.blank], msg, false)
		}
		return a, nil
	}
	switch k {
	case "tab":
		return a.show(screens[(int(a.screen)+1)%len(screens)]), nil
	case "shift+tab":
		return a.show(screens[(int(a.screen)+len(screens)-1)%len(screens)]), nil
	case "enter":
		if strings.TrimSpace(q.text) != "" {
			return a.submit(), nil
		}
	default:
		multiline := q.pick.Item.Type == domain.Output
		q.text = edit(q.text, msg, multiline)
	}
	return a, nil
}

// edit applies a key to a text input: backspace deletes a rune, ctrl+u
// clears, alt+enter or ctrl+j adds a line when multiline, and printable
// text is appended.
func edit(s string, msg tea.KeyPressMsg, multiline bool) string {
	switch msg.String() {
	case "backspace":
		if r := []rune(s); len(r) > 0 {
			return string(r[:len(r)-1])
		}
		return s
	case "ctrl+u":
		return ""
	case "alt+enter", "ctrl+j":
		if multiline {
			return s + "\n"
		}
		return s
	}
	return s + msg.Text
}

func isDigit(k string) bool { return len(k) == 1 && k[0] >= '1' && k[0] <= '9' }

// showHint reveals the item's hint; only an existing hint counts as used.
func (a *App) showHint() {
	q := &a.session.q
	if q.pick.Item.Hint == "" {
		q.noHint = true
		return
	}
	q.hint = true
}

// response builds the check.Response from what is on screen.
func (q question) response() check.Response {
	switch q.kind {
	case pickOne:
		return check.Response{Choice: q.cursor}
	case pickMany:
		var idx []int
		for i, c := range q.chosen {
			if c {
				idx = append(idx, i)
			}
		}
		return check.Response{Choices: idx}
	case blanks:
		return check.Response{Blanks: slices.Clone(q.fills)}
	case reorder:
		return check.Response{Order: slices.Clone(q.order)}
	case pickLine:
		return check.Response{Line: q.cursor + 1}
	case editor:
		return check.Response{Code: q.source()}
	}
	return check.Response{Text: q.text}
}

// submit grades the answer, schedules the concept and saves the review.
func (a App) submit() App {
	q := &a.session.q
	now := a.now()
	if q.shown.IsZero() {
		q.shown = now
	}
	resp := q.response()
	res := check.Grade(q.pick.Item, resp)
	took := now.Sub(q.shown)
	return a.record(resp, res, srs.DeriveGrade(res.Correct, q.hint, took, srs.DefaultThresholds), took)
}

// record schedules the concept with grade and saves the review of the
// current question, answered in took.
func (a App) record(resp check.Response, res check.Result, grade domain.Grade, took time.Duration) App {
	q := &a.session.q
	now := a.now()
	it, c := q.pick.Item, q.pick.Concept
	q.resp, q.res, q.grade = resp, res, grade

	sc := a.schedules[c.ID]
	sc.ConceptID = c.ID
	q.prev, q.took = sc, took
	q.next = a.sched.Review(sc, q.grade, it.ID, now)
	ans := domain.Answer{
		ConceptID:     c.ID,
		ItemID:        it.ID,
		Correct:       q.res.Correct,
		Grade:         q.grade,
		Misconception: q.res.ChosenMisconception,
		Duration:      took,
		HintUsed:      q.hint,
		At:            now,
	}
	if err := a.progress.RecordReview(context.Background(), ans, q.next); err != nil {
		a.err = err
		return a
	}
	q.answered = true
	a.session.results = append(a.session.results, q.res.Correct)
	if q.res.Correct {
		a.session.correct++
	}
	a.schedules = cloneMap(a.schedules)
	a.schedules[c.ID] = q.next
	a.history = cloneMap(a.history)
	a.history[c.ID] = append(slices.Clone(a.history[c.ID]), ans)
	if a.exam != nil && a.exam.result == nil && len(a.session.results) == len(a.session.picks) {
		if err := a.finishExam(now); err != nil {
			a.err = err
			return a
		}
	}
	if err := a.refresh(); err != nil {
		a.err = err
	}
	return a
}

// advance moves to the next pick of the session.
func (a *App) advance() {
	s := &a.session
	s.pos++
	if !s.done() {
		s.q = newQuestion(s.picks[s.pos])
		a.startQuestion()
	}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// explainMsg carries the tutor's answer for the pick at pos, asked by
// request req.
type explainMsg struct {
	pos  int
	req  int
	text string
	err  error
}

var errNoTutor = errTutorOff{}

type errTutorOff struct{}

func (errTutorOff) Error() string { return "the AI tutor is not configured" }

func (a App) explainCmd() tea.Cmd {
	q, pos, t := a.session.q, a.session.pos, a.tutor
	req := q.explainReq
	return func() tea.Msg {
		if t == nil {
			return explainMsg{pos: pos, req: req, err: errNoTutor}
		}
		text, err := t.Explain(context.Background(), q.pick.Item, q.pick.Concept, q.resp, q.res)
		return explainMsg{pos: pos, req: req, text: text, err: err}
	}
}

func (a App) gotExplanation(msg explainMsg) App {
	s := &a.session
	if s.done() || msg.pos != s.pos || msg.req != s.q.explainReq || s.q.explain != explainLoading {
		return a // the learner moved on, or the reply is for another session
	}
	if msg.err != nil {
		s.q.explain, s.q.explainErr = explainFailed, msg.err
		return a
	}
	s.q.explain, s.q.explanation = explainDone, msg.text
	return a
}
