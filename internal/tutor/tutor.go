// Package tutor holds devtrainer's AI use cases: explaining an answer in
// more depth and drafting new content items. It writes the prompts, asks a
// Model and checks what comes back. It knows nothing about which AI runs
// behind Model; adapters such as internal/llm/pi do.
package tutor

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Request is one question to a model.
type Request struct {
	System string // who the model is and the answer format
	Prompt string // the question itself
}

// Model is any AI that answers a request with text.
type Model interface {
	Complete(ctx context.Context, r Request) (string, error)
}

// Models is the model for each capability. A nil model turns the
// capability off.
type Models struct {
	Explain Model // deeper explanations after an answer
	Draft   Model // candidate content items
}

// ErrNoModel means no model is configured for a capability.
var ErrNoModel = errors.New("no AI model configured")

// Tutor runs the AI use cases.
type Tutor struct {
	models Models
}

// New returns a tutor that uses models.
func New(models Models) *Tutor {
	return &Tutor{models: models}
}

// ask sends r to m and returns the trimmed answer.
func ask(ctx context.Context, m Model, r Request) (string, error) {
	if m == nil {
		return "", ErrNoModel
	}
	out, err := m.Complete(ctx, r)
	if err != nil {
		return "", fmt.Errorf("tutor: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errors.New("tutor: the model gave an empty answer")
	}
	return out, nil
}
