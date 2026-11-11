package pi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/tutor"
)

// fakePi writes a shell script that stands in for the pi CLI.
func fakePi(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pi")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCompletePassesModelSystemAndPrompt(t *testing.T) {
	// Print every argument on its own line so the test can check them.
	bin := fakePi(t, `for a in "$@"; do printf '%s\n' "$a"; done; echo "[warn] noise" >&2`)
	m := Model{Command: bin, Model: "claude-bridge/claude-sonnet-5-5"}

	out, err := m.Complete(context.Background(), tutor.Request{System: "You are a tutor.", Prompt: "@not-a-file"})

	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(out), "\n")
	for _, want := range []string{"-p", "--no-session", "--no-tools", "--no-skills", "--no-context-files", "--no-mcp", "--no-prompt-templates", "--model", "claude-bridge/claude-sonnet-5-5", "--system-prompt", "You are a tutor.", "--"} {
		if !contains(args, want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if last := args[len(args)-1]; last != " @not-a-file" {
		t.Errorf("prompt arg = %q; a leading @ must not be read as a file", last)
	}
	if strings.Contains(out, "noise") {
		t.Error("stderr must not be part of the answer")
	}
}

func TestCompleteReportsAFailingCommand(t *testing.T) {
	bin := fakePi(t, `echo "Error: Model not found" >&2; exit 1`)

	_, err := Model{Command: bin, Model: "x/y"}.Complete(context.Background(), tutor.Request{Prompt: "hi"})

	if err == nil || !strings.Contains(err.Error(), "Model not found") {
		t.Errorf("err = %v, want stderr in the error", err)
	}
}

func TestCompleteMissingCommand(t *testing.T) {
	_, err := Model{Command: filepath.Join(t.TempDir(), "nope"), Model: "x/y"}.Complete(context.Background(), tutor.Request{Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "Gentle Shell") {
		t.Errorf("err = %v, want a hint about installing Gentle Shell", err)
	}
}

func TestCompleteEmptyAnswer(t *testing.T) {
	_, err := Model{Command: fakePi(t, `echo "   "`), Model: "x/y"}.Complete(context.Background(), tutor.Request{Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %v", err)
	}
}

func TestCompleteTimesOut(t *testing.T) {
	m := Model{Command: fakePi(t, `exec sleep 5`), Model: "x/y", Timeout: 100 * time.Millisecond}
	start := time.Now()

	_, err := m.Complete(context.Background(), tutor.Request{Prompt: "hi"})

	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(start) > 3*time.Second {
		t.Errorf("err = %v after %v", err, time.Since(start))
	}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

func TestCompleteDefaultsCommandAndTimeout(t *testing.T) {
	m := Model{Model: "x/y"}
	if m.command() != "pi" || m.timeout() != DefaultTimeout {
		t.Errorf("command %q timeout %v", m.command(), m.timeout())
	}
}
