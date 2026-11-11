// Package runner runs a learner's code exercise for real: sql in-process
// against a fresh SQLite :memory: database, every other language as a
// local toolchain process in a fresh temporary directory. Each run has a
// time limit and an output cap, and reports the real compiler or runtime
// error when the program fails. A language whose toolchain is not
// installed reports a *MissingError and runs nothing.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
)

// Program is one piece of code to run.
type Program struct {
	Language string // one of domain.Languages
	Code     string
	Setup    string // sql only: statements run before Code
}

// DefaultMaxOutput is the output cap of one run, in bytes of stdout and
// stderr together (for sql, of the result text).
const DefaultMaxOutput = 64 << 10

// MaxRows caps the rows a sql run returns.
const MaxRows = 1000

// MinJavaMajor is the JDK the java track is written for.
const MinJavaMajor = 21

// DefaultTimeouts is how long each language may take, compiling included.
// C# gets the largest budget: `dotnet run app.cs` builds a project first.
var DefaultTimeouts = map[string]time.Duration{
	"sql":        5 * time.Second,
	"python":     10 * time.Second,
	"javascript": 10 * time.Second,
	"typescript": 10 * time.Second,
	"go":         30 * time.Second,
	"java":       30 * time.Second,
	"rust":       30 * time.Second,
	"csharp":     120 * time.Second,
}

// MissingError says which tool a language needs and could not be found.
type MissingError struct {
	Language string
	Tool     string // the program looked for: python3, dotnet, java…
	Detail   string // optional: where it was looked for
}

func (e *MissingError) Error() string {
	msg := fmt.Sprintf("%s: %s not found", e.Language, e.Tool)
	if e.Detail != "" {
		msg += " (" + e.Detail + ")"
	}
	return msg
}

// Toolchain is how a language runs on this machine.
type Toolchain struct {
	Language string
	Tool     string // the program used: "python3", "java"…; "built in" for sql
	Path     string // its resolved path; empty for sql
	Note     string // a warning worth showing, such as an old JDK
}

// Runner runs programs. The zero value is not usable; call New.
type Runner struct {
	// Timeouts overrides DefaultTimeouts per language.
	Timeouts map[string]time.Duration
	// MaxOutput is the output cap in bytes (DefaultMaxOutput when 0).
	MaxOutput int
	// LookPath finds a program on PATH (exec.LookPath).
	LookPath func(string) (string, error)
	// Getenv reads the environment (os.Getenv).
	Getenv func(string) string
	// TempDir is the parent of each run's directory (os.TempDir when "").
	TempDir string

	mu    sync.Mutex
	cache map[string]toolResult
}

type toolResult struct {
	tc  Toolchain
	err error
}

// New returns a runner using the real PATH and environment.
func New() *Runner {
	return &Runner{LookPath: exec.LookPath, Getenv: os.Getenv}
}

func (r *Runner) timeout(lang string) time.Duration {
	if d, ok := r.Timeouts[lang]; ok && d > 0 {
		return d
	}
	if d, ok := DefaultTimeouts[lang]; ok {
		return d
	}
	return 10 * time.Second
}

func (r *Runner) maxOutput() int {
	if r.MaxOutput > 0 {
		return r.MaxOutput
	}
	return DefaultMaxOutput
}

// Toolchain resolves how lang runs here, once per runner. A missing tool
// is a *MissingError.
func (r *Runner) Toolchain(lang string) (Toolchain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if res, ok := r.cache[lang]; ok {
		return res.tc, res.err
	}
	tc, err := r.resolve(lang)
	if r.cache == nil {
		r.cache = map[string]toolResult{}
	}
	r.cache[lang] = toolResult{tc, err}
	return tc, err
}

// Missing returns the *MissingError of every language in langs whose
// toolchain is not installed, by language.
func (r *Runner) Missing(langs []string) map[string]error {
	out := map[string]error{}
	for _, l := range langs {
		if _, err := r.Toolchain(l); err != nil {
			out[l] = err
		}
	}
	return out
}

