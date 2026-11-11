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

// newTracks are the language tracks whose concepts each need a code item
// written in the track's own language.
var newTracks = []string{"javascript", "java", "csharp", "rust"}

// TestNewTracksHaveCodeItems keeps at least one code item per concept of
// the javascript, java, csharp and rust tracks, in the track's language.
func TestNewTracksHaveCodeItems(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, track := range newTracks {
		cs := cat.ConceptsIn(track)
		if len(cs) == 0 {
			t.Errorf("track %s has no concepts", track)
		}
		has := map[string]bool{}
		for _, it := range cat.Items {
			if it.Type == domain.Code && it.Language == track {
				has[it.ConceptID] = true
			}
		}
		for _, c := range cs {
			if !has[c.ID] {
				t.Errorf("%s has no %s code item", c.ID, track)
			}
		}
	}
}

// firstLine is the first line of s naming an error, else its first
// non-blank line, for short test logs.
func firstLine(s string) string {
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		if strings.Contains(l, "rror") || strings.Contains(l, "xception") || strings.Contains(l, "panicked") {
			return strings.TrimSpace(l)
		}
	}
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// TestNewTracksCodeItemsRun runs the code items of the new tracks for real:
// each solution must pass and each starter must fail. A language whose
// toolchain is not installed is skipped and says which tool is missing;
// it never counts as a pass. Every run happens under a temp directory the
// test owns.
func TestNewTracksCodeItemsRun(t *testing.T) {
	cat, err := loader.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // tool caches (dotnet run app.cs) stay in it too
	r := runner.New()
	r.TempDir = tmp

	byTrack := map[string][]domain.Item{}
	for _, it := range cat.Items {
		if it.Type != domain.Code {
			continue
		}
		c, ok := cat.Concept(it.ConceptID)
		if ok {
			byTrack[c.Track] = append(byTrack[c.Track], it)
		}
	}
	for _, track := range newTracks {
		t.Run(track, func(t *testing.T) {
			items := byTrack[track]
			if len(items) == 0 {
				t.Fatalf("track %s has no code items", track)
			}
			tc, err := r.Toolchain(track)
			if err != nil {
				t.Skipf("toolchain missing, %d items not run: %v", len(items), err)
			}
			t.Logf("%s via %s (%s) %s", track, tc.Tool, tc.Path, tc.Note)
			for _, it := range items {
				t.Run(it.ID, func(t *testing.T) {
					if it.Language != track {
						t.Fatalf("language %q in track %s", it.Language, track)
					}
					run := func(src string) check.CodeRun {
						t.Helper()
						out, err := r.Run(context.Background(), runner.Program{Language: it.Language, Code: src})
						if err != nil {
							t.Fatalf("run: %v", err)
						}
						return out
					}
					if got := run(it.Solution); !check.Code(it, got).Correct {
						t.Errorf("solution fails: %+v\nwant stdout %q", got, it.Expected)
					}
					if got := run(it.Starter); check.Code(it, got).Correct {
						t.Error("the starter already passes")
					} else if got.TimedOut || got.Truncated {
						t.Errorf("the starter fails by timeout or output cap, not by its mistake: %+v", got)
					} else {
						t.Logf("starter fails as intended: stdout %q, error %q", firstLine(got.Stdout), firstLine(got.Error))
					}
				})
			}
		})
	}
}
