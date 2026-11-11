package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
	"github.com/kevnnard/devtrainer/internal/srs"
	"github.com/kevnnard/devtrainer/internal/store"
	"github.com/kevnnard/devtrainer/internal/tutor"
)

var testContent = fstest.MapFS{"git/git.yaml": {Data: []byte(`track:
  id: git
  title: Git
  order: 50
concepts:
  - id: git.commit
    title: Commits
    level: beginner
    items:
      - id: git.commit.mcq-what
        type: mcq
        prompt: What does git commit record?
        options:
          - text: The staged changes
            correct: true
          - text: Every change in the working tree
            misconception: commit-takes-worktree
            explanation: Only staged changes are committed.
      - id: git.commit.cmd-basic
        type: command
        prompt: Commit with the message "init".
        answers: ['git commit -m "init"']
  - id: git.branch
    title: Branches
    level: beginner
    prereqs: [git.commit]
    items:
      - id: git.branch.cmd-create
        type: command
        prompt: Create and switch to a branch named feature.
        answers: [git switch -c feature]
`)}}

const draftYAML = `track:
  id: git
  title: Git
concepts:
  - id: git.stash
    title: Stash
    level: beginner
    prereqs: [git.commit]
    items:
      - id: git.stash.cmd-save
        type: command
        prompt: Put your uncommitted changes aside.
        answers: [git stash]
`

// fakeModel answers every request with out or err.
type fakeModel struct {
	out string
	err error
}

func (f fakeModel) Complete(context.Context, tutor.Request) (string, error) { return f.out, f.err }

// setup points the package at test content and a fake model, and moves
// into a temp working directory.
func setup(t *testing.T, model tutor.Model) string {
	t.Helper()
	oldFS, oldModel, oldApp := contentFS, modelFor, startApp
	t.Cleanup(func() { contentFS, modelFor, startApp = oldFS, oldModel, oldApp })
	contentFS = testContent
	modelFor = func(config, string) tutor.Model { return model }
	startApp = func(appOptions) error { return errors.New("the TUI must not start in tests") }
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	return dir
}

func runOut(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(args, &out)
	return out.String(), err
}

func TestHelpDocumentsEveryCommandAndFlag(t *testing.T) {
	setup(t, nil)
	out, err := runOut(t, "-h")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"devtrainer", "stats", "validate", "draft <track> <topic>", "--db", "--new", "--config", "drafts/"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
}

func TestNoCommandStartsTheApp(t *testing.T) {
	setup(t, nil)
	var got appOptions
	startApp = func(o appOptions) error { got = o; return nil }
	db := filepath.Join(t.TempDir(), "x.db")

	if _, err := runOut(t, "--db", db, "--new", "5"); err != nil {
		t.Fatal(err)
	}
	if got.dbPath != db || got.maxNew != 5 {
		t.Errorf("options = %+v", got)
	}
}

func TestDefaultFlags(t *testing.T) {
	dir := setup(t, nil)
	var got appOptions
	startApp = func(o appOptions) error { got = o; return nil }
	if _, err := runOut(t); err != nil {
		t.Fatal(err)
	}
	if got.dbPath != filepath.Join(dir, "data", "devtrainer", "devtrainer.db") || got.maxNew != srs.DefaultMaxNew ||
		got.configPath != filepath.Join(dir, "config", "devtrainer", "config.json") {
		t.Errorf("defaults = %+v", got)
	}
}

func TestBadUsage(t *testing.T) {
	setup(t, nil)
	for _, args := range [][]string{{"nope"}, {"--new", "-1"}, {"draft", "git"}, {"validate", "x"}, {"--bogus"}} {
		if _, err := runOut(t, args...); err == nil {
			t.Errorf("%q: want an error", args)
		}
	}
}

