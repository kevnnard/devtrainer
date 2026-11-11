package check

import (
	"slices"
	"strings"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// CodeRun is what running a learner's program produced.
type CodeRun struct {
	Stdout    string     // standard output (not sql)
	Columns   []string   // sql: column names of the result
	Rows      [][]string // sql: result rows as text
	Error     string     // the compiler or runtime error; empty when it ran cleanly
	TimedOut  bool       // killed at the time limit
	Truncated bool       // killed at the output cap
}

// Failed reports whether the program did not run to a clean end.
func (r CodeRun) Failed() bool {
	return r.Error != "" || r.TimedOut || r.Truncated
}

// Code grades a code item by what its program produced: it must run
// cleanly, and print the expected output (compared like a typed output
// answer) or, for sql, return the expected rows (in order when the item
// is ordered, otherwise as a multiset).
func Code(it domain.Item, run CodeRun) Result {
	res := Result{Feedback: it.Explanation, Expected: codeExpected(it)}
	if run.Failed() {
		return res
	}
	if it.Language == domain.SQL {
		res.Correct = sameRows(it.Rows, run.Rows, it.Ordered)
		return res
	}
	res.Correct = normalizeOutput(run.Stdout) == normalizeOutput(it.Expected)
	return res
}

func codeExpected(it domain.Item) string {
	if it.Language == domain.SQL {
		return FormatRows(it.Rows)
	}
	return normalizeOutput(it.Expected)
}

// FormatRows renders result rows one per line, values separated by " | ".
func FormatRows(rows [][]string) string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = strings.Join(r, " | ")
	}
	return strings.Join(lines, "\n")
}

func sameRows(want, got [][]string, ordered bool) bool {
	if len(want) != len(got) {
		return false
	}
	key := func(r []string) string { return strings.Join(r, "\x00") + "\x01" + string(rune(len(r))) }
	w := make([]string, len(want))
	g := make([]string, len(got))
	for i := range want {
		w[i], g[i] = key(want[i]), key(got[i])
	}
	if !ordered {
		slices.Sort(w)
		slices.Sort(g)
	}
	return slices.Equal(w, g)
}
