package check

import (
	"strings"
	"testing"

	"github.com/kevnnard/devtrainer/internal/domain"
)

func mcqItem() domain.Item {
	return domain.Item{
		ID: "git.stage.1", Type: domain.MCQ, Prompt: "Which command stages a file?",
		Options: []domain.Option{
			{Text: "git add f", Correct: true, Explanation: "add puts the change in the index."},
			{Text: "git commit f", Misconception: "commit-stages", Explanation: "commit records what is already staged."},
			{Text: "git push f", Misconception: "push-local", Explanation: "push talks to a remote."},
		},
		Explanation: "The index (staging area) is filled with git add.",
	}
}

func TestMCQ(t *testing.T) {
	it := mcqItem()
	r := MCQ(it, 0)
	if !r.Correct || r.ChosenMisconception != "" {
		t.Errorf("correct choice: %+v", r)
	}
	if !strings.Contains(r.Feedback, "index (staging area)") {
		t.Errorf("feedback should carry the item explanation: %q", r.Feedback)
	}

	r = MCQ(it, 1)
	if r.Correct || r.ChosenMisconception != "commit-stages" {
		t.Errorf("wrong choice: %+v", r)
	}
	if !strings.Contains(r.Feedback, "commit records what is already staged") {
		t.Errorf("feedback should explain why the option is wrong: %q", r.Feedback)
	}
	if r.Expected != "git add f" {
		t.Errorf("Expected = %q", r.Expected)
	}

	for _, out := range []int{-1, 3} {
		if r := MCQ(it, out); r.Correct {
			t.Errorf("out-of-range choice %d graded correct", out)
		}
	}
}

func TestMultiRequiresExactSet(t *testing.T) {
	it := domain.Item{Type: domain.Multi, Options: []domain.Option{
		{Text: "a", Correct: true},
		{Text: "b", Misconception: "b-wrong", Explanation: "b is not it"},
		{Text: "c", Correct: true},
	}}
	cases := []struct {
		name    string
		choices []int
		correct bool
		misc    string
	}{
		{"exact", []int{0, 2}, true, ""},
		{"exact reordered", []int{2, 0}, true, ""},
		{"duplicates ignored", []int{2, 0, 2}, true, ""},
		{"subset", []int{0}, false, ""},
		{"superset", []int{0, 1, 2}, false, "b-wrong"},
		{"none", nil, false, ""},
		{"out of range", []int{0, 2, 7}, false, ""},
	}
	for _, c := range cases {
		r := Multi(it, c.choices)
		if r.Correct != c.correct || r.ChosenMisconception != c.misc {
			t.Errorf("%s: got %+v, want correct=%v misc=%q", c.name, r, c.correct, c.misc)
		}
	}
}

func TestOutputChoiceAndTyped(t *testing.T) {
	choice := domain.Item{Type: domain.Output, Code: "fmt.Println(1+1)", Options: []domain.Option{
		{Text: "11", Misconception: "string-concat", Explanation: "1+1 are ints, not strings"},
		{Text: "2", Correct: true},
	}}
	if r := OutputChoice(choice, 1); !r.Correct {
		t.Errorf("choice 1: %+v", r)
	}
	if r := OutputChoice(choice, 0); r.Correct || r.ChosenMisconception != "string-concat" {
		t.Errorf("choice 0: %+v", r)
	}

	typed := domain.Item{Type: domain.Output, Code: "...", Answers: []string{"a\nb"}}
	for _, in := range []string{"a\nb", "a  \nb\n", "\r\na\r\nb\r\n\n"} {
		if r := OutputTyped(typed, in); !r.Correct {
			t.Errorf("OutputTyped(%q) should be correct", in)
		}
	}
	for _, in := range []string{"A\nb", "a b", "b\na"} {
		if r := OutputTyped(typed, in); r.Correct {
			t.Errorf("OutputTyped(%q) should be wrong", in)
		}
	}
}

