package check

import (
	"strings"
	"testing"

	"github.com/kevnnard/devtrainer/internal/domain"
)

func pyItem() domain.Item {
	return domain.Item{ID: "py.print.code", Type: domain.Code, Language: "python", Prompt: "Print 1 to 3.",
		Starter: "for i in range(3):\n    pass\n", Expected: "1\n2\n3\n", Explanation: "range(1, 4) stops before 4."}
}

func sqlItem(ordered bool) domain.Item {
	return domain.Item{ID: "sql.select.code", Type: domain.Code, Language: "sql", Prompt: "Names.",
		Rows: [][]string{{"Ana", "3"}, {"Bo", "NULL"}}, Ordered: ordered}
}

func TestCodeStdout(t *testing.T) {
	it := pyItem()
	cases := []struct {
		name string
		run  CodeRun
		want bool
	}{
		{"exact", CodeRun{Stdout: "1\n2\n3\n"}, true},
		{"trailing spaces and blank lines", CodeRun{Stdout: "\n1  \r\n2\n3\n\n"}, true},
		{"wrong output", CodeRun{Stdout: "0\n1\n2\n"}, false},
		{"right output but the program failed", CodeRun{Stdout: "1\n2\n3\n", Error: "exit status 1"}, false},
		{"timed out", CodeRun{Stdout: "1\n2\n3\n", TimedOut: true}, false},
		{"output over the cap", CodeRun{Stdout: "1\n2\n3\n", Truncated: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Grade(it, Response{Code: "x", Run: &c.run})
			if res.Correct != c.want {
				t.Errorf("Correct = %v, want %v", res.Correct, c.want)
			}
			if res.Expected != "1\n2\n3" {
				t.Errorf("Expected = %q", res.Expected)
			}
			if !strings.Contains(res.Feedback, "range(1, 4)") {
				t.Errorf("Feedback = %q", res.Feedback)
			}
		})
	}
}

func TestCodeRows(t *testing.T) {
	right := [][]string{{"Ana", "3"}, {"Bo", "NULL"}}
	swapped := [][]string{{"Bo", "NULL"}, {"Ana", "3"}}
	if !Code(sqlItem(true), CodeRun{Rows: right}).Correct {
		t.Error("same rows in order should pass")
	}
	if Code(sqlItem(true), CodeRun{Rows: swapped}).Correct {
		t.Error("ordered item: swapped rows should fail")
	}
	if !Code(sqlItem(false), CodeRun{Rows: swapped}).Correct {
		t.Error("unordered item: swapped rows should pass")
	}
	if Code(sqlItem(false), CodeRun{Rows: right[:1]}).Correct {
		t.Error("a missing row should fail")
	}
	if Code(sqlItem(false), CodeRun{Rows: [][]string{{"Ana", "3"}, {"Ana", "3"}}}).Correct {
		t.Error("a duplicated row is not the same multiset")
	}
	if Code(sqlItem(false), CodeRun{Rows: right, Error: "no such table"}).Correct {
		t.Error("an error fails even with rows")
	}
	if got := Code(sqlItem(true), CodeRun{}).Expected; got != "Ana | 3\nBo | NULL" {
		t.Errorf("Expected = %q", got)
	}
	if !Code(domain.Item{Type: domain.Code, Language: "sql", Rows: [][]string{}}, CodeRun{}).Correct {
		t.Error("an empty expected result matches no rows")
	}
}

func TestCodeWithoutRun(t *testing.T) {
	if Grade(pyItem(), Response{Code: "print(1)"}).Correct {
		t.Error("a code response that never ran must not pass")
	}
}
