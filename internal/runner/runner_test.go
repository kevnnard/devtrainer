package runner_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/runner"
)

// available returns a real runner, skipping the test when lang's
// toolchain is not installed here.
func available(t *testing.T, lang string) *runner.Runner {
	t.Helper()
	r := runner.New()
	if _, err := r.Toolchain(lang); err != nil {
		t.Skipf("toolchain missing: %v", err)
	}
	return r
}

func run(t *testing.T, r *runner.Runner, p runner.Program) check.CodeRun {
	t.Helper()
	got, err := r.Run(context.Background(), p)
	if err != nil {
		t.Fatalf("Run(%s): %v", p.Language, err)
	}
	return got
}

// hello is a program per language printing "hello 3", and a broken one
// whose compiler or runtime error mentions want.
var hello = []struct {
	lang, ok, bad, want string
}{
	{"python", "print('hello', 1 + 2)\n", "print(1 / 0)\n", "ZeroDivisionError"},
	{"go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\", 1+2) }\n",
		"package main\n\nfunc main() { x := 1 }\n", "declared and not used"},
	{"javascript", "console.log('hello', 1 + 2);\n", "console.log(missing);\n", "ReferenceError"},
	{"typescript", "const n: number = 1 + 2;\nconsole.log(`hello ${n}`);\n", "const n: number = 1;\nnope(n);\n", "nope"},
	{"java", "public class Main {\n  public static void main(String[] a) {\n    System.out.println(\"hello \" + (1 + 2));\n  }\n}\n",
		"public class Main {\n  public static void main(String[] a) {\n    int x = \"s\";\n  }\n}\n", "incompatible types"},
	{"csharp", "Console.WriteLine($\"hello {1 + 2}\");\n", "int x = \"s\";\n", "CS0029"},
	{"rust", "fn main() {\n    println!(\"hello {}\", 1 + 2);\n}\n", "fn main() {\n    let x: i32 = \"s\";\n}\n", "mismatched types"},
}

func TestRunPassAndFail(t *testing.T) {
	for _, c := range hello {
		t.Run(c.lang, func(t *testing.T) {
			r := available(t, c.lang)
			got := run(t, r, runner.Program{Language: c.lang, Code: c.ok})
			if got.Failed() || strings.TrimSpace(got.Stdout) != "hello 3" {
				t.Fatalf("ok program: %+v", got)
			}
			got = run(t, r, runner.Program{Language: c.lang, Code: c.bad})
			if !got.Failed() || !strings.Contains(got.Error, c.want) {
				t.Fatalf("broken program: want an error mentioning %q, got %+v", c.want, got)
			}
			if strings.Contains(got.Error, "devtrainer-run-") {
				t.Errorf("the error leaks the temp directory: %s", got.Error)
			}
		})
	}
}

func TestRunEachInAFreshDirectory(t *testing.T) {
	r := available(t, "python")
	write := "open('left.txt', 'w').write('x')\nprint('ok')\n"
	read := "import os\nprint(os.path.exists('left.txt'))\n"
	run(t, r, runner.Program{Language: "python", Code: write})
	if got := run(t, r, runner.Program{Language: "python", Code: read}); strings.TrimSpace(got.Stdout) != "False" {
		t.Errorf("a file from the previous run is visible: %+v", got)
	}
}

func TestRunTimeout(t *testing.T) {
	r := available(t, "python")
	r.Timeouts = map[string]time.Duration{"python": 500 * time.Millisecond}
	start := time.Now()
	got := run(t, r, runner.Program{Language: "python", Code: "while True:\n    pass\n"})
	if !got.TimedOut || !got.Failed() {
		t.Errorf("want a timeout, got %+v", got)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the run took %v after a 500ms limit", d)
	}
}

func TestRunTimeoutKillsChildren(t *testing.T) {
	r := available(t, "go")
	r.Timeouts = map[string]time.Duration{"go": 3 * time.Second}
	start := time.Now()
	got := run(t, r, runner.Program{Language: "go", Code: "package main\n\nfunc main() { for {} }\n"})
	if !got.TimedOut {
		t.Errorf("want a timeout, got %+v", got)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("go run's binary outlived the limit: %v", d)
	}
}

func TestRunOutputCap(t *testing.T) {
	r := available(t, "python")
	r.MaxOutput = 1 << 10
	start := time.Now()
	got := run(t, r, runner.Program{Language: "python", Code: "while True:\n    print('x' * 80)\n"})
	if !got.Truncated || !got.Failed() {
		t.Errorf("want the output cap to stop it, got truncated=%v", got.Truncated)
	}
	if len(got.Stdout)+len(got.Error) > 1<<10 {
		t.Errorf("kept %d bytes, cap is 1024", len(got.Stdout)+len(got.Error))
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the capped run took %v", d)
	}
}

func TestMissingToolchain(t *testing.T) {
	r := runner.New()
	r.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	r.Getenv = func(string) string { return "" }
	for _, c := range []struct{ lang, tool string }{
		{"python", "python3"}, {"go", "go"}, {"javascript", "node"}, {"typescript", "node"},
		{"java", "java"}, {"csharp", "dotnet"}, {"rust", "rustc"},
	} {
		_, err := r.Run(context.Background(), runner.Program{Language: c.lang, Code: "x"})
		var miss *runner.MissingError
		if !errors.As(err, &miss) || miss.Tool != c.tool || !strings.Contains(err.Error(), c.tool) {
			t.Errorf("%s: err = %v, want a MissingError naming %s", c.lang, err, c.tool)
		}
	}
	if _, err := r.Toolchain("sql"); err != nil {
		t.Errorf("sql is built in, got %v", err)
	}
	missing := r.Missing([]string{"sql", "python", "rust"})
	if len(missing) != 2 || missing["python"] == nil || missing["rust"] == nil {
		t.Errorf("Missing = %v", missing)
	}
}