func TestCloze(t *testing.T) {
	one := domain.Item{Type: domain.Cloze, Prompt: "A ___ points to a commit.", Answers: []string{"branch", "ref"}}
	for _, in := range []string{"branch", "  Branch ", "REF"} {
		if r := Cloze(one, []string{in}); !r.Correct {
			t.Errorf("Cloze(%q) should be correct", in)
		}
	}
	if r := Cloze(one, []string{"tag"}); r.Correct || r.Expected != "branch" {
		t.Errorf("Cloze(tag): %+v", r)
	}
	if r := Cloze(one, nil); r.Correct {
		t.Error("no blanks answered must be wrong")
	}

	two := domain.Item{Type: domain.Cloze, Code: "for i := ___; i < ___; i++ {}", Answers: []string{"0 | 10"}}
	if Blanks(two) != 2 {
		t.Errorf("Blanks = %d, want 2", Blanks(two))
	}
	if r := Cloze(two, []string{"0", " 10"}); !r.Correct {
		t.Errorf("two blanks: %+v", r)
	}
	if r := Cloze(two, []string{"10", "0"}); r.Correct {
		t.Error("blanks are positional")
	}

	cs := domain.Item{Type: domain.Cloze, Prompt: "fmt.___(x)", Answers: []string{"Println"}, CaseSensitive: true}
	if r := Cloze(cs, []string{"println"}); r.Correct {
		t.Error("case-sensitive cloze accepted wrong case")
	}
	if r := Cloze(cs, []string{"Println"}); !r.Correct {
		t.Error("case-sensitive cloze rejected exact answer")
	}
}

func TestParsonsExactOrder(t *testing.T) {
	it := domain.Item{Type: domain.Parsons, Lines: []string{"func f() {", "\treturn 1", "}"}}
	if r := Parsons(it, []string{"func f() {", "\treturn 1", "}"}); !r.Correct {
		t.Errorf("exact order: %+v", r)
	}
	if r := Parsons(it, []string{"\treturn 1", "func f() {", "}"}); r.Correct {
		t.Error("swapped order graded correct")
	}
	if r := Parsons(it, []string{"func f() {", "\treturn 1"}); r.Correct {
		t.Error("missing line graded correct")
	}
	if r := Parsons(it, nil); r.Correct || r.Expected != "func f() {\n\treturn 1\n}" {
		t.Errorf("empty: %+v", r)
	}
}

func TestBug(t *testing.T) {
	it := domain.Item{Type: domain.Bug, Code: "x := 1\nif x = 1 {\n}", BugLine: 2, Explanation: "= assigns; use =="}
	if r := Bug(it, 2); !r.Correct {
		t.Errorf("line 2: %+v", r)
	}
	r := Bug(it, 1)
	if r.Correct || r.Expected != "line 2: if x = 1 {" {
		t.Errorf("line 1: %+v", r)
	}
}

func TestCommandNormalization(t *testing.T) {
	it := domain.Item{Type: domain.Command, Answers: []string{`git commit -m "fix bug"`, "git commit --message 'fix bug'"}}
	for _, in := range []string{
		`git commit -m "fix bug"`,
		`  git   commit  -m   'fix bug'  `,
		`git commit --message "fix bug"`,
		`$ git commit -m "fix bug"`,
	} {
		if r := Command(it, in); !r.Correct {
			t.Errorf("Command(%q) should be correct", in)
		}
	}
	for _, in := range []string{
		`git commit -M "fix bug"`, // flags are case-sensitive
		`Git commit -m "fix bug"`, // so are program names on Linux
		`git commit -m "fix  bug"`,
		`git commit -m fix bug`,
		``,
	} {
		if r := Command(it, in); r.Correct {
			t.Errorf("Command(%q) should be wrong", in)
		}
	}
}

func TestGradeDispatches(t *testing.T) {
	if r := Grade(mcqItem(), Response{Choice: 0}); !r.Correct {
		t.Error("Grade mcq")
	}
	typed := domain.Item{Type: domain.Output, Answers: []string{"2"}}
	if r := Grade(typed, Response{Text: "2"}); !r.Correct {
		t.Error("Grade output typed")
	}
	cmd := domain.Item{Type: domain.Command, Answers: []string{"git status"}}
	if r := Grade(cmd, Response{Text: "git status"}); !r.Correct {
		t.Error("Grade command")
	}
	bug := domain.Item{Type: domain.Bug, Code: "a\nb", BugLine: 1}
	if r := Grade(bug, Response{Line: 1}); !r.Correct {
		t.Error("Grade bug")
	}
	if r := Grade(domain.Item{Type: "essay"}, Response{}); r.Correct {
		t.Error("unknown type must not be correct")
	}
}
