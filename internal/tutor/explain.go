package tutor

import (
	"context"
	"fmt"
	"strings"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
)

// explainSystem sets the tutor's voice for explanations.
const explainSystem = `You are a senior software engineer tutoring a learner who practices programming, ` +
	`architecture, Git, Docker, Linux, SQL and AWS with short closed questions. ` +
	`Explain in clear, plain English. Be concise: at most 150 words, short paragraphs, ` +
	`no headings, no markdown tables. If the learner was wrong, name the exact misconception ` +
	`behind their answer and why it fails, then the mental model that gets it right. ` +
	`If they were right, deepen the idea with one practical example or pitfall. ` +
	`Code, when it helps, is at most 6 lines. Reply with the explanation only.`

// Explain asks the model for a deeper explanation of an answered item.
func (t *Tutor) Explain(ctx context.Context, it domain.Item, c domain.Concept, r check.Response, res check.Result) (string, error) {
	return ask(ctx, t.models.Explain, Request{System: explainSystem, Prompt: explainPrompt(it, c, r, res)})
}

func explainPrompt(it domain.Item, c domain.Concept, r check.Response, res check.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Concept: %s", c.Title)
	if c.Summary != "" {
		fmt.Fprintf(&b, " — %s", c.Summary)
	}
	fmt.Fprintf(&b, "\nQuestion type: %s\nQuestion: %s\n", it.Type, it.Prompt)
	if it.Code != "" {
		fmt.Fprintf(&b, "Code:\n%s\n", strings.TrimRight(it.Code, "\n"))
	}
	if len(it.Options) > 0 {
		b.WriteString("Options:\n")
		for i, o := range it.Options {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, o.Text)
		}
	}
	if it.Type == domain.Code {
		fmt.Fprintf(&b, "Language: %s\n", it.Language)
		if it.Setup != "" {
			fmt.Fprintf(&b, "Setup SQL:\n%s\n", strings.TrimRight(it.Setup, "\n"))
		}
	}
	if it.Type == domain.Parsons {
		b.WriteString("Lines to order:\n" + strings.Join(it.Lines, "\n") + "\n")
	}
	verdict := "incorrect"
	if res.Correct {
		verdict = "correct"
	}
	fmt.Fprintf(&b, "\nLearner's answer (%s):\n%s\n", verdict, DescribeResponse(it, r))
	fmt.Fprintf(&b, "Correct answer:\n%s\n", res.Expected)
	if res.ChosenMisconception != "" {
		fmt.Fprintf(&b, "Misconception behind the answer: %s\n", res.ChosenMisconception)
	}
	if it.Explanation != "" {
		fmt.Fprintf(&b, "Explanation already shown: %s\n", it.Explanation)
	}
	b.WriteString("\nExplain this more deeply than the explanation already shown.")
	return b.String()
}

// DescribeResponse renders what the learner answered as readable text.
func DescribeResponse(it domain.Item, r check.Response) string {
	option := func(i int) (string, bool) {
		if i < 0 || i >= len(it.Options) {
			return "", false
		}
		return it.Options[i].Text, true
	}
	var s string
	switch it.Type {
	case domain.MCQ, domain.Output:
		if len(it.Options) > 0 {
			s, _ = option(r.Choice)
		} else {
			s = r.Text
		}
	case domain.Multi:
		var picked []string
		for _, i := range r.Choices {
			if t, ok := option(i); ok {
				picked = append(picked, t)
			}
		}
		s = strings.Join(picked, "; ")
	case domain.Cloze:
		s = strings.Join(r.Blanks, " | ")
	case domain.Parsons:
		s = strings.Join(r.Order, "\n")
	case domain.Bug:
		lines := strings.Split(it.Code, "\n")
		if r.Line >= 1 && r.Line <= len(lines) {
			s = fmt.Sprintf("line %d: %s", r.Line, strings.TrimSpace(lines[r.Line-1]))
		}
	case domain.Command:
		s = r.Text
	case domain.Code:
		s = r.Code
		if run := r.Run; run != nil {
			switch {
			case run.TimedOut:
				s += "\n\nIt was stopped at the time limit."
			case run.Truncated:
				s += "\n\nIt was stopped at the output limit."
			}
			if run.Error != "" {
				s += "\n\nIt failed with:\n" + run.Error
			} else if it.Language == domain.SQL {
				s += "\n\nIt returned:\n" + check.FormatRows(run.Rows)
			} else {
				s += "\n\nIt printed:\n" + run.Stdout
			}
		}
	}
	if strings.TrimSpace(s) == "" {
		return "(no answer)"
	}
	return s
}
