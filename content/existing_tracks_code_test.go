package content_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/content"
	"github.com/kevnnard/devtrainer/internal/check"
	loader "github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
)

// existingCodeTracks are the M1 tracks that gained code items in M2 (S6),
// with the languages their code items may use. SQL is covered by
// TestSQLConceptsHaveCodeItems and is deliberately left out here.
var existingCodeTracks = map[string][]string{
	"python":     {"python"},
	"go":         {"go"},
	"typescript": {"typescript"},
	"logic":      {"python", "go"}, // logic is language-neutral; algorithms run in Python or Go
}

// noCodeConcepts lists concepts of existingCodeTracks where writing code
// does not make sense, with the reason. Empty: every concept of these
// tracks is about behaviour a short program can show.
var noCodeConcepts = map[string]string{}

// TestExistingTracksHaveCodeItems keeps at least one runnable code item, in
// a language the track allows, on every concept of python, go, typescript
// and logic, except the documented exclusions.
func TestExistingTracksHaveCodeItems(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for track, langs := range existingCodeTracks {
		cs := cat.ConceptsIn(track)
		if len(cs) == 0 {
			t.Errorf("track %s has no concepts", track)
		}
		for _, c := range cs {
			n := 0
			for _, it := range cat.ItemsFor(c.ID) {
				if it.Type != domain.Code {
					continue
				}
				if !slices.Contains(langs, it.Language) {
					t.Errorf("%s: language %q is not one of %v", it.ID, it.Language, langs)
					continue
				}
				n++
			}
			_, excluded := noCodeConcepts[c.ID]
			switch {
			case n == 0 && !excluded:
				t.Errorf("%s has no code item", c.ID)
			case n > 0 && excluded:
				t.Errorf("%s is listed as excluded but has %d code items", c.ID, n)
			}
		}
	}
}

// TestSQLCodeItemsUnchanged guards S6's constraint that the existing SQL
// code exercises stay as shipped by T1: nine of them.
func TestSQLCodeItemsUnchanged(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, it := range cat.Items {
		if it.Type == domain.Code && it.Language == domain.SQL {
			n++
		}
	}
	if n != 9 {
		t.Errorf("sql has %d code items, want 9", n)
	}
}

