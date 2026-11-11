package srs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// pureImports keeps domain, srs and check pure (spec S7): no I/O, network,
// UI or clock. The FSRS library is pure as configured (fuzz disabled).
var pureImports = map[string]bool{
	"cmp":     true,
	"fmt":     true,
	"math":    true,
	"slices":  true,
	"strings": true,
	"time":    true,
	"unicode": true,
	"github.com/kevnnard/devtrainer/internal/domain": true,
	"github.com/open-spaced-repetition/go-fsrs/v3":   true,
}

// contentImports adds what the loader needs to read YAML from an fs.FS,
// and nothing that touches the OS file system directly.
var contentImports = map[string]bool{
	"bytes":            true,
	"errors":           true,
	"io/fs":            true,
	"path":             true,
	"gopkg.in/yaml.v3": true,
	"github.com/kevnnard/devtrainer/internal/check": true,
}

func TestPackagesArePure(t *testing.T) {
	for _, dir := range []string{".", "../domain", "../check"} {
		checkDir(t, dir, pureImports)
	}
	both := map[string]bool{}
	for k := range pureImports {
		both[k] = true
	}
	for k := range contentImports {
		both[k] = true
	}
	checkDir(t, "../content", both)
}

func checkDir(t *testing.T, dir string, allowed map[string]bool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no Go files in %s", dir)
	}
	for _, path := range files {
		if !strings.HasSuffix(path, "_test.go") {
			checkPure(t, path, allowed)
		}
	}
}

func checkPure(t *testing.T, path string, allowed map[string]bool) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if !allowed[p] {
			t.Errorf("%s imports %q, which is not allowed in pure packages", path, p)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until") {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" {
				t.Errorf("%s calls time.%s; time must be injected", path, sel.Sel.Name)
			}
		}
		return true
	})
}