func TestValidatePrintsCounts(t *testing.T) {
	setup(t, nil)
	out, err := runOut(t, "validate")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 track", "2 concepts", "3 items", "git", "command 2", "mcq 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestValidateReportsCodeToolchains(t *testing.T) {
	setup(t, nil)
	contentFS = fstest.MapFS{"py/py.yaml": {Data: []byte(`track:
  id: py
  title: Python
concepts:
  - id: py.print
    title: Print
    level: beginner
    items:
      - id: py.print.code-hi
        type: code
        language: python
        prompt: Print hi.
        starter: print()
        solution: print("hi")
        expected: hi
      - id: py.print.code-sql
        type: code
        language: sql
        prompt: Select 1.
        starter: SELECT
        solution: SELECT 1;
        rows: [[1]]
`)}}
	old := newRunner
	t.Cleanup(func() { newRunner = old })
	newRunner = func() *runner.Runner {
		r := runner.New()
		r.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		return r
	}
	out, err := runOut(t, "validate")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"code 2", "sql", "built in", "python", "python3 not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestValidateReportsErrors(t *testing.T) {
	setup(t, nil)
	contentFS = fstest.MapFS{"git/git.yaml": {Data: []byte(strings.Replace(string(testContent["git/git.yaml"].Data),
		"prereqs: [git.commit]", "prereqs: [git.nope]", 1))}}
	_, err := runOut(t, "validate")
	if err == nil || !strings.Contains(err.Error(), "git.nope") {
		t.Errorf("err = %v", err)
	}
}

func TestStatsOnAnEmptyDatabase(t *testing.T) {
	setup(t, nil)
	// Flags work before and after the command.
	if _, err := runOut(t, "--db", filepath.Join(t.TempDir(), "t.db"), "stats"); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "t.db")
	out, err := runOut(t, "stats", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db); err != nil {
		t.Errorf("--db after the command was ignored: %v", err)
	}
	for _, want := range []string{"Streak", "0 days", "Answers", "Git", "0%", "Top misconceptions", "none yet"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestStatsWithHistory(t *testing.T) {
	setup(t, nil)
	db := filepath.Join(t.TempDir(), "t.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour)
	sc := srs.NewScheduler().Review(domain.Schedule{ConceptID: "git.commit"}, domain.Again, "git.commit.mcq-what", at)
	if err := st.RecordReview(context.Background(), domain.Answer{ConceptID: "git.commit", ItemID: "git.commit.mcq-what",
		Grade: domain.Again, Misconception: "commit-takes-worktree", At: at}, sc); err != nil {
		t.Fatal(err)
	}
	st.Close()

	out, err := runOut(t, "--db", db, "stats")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 day", "commit-takes-worktree", "Commits", "0/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestDraftWritesTheValidatedYAML(t *testing.T) {
	dir := setup(t, fakeModel{out: "```yaml\n" + draftYAML + "```"})

	out, err := runOut(t, "draft", "git", "Stash & pop")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "drafts", "git-stash-pop.yaml")
	if !strings.Contains(out, filepath.Join("drafts", "git-stash-pop.yaml")) {
		t.Errorf("output %q lacks the path", out)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != draftYAML {
		t.Errorf("file = %q, %v", data, err)
	}

	// A second draft on the same topic never overwrites the first.
	out, err = runOut(t, "draft", "git", "Stash & pop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "git-stash-pop-2.yaml") {
		t.Errorf("second draft: %q", out)
	}
	if data2, _ := os.ReadFile(path); string(data2) != draftYAML {
		t.Error("the first draft changed")
	}
}

func TestDraftInvalidWritesNothing(t *testing.T) {
	dir := setup(t, fakeModel{out: strings.Replace(draftYAML, "[git.commit]", "[git.nope]", 1)})

	out, err := runOut(t, "draft", "git", "stash")

	if err == nil || !strings.Contains(err.Error(), "git.nope") {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "drafts")); !os.IsNotExist(statErr) {
		t.Errorf("drafts/ was created: %v; output %q", statErr, out)
	}
}

func TestDraftWithoutPi(t *testing.T) {
	setup(t, fakeModel{err: errors.New("pi not found (pi): install Gentle Shell or set its path in the config")})
	_, err := runOut(t, "draft", "git", "stash")
	if err == nil || !strings.Contains(err.Error(), "Gentle Shell") {
		t.Errorf("err = %v", err)
	}
}

func TestDraftRefusesTheContentDirectory(t *testing.T) {
	dir := setup(t, fakeModel{out: draftYAML})
	contentDir := filepath.Join(dir, "content")
	if err := os.Mkdir(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(contentDir)

	if _, err := runOut(t, "draft", "git", "stash"); err == nil {
		t.Error("drafting inside content/ must fail")
	}
	if _, err := os.Stat(filepath.Join(contentDir, "drafts")); !os.IsNotExist(err) {
		t.Error("nothing may be written under content/")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Stash & pop":          "stash-pop",
		"  Multi-stage builds": "multi-stage-builds",
		"S3 / IAM!":            "s3-iam",
		"¿Qué?":                "qu",
		"!!!":                  "topic",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
