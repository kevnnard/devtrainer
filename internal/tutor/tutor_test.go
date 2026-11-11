package tutor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
)

// fakeModel answers every request with out (or err) and remembers the last request.
type fakeModel struct {
	out  string
	err  error
	last Request
}

func (f *fakeModel) Complete(_ context.Context, r Request) (string, error) {
	f.last = r
	return f.out, f.err
}

// catalogFS is a tiny valid catalog with one git track.
var catalogFS = fstest.MapFS{"git/git.yaml": {Data: []byte(`track:
  id: git
  title: Git
  order: 50
concepts:
  - id: git.commit
    title: Commits
    level: beginner
    items:
      - id: git.commit.cmd-basic
        type: command
        prompt: Commit the staged changes with the message "init".
        answers: ['git commit -m "init"']
`)}}

const draftValid = `track:
  id: git
  title: Git
concepts:
  - id: git.stash
    title: Stash
    summary: Put work aside without committing it.
    level: beginner
    prereqs: [git.commit]
    items:
      - id: git.stash.mcq-what
        type: mcq
        prompt: What does git stash do?
        options:
          - text: Saves uncommitted changes and cleans the working tree
            correct: true
          - text: Deletes uncommitted changes
            misconception: stash-deletes
            explanation: The changes are kept and can be re-applied with git stash pop.
        explanation: Stash shelves work in progress.
`

func TestDraftReturnsValidatedYAML(t *testing.T) {
	m := &fakeModel{out: draftValid}
	tu := New(Models{Draft: m})

	got, err := tu.Draft(context.Background(), "git", "stash", catalogFS)

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "git.stash.mcq-what") {
		t.Errorf("draft = %q", got)
	}
	for _, want := range []string{"git.commit", "stash", "misconception"} {
		if !strings.Contains(m.last.System+m.last.Prompt, want) {
			t.Errorf("request lacks %q:\nsystem: %s\nprompt: %s", want, m.last.System, m.last.Prompt)
		}
	}
}

func TestDraftExtractsFencedYAML(t *testing.T) {
	m := &fakeModel{out: "Here are your items:\n\n```yaml\n" + draftValid + "```\n\nEnjoy!"}

	got, err := New(Models{Draft: m}).Draft(context.Background(), "git", "stash", catalogFS)

	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "```") || strings.Contains(got, "Enjoy") || !strings.HasPrefix(got, "track:") {
		t.Errorf("fences or chatter left in the draft:\n%s", got)
	}
}

func TestDraftReportsValidationErrors(t *testing.T) {
	bad := strings.Replace(draftValid, "prereqs: [git.commit]", "prereqs: [git.nope]", 1)
	bad = strings.Replace(bad, "            misconception: stash-deletes\n", "", 1)
	m := &fakeModel{out: bad}

	got, err := New(Models{Draft: m}).Draft(context.Background(), "git", "stash", catalogFS)

	var inv *InvalidDraftError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v, want *InvalidDraftError", err)
	}
	for _, want := range []string{"git.nope", "misconception"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if got != "" || inv.YAML == "" {
		t.Errorf("got %q, inv.YAML %q: invalid drafts are only in the error", got, inv.YAML)
	}
}

func TestDraftRejectsDuplicatesOfTheCatalog(t *testing.T) {
	dup := strings.Replace(draftValid, "id: git.stash\n", "id: git.commit\n", 1)

	_, err := New(Models{Draft: &fakeModel{out: dup}}).Draft(context.Background(), "git", "stash", catalogFS)

	if err == nil || !strings.Contains(err.Error(), "git.commit") {
		t.Errorf("err = %v, want a duplicate concept error", err)
	}
}

func TestDraftForANewTrack(t *testing.T) {
	newTrack := strings.NewReplacer("id: git\n", "id: docker\n", "title: Git", "title: Docker",
		"git.stash", "docker.run", "prereqs: [git.commit]", "prereqs: []").Replace(draftValid)

	got, err := New(Models{Draft: &fakeModel{out: newTrack}}).Draft(context.Background(), "docker", "run", catalogFS)

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "docker.run") {
		t.Errorf("draft = %q", got)
	}
}

func TestDraftRejectsAnotherTrackOrNoConcepts(t *testing.T) {
	for name, out := range map[string]string{
		"other track": strings.Replace(draftValid, "id: git\n", "id: go\n", 1),
		"no concepts": "track:\n  id: git\n  title: Git\nconcepts: []\n",
		"not yaml":    "Sorry, I cannot help with that.",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(Models{Draft: &fakeModel{out: out}}).Draft(context.Background(), "git", "stash", catalogFS)
			if err == nil {
				t.Error("want an error")
			}
		})
	}
}

