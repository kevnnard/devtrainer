package content

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kevnnard/devtrainer/internal/domain"
)

const gitYAML = `
track:
  id: git
  title: Git
  description: Version control.
  order: 2
concepts:
  - id: git.commit
    title: Commits
    summary: A commit is a snapshot.
    level: beginner
    items:
      - id: git.commit.mcq
        type: mcq
        prompt: What does git commit record?
        options:
          - text: The staged changes
            correct: true
          - text: Every change in the working tree
            misconception: commit-takes-worktree
            explanation: Only staged changes are committed.
        explanation: git commit snapshots the index.
        hint: Think about the staging area.
      - id: git.commit.cmd
        type: command
        prompt: Commit with the message "init".
        answers: ['git commit -m "init"']
  - id: git.branch
    title: Branches
    summary: A branch is a movable pointer.
    level: intermediate
    prereqs: [git.commit]
    items:
      - id: git.branch.parsons
        type: parsons
        prompt: Create and switch to a branch, then commit.
        lines:
          - git switch -c feature
          - git add .
          - git commit -m "wip"
      - id: git.branch.bug
        type: bug
        prompt: Which line is wrong?
        code: |
          git switch -c feature
          git comit -m "x"
        bug_line: 2
      - id: git.branch.cloze
        type: cloze
        prompt: git ___ -c feature
        answers: [switch, checkout -b]
      - id: git.branch.out
        type: output
        prompt: What does it print?
        code: git branch --show-current
        answers: [feature]
      - id: git.branch.multi
        type: multi
        prompt: Which create a branch?
        options:
          - {text: git branch x, correct: true}
          - {text: git switch -c x, correct: true}
          - {text: git tag x, misconception: tag-is-branch, explanation: A tag does not move.}
`

const goYAML = `
track:
  id: go
  title: Go
  order: 1
concepts:
  - id: go.vars
    title: Variables
    level: beginner
    items:
      - id: go.vars.1
        type: mcq
        prompt: Which declares and assigns?
        options:
          - {text: "x := 1", correct: true}
          - {text: "x = 1", misconception: assign-declares, explanation: = only assigns an existing variable.}
`

func TestLoadValid(t *testing.T) {
	fsys := fstest.MapFS{
		"git/git.yaml": {Data: []byte(gitYAML)},
		"go/go.yaml":   {Data: []byte(goYAML)},
		"README.md":    {Data: []byte("ignored")},
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.Tracks) != 2 || cat.Tracks[0].ID != "go" || cat.Tracks[1].ID != "git" {
		t.Fatalf("tracks must be sorted by order: %+v", cat.Tracks)
	}
	if cat.Tracks[1].Description != "Version control." {
		t.Errorf("track description = %q", cat.Tracks[1].Description)
	}
	if len(cat.Concepts) != 3 || len(cat.Items) != 8 {
		t.Fatalf("got %d concepts, %d items", len(cat.Concepts), len(cat.Items))
	}
	br, ok := cat.Concept("git.branch")
	if !ok || br.Track != "git" || br.Level != domain.Intermediate || len(br.Prereqs) != 1 || br.Prereqs[0] != "git.commit" {
		t.Errorf("git.branch = %+v", br)
	}
	items := cat.ItemsFor("git.commit")
	if len(items) != 2 {
		t.Fatalf("ItemsFor(git.commit) = %d items", len(items))
	}
	mcq := items[0]
	if mcq.ConceptID != "git.commit" || mcq.Type != domain.MCQ || mcq.Hint == "" || len(mcq.Options) != 2 {
		t.Errorf("mcq = %+v", mcq)
	}
	if !mcq.Options[0].Correct || mcq.Options[1].Misconception != "commit-takes-worktree" {
		t.Errorf("mcq options = %+v", mcq.Options)
	}
	bug := cat.ItemsFor("git.branch")[1]
	if bug.BugLine != 2 || !strings.HasPrefix(bug.Code, "git switch") {
		t.Errorf("bug = %+v", bug)
	}
}

// load runs Load on a single track file and returns the error text.
func loadErr(t *testing.T, files map[string]string) string {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, data := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(data)}
	}
	_, err := Load(fsys)
	if err == nil {
		t.Fatal("Load succeeded, want an error")
	}
	return err.Error()
}

func wantAll(t *testing.T, msg string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(msg, p) {
			t.Errorf("error %q does not mention %q", msg, p)
		}
	}
}

