package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kevnnard/devtrainer/internal/check"
)

// plan is how one language runs: the file the code goes into, the
// commands to run in order in the run directory, and extra environment.
type plan struct {
	file  string
	steps [][]string
	env   []string
}

func planFor(tc Toolchain) plan {
	switch tc.Language {
	case "python":
		return plan{file: "main.py", steps: [][]string{{tc.Path, "-I", "main.py"}}, env: []string{"PYTHONDONTWRITEBYTECODE=1"}}
	case "go":
		return plan{file: "main.go", steps: [][]string{{tc.Path, "run", "main.go"}}, env: []string{"GOTOOLCHAIN=local", "GOFLAGS="}}
	case "javascript":
		return plan{file: "main.mjs", steps: [][]string{{tc.Path, "main.mjs"}}}
	case "typescript":
		if tc.Tool == "deno" {
			return plan{file: "main.ts", steps: [][]string{{tc.Path, "run", "--quiet", "main.ts"}}, env: []string{"NO_COLOR=1"}}
		}
		return plan{file: "main.ts", steps: [][]string{{tc.Path, "main.ts"}}}
	case "java":
		return plan{file: "Main.java", steps: [][]string{{tc.Path, "Main.java"}}}
	case "csharp":
		return plan{file: "app.cs", steps: [][]string{{tc.Path, "run", "app.cs"}},
			env: []string{"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1"}}
	case "rust":
		return plan{file: "main.rs", steps: [][]string{
			{tc.Path, "--edition", "2024", "-o", "main", "main.rs"},
			{"./main"},
		}}
	}
	return plan{}
}

// runProcess writes the code into a fresh temporary directory, runs the
// language's commands there and removes the directory.
func (r *Runner) runProcess(ctx context.Context, tc Toolchain, p Program) (check.CodeRun, error) {
	pl := planFor(tc)
	if pl.file == "" {
		return check.CodeRun{}, fmt.Errorf("runner: no plan for %s", tc.Language)
	}
	dir, err := os.MkdirTemp(r.TempDir, "devtrainer-run-*")
	if err != nil {
		return check.CodeRun{}, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, pl.file), []byte(p.Code), 0o600); err != nil {
		return check.CodeRun{}, err
	}

	ctx, kill := context.WithCancel(ctx)
	defer kill()
	out := &sink{left: r.maxOutput(), kill: kill}
	stdout, stderr := &stream{s: out}, &stream{s: out}
	var run check.CodeRun
	for _, args := range pl.steps {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), pl.env...)
		cmd.Stdin = nil
		stdout.b.Reset() // only the last step's stdout is the program's output
		cmd.Stdout, cmd.Stderr = stdout, stderr
		cmd.WaitDelay = time.Second
		groupKill(cmd)
		err := cmd.Run()
		if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
			err = nil // a helper (a build server) kept the pipes open after a clean exit
		}
		run.Stdout = clean(stdout.String(), dir)
		run.Error = clean(strings.TrimRight(stderr.String(), "\n"), dir)
		switch {
		case out.isOver():
			run.Truncated = true
			return run, nil
		case ctx.Err() != nil:
			run.TimedOut = true
			return run, nil
		case err != nil:
			if tc.Language == "csharp" && strings.Contains(run.Error, "build failed") {
				// dotnet prints compiler errors on stdout
				run.Error = strings.TrimRight(run.Stdout, "\n") + "\n\n" + run.Error
				run.Stdout = ""
			}
			if run.Error == "" {
				run.Error = err.Error()
			}
			return run, nil
		}
	}
	run.Error = "" // a clean exit with warnings on stderr still passes
	return run, nil
}

// clean shows paths inside the run directory relative to it, so errors
// read "main.py", not "/tmp/devtrainer-run-123/main.py".
func clean(s, dir string) string {
	s = strings.ReplaceAll(s, dir+string(filepath.Separator), "")
	return strings.ReplaceAll(s, dir, ".")
}

// sink is the output budget shared by a run's stdout and stderr; going
// over it stops the program.
type sink struct {
	mu   sync.Mutex
	left int
	over bool
	kill func()
}

func (s *sink) isOver() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.over
}

type stream struct {
	s *sink
	b bytes.Buffer
}

func (w *stream) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	if w.s.over {
		return len(p), nil
	}
	if len(p) > w.s.left {
		w.b.Write(p[:w.s.left])
		w.s.left, w.s.over = 0, true
		w.s.kill()
		return len(p), nil
	}
	w.s.left -= len(p)
	return w.b.Write(p)
}

func (w *stream) String() string {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	return w.b.String()
}