func TestDraftModelErrors(t *testing.T) {
	_, err := New(Models{Draft: &fakeModel{err: errors.New("pi not found")}}).Draft(context.Background(), "git", "x", catalogFS)
	if err == nil || !strings.Contains(err.Error(), "pi not found") {
		t.Errorf("err = %v", err)
	}
	_, err = New(Models{}).Draft(context.Background(), "git", "x", catalogFS)
	if !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel", err)
	}
}

func TestExplainSendsTheQuestionAndTheAnswer(t *testing.T) {
	m := &fakeModel{out: "  A commit records only the staged snapshot.  \n"}
	it := domain.Item{ID: "git.commit.mcq", Type: domain.MCQ, Prompt: "What does git commit record?",
		Options: []domain.Option{
			{Text: "The staged changes", Correct: true},
			{Text: "Every change in the working tree", Misconception: "commit-takes-worktree", Explanation: "Only staged."},
		}, Explanation: "Commits take the index."}
	c := domain.Concept{ID: "git.commit", Title: "Commits", Summary: "Snapshots of the index."}
	res := check.Grade(it, check.Response{Choice: 1})

	got, err := New(Models{Explain: m}).Explain(context.Background(), it, c, check.Response{Choice: 1}, res)

	if err != nil {
		t.Fatal(err)
	}
	if got != "A commit records only the staged snapshot." {
		t.Errorf("explanation = %q, want it trimmed", got)
	}
	for _, want := range []string{"Commits", "What does git commit record?", "Every change in the working tree",
		"The staged changes", "commit-takes-worktree", "incorrect"} {
		if !strings.Contains(m.last.Prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, m.last.Prompt)
		}
	}
	if !strings.Contains(m.last.System, "English") {
		t.Errorf("system prompt should ask for English: %s", m.last.System)
	}
}

func TestDescribeResponse(t *testing.T) {
	cases := []struct {
		it   domain.Item
		r    check.Response
		want string
	}{
		{domain.Item{Type: domain.Multi, Options: []domain.Option{{Text: "a"}, {Text: "b"}, {Text: "c"}}},
			check.Response{Choices: []int{0, 2}}, "a; c"},
		{domain.Item{Type: domain.Command}, check.Response{Text: "git add ."}, "git add ."},
		{domain.Item{Type: domain.Cloze}, check.Response{Blanks: []string{"x", "y"}}, "x | y"},
		{domain.Item{Type: domain.Parsons}, check.Response{Order: []string{"b", "a"}}, "b\na"},
		{domain.Item{Type: domain.Bug, Code: "x := 1\ny = 2"}, check.Response{Line: 2}, "line 2: y = 2"},
		{domain.Item{Type: domain.MCQ, Options: []domain.Option{{Text: "a"}}}, check.Response{Choice: 5}, "(no answer)"},
		{domain.Item{Type: domain.Code, Language: "python"},
			check.Response{Code: "print(x)", Run: &check.CodeRun{Error: "NameError: x"}}, "print(x)\n\nIt failed with:\nNameError: x"},
		{domain.Item{Type: domain.Code, Language: "sql"},
			check.Response{Code: "SELECT 1", Run: &check.CodeRun{Rows: [][]string{{"1"}}}}, "SELECT 1\n\nIt returned:\n1"},
	}
	for _, c := range cases {
		if got := DescribeResponse(c.it, c.r); got != c.want {
			t.Errorf("%s: got %q, want %q", c.it.Type, got, c.want)
		}
	}
}

func TestExplainWithoutModel(t *testing.T) {
	_, err := New(Models{}).Explain(context.Background(), domain.Item{}, domain.Concept{}, check.Response{}, check.Result{})
	if !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v", err)
	}
}

func TestExtractYAML(t *testing.T) {
	cases := map[string]string{
		"track:\n  id: x\n":                       "track:\n  id: x\n",
		"```yaml\ntrack:\n  id: x\n```":           "track:\n  id: x\n",
		"```\ntrack:\n  id: x\n```\n":             "track:\n  id: x\n",
		"Sure! Here:\ntrack:\n  id: x\n":          "track:\n  id: x\n",
		"text\n```yml\nconcepts: []\n```\nmore\n": "concepts: []\n",
		"\n\ntrack:\n  id: x\n\n\n":               "track:\n  id: x\n",
	}
	for in, want := range cases {
		if got := ExtractYAML(in); got != want {
			t.Errorf("ExtractYAML(%q) = %q, want %q", in, got, want)
		}
	}
}
