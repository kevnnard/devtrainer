// Package content loads and validates the YAML study content (see
// content/SCHEMA.md) from any fs.FS, typically the embedded one. Its only
// I/O is reading from that fs.FS.
package content

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
)

// Pattern is where track files live inside the file system.
const Pattern = "*/*.yaml"

type fileDoc struct {
	Track    trackDoc     `yaml:"track"`
	Concepts []conceptDoc `yaml:"concepts"`
	Sections []sectionDoc `yaml:"sections"`
}

type sectionDoc struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Concepts []string `yaml:"concepts"`
}

type trackDoc struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Order       int    `yaml:"order"`
}

type conceptDoc struct {
	ID      string     `yaml:"id"`
	Title   string     `yaml:"title"`
	Summary string     `yaml:"summary"`
	Level   string     `yaml:"level"`
	Prereqs []string   `yaml:"prereqs"`
	Lesson  *lessonDoc `yaml:"lesson"`
	Items   []itemDoc  `yaml:"items"`
}

type lessonDoc struct {
	Intro    string      `yaml:"intro"`
	Example  *exampleDoc `yaml:"example"`
	Steps    []stepDoc   `yaml:"steps"`
	Takeaway string      `yaml:"takeaway"`
}

type exampleDoc struct {
	Code string `yaml:"code"`
	Lang string `yaml:"lang"`
}

type stepDoc struct {
	Text    string      `yaml:"text"`
	Error   string      `yaml:"error"`
	Predict *predictDoc `yaml:"predict"`
}

type predictDoc struct {
	Prompt  string             `yaml:"prompt"`
	Options []predictOptionDoc `yaml:"options"`
}

type predictOptionDoc struct {
	Text        string `yaml:"text"`
	Correct     bool   `yaml:"correct"`
	Explanation string `yaml:"explanation"`
}

type itemDoc struct {
	ID            string      `yaml:"id"`
	Type          string      `yaml:"type"`
	Prompt        string      `yaml:"prompt"`
	Code          string      `yaml:"code"`
	Options       []optionDoc `yaml:"options"`
	Answers       []string    `yaml:"answers"`
	Lines         []string    `yaml:"lines"`
	BugLine       int         `yaml:"bug_line"`
	CaseSensitive bool        `yaml:"case_sensitive"`
	Explanation   string      `yaml:"explanation"`
	Hint          string      `yaml:"hint"`

	Language string   `yaml:"language"`
	Starter  string   `yaml:"starter"`
	Solution string   `yaml:"solution"`
	Expected string   `yaml:"expected"`
	Setup    string   `yaml:"setup"`
	Rows     []rowDoc `yaml:"rows"`
	Ordered  bool     `yaml:"ordered"`
}

// rowDoc is one expected sql row: each scalar's text as written, with
// any YAML null (null, NULL, ~) read as SQL NULL. It decodes the sequence
// itself because yaml.v3 drops null elements of a []string.
type rowDoc []string

// NullCell is how an expected row writes SQL NULL.
const NullCell = "NULL"

func (r *rowDoc) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: a row must be a list of values", n.Line)
	}
	out := make(rowDoc, 0, len(n.Content))
	for _, c := range n.Content {
		switch {
		case c.Kind != yaml.ScalarNode:
			return fmt.Errorf("line %d: a row value must be a scalar", c.Line)
		case c.Tag == "!!null":
			out = append(out, NullCell)
		default:
			out = append(out, c.Value)
		}
	}
	*r = out
	return nil
}

// rows converts the YAML rows to text.
func rows(d []rowDoc) [][]string {
	if d == nil {
		return nil
	}
	out := make([][]string, len(d))
	for i, r := range d {
		out[i] = []string(r)
	}
	return out
}

type optionDoc struct {
	Text          string `yaml:"text"`
	Correct       bool   `yaml:"correct"`
	Misconception string `yaml:"misconception"`
	Explanation   string `yaml:"explanation"`
}

// parsed is one decoded track file.
type parsed struct {
	path string
	doc  fileDoc
}

