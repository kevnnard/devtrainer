// Package pi runs the AI through the pi CLI, the engine of Gentle Shell.
// It runs pi in non-interactive mode with no tools, session, skills, MCP,
// prompt templates or project context, so the model only answers the
// question. The app never reads pi's credentials: pi uses the learner's
// own login.
package pi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kevnnard/devtrainer/internal/tutor"
)

// DefaultTimeout is how long one answer may take.
const DefaultTimeout = 90 * time.Second

// Model is one model reached through pi. It implements tutor.Model.
type Model struct {
	Command string        // the pi binary, "pi" by default
	Model   string        // like "claude-bridge/claude-sonnet-5-5"
	Timeout time.Duration // DefaultTimeout when zero
}

var _ tutor.Model = Model{}

func (m Model) command() string {
	if m.Command == "" {
		return "pi"
	}
	return m.Command
}

func (m Model) timeout() time.Duration {
	if m.Timeout <= 0 {
		return DefaultTimeout
	}
	return m.Timeout
}

// Complete asks the model and returns its answer.
func (m Model) Complete(ctx context.Context, r tutor.Request) (string, error) {
	command, timeout := m.command(), m.timeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// pi reads a message that starts with @ as a file to attach.
	prompt := r.Prompt
	if strings.HasPrefix(prompt, "@") {
		prompt = " " + prompt
	}
	args := []string{
		"-p", "--no-session", "--no-tools", "--no-skills", "--no-context-files",
		"--no-mcp", "--no-prompt-templates", "--model", m.Model,
	}
	if r.System != "" {
		args = append(args, "--system-prompt", r.System)
	}
	args = append(args, "--", prompt)

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.WaitDelay = time.Second // do not wait for children that keep the pipes open
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("pi %s timed out after %v", m.Model, timeout)
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, exec.ErrDot) || isNotExist(err):
		return "", fmt.Errorf("pi not found (%s): install Gentle Shell or set its path in the config", command)
	case err != nil:
		return "", fmt.Errorf("pi %s: %v: %s", m.Model, err, lastLine(stderr.String()))
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("pi %s gave an empty answer: %s", m.Model, lastLine(stderr.String()))
	}
	return out, nil
}

func isNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such file")
}

// lastLine is the last non-empty line of s: pi prints warnings first and
// the real error last.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