func (r *Runner) resolve(lang string) (Toolchain, error) {
	tc := Toolchain{Language: lang}
	find := func(tools ...string) (Toolchain, error) {
		for _, t := range tools {
			if p, err := r.LookPath(t); err == nil {
				tc.Tool, tc.Path = t, p
				return tc, nil
			}
		}
		return tc, &MissingError{Language: lang, Tool: tools[0], Detail: "not on PATH"}
	}
	switch lang {
	case domain.SQL:
		tc.Tool = "built in"
		return tc, nil
	case "python":
		return find("python3", "python")
	case "go":
		return find("go")
	case "javascript":
		return find("node")
	case "typescript":
		return find("node", "deno")
	case "rust":
		return find("rustc")
	case "csharp":
		return find("dotnet")
	case "java":
		return r.resolveJava()
	}
	return tc, fmt.Errorf("runner: unknown language %q", lang)
}

// resolveJava picks the JDK: $DEVTRAINER_JAVA if set, else Homebrew's
// openjdk@21 when it exists, else java on PATH (noting its major version
// when it is older than MinJavaMajor).
func (r *Runner) resolveJava() (Toolchain, error) {
	tc := Toolchain{Language: "java", Tool: "java"}
	if p := r.Getenv("DEVTRAINER_JAVA"); p != "" {
		if !executable(p) {
			return tc, &MissingError{Language: "java", Tool: p, Detail: "set by DEVTRAINER_JAVA"}
		}
		tc.Path = p
		return tc, nil
	}
	if prefix := r.brewPrefix(); prefix != "" {
		if p := filepath.Join(prefix, "opt", "openjdk@21", "bin", "java"); executable(p) {
			tc.Path = p
			return tc, nil
		}
	}
	p, err := r.LookPath("java")
	if err != nil {
		return tc, &MissingError{Language: "java", Tool: "java",
			Detail: "no DEVTRAINER_JAVA, no Homebrew openjdk@21, none on PATH"}
	}
	tc.Path = p
	if major, err := javaMajor(p); err == nil && major < MinJavaMajor {
		tc.Note = fmt.Sprintf("java on PATH is JDK %d; the java track targets JDK %d (set DEVTRAINER_JAVA or brew install openjdk@21)", major, MinJavaMajor)
	}
	return tc, nil
}

// brewPrefix is $HOMEBREW_PREFIX, else what `brew --prefix` prints.
func (r *Runner) brewPrefix() string {
	if p := r.Getenv("HOMEBREW_PREFIX"); p != "" {
		return p
	}
	brew, err := r.LookPath("brew")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, brew, "--prefix").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

var javaVersionRE = regexp.MustCompile(`version "(\d+)(?:\.(\d+))?`)

// javaMajor runs `java -version` and returns the major version.
func javaMajor(java string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, java, "-version").CombinedOutput()
	if err != nil {
		return 0, err
	}
	m := javaVersionRE.FindSubmatch(out)
	if m == nil {
		return 0, errors.New("runner: cannot read the java version")
	}
	major, _ := strconv.Atoi(string(m[1]))
	if major == 1 && len(m[2]) > 0 { // "1.8.0" is Java 8
		major, _ = strconv.Atoi(string(m[2]))
	}
	return major, nil
}

func executable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// Run runs p and reports what it produced. A failing program is not an
// error: its compiler or runtime message is in the result. The error is
// a *MissingError when the toolchain is not installed, or a problem of
// the runner itself (such as no temporary directory).
func (r *Runner) Run(ctx context.Context, p Program) (check.CodeRun, error) {
	tc, err := r.Toolchain(p.Language)
	if err != nil {
		return check.CodeRun{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout(p.Language))
	defer cancel()
	if p.Language == domain.SQL {
		return r.runSQL(ctx, p)
	}
	return r.runProcess(ctx, tc, p)
}
