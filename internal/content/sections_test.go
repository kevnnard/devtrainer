package content

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// withSections appends a sections block to a track file.
func withSections(track, block string) string {
	return track + "sections:\n" + block
}

func TestLoadSections(t *testing.T) {
	git := withSections(gitYAML, `  - id: git/basics
    title: Basics
    concepts: [git.commit, git.branch]
`)
	cat, err := Load(fstest.MapFS{
		"git/git.yaml": {Data: []byte(git)},
		"go/go.yaml":   {Data: []byte(goYAML)},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.Sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(cat.Sections))
	}
	s := cat.Sections[0]
	if s.ID != "git/basics" || s.Track != "git" || s.Title != "Basics" || !slices.Equal(s.Concepts, []string{"git.commit", "git.branch"}) {
		t.Errorf("section = %+v", s)
	}
	if got := cat.SectionsIn("git"); len(got) != 1 || got[0].ID != "git/basics" {
		t.Errorf("SectionsIn(git) = %+v", got)
	}
	if got := cat.SectionsIn("go"); len(got) != 0 {
		t.Errorf("a track without sections has none, got %+v", got)
	}
	if got, ok := cat.SectionOf("git.branch"); !ok || got.ID != "git/basics" {
		t.Errorf("SectionOf(git.branch) = %+v, %v", got, ok)
	}
	if _, ok := cat.SectionOf("go.vars"); ok {
		t.Error("go.vars is in no section")
	}
}

func TestLoadSectionErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		parts []string
	}{
		{"unknown concept",
			map[string]string{"git/git.yaml": withSections(gitYAML, "  - id: git/basics\n    title: Basics\n    concepts: [git.commit, git.nope]\n")},
			[]string{"git/git.yaml", "git/basics", "unknown concept", "git.nope"}},
		{"concept of another track",
			map[string]string{
				"git/git.yaml": withSections(gitYAML, "  - id: git/basics\n    title: Basics\n    concepts: [git.commit, go.vars]\n"),
				"go/go.yaml":   goYAML,
			},
			[]string{"git/git.yaml", "git/basics", "go.vars", "track go"}},
		{"concept in two sections",
			map[string]string{"git/git.yaml": withSections(gitYAML,
				"  - id: git/a\n    title: A\n    concepts: [git.commit]\n  - id: git/b\n    title: B\n    concepts: [git.commit, git.branch]\n")},
			[]string{"git/git.yaml", "git/b", "git.commit", "two sections", "git/a"}},
		{"duplicate section id",
			map[string]string{"git/git.yaml": withSections(gitYAML,
				"  - id: git/a\n    title: A\n    concepts: [git.commit]\n  - id: git/a\n    title: B\n    concepts: [git.branch]\n")},
			[]string{"git/git.yaml", "duplicate section ID", "git/a"}},
		{"duplicate section id across files",
			map[string]string{
				"git/git.yaml": withSections(gitYAML, "  - id: shared\n    title: A\n    concepts: [git.commit]\n"),
				"go/go.yaml":   withSections(goYAML, "  - id: shared\n    title: B\n    concepts: [go.vars]\n"),
			},
			[]string{"go/go.yaml", "duplicate section ID", "shared", "git/git.yaml"}},
		{"empty section",
			map[string]string{"git/git.yaml": withSections(gitYAML, "  - id: git/empty\n    title: Empty\n    concepts: []\n")},
			[]string{"git/git.yaml", "git/empty", "no concepts"}},
		{"section without id",
			map[string]string{"git/git.yaml": withSections(gitYAML, "  - title: Nameless\n    concepts: [git.commit]\n")},
			[]string{"git/git.yaml", "Nameless", "no id"}},
		{"section without title",
			map[string]string{"git/git.yaml": withSections(gitYAML, "  - id: git/untitled\n    concepts: [git.commit]\n")},
			[]string{"git/git.yaml", "git/untitled", "no title"}},
		{"unknown section field",
			map[string]string{"git/git.yaml": withSections(gitYAML, "  - id: git/a\n    title: A\n    concpets: [git.commit]\n")},
			[]string{"git/git.yaml", "concpets"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantAll(t, loadErr(t, c.files), c.parts...)
		})
	}
}

func TestLoadSectionsReportsEveryError(t *testing.T) {
	bad := withSections(gitYAML, "  - id: git/a\n    title: A\n    concepts: [git.nope]\n  - id: git/b\n    title: B\n    concepts: []\n")
	msg := loadErr(t, map[string]string{"git/git.yaml": bad})
	wantAll(t, msg, "git.nope", "git/b")
	if n := strings.Count(msg, "\n") + 1; n < 2 {
		t.Errorf("want both errors, got %q", msg)
	}
}