// Load reads every track file matching Pattern, validates it and returns the
// catalog with tracks sorted by order then ID. All problems are reported
// together; each names the file and the track, concept or item ID.
func Load(fsys fs.FS) (domain.Catalog, error) {
	paths, err := fs.Glob(fsys, Pattern)
	if err != nil {
		return domain.Catalog{}, err
	}
	if len(paths) == 0 {
		return domain.Catalog{}, fmt.Errorf("content: no track files matching %s", Pattern)
	}
	slices.Sort(paths)

	var errs []error
	var files []parsed
	for _, p := range paths {
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		var doc fileDoc
		if err := dec.Decode(&doc); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		files = append(files, parsed{path: p, doc: doc})
	}

	v := validator{
		trackFile:   map[string]string{},
		conceptFile: map[string]string{},
		itemFile:    map[string]string{},
		sectionFile: map[string]string{},
	}
	for _, f := range files {
		v.file(f)
	}
	v.prereqs()
	for _, f := range files {
		v.fileSections(f)
	}
	errs = append(errs, v.errs...)
	if len(errs) > 0 {
		return domain.Catalog{}, errors.Join(errs...)
	}
	return v.catalog(), nil
}

type validator struct {
	errs        []error
	trackFile   map[string]string // track ID -> file
	conceptFile map[string]string // concept ID -> file
	itemFile    map[string]string // item ID -> file
	sectionFile map[string]string // section ID -> file
	tracks      []domain.Track
	concepts    []domain.Concept
	items       []domain.Item
	sections    []domain.Section
}

func (v *validator) errorf(format string, args ...any) {
	v.errs = append(v.errs, fmt.Errorf(format, args...))
}

func (v *validator) file(f parsed) {
	p, t := f.path, f.doc.Track
	dir := path.Dir(p)
	switch {
	case t.ID == "":
		v.errorf("%s: track has no id", p)
		return
	case t.ID != dir:
		v.errorf("%s: track ID %q does not match its directory %q", p, t.ID, dir)
	}
	if first, dup := v.trackFile[t.ID]; dup {
		v.errorf("%s: duplicate track ID %s (first in %s)", p, t.ID, first)
		return
	}
	v.trackFile[t.ID] = p
	if strings.TrimSpace(t.Title) == "" {
		v.errorf("%s: track %s has no title", p, t.ID)
	}
	v.tracks = append(v.tracks, domain.Track{ID: t.ID, Title: t.Title, Description: t.Description, Order: t.Order})
	if len(f.doc.Concepts) == 0 {
		v.errorf("%s: track %s has no concepts", p, t.ID)
	}

	for _, c := range f.doc.Concepts {
		v.concept(p, t.ID, c)
	}
}

func (v *validator) concept(p, track string, c conceptDoc) {
	if c.ID == "" {
		v.errorf("%s: concept with title %q has no id", p, c.Title)
		return
	}
	if first, dup := v.conceptFile[c.ID]; dup {
		v.errorf("%s: duplicate concept ID %s (first in %s)", p, c.ID, first)
		return
	}
	v.conceptFile[c.ID] = p
	if strings.TrimSpace(c.Title) == "" {
		v.errorf("%s: concept %s has no title", p, c.ID)
	}
	level := domain.Level(c.Level)
	if !level.Valid() {
		v.errorf("%s: concept %s: level %q must be beginner, intermediate or advanced", p, c.ID, c.Level)
	}
	if len(c.Items) == 0 {
		v.errorf("%s: concept %s has no items", p, c.ID)
	}
	v.concepts = append(v.concepts, domain.Concept{
		ID: c.ID, Track: track, Title: c.Title, Summary: c.Summary,
		Prereqs: c.Prereqs, Level: level, Lesson: v.lesson(p, c.ID, c.Lesson),
	})
	for _, it := range c.Items {
		v.item(p, c.ID, it)
	}
}

// lesson converts a concept's optional lesson and reports its problems.
func (v *validator) lesson(p, concept string, d *lessonDoc) *domain.Lesson {
	if d == nil {
		return nil
	}
	l := &domain.Lesson{Intro: d.Intro, Takeaway: d.Takeaway}
	if d.Example != nil {
		l.Example = domain.Example{Code: d.Example.Code, Lang: d.Example.Lang}
	}
	for _, s := range d.Steps {
		step := domain.LessonStep{Text: s.Text, Error: s.Error}
		if s.Predict != nil {
			pr := &domain.Predict{Prompt: s.Predict.Prompt}
			for _, o := range s.Predict.Options {
				pr.Options = append(pr.Options, domain.PredictOption(o))
			}
			step.Predict = pr
		}
		l.Steps = append(l.Steps, step)
	}
	for _, msg := range lessonProblems(*l, d.Example != nil) {
		v.errorf("%s: concept %s: lesson: %s", p, concept, msg)
	}
	return l
}

