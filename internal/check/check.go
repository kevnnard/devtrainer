// Package check grades closed items without AI. It is pure: every function
// depends only on its arguments.
package check

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// Result is the outcome of grading one response.
type Result struct {
	Correct             bool
	ChosenMisconception string // misconception of the (first) wrong option chosen
	Feedback            string // why the answer is right or wrong
	Expected            string // the correct answer, for display
}

// Response is what the learner answered. Only the field matching the item
// type is read.
type Response struct {
	Choice  int      // mcq, output with options
	Choices []int    // multi
	Text    string   // output typed, command
	Blanks  []string // cloze, one per ___ in order
	Order   []string // parsons, lines in the learner's order
	Line    int      // bug, 1-based
	Code    string   // code: the learner's program
	Run     *CodeRun // code: what running it produced; nil if it never ran
}

// BlankMarker marks a blank in a cloze prompt or code.
const BlankMarker = "___"

// blankSeparator separates blanks inside one accepted cloze answer.
const blankSeparator = "|"

// Grade dispatches to the grader of the item's type.
func Grade(it domain.Item, r Response) Result {
	switch it.Type {
	case domain.MCQ:
		return MCQ(it, r.Choice)
	case domain.Multi:
		return Multi(it, r.Choices)
	case domain.Output:
		if len(it.Options) > 0 {
			return OutputChoice(it, r.Choice)
		}
		return OutputTyped(it, r.Text)
	case domain.Cloze:
		return Cloze(it, r.Blanks)
	case domain.Parsons:
		return Parsons(it, r.Order)
	case domain.Bug:
		return Bug(it, r.Line)
	case domain.Command:
		return Command(it, r.Text)
	case domain.Code:
		if r.Run == nil {
			return Result{Feedback: it.Explanation, Expected: codeExpected(it)}
		}
		return Code(it, *r.Run)
	}
	return Result{Feedback: fmt.Sprintf("unknown item type %q", it.Type)}
}

// MCQ grades a single-choice item; choice is an index into Options.
func MCQ(it domain.Item, choice int) Result {
	res := Result{Expected: optionTexts(it, it.CorrectOptions())}
	if choice < 0 || choice >= len(it.Options) {
		res.Feedback = it.Explanation
		return res
	}
	o := it.Options[choice]
	res.Correct = o.Correct
	if !o.Correct {
		res.ChosenMisconception = o.Misconception
	}
	res.Feedback = join(o.Explanation, it.Explanation)
	return res
}

// OutputChoice grades an output item answered by picking an option.
func OutputChoice(it domain.Item, choice int) Result {
	return MCQ(it, choice)
}

// Multi grades a several-correct item: the chosen set must equal the set of
// correct options exactly. Repeated indices count once.
func Multi(it domain.Item, choices []int) Result {
	want := it.CorrectOptions()
	res := Result{Expected: optionTexts(it, want)}

	got := slices.Clone(choices)
	slices.Sort(got)
	got = slices.Compact(got)

	var wrongWhy []string
	for _, c := range got {
		if c < 0 || c >= len(it.Options) {
			res.Feedback = it.Explanation
			return res
		}
		if o := it.Options[c]; !o.Correct {
			if res.ChosenMisconception == "" {
				res.ChosenMisconception = o.Misconception
			}
			wrongWhy = append(wrongWhy, o.Explanation)
		}
	}
	res.Correct = slices.Equal(got, want)
	if !res.Correct && len(wrongWhy) == 0 {
		wrongWhy = append(wrongWhy, "Some correct options were not selected.")
	}
	res.Feedback = join(append(wrongWhy, it.Explanation)...)
	return res
}

// OutputTyped grades an output item answered by typing what the code prints.
// Line endings, trailing spaces and surrounding blank lines are ignored;
// everything else, including case, must match.
func OutputTyped(it domain.Item, text string) Result {
	got := normalizeOutput(text)
	for _, a := range it.Answers {
		if got == normalizeOutput(a) {
			return Result{Correct: true, Feedback: it.Explanation, Expected: first(it.Answers)}
		}
	}
	return Result{Feedback: it.Explanation, Expected: first(it.Answers)}
}

// Blanks counts the ___ blanks in the item's prompt and code.
func Blanks(it domain.Item) int {
	return strings.Count(it.Prompt, BlankMarker) + strings.Count(it.Code, BlankMarker)
}

// Cloze grades fill-in-the-blank answers, one per blank in order. An
// accepted answer lists its blanks separated by "|". Whitespace is collapsed;
// case is ignored unless the item is case-sensitive.
func Cloze(it domain.Item, blanks []string) Result {
	res := Result{Feedback: it.Explanation, Expected: first(it.Answers)}
	got := make([]string, len(blanks))
	for i, b := range blanks {
		got[i] = normalizeBlank(b, it.CaseSensitive)
	}
	for _, a := range it.Answers {
		parts := strings.Split(a, blankSeparator)
		want := make([]string, len(parts))
		for i, p := range parts {
			want[i] = normalizeBlank(p, it.CaseSensitive)
		}
		if slices.Equal(got, want) {
			res.Correct = true
			return res
		}
	}
	return res
}

// Parsons grades a reordering: the lines must match Lines exactly, in order.
func Parsons(it domain.Item, order []string) Result {
	return Result{
		Correct:  len(it.Lines) > 0 && slices.Equal(order, it.Lines),
		Feedback: it.Explanation,
		Expected: strings.Join(it.Lines, "\n"),
	}
}

// Bug grades a spot-the-bug item; line is 1-based.
func Bug(it domain.Item, line int) Result {
	lines := strings.Split(it.Code, "\n")
	expected := fmt.Sprintf("line %d", it.BugLine)
	if it.BugLine >= 1 && it.BugLine <= len(lines) {
		expected += ": " + strings.TrimSpace(lines[it.BugLine-1])
	}
	return Result{Correct: line == it.BugLine && line > 0, Feedback: it.Explanation, Expected: expected}
}

// Command grades a typed command against the accepted variants. Words are
// compared after shell-like splitting: extra spaces and the quote style do
// not matter, a leading "$ " prompt is ignored, and case always matters
// (flags such as -m and -M differ, and so do program names on Linux).
func Command(it domain.Item, text string) Result {
	res := Result{Feedback: it.Explanation, Expected: first(it.Answers)}
	got := commandWords(text)
	if len(got) == 0 {
		return res
	}
	for _, a := range it.Answers {
		if slices.Equal(got, commandWords(a)) {
			res.Correct = true
			return res
		}
	}
	return res
}

// commandWords splits a command line into words the way a shell would for
// simple quoting: whitespace separates words outside quotes, and single or
// double quotes group a word without being part of it.
func commandWords(s string) []string {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case unicode.IsSpace(r):
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	if len(words) > 0 && words[0] == "$" {
		words = words[1:]
	}
	return words
}

func normalizeBlank(s string, caseSensitive bool) string {
	s = strings.Join(strings.Fields(s), " ")
	if !caseSensitive {
		s = strings.ToLower(s)
	}
	return s
}

func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

func optionTexts(it domain.Item, idx []int) string {
	texts := make([]string, 0, len(idx))
	for _, i := range idx {
		texts = append(texts, it.Options[i].Text)
	}
	return strings.Join(texts, "; ")
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// join joins the non-empty parts with a blank line.
func join(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "\n\n")
}
