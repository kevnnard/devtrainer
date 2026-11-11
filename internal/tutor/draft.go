package tutor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/domain"
)

// draftFile is the name the draft takes inside the validation file system.
// Load names the file in each error, so errors point at the draft.
const draftFile = "draft.yaml"

// InvalidDraftError is a draft the model wrote that does not pass content
// validation. YAML is what the model wrote, for inspection.
type InvalidDraftError struct {
	YAML string
	Err  error
}

func (e *InvalidDraftError) Error() string {
	return "the drafted YAML is not valid content:\n" + e.Err.Error()
}

func (e *InvalidDraftError) Unwrap() error { return e.Err }

// draftSystem teaches the model the content schema (content/SCHEMA.md).
const draftSystem = `You write study content for devtrainer, a spaced-repetition trainer for programming, ` +
	`architecture and tooling. Content is YAML in exactly this schema; unknown keys are errors:

track:
  id: <track id>
  title: <track title>
concepts:
  - id: <track>.<concept>          # unique, kebab-case after the dot
    title: <short title>
    summary: <one sentence>
    level: beginner | intermediate | advanced
    prereqs: [<existing concept IDs>]   # optional
    items:
      - id: <concept id>.<type>-<slug>  # unique
        type: mcq | multi | output | cloze | parsons | bug | command
        prompt: <question>
        code: |                    # optional; required for output and bug
          <snippet>
        options:                   # mcq, multi, output (choice)
          - text: <right answer>
            correct: true
          - text: <wrong answer>
            misconception: <kebab-case-key>   # required on every wrong option
            explanation: <why it is wrong>    # required on every wrong option
        answers: [<accepted variants>]  # cloze, command, output (typed)
        lines: [<lines in the correct order>]  # parsons, at least 2
        bug_line: <1-based faulty line of code>  # bug
        explanation: <why the right answer is right>
        hint: <a nudge>

Rules:
- mcq and choice output have exactly one correct option and at least 2 options; multi has at least one correct option.
- output has code and either options or answers, never both.
- cloze puts ___ blanks in prompt or code; each answer lists one value per blank separated by " | ".
- command answers are exact commands; case matters.
- Reuse existing misconception keys when they fit.
- Questions are accurate, unambiguous and test understanding, not trivia. Wrong options are plausible mistakes.
- Use a variety of item types.

Reply with ONLY the YAML document, no markdown fences and no text around it.`

// Draft asks the model for one new concept about topic in track, with
// items, and validates it together with the existing content in catalog
// (the content file system, typically content.FS) so prerequisites and
// duplicate IDs are checked. It returns the drafted YAML; a draft that
// fails validation is returned as an *InvalidDraftError.
func (t *Tutor) Draft(ctx context.Context, track, topic string, catalog fs.FS) (string, error) {
	cat, err := content.Load(catalog)
	if err != nil {
		return "", fmt.Errorf("load the existing content: %w", err)
	}
	out, err := ask(ctx, t.models.Draft, Request{System: draftSystem, Prompt: draftPrompt(track, topic, cat)})
	if err != nil {
		return "", err
	}
	draft := ExtractYAML(out)
	if err := ValidateDraft(draft, track, catalog); err != nil {
		return "", &InvalidDraftError{YAML: draft, Err: err}
	}
	return draft, nil
}

func draftPrompt(track, topic string, cat domain.Catalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Write one new concept about %q for the track %q, with 3 to 5 items of different types.\n", topic, track)
	if tr, ok := cat.Track(track); ok {
		fmt.Fprintf(&b, "The track exists: id %s, title %q. Repeat that track header exactly and add only the new concept.\n", tr.ID, tr.Title)
		b.WriteString("Concepts already in this track (do not repeat them):\n")
		for _, c := range cat.ConceptsIn(track) {
			fmt.Fprintf(&b, "  %s — %s (%s)\n", c.ID, c.Title, c.Level)
		}
	} else {
		fmt.Fprintf(&b, "The track %q is new: give it a short title.\n", track)
	}
	b.WriteString("Existing concept IDs you may use as prereqs:\n")
	for _, c := range cat.Concepts {
		fmt.Fprintf(&b, "  %s\n", c.ID)
	}
	if keys := misconceptions(cat); len(keys) > 0 {
		b.WriteString("Misconception keys already in use: " + strings.Join(keys, ", ") + "\n")
	}
	return b.String()
}