// lessonProblems lists everything wrong with one lesson; hasExample says
// whether the YAML had an example block.
func lessonProblems(l domain.Lesson, hasExample bool) []string {
	var probs []string
	add := func(format string, args ...any) { probs = append(probs, fmt.Sprintf(format, args...)) }

	if strings.TrimSpace(l.Intro) == "" {
		add("no intro")
	}
	if strings.TrimSpace(l.Takeaway) == "" {
		add("no takeaway")
	}
	code := strings.TrimRight(l.Example.Code, "\n")
	switch {
	case hasExample && strings.TrimSpace(code) == "":
		add("example has no code")
	case strings.Count(code, "\n")+1 > domain.MaxLessonCodeLines && code != "":
		add("example code has %d lines, at most %d", strings.Count(code, "\n")+1, domain.MaxLessonCodeLines)
	}
	if len(l.Steps) == 0 {
		add("no steps")
	}
	for i, s := range l.Steps {
		n := i + 1
		if strings.TrimSpace(s.Text) == "" {
			add("step %d has no text", n)
		}
		if strings.TrimSpace(s.Error) != "" {
			if s.Predict == nil {
				add("step %d shows an error but has no predict asking why it fails", n)
			}
			if !hasExample || strings.TrimSpace(code) == "" {
				add("step %d shows an error but the lesson has no example that produces it", n)
			}
		}
		p := s.Predict
		if p == nil {
			continue
		}
		if strings.TrimSpace(p.Prompt) == "" {
			add("step %d: predict has no prompt", n)
		}
		switch {
		case len(p.Options) < 2:
			add("step %d: predict needs at least 2 options, has %d", n, len(p.Options))
		case len(p.Options) > domain.MaxPredictOptions:
			add("step %d: predict has %d options, at most %d", n, len(p.Options), domain.MaxPredictOptions)
		}
		correct := 0
		for j, o := range p.Options {
			if o.Correct {
				correct++
			}
			if strings.TrimSpace(o.Text) == "" {
				add("step %d: predict option %d has no text", n, j+1)
			}
		}
		if correct != 1 {
			add("step %d: predict needs exactly one correct option, has %d", n, correct)
		}
	}
	return probs
}

func (v *validator) item(p, concept string, d itemDoc) {
	if d.ID == "" {
		v.errorf("%s: concept %s has an item without id", p, concept)
		return
	}
	if first, dup := v.itemFile[d.ID]; dup {
		v.errorf("%s: duplicate item ID %s (first in %s)", p, d.ID, first)
		return
	}
	v.itemFile[d.ID] = p

	it := domain.Item{
		ID: d.ID, ConceptID: concept, Type: domain.ItemType(d.Type),
		Prompt: d.Prompt, Code: d.Code, Answers: d.Answers, Lines: d.Lines,
		BugLine: d.BugLine, CaseSensitive: d.CaseSensitive,
		Explanation: d.Explanation, Hint: d.Hint,
		Language: d.Language, Starter: d.Starter, Solution: d.Solution,
		Expected: d.Expected, Setup: d.Setup, Rows: rows(d.Rows), Ordered: d.Ordered,
	}
	if it.Type != domain.Code {
		for _, f := range codeFieldsSet(d) {
			v.errorf("%s: item %s: %s is only for code items", p, d.ID, f)
		}
	}
	for _, o := range d.Options {
		it.Options = append(it.Options, domain.Option(o))
	}
	for _, msg := range itemProblems(it) {
		v.errorf("%s: item %s: %s", p, d.ID, msg)
	}
	v.items = append(v.items, it)
}