func TestLoadValidationErrors(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(string) string
		parts []string
	}{
		{"unknown prerequisite",
			func(s string) string { return strings.Replace(s, "prereqs: [git.commit]", "prereqs: [git.nope]", 1) },
			[]string{"git/git.yaml", "git.branch", "unknown prerequisite", "git.nope"}},
		{"mcq with two correct",
			func(s string) string {
				return strings.Replace(s, "misconception: commit-takes-worktree", "correct: true\n            misconception: commit-takes-worktree", 1)
			},
			[]string{"git/git.yaml", "git.commit.mcq", "exactly one correct", "2"}},
		{"mcq with no correct",
			func(s string) string {
				return strings.Replace(s, "          - text: The staged changes\n            correct: true", "          - text: The staged changes\n            misconception: m\n            explanation: e", 1)
			},
			[]string{"git.commit.mcq", "exactly one correct", "0"}},
		{"parsons without lines",
			func(s string) string {
				return strings.Replace(s, "        lines:\n          - git switch -c feature\n          - git add .\n          - git commit -m \"wip\"\n", "", 1)
			},
			[]string{"git/git.yaml", "git.branch.parsons", "lines"}},
		{"duplicate item id",
			func(s string) string { return strings.Replace(s, "id: git.commit.cmd", "id: git.commit.mcq", 1) },
			[]string{"git/git.yaml", "duplicate item ID", "git.commit.mcq"}},
		{"invalid type",
			func(s string) string { return strings.Replace(s, "type: command", "type: essay", 1) },
			[]string{"git.commit.cmd", "unknown type", "essay"}},
		{"bug line out of range",
			func(s string) string { return strings.Replace(s, "bug_line: 2", "bug_line: 9", 1) },
			[]string{"git.branch.bug", "bug_line 9"}},
		{"wrong option without misconception",
			func(s string) string {
				return strings.Replace(s, "            misconception: commit-takes-worktree\n", "", 1)
			},
			[]string{"git.commit.mcq", "option 2", "misconception"}},
		{"cloze answer with the wrong number of blanks",
			func(s string) string {
				return strings.Replace(s, "answers: [switch, checkout -b]", "answers: [switch, a | b]", 1)
			},
			[]string{"git.branch.cloze", "2 blanks", "1"}},
		{"unknown field",
			func(s string) string { return strings.Replace(s, "hint: Think", "hnit: Think", 1) },
			[]string{"git/git.yaml", "hnit"}},
		{"bad level",
			func(s string) string { return strings.Replace(s, "level: intermediate", "level: guru", 1) },
			[]string{"git.branch", "level", "guru"}},
		{"self prerequisite cycle",
			func(s string) string {
				return strings.Replace(s, "    level: beginner\n    items:", "    level: beginner\n    prereqs: [git.branch]\n    items:", 1)
			},
			[]string{"prerequisite cycle", "git.commit", "git.branch"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := loadErr(t, map[string]string{"git/git.yaml": c.edit(gitYAML)})
			wantAll(t, msg, c.parts...)
		})
	}
}

func TestLoadCrossFileErrors(t *testing.T) {
	dupConcept := strings.Replace(goYAML, "id: go.vars\n", "id: git.commit\n", 1)
	dupConcept = strings.Replace(dupConcept, "id: go.vars.1", "id: go.other.1", 1)
	msg := loadErr(t, map[string]string{"git/git.yaml": gitYAML, "go/go.yaml": dupConcept})
	wantAll(t, msg, "go/go.yaml", "duplicate concept ID", "git.commit", "git/git.yaml")

	msg = loadErr(t, map[string]string{"golang/go.yaml": goYAML})
	wantAll(t, msg, "golang/go.yaml", "track ID", "go", "directory")

	msg = loadErr(t, map[string]string{"git/a.yaml": gitYAML, "git/b.yaml": strings.ReplaceAll(gitYAML, "git.", "git2.")})
	wantAll(t, msg, "git/b.yaml", "duplicate track ID", "git")

	empty := strings.Replace(goYAML, "    items:\n", "    items: []\n    unused:\n", 1)
	msg = loadErr(t, map[string]string{"go/go.yaml": empty})
	wantAll(t, msg, "go/go.yaml")
}

func TestLoadReportsEveryError(t *testing.T) {
	bad := strings.Replace(gitYAML, "prereqs: [git.commit]", "prereqs: [git.nope]", 1)
	bad = strings.Replace(bad, "bug_line: 2", "bug_line: 0", 1)
	msg := loadErr(t, map[string]string{"git/git.yaml": bad})
	wantAll(t, msg, "git.nope", "git.branch.bug")
}

func TestLoadNoFiles(t *testing.T) {
	_, err := Load(fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "no track files") {
		t.Errorf("Load(empty) = %v, want a no track files error", err)
	}
}

func TestConceptWithoutItems(t *testing.T) {
	noItems := strings.Replace(goYAML, `    items:
      - id: go.vars.1
        type: mcq
        prompt: Which declares and assigns?
        options:
          - {text: "x := 1", correct: true}
          - {text: "x = 1", misconception: assign-declares, explanation: = only assigns an existing variable.}
`, "", 1)
	msg := loadErr(t, map[string]string{"go/go.yaml": noItems})
	wantAll(t, msg, "go/go.yaml", "go.vars", "no items")
}
