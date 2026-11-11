package content

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kevnnard/devtrainer/internal/domain"
)

// codeYAML has one sql and one python code item, and a lesson with an
// error-on-purpose step.
const codeYAML = `
track:
  id: sql
  title: SQL
  order: 1
concepts:
  - id: sql.select
    title: SELECT
    level: beginner
    lesson:
      intro: A query names the columns it wants.
      example:
        lang: sql
        code: |
          SELECT nme FROM users;
      steps:
        - text: The column is called name, not nme, so SQLite cannot resolve it.
          error: "SQL logic error: no such column: nme (1)"
          predict:
            prompt: Why does this query fail?
            options:
              - {text: The column name is misspelled, correct: true}
              - {text: SELECT needs a WHERE clause}
      takeaway: Column names must match the table.
    items:
      - id: sql.select.code-names
        type: code
        language: sql
        prompt: List every user's name, alphabetically.
        setup: |
          CREATE TABLE users (id INTEGER, name TEXT);
          INSERT INTO users VALUES (1, 'Bo'), (2, 'Ana');
        starter: |
          SELECT
        solution: |
          SELECT name FROM users ORDER BY name;
        rows:
          - [Ana]
          - [Bo]
        ordered: true
        explanation: ORDER BY sorts the rows.
      - id: sql.select.code-py
        type: code
        language: python
        prompt: Print hello.
        starter: |
          print()
        solution: |
          print("hello")
        expected: |
          hello
`

func TestLoadCodeItems(t *testing.T) {
	cat, err := Load(fstest.MapFS{"sql/sql.yaml": {Data: []byte(codeYAML)}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.Items) != 2 {
		t.Fatalf("got %d items", len(cat.Items))
	}
	q := cat.Items[0]
	if q.Type != domain.Code || q.Language != "sql" || !strings.HasPrefix(q.Setup, "CREATE TABLE") ||
		q.Starter != "SELECT\n" || !strings.Contains(q.Solution, "ORDER BY") || !q.Ordered {
		t.Errorf("sql item = %+v", q)
	}
	if len(q.Rows) != 2 || q.Rows[0][0] != "Ana" || q.Rows[1][0] != "Bo" {
		t.Errorf("rows = %v", q.Rows)
	}
	p := cat.Items[1]
	if p.Language != "python" || p.Expected != "hello\n" || p.Rows != nil {
		t.Errorf("python item = %+v", p)
	}
	c, _ := cat.Concept("sql.select")
	st := c.Lesson.Steps[0]
	if st.Error != "SQL logic error: no such column: nme (1)" || st.Predict == nil {
		t.Errorf("error step = %+v", st)
	}
}

func TestLoadCodeRowsKeepScalarText(t *testing.T) {
	src := strings.Replace(codeYAML, "          - [Ana]\n          - [Bo]\n", "          - [3, 2.0, NULL, true]\n", 1)
	cat, err := Load(fstest.MapFS{"sql/sql.yaml": {Data: []byte(src)}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := strings.Join(cat.Items[0].Rows[0], ",")
	if got != "3,2.0,NULL,true" {
		t.Errorf("row = %q, want the YAML text of each scalar", got)
	}
}

func TestLoadCodeValidationErrors(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(string) string
		parts []string
	}{
		{"unknown language",
			func(s string) string { return strings.Replace(s, "language: python", "language: cobol", 1) },
			[]string{"sql/sql.yaml", "sql.select.code-py", "unknown language", "cobol"}},
		{"missing language",
			func(s string) string { return strings.Replace(s, "        language: python\n", "", 1) },
			[]string{"sql.select.code-py", "language"}},
		{"missing starter",
			func(s string) string { return strings.Replace(s, "        starter: |\n          print()\n", "", 1) },
			[]string{"sql/sql.yaml", "sql.select.code-py", "starter"}},
		{"missing expected output",
			func(s string) string { return strings.Replace(s, "        expected: |\n          hello\n", "", 1) },
			[]string{"sql/sql.yaml", "sql.select.code-py", "expected"}},
		{"missing solution",
			func(s string) string {
				return strings.Replace(s, "        solution: |\n          print(\"hello\")\n", "", 1)
			},
			[]string{"sql.select.code-py", "solution"}},
		{"sql without rows",
			func(s string) string {
				return strings.Replace(s, "        rows:\n          - [Ana]\n          - [Bo]\n", "", 1)
			},
			[]string{"sql/sql.yaml", "sql.select.code-names", "rows"}},
		{"sql with expected output",
			func(s string) string {
				return strings.Replace(s, "        ordered: true\n", "        ordered: true\n        expected: Ana\n", 1)
			},
			[]string{"sql.select.code-names", "expected"}},
		{"sql rows of different widths",
			func(s string) string { return strings.Replace(s, "- [Bo]", "- [Bo, 2]", 1) },
			[]string{"sql.select.code-names", "row 2", "2 columns", "1"}},
		{"rows on a non-sql item",
			func(s string) string {
				return strings.Replace(s, "        expected: |\n          hello\n", "        expected: hello\n        rows: [[hello]]\n", 1)
			},
			[]string{"sql.select.code-py", "rows"}},
		{"setup on a non-sql item",
			func(s string) string {
				return strings.Replace(s, "        expected: |\n          hello\n", "        expected: hello\n        setup: x\n", 1)
			},
			[]string{"sql.select.code-py", "setup"}},
		{"error step without predict",
			func(s string) string {
				return strings.Replace(s, "          predict:\n            prompt: Why does this query fail?\n            options:\n              - {text: The column name is misspelled, correct: true}\n              - {text: SELECT needs a WHERE clause}\n", "", 1)
			},
			[]string{"sql/sql.yaml", "sql.select", "step 1", "error", "predict"}},
		{"error step without example",
			func(s string) string {
				return strings.Replace(s, "      example:\n        lang: sql\n        code: |\n          SELECT nme FROM users;\n", "", 1)
			},
			[]string{"sql.select", "step 1", "error", "example"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := loadErr(t, map[string]string{"sql/sql.yaml": c.edit(codeYAML)})
			wantAll(t, msg, c.parts...)
		})
	}
}
