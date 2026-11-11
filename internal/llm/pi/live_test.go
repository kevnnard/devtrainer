package pi

import (
	"context"
	"os"
	"testing"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/tutor"
)

// TestLiveExplain asks the real model through pi. It only runs with
// DEVTRAINER_LIVE=1, so the normal test suite never needs pi or the network.
func TestLiveExplain(t *testing.T) {
	if os.Getenv("DEVTRAINER_LIVE") != "1" {
		t.Skip("set DEVTRAINER_LIVE=1 to call the real model")
	}
	model := os.Getenv("DEVTRAINER_LIVE_MODEL")
	if model == "" {
		model = "claude-bridge/claude-sonnet-5-5"
	}
	m := Model{Model: model}
	it := domain.Item{ID: "git.commit.mcq", Type: domain.MCQ, Prompt: "What does `git commit` record?",
		Options: []domain.Option{
			{Text: "The staged changes", Correct: true},
			{Text: "Every change in the working tree", Misconception: "commit-takes-worktree", Explanation: "Only staged changes are committed."},
		}}
	r := check.Response{Choice: 1}

	out, err := tutor.New(tutor.Models{Explain: m}).Explain(context.Background(), it,
		domain.Concept{ID: "git.commit", Title: "Commits"}, r, check.Grade(it, r))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("explanation:\n%s", out)
}