// itemProblems lists everything wrong with one item.
func itemProblems(it domain.Item) []string {
	var probs []string
	add := func(format string, args ...any) { probs = append(probs, fmt.Sprintf(format, args...)) }

	if !it.Type.Valid() {
		add("unknown type %q", it.Type)
		return probs
	}
	if strings.TrimSpace(it.Prompt) == "" {
		add("prompt is empty")
	}
	correct := len(it.CorrectOptions())

	switch it.Type {
	case domain.MCQ:
		needOptions(add, it)
		if correct != 1 {
			add("mcq needs exactly one correct option, has %d", correct)
		}
	case domain.Multi:
		needOptions(add, it)
		if correct < 1 {
			add("multi needs at least one correct option, has 0")
		}
	case domain.Output:
		if strings.TrimSpace(it.Code) == "" {
			add("output needs code")
		}
		switch {
		case len(it.Options) > 0 && len(it.Answers) > 0:
			add("output takes options or answers, not both")
		case len(it.Options) > 0:
			needOptions(add, it)
			if correct != 1 {
				add("output with options needs exactly one correct option, has %d", correct)
			}
		case len(it.Answers) == 0:
			add("output needs options or answers")
		}
	case domain.Cloze:
		blanks := check.Blanks(it)
		if blanks == 0 {
			add("cloze has no %s blank in prompt or code", check.BlankMarker)
		}
		if len(it.Answers) == 0 {
			add("cloze needs answers")
		}
		for i, a := range it.Answers {
			if n := len(strings.Split(a, "|")); blanks > 0 && n != blanks {
				add("cloze answer %d fills %d blanks (separated by |), the item has %d", i+1, n, blanks)
			}
		}
	case domain.Parsons:
		if len(it.Lines) < 2 {
			add("parsons needs at least 2 lines, has %d", len(it.Lines))
		}
	case domain.Bug:
		n := len(strings.Split(strings.TrimRight(it.Code, "\n"), "\n"))
		if strings.TrimSpace(it.Code) == "" {
			add("bug needs code")
		} else if it.BugLine < 1 || it.BugLine > n {
			add("bug_line %d is outside the code's %d lines", it.BugLine, n)
		}
	case domain.Command:
		if len(it.Answers) == 0 {
			add("command needs answers")
		}
	case domain.Code:
		codeProblems(add, it)
	}
	return probs
}

// codeProblems checks a code item: a known language, starter code, a
// reference solution, and the expected stdout or, for sql, result rows.
func codeProblems(add func(string, ...any), it domain.Item) {
	switch {
	case it.Language == "":
		add("code needs a language (%s)", strings.Join(domain.Languages, ", "))
	case !domain.ValidLanguage(it.Language):
		add("unknown language %q (want %s)", it.Language, strings.Join(domain.Languages, ", "))
	}
	if strings.TrimSpace(it.Starter) == "" {
		add("code needs starter code")
	}
	if strings.TrimSpace(it.Solution) == "" {
		add("code needs a solution")
	}
	if it.Language != domain.SQL {
		if strings.TrimSpace(it.Expected) == "" {
			add("code needs the expected output")
		}
		if it.Rows != nil {
			add("rows are only for sql code items; use expected")
		}
		if it.Setup != "" {
			add("setup is only for sql code items")
		}
		if it.Ordered {
			add("ordered is only for sql code items")
		}
		return
	}
	if it.Expected != "" {
		add("sql code compares rows, not expected output; use rows")
	}
	if it.Rows == nil {
		add("sql code needs the expected rows")
	}
	for i, row := range it.Rows {
		if len(row) != len(it.Rows[0]) {
			add("row %d has %d columns, row 1 has %d", i+1, len(row), len(it.Rows[0]))
		}
	}
}

// codeFieldsSet names the code-only fields set on an item doc.
func codeFieldsSet(d itemDoc) []string {
	var out []string
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"language", d.Language != ""}, {"starter", d.Starter != ""}, {"solution", d.Solution != ""},
		{"expected", d.Expected != ""}, {"setup", d.Setup != ""}, {"rows", d.Rows != nil}, {"ordered", d.Ordered},
	} {
		if f.set {
			out = append(out, f.name)
		}
	}
	return out
}

// needOptions checks the shared option rules: at least two options, all
// with text, and every wrong one naming a misconception and an explanation.
func needOptions(add func(string, ...any), it domain.Item) {
	if len(it.Options) < 2 {
		add("%s needs at least 2 options, has %d", it.Type, len(it.Options))
	}
	for i, o := range it.Options {
		if strings.TrimSpace(o.Text) == "" {
			add("option %d has no text", i+1)
		}
		if o.Correct {
			continue
		}
		if strings.TrimSpace(o.Misconception) == "" {
			add("wrong option %d needs a misconception", i+1)
		}
		if strings.TrimSpace(o.Explanation) == "" {
			add("wrong option %d needs an explanation", i+1)
		}
	}
}