// misconceptions lists the distinct misconception keys of the catalog.
func misconceptions(cat domain.Catalog) []string {
	var keys []string
	for _, it := range cat.Items {
		for _, o := range it.Options {
			if o.Misconception != "" && !slices.Contains(keys, o.Misconception) {
				keys = append(keys, o.Misconception)
			}
		}
	}
	slices.Sort(keys)
	return keys
}

// ValidateDraft checks that draft is a track file for track with at least
// one concept, and that it loads together with the content in catalog. For
// an existing track, the draft's concepts are appended to that track's
// file so prerequisites resolve and duplicate IDs are caught.
func ValidateDraft(draft, track string, catalog fs.FS) error {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(draft), &doc); err != nil {
		return fmt.Errorf("%s: %w", draftFile, err)
	}
	root := documentMap(&doc)
	if root == nil {
		return fmt.Errorf("%s: not a YAML mapping with track and concepts", draftFile)
	}
	if id := scalar(mapValue(mapValue(root, "track"), "id")); id != track {
		return fmt.Errorf("%s: track id is %q, want %q", draftFile, id, track)
	}
	concepts := mapValue(root, "concepts")
	if concepts == nil || concepts.Kind != yaml.SequenceNode || len(concepts.Content) == 0 {
		return fmt.Errorf("%s: no concepts", draftFile)
	}

	fsys := fstest.MapFS{}
	existing := ""
	err := fs.WalkDir(catalog, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ok, _ := path.Match(content.Pattern, p); !ok {
			return nil
		}
		data, err := fs.ReadFile(catalog, p)
		if err != nil {
			return err
		}
		if path.Dir(p) == track && existing == "" {
			existing = p
		}
		fsys[p] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read the existing content: %w", err)
	}

	merged := []byte(draft)
	if existing != "" {
		if merged, err = appendConcepts(fsys[existing].Data, concepts); err != nil {
			return fmt.Errorf("%s: %w", existing, err)
		}
		delete(fsys, existing)
	}
	fsys[path.Join(track, draftFile)] = &fstest.MapFile{Data: merged}
	_, err = content.Load(fsys)
	return err
}

// appendConcepts appends the concept nodes to the concepts of a track file.
func appendConcepts(file []byte, concepts *yaml.Node) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(file, &doc); err != nil {
		return nil, err
	}
	root := documentMap(&doc)
	list := mapValue(root, "concepts")
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil, errors.New("no concepts list")
	}
	list.Content = append(list.Content, concepts.Content...)
	return yaml.Marshal(&doc)
}

func documentMap(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	return doc.Content[0]
}

// mapValue returns the value of key in mapping node m, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// ExtractYAML returns the YAML document in a model's answer: the body of
// the first ``` fence when there is one, else everything from the first
// line that starts a track file ("track:" or "concepts:"). Trailing blank
// lines are dropped and the result ends with one newline.
func ExtractYAML(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if start := strings.Index(s, "```"); start >= 0 {
		body := s[start+3:]
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			body = body[nl+1:] // drop the info string, like "yaml"
		}
		if end := strings.Index(body, "```"); end >= 0 {
			body = body[:end]
		}
		s = body
	} else {
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			if strings.HasPrefix(l, "track:") || strings.HasPrefix(l, "concepts:") {
				s = strings.Join(lines[i:], "\n")
				break
			}
		}
	}
	s = strings.Trim(s, "\n")
	if s == "" {
		return ""
	}
	return s + "\n"
}
