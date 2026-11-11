package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	contentload "github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
	"github.com/kevnnard/devtrainer/internal/store"
)

// runValidate loads the embedded content and prints what it holds.
func runValidate(stdout io.Writer) error {
	cat, err := contentload.Load(contentFS)
	if err != nil {
		return fmt.Errorf("content is invalid:\n%w", err)
	}
	fmt.Fprintf(stdout, "Content OK: %d %s, %d %s, %d %s\n\n",
		len(cat.Tracks), plural(len(cat.Tracks), "track", "tracks"),
		len(cat.Concepts), plural(len(cat.Concepts), "concept", "concepts"),
		len(cat.Items), plural(len(cat.Items), "item", "items"))

	byConcept := cat.ItemsByConcept()
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Track\tTitle\tConcepts\tItems")
	for _, t := range cat.Tracks {
		concepts := cat.ConceptsIn(t.ID)
		items := 0
		for _, c := range concepts {
			items += len(byConcept[c.ID])
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", t.ID, t.Title, len(concepts), items)
	}
	tw.Flush()

	types := map[domain.ItemType]int{}
	for _, it := range cat.Items {
		types[it.Type]++
	}
	var parts []string
	for _, t := range domain.ItemTypes {
		if types[t] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", t, types[t]))
		}
	}
	fmt.Fprintf(stdout, "\nItem types: %s\n", strings.Join(parts, " · "))
	printToolchains(stdout, cat)
	return nil
}

// printToolchains lists, per language with code items, how its code runs
// here or which tool is missing (those items are not served).
func printToolchains(stdout io.Writer, cat domain.Catalog) {
	count := map[string]int{}
	for _, it := range cat.Items {
		if it.Type == domain.Code {
			count[it.Language]++
		}
	}
	if len(count) == 0 {
		return
	}
	r := newRunner()
	fmt.Fprintln(stdout, "\nCode exercises:")
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, l := range domain.Languages {
		if count[l] == 0 {
			continue
		}
		tc, err := r.Toolchain(l)
		status := tc.Tool
		switch {
		case err != nil:
			status = "off: " + strings.TrimPrefix(err.Error(), l+": ")
		case tc.Path != "":
			status += " (" + tc.Path + ")"
		}
		if tc.Note != "" {
			status += "; " + tc.Note
		}
		fmt.Fprintf(tw, "  %s\t%d %s\t%s\n", l, count[l], plural(count[l], "item", "items"), status)
	}
	tw.Flush()
}

// runStats prints the learner's progress as plain text.
func runStats(opts appOptions, stdout io.Writer) error {
	cat, err := contentload.Load(contentFS)
	if err != nil {
		return err
	}
	st, err := store.Open(opts.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	now := time.Now()
	stats, err := st.Stats(context.Background(), cat, now)
	if err != nil {
		return err
	}
	scheds, err := st.Schedules(context.Background())
	if err != nil {
		return err
	}
	due, unlocked := 0, 0
	for _, c := range cat.Concepts {
		sc, ok := scheds[c.ID]
		switch {
		case ok && sc.Reviewed() && !sc.Due.After(now):
			due++
		case !sc.Reviewed() && srs.Unlocked(c, stats.States):
			unlocked++
		}
	}

	fmt.Fprintf(stdout, "devtrainer progress — %s\n\n", now.Format("Monday 2 January 2006"))
	today := "not studied today"
	if stats.StudiedToday {
		today = "studied today"
	}
	acc := 0
	if stats.TotalAnswers > 0 {
		acc = 100 * stats.TotalCorrect / stats.TotalAnswers
	}
	fmt.Fprintf(stdout, "Streak     %d %s (%s)\n", stats.Streak, plural(stats.Streak, "day", "days"), today)
	fmt.Fprintf(stdout, "Answers    %d · %d%% correct\n", stats.TotalAnswers, acc)
	fmt.Fprintf(stdout, "Due now    %d · new concepts unlocked: %d (up to %d per day)\n\n", due, unlocked, opts.maxNew)

	nameW := len("Track")
	for _, p := range stats.Tracks {
		nameW = max(nameW, len(p.Track.Title))
	}
	fmt.Fprintf(stdout, "%-*s  %7s  %4s  %8s  %6s  %8s\n", nameW, "Track", "Mastery", "New", "Learning", "Review", "Mastered")
	for _, p := range stats.Tracks {
		fmt.Fprintf(stdout, "%-*s  %6.0f%%  %4d  %8d  %6d  %8d\n", nameW, p.Track.Title, p.MasteryPercent(), p.New, p.Learning, p.Review, p.Mastered)
	}

	fmt.Fprintln(stdout, "\nTop misconceptions")
	if len(stats.TopMisconceptions) == 0 {
		fmt.Fprintln(stdout, "  none yet")
	}
	for _, m := range stats.TopMisconceptions {
		fmt.Fprintf(stdout, "  %-32s %d×\n", m.Misconception, m.Count)
	}
	fmt.Fprintln(stdout, "\nWeakest concepts")
	if len(stats.Weakest) == 0 {
		fmt.Fprintln(stdout, "  none yet")
	}
	for _, w := range stats.Weakest {
		name := w.ConceptID
		if c, ok := cat.Concept(w.ConceptID); ok {
			name = fmt.Sprintf("%s (%s)", c.Title, c.ID)
		}
		fmt.Fprintf(stdout, "  %-40s %3.0f%%  %d/%d\n", name, 100*w.Accuracy(), w.Correct, w.Answers)
	}
	return nil
}

// draftsDir is where drafts go, relative to the working directory.
const draftsDir = "drafts"

// runDraft asks the AI for candidate items and writes them, validated, to
// drafts/<track>-<slug>.yaml. Invalid drafts are reported and not written.
func runDraft(opts appOptions, track, topic string, stdout io.Writer) error {
	if slug(track) != track {
		return fmt.Errorf("track %q must be a lowercase ID like git or aws", track)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if slices.Contains(strings.Split(filepath.ToSlash(cwd), "/"), "content") {
		return errors.New("drafts are never written inside content/: run draft from the repository root")
	}
	conf, err := loadConfig(opts.configPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Drafting %q for %s with %s…\n", topic, track, conf.Models["draft"])
	yaml, err := newTutor(conf).Draft(context.Background(), track, topic, contentFS)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(draftsDir, 0o755); err != nil {
		return err
	}
	path, err := writeNew(filepath.Join(draftsDir, track+"-"+slug(topic)), ".yaml", []byte(yaml))
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Wrote %s — review it, then move the concept into content/%s/ by hand.\n", path, track)
	return nil
}

// writeNew writes data to base+ext, or base-2+ext, base-3+ext… never
// overwriting an existing file. It returns the path written.
func writeNew(base, ext string, data []byte) (string, error) {
	for n := 1; n < 1000; n++ {
		path := base + ext
		if n > 1 {
			path = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", err
		}
		return path, f.Close()
	}
	return "", fmt.Errorf("too many drafts named %s*%s", base, ext)
}

// slug turns a topic into a file-name part: lowercase ASCII letters and
// digits joined by single dashes.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	if b.Len() == 0 {
		return "topic"
	}
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