// existingTypeScriptItems returns the typescript code items of the
// existing typescript track.
func existingTypeScriptItems(t *testing.T) []domain.Item {
	t.Helper()
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	var out []domain.Item
	for _, it := range cat.ItemsByConcept() {
		for _, i := range it {
			if c, ok := cat.Concept(i.ConceptID); ok && c.Track == "typescript" && i.Type == domain.Code {
				out = append(out, i)
			}
		}
	}
	slices.SortFunc(out, func(a, b domain.Item) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if len(out) == 0 {
		t.Fatal("the typescript track has no code items")
	}
	return out
}

// TestTypeScriptSolutionsTypeCheck runs `deno check` on every typescript
// solution: node strips types without checking them, so this is what
// keeps the type-level constructs the items teach accurate. Starters are
// checked and logged only (they may type-check and still print the wrong
// output). Deno's cache lives in this test's temp dir. Skips when deno is
// not installed.
func TestTypeScriptSolutionsTypeCheck(t *testing.T) {
	deno, err := exec.LookPath("deno")
	if err != nil {
		t.Skipf("deno not installed: %v", err)
	}
	t.Setenv("DENO_DIR", filepath.Join(t.TempDir(), "deno-cache"))
	t.Setenv("NO_COLOR", "1")
	checkTS := func(t *testing.T, src string) (string, error) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.ts"), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, deno, "check", "--quiet", "main.ts")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	// Control: a real type error must fail, or a passing check proves nothing.
	if out, err := checkTS(t, "const n: number = \"x\";\nconsole.log(n);\n"); err == nil {
		t.Fatalf("deno check accepted a type error; output:\n%s", out)
	} else {
		t.Logf("control rejected as expected (%v)", err)
	}
	for _, it := range existingTypeScriptItems(t) {
		t.Run(it.ID, func(t *testing.T) {
			if out, err := checkTS(t, it.Solution); err != nil {
				t.Errorf("solution does not type-check (%v):\n%s", err, out)
			}
			if out, err := checkTS(t, it.Starter); err != nil {
				t.Logf("starter does not type-check (allowed) (%v):\n%s", err, out)
			} else {
				t.Log("starter type-checks")
			}
		})
	}
}

// TestTypeScriptItemsRunOnDeno runs the typescript items through the
// runner's deno plan (node hidden from its LookPath): the alternate
// toolchain must grade them the same way. Skips when deno is not installed.
func TestTypeScriptItemsRunOnDeno(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skipf("deno not installed: %v", err)
	}
	t.Setenv("DENO_DIR", filepath.Join(t.TempDir(), "deno-cache"))
	r := runner.New()
	r.TempDir = t.TempDir()
	r.LookPath = func(name string) (string, error) {
		if name == "node" {
			return "", errors.New("hidden: test runs typescript on deno")
		}
		return exec.LookPath(name)
	}
	tc, err := r.Toolchain("typescript")
	if err != nil || tc.Tool != "deno" {
		t.Fatalf("typescript toolchain = %+v, %v; want deno", tc, err)
	}
	t.Logf("typescript via %s (%s)", tc.Tool, tc.Path)
	for _, it := range existingTypeScriptItems(t) {
		t.Run(it.ID, func(t *testing.T) {
			run := func(src string) check.CodeRun {
				out, err := r.Run(context.Background(), runner.Program{Language: it.Language, Code: src})
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				return out
			}
			if got := run(it.Solution); !check.Code(it, got).Correct {
				t.Errorf("solution fails on deno: stdout %q error %q\nwant %q", got.Stdout, got.Error, it.Expected)
			}
			got := run(it.Starter)
			if got.Failed() {
				t.Errorf("starter does not run cleanly on deno: error %q", got.Error)
			}
			if check.Code(it, got).Correct {
				t.Error("the starter already passes on deno")
			}
		})
	}
}

// TestExistingTracksCodeRuns runs every code item of the existing tracks
// for real, each in a directory under this test's own temp dir. The
// solution must pass. The starter must run cleanly (no compile or runtime
// error, so it fails for what it teaches, not for missing setup) and
// print the wrong output. A missing toolchain skips, never passes.
func TestExistingTracksCodeRuns(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	r := runner.New()
	r.TempDir = t.TempDir()
	for _, it := range cat.Items {
		if it.Type != domain.Code {
			continue
		}
		c, ok := cat.Concept(it.ConceptID)
		if !ok {
			t.Fatalf("%s: unknown concept %s", it.ID, it.ConceptID)
		}
		if _, ok := existingCodeTracks[c.Track]; !ok {
			continue
		}
		t.Run(it.ID, func(t *testing.T) {
			tc, err := r.Toolchain(it.Language)
			if err != nil {
				t.Skipf("toolchain missing: %v", err)
			}
			t.Logf("%s via %s (%s)", it.Language, tc.Tool, tc.Path)
			run := func(src string) check.CodeRun {
				out, err := r.Run(context.Background(), runner.Program{Language: it.Language, Code: src})
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				return out
			}
			if got := run(it.Solution); !check.Code(it, got).Correct {
				t.Errorf("solution fails: stdout %q error %q timedOut %v truncated %v\nwant %q",
					got.Stdout, got.Error, got.TimedOut, got.Truncated, it.Expected)
			}
			got := run(it.Starter)
			if got.Failed() {
				t.Errorf("starter does not run cleanly (it should fail on output only): error %q timedOut %v", got.Error, got.TimedOut)
			}
			if check.Code(it, got).Correct {
				t.Error("the starter already passes")
			}
		})
	}
}