func TestJavaResolution(t *testing.T) {
	onPath := func(name string) (string, error) {
		if name == "java" {
			return "/usr/bin/java-on-path", nil
		}
		return "", exec.ErrNotFound
	}
	// DEVTRAINER_JAVA wins, and must exist.
	r := runner.New()
	r.LookPath = onPath
	r.Getenv = func(k string) string {
		if k == "DEVTRAINER_JAVA" {
			return "/nope/java"
		}
		return ""
	}
	_, err := r.Toolchain("java")
	var miss *runner.MissingError
	if !errors.As(err, &miss) || !strings.Contains(err.Error(), "DEVTRAINER_JAVA") {
		t.Errorf("a bad DEVTRAINER_JAVA: err = %v", err)
	}

	// No DEVTRAINER_JAVA and no Homebrew: java on PATH.
	r = runner.New()
	r.LookPath = onPath
	r.Getenv = func(k string) string {
		if k == "HOMEBREW_PREFIX" {
			return t.TempDir() // a prefix without openjdk@21
		}
		return ""
	}
	tc, err := r.Toolchain("java")
	if err != nil || tc.Path != "/usr/bin/java-on-path" {
		t.Errorf("java on PATH: %+v, %v", tc, err)
	}
}

func TestJavaPrefersJDK21(t *testing.T) {
	r := available(t, "java")
	tc, _ := r.Toolchain("java")
	if tc.Note != "" {
		t.Logf("note: %s", tc.Note)
	}
	got := run(t, r, runner.Program{Language: "java", Code: "public class Main {\n  public static void main(String[] a) {\n    System.out.println(Runtime.version().feature());\n  }\n}\n"})
	if strings.TrimSpace(got.Stdout) < "21" && tc.Note == "" {
		t.Errorf("ran JDK %q without a note", strings.TrimSpace(got.Stdout))
	}
}

const shop = `
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, score REAL, city TEXT);
INSERT INTO users VALUES (1, 'Bo', 2.5, NULL), (2, 'Ana', 3, 'Lima');
`

func TestRunSQL(t *testing.T) {
	r := runner.New()
	got := run(t, r, runner.Program{Language: "sql", Setup: shop, Code: "SELECT name, score, city, id FROM users ORDER BY name;"})
	if got.Failed() {
		t.Fatalf("query failed: %+v", got)
	}
	if want := "Ana | 3.0 | Lima | 2\nBo | 2.5 | NULL | 1"; check.FormatRows(got.Rows) != want {
		t.Errorf("rows = %q, want %q", check.FormatRows(got.Rows), want)
	}
	if strings.Join(got.Columns, ",") != "name,score,city,id" {
		t.Errorf("columns = %v", got.Columns)
	}

	got = run(t, r, runner.Program{Language: "sql", Setup: shop, Code: "SELECT nme FROM users;"})
	if !got.Failed() || !strings.Contains(got.Error, "no such column: nme") {
		t.Errorf("bad column: %+v", got)
	}

	// Each run gets its own database.
	run(t, r, runner.Program{Language: "sql", Setup: shop, Code: "DELETE FROM users; SELECT 1;"})
	got = run(t, r, runner.Program{Language: "sql", Setup: shop, Code: "SELECT count(*) FROM users;"})
	if check.FormatRows(got.Rows) != "2" {
		t.Errorf("count = %v: a previous run leaked into this one", got.Rows)
	}

	// Several statements: the rows of the last query.
	got = run(t, r, runner.Program{Language: "sql", Setup: shop,
		Code: "INSERT INTO users (name) VALUES ('Cy');\nSELECT name FROM users ORDER BY id;"})
	if check.FormatRows(got.Rows) != "Bo\nAna\nCy" {
		t.Errorf("multi-statement rows = %v (%s)", got.Rows, got.Error)
	}

	got = run(t, r, runner.Program{Language: "sql", Setup: shop, Code: "SELECT name FROM users WHERE id = 99;"})
	if got.Failed() || len(got.Rows) != 0 {
		t.Errorf("empty result: %+v", got)
	}
}

func TestRunSQLTimeoutAndCap(t *testing.T) {
	r := runner.New()
	r.Timeouts = map[string]time.Duration{"sql": 300 * time.Millisecond}
	forever := "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n) SELECT count(*) FROM n;"
	start := time.Now()
	got := run(t, r, runner.Program{Language: "sql", Code: forever})
	if !got.TimedOut {
		t.Errorf("want a timeout, got %+v", got)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("the query ran %v past a 300ms limit", d)
	}

	r = runner.New()
	many := "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n LIMIT 5000) SELECT i FROM n;"
	got = run(t, r, runner.Program{Language: "sql", Code: many})
	if !got.Truncated || len(got.Rows) > runner.MaxRows {
		t.Errorf("want the row cap, got truncated=%v with %d rows", got.Truncated, len(got.Rows))
	}
}

func TestRunSQLBadSetupIsTheExercisesFault(t *testing.T) {
	_, err := runner.New().Run(context.Background(), runner.Program{Language: "sql", Setup: "CREATE TABL x;", Code: "SELECT 1;"})
	if err == nil || !strings.Contains(err.Error(), "setup") {
		t.Errorf("err = %v, want a setup error", err)
	}
}
