package content_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kevnnard/devtrainer/content"
	"github.com/kevnnard/devtrainer/internal/check"
	loader "github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/runner"
)

// TestEmbeddedContentIsValid keeps the shipped YAML loadable: any content
// error fails the build's tests, not the learner's session.
func TestEmbeddedContentIsValid(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatalf("embedded content does not load:\n%v", err)
	}
	if len(cat.Tracks) == 0 || len(cat.Concepts) == 0 || len(cat.Items) == 0 {
		t.Fatalf("embedded content is empty: %d tracks, %d concepts, %d items",
			len(cat.Tracks), len(cat.Concepts), len(cat.Items))
	}
	if _, ok := cat.Track("git"); !ok {
		t.Error("the git track is missing")
	}
}

// TestEveryTrackStartsWithALesson keeps a lesson on the first concept of
// each track (by order, with no prerequisite inside its own track): the
// concept a newcomer meets first.
func TestEveryTrackStartsWithALesson(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range cat.Tracks {
		cs := cat.ConceptsIn(tr.ID)
		inTrack := map[string]bool{}
		for _, c := range cs {
			inTrack[c.ID] = true
		}
		found := false
		for _, c := range cs {
			local := false
			for _, p := range c.Prereqs {
				local = local || inTrack[p]
			}
			if local {
				continue
			}
			found = true
			if c.Lesson == nil {
				t.Errorf("track %s: first concept %s has no lesson", tr.ID, c.ID)
			}
			break
		}
		if !found {
			t.Errorf("track %s has no starting concept", tr.ID)
		}
	}
}

// TestCodeSolutionsPass runs every shipped code item for real: the
// solution must pass and the starter must not. Languages whose toolchain
// is not installed here are skipped, not failed.
func TestCodeSolutionsPass(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	r := runner.New()
	for _, it := range cat.Items {
		if it.Type != domain.Code {
			continue
		}
		t.Run(it.ID, func(t *testing.T) {
			if _, err := r.Toolchain(it.Language); err != nil {
				t.Skipf("toolchain missing: %v", err)
			}
			run := func(src string) check.CodeRun {
				out, err := r.Run(context.Background(), runner.Program{Language: it.Language, Code: src, Setup: it.Setup})
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				return out
			}
			if got := run(it.Solution); !check.Code(it, got).Correct {
				t.Errorf("solution fails: %+v\nwant rows %v / stdout %q", got, it.Rows, it.Expected)
			}
			if check.Code(it, run(it.Starter)).Correct {
				t.Error("the starter already passes")
			}
		})
	}
}

// TestSQLConceptsHaveCodeItems keeps at least one code item per SQL concept.
func TestSQLConceptsHaveCodeItems(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, it := range cat.Items {
		if it.Type == domain.Code && it.Language == domain.SQL {
			has[it.ConceptID] = true
		}
	}
	for _, c := range cat.ConceptsIn("sql") {
		if !has[c.ID] {
			t.Errorf("%s has no sql code item", c.ID)
		}
	}
}

// TestErrorStepsMatchTheExample runs the example of every lesson with an
// error-on-purpose step: it must really fail with the error the step shows.
func TestErrorStepsMatchTheExample(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	r := runner.New()
	for _, c := range cat.Concepts {
		if c.Lesson == nil {
			continue
		}
		for i, st := range c.Lesson.Steps {
			if st.Error == "" {
				continue
			}
			ex := c.Lesson.Example
			if !domain.ValidLanguage(ex.Lang) {
				t.Errorf("%s step %d: example lang %q cannot run", c.ID, i+1, ex.Lang)
				continue
			}
			if _, err := r.Toolchain(ex.Lang); err != nil {
				t.Logf("%s: skipped, toolchain missing: %v", c.ID, err)
				continue
			}
			out, err := r.Run(context.Background(), runner.Program{Language: ex.Lang, Code: ex.Code})
			if err != nil {
				t.Fatalf("%s: %v", c.ID, err)
			}
			if !out.Failed() || !strings.Contains(out.Error, strings.TrimSpace(st.Error)) {
				t.Errorf("%s step %d: example error = %q, step shows %q", c.ID, i+1, out.Error, st.Error)
			}
		}
	}
}

// TestTracksHaveSectionExams keeps section exams in the shipped content:
// at least three tracks group their concepts into sections, and every
// section's exam has closed items to ask even without any toolchain.
func TestTracksHaveSectionExams(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	tracks := map[string]bool{}
	for _, s := range cat.Sections {
		tracks[s.Track] = true
		closed := 0
		for _, id := range s.Concepts {
			for _, it := range cat.ItemsFor(id) {
				if it.Type != domain.Code {
					closed++
				}
			}
		}
		if closed == 0 {
			t.Errorf("section %s has no closed item for its exam", s.ID)
		}
	}
	for _, want := range []string{"go", "sql", "javascript"} {
		if !tracks[want] {
			t.Errorf("track %s has no sections", want)
		}
	}
	if len(tracks) < 3 {
		t.Errorf("%d tracks have sections, want at least 3", len(tracks))
	}
}