// prereqs checks that every prerequisite exists and that there are no cycles.
func (v *validator) prereqs() {
	known := map[string]domain.Concept{}
	for _, c := range v.concepts {
		known[c.ID] = c
	}
	for _, c := range v.concepts {
		for _, pre := range c.Prereqs {
			if _, ok := known[pre]; !ok {
				v.errorf("%s: concept %s: unknown prerequisite %s", v.conceptFile[c.ID], c.ID, pre)
			}
		}
	}

	const (
		unvisited = iota
		visiting
		done
	)
	mark := map[string]int{}
	var stack []string
	var visit func(id string)
	visit = func(id string) {
		mark[id] = visiting
		stack = append(stack, id)
		for _, pre := range known[id].Prereqs {
			if _, ok := known[pre]; !ok {
				continue
			}
			switch mark[pre] {
			case visiting:
				start := slices.Index(stack, pre)
				cycle := append(slices.Clone(stack[start:]), pre)
				v.errorf("%s: concept %s: prerequisite cycle %s", v.conceptFile[pre], pre, strings.Join(cycle, " -> "))
			case unvisited:
				visit(pre)
			}
		}
		stack = stack[:len(stack)-1]
		mark[id] = done
	}
	for _, c := range v.concepts {
		if mark[c.ID] == unvisited {
			visit(c.ID)
		}
	}
}

// fileSections checks the optional sections of a track file once every
// concept is known: each has an ID unique across all tracks, a title and
// at least one concept; every concept exists, belongs to the track and is
// in no other section.
func (v *validator) fileSections(f parsed) {
	p, track := f.path, f.doc.Track.ID
	if track == "" || v.trackFile[track] != p {
		return // the track itself is broken or a duplicate; already reported
	}
	conceptTrack := map[string]string{}
	for _, c := range v.concepts {
		conceptTrack[c.ID] = c.Track
	}
	owner := map[string]string{} // concept ID -> section ID, within this track
	for _, d := range f.doc.Sections {
		if d.ID == "" {
			v.errorf("%s: section with title %q has no id", p, d.Title)
			continue
		}
		if first, dup := v.sectionFile[d.ID]; dup {
			v.errorf("%s: duplicate section ID %s (first in %s)", p, d.ID, first)
			continue
		}
		v.sectionFile[d.ID] = p
		if strings.TrimSpace(d.Title) == "" {
			v.errorf("%s: section %s has no title", p, d.ID)
		}
		if len(d.Concepts) == 0 {
			v.errorf("%s: section %s has no concepts", p, d.ID)
		}
		for _, id := range d.Concepts {
			t, known := conceptTrack[id]
			switch {
			case !known:
				v.errorf("%s: section %s: unknown concept %s", p, d.ID, id)
				continue
			case t != track:
				v.errorf("%s: section %s: concept %s belongs to track %s, not %s", p, d.ID, id, t, track)
				continue
			}
			if first, dup := owner[id]; dup {
				v.errorf("%s: section %s: concept %s is in two sections (also %s)", p, d.ID, id, first)
				continue
			}
			owner[id] = d.ID
		}
		v.sections = append(v.sections, domain.Section{ID: d.ID, Track: track, Title: d.Title, Concepts: d.Concepts})
	}
}

// catalog assembles the validated content with tracks sorted by order then
// ID, and concepts and items following their track's position.
func (v *validator) catalog() domain.Catalog {
	tracks := slices.Clone(v.tracks)
	slices.SortStableFunc(tracks, func(a, b domain.Track) int {
		return cmp.Or(cmp.Compare(a.Order, b.Order), cmp.Compare(a.ID, b.ID))
	})
	rank := map[string]int{}
	for i, t := range tracks {
		rank[t.ID] = i
	}
	conceptTrack := map[string]string{}
	for _, c := range v.concepts {
		conceptTrack[c.ID] = c.Track
	}

	concepts := slices.Clone(v.concepts)
	slices.SortStableFunc(concepts, func(a, b domain.Concept) int {
		return cmp.Compare(rank[a.Track], rank[b.Track])
	})
	items := slices.Clone(v.items)
	slices.SortStableFunc(items, func(a, b domain.Item) int {
		return cmp.Compare(rank[conceptTrack[a.ConceptID]], rank[conceptTrack[b.ConceptID]])
	})
	sections := slices.Clone(v.sections)
	slices.SortStableFunc(sections, func(a, b domain.Section) int {
		return cmp.Compare(rank[a.Track], rank[b.Track])
	})
	return domain.Catalog{Tracks: tracks, Concepts: concepts, Items: items, Sections: sections}
}
