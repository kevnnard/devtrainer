# Content schema

Study content is YAML, embedded into the binary from `content/<track>/<track>.yaml`
(pattern `*/*.yaml`). One file holds one track: its metadata, its concepts and each
concept's items. `go test ./content/` loads and validates everything that ships.

## Model

```
Track ──< Concept ──< Item
            │
            └── prereqs: other concept IDs (any track)
```

- **Concepts** are what gets scheduled (FSRS) and mastered.
- **Items** are the closed questions that exercise a concept; each review shows a
  different item than last time when the concept has more than one.
- **Sections** optionally group some concepts of a track; each section ends with
  an exam (see Sections and exams).

## File layout

```yaml
track:
  id: git                     # required; must equal the directory name
  title: Git                  # required
  description: Version control from the first commit to branching.
  order: 50                   # display and new-concept order; lower first

concepts:
  - id: git.branches          # required; unique across all tracks
    title: Branches           # required
    summary: A branch is a movable pointer to a commit.
    level: beginner           # required: beginner | intermediate | advanced
    prereqs: [git.staging]    # optional; concept IDs that must exist, no cycles
    lesson: {...}             # optional; a worked example, see Lessons below
    items:                    # required; at least one
      - id: git.branches.mcq-what-is   # required; unique across all tracks
        type: mcq                      # required; see the types below
        prompt: What is a branch?      # required
        code: |                        # optional snippet shown with the prompt
          git branch feature
        options: [...]                 # mcq, multi, output (choice)
        answers: [...]                 # cloze, command, output (typed)
        lines: [...]                   # parsons
        bug_line: 2                    # bug
        case_sensitive: false          # cloze only; default false
        language: python               # code; see Code exercises below
        starter: | ...                 # code
        solution: | ...                # code
        expected: | ...                # code, every language but sql
        setup: | ...                   # code, sql only
        rows: [...]                    # code, sql only
        ordered: true                  # code, sql only; default false
        explanation: Why the right answer is right.
        hint: A nudge shown on request (using it grades the answer Hard).

sections:                     # optional; see Sections and exams
  - id: git/basics            # required; unique across all tracks
    title: Basics             # required
    concepts: [git.staging, git.branches]   # required; concepts of this track
```

Unknown keys are errors, so typos (`hnit:`) fail loudly.

### Options

```yaml
options:
  - text: The staged changes
    correct: true
    explanation: Optional for correct options.
  - text: Every change in the working tree
    misconception: commit-takes-worktree   # required on wrong options
    explanation: Only staged changes are committed.  # required on wrong options
```

Every wrong option names the **misconception** that leads to it (a short stable
kebab-case key, reused across items so stats can group them) and **explains** why
it is wrong.

## Lessons

A concept may carry one `lesson`: a short worked example shown the first time the
concept comes up, before its first question (and later on demand from Tracks).
Predictions in a lesson are never graded or scheduled; only "lesson seen" is stored.

```yaml
lesson:
  intro: Git keeps three places...          # required; one or two sentences
  example:                                  # optional
    lang: shell                             # optional label (go, python, ts, sql, shell, json...)
    code: |                                 # required when example is set; at most 15 lines
      $ git add a.txt
  steps:                                    # required; at least one, revealed one by one
    - text: git add copies a.txt into the index.     # required
      predict:                              # optional: asked before the step's text is shown
        prompt: Where is the new a.txt now?  # required
        options:                            # 2 to 9 options (keys 1-9); exactly one correct
          - text: In the working tree and in the index
            correct: true
          - text: In a new commit
            explanation: git add does not commit.    # optional; shown when this option is picked
  takeaway: git add stages; git commit records the index.   # required
```

The learner sees the verdict (right, or the correct option) together with the
step's text and the picked option's `explanation`. Vary which option is correct.

### Error on purpose

A step may show the real error the example produces and ask why it fails
before the explanation (`text`) is revealed:

```yaml
example:
  lang: python
  code: |
    print(totl)
steps:
  - text: totl was never assigned; Python only knows total.
    error: "NameError: name 'totl' is not defined"   # copied from a real run
    predict:                                         # required with error
      prompt: Why does it fail?
      options:
        - text: The name is misspelled
          correct: true
        - text: print needs two arguments
```

`error` must be captured by actually running `example.code` (a sql example
runs on an empty in-memory SQLite database), never written by hand. When the
example's `lang` is a code language, the content tests run the example and
fail if its error does not contain `error`.

## Item types

| type      | fields used            | grading |
|-----------|------------------------|---------|
| `mcq`     | `options`              | exactly one option is `correct` |
| `multi`   | `options`              | the chosen set must equal the correct set |
| `output`  | `code` + `options` **or** `answers` | predict what `code` prints; typed answers ignore trailing spaces and surrounding blank lines, case matters |
| `cloze`   | `prompt`/`code` with `___`, `answers` | one accepted answer per variant; several blanks in one variant are separated by `\|`, e.g. `0 \| 10`; whitespace collapsed, case ignored unless `case_sensitive: true` |
| `parsons` | `lines`                | lines in the correct order; the app shuffles them; order must match exactly |
| `bug`     | `code`, `bug_line`     | `bug_line` is the 1-based faulty line of `code` |
| `command` | `answers`              | accepted variants; extra spaces and quote style are ignored, a leading `$ ` is dropped, case always matters |
| `code`    | `language`, `starter`, `solution`, `expected` **or** `setup` + `rows` | the learner's program is run for real; see Code exercises |

## Code exercises

The learner edits `starter` in a multi-line editor and runs it with ctrl+r.
The program runs in a fresh temporary directory with a time limit and an
output cap; the real compiler or runtime error is shown when it fails.

```yaml
- id: py.loops.code-count
  type: code
  language: python          # sql | python | go | javascript | typescript | java | csharp | rust
  prompt: Print the numbers 1 to 3, one per line.
  starter: |                # required; what the editor opens with (must not already pass)
    for i in range(3):
        print(i)
  solution: |               # required; shown after a failed run, and must pass
    for i in range(1, 4):
        print(i)
  expected: |               # required for every language but sql: the exact stdout
    1
    2
    3
```

`expected` is compared with trailing spaces and surrounding blank lines
ignored; case and inner spacing matter. A program that exits non-zero, times
out or prints past the cap fails.

`sql` runs in-process on a fresh in-memory SQLite database (no tool needed):
`setup` runs first, then the learner's SQL; the rows of its last statement that
returns rows are compared with `rows`.

```yaml
- id: sql.select.code-names
  type: code
  language: sql
  prompt: List every user's name, alphabetically.
  setup: |                  # optional: schema and data, run before the learner's SQL
    CREATE TABLE users (id INTEGER, name TEXT);
    INSERT INTO users VALUES (1, 'Bo'), (2, 'Ana');
  starter: |
    SELECT
  solution: |
    SELECT name FROM users ORDER BY name;
  rows:                     # required for sql; one list per row, all the same width
    - [Ana]
    - [Bo]
  ordered: true             # optional; default false compares rows as a multiset
```

Cells are compared as text, the way the sqlite3 shell prints them: write each
value as it appears (`150`, `2.5`, `3.0`, `Ana`); YAML `null` (or `~`) stands
for SQL NULL. Column names are not compared.

Grading: a pass is Good (Easy when submitted within 90 s of the question
appearing), a fail is Again, a pass after the hint is Hard. Run time does not
count toward the answer time.

Toolchains: python uses `python3` (or `python`), go `go`, javascript `node`,
typescript `node` (or `deno`), rust `rustc`, csharp `dotnet` (`dotnet run app.cs`),
java resolves `$DEVTRAINER_JAVA`, then Homebrew's `openjdk@21`, then `java` on
PATH (JDK 21+). When a language's tool is missing its items are not served, the
app says which tool is missing, and nothing is graded; `devtrainer validate`
lists the toolchain of each language in use. The content tests run every
solution and starter for the toolchains installed and skip the others.

## Sections and exams

A track may group its concepts into `sections` (a top-level block of the track
file, after `concepts`). Each section ends with an exam that mixes items from
all its concepts. Tracks without sections work as before; a concept may be in
no section.

```yaml
sections:
  - id: go/foundations          # unique across all tracks: <track>/<slug>
    title: Foundations
    concepts: [go.basics, go.slices, go.errors]   # Tracks lists them in this order
```

- **Available** when every concept of the section is at least *learning*
  (answered correctly once). Until then Tracks names the concepts still missing.
- **Questions**: up to 2 items of each concept (the next ones after the item
  shown last), taken round robin across the concepts. Closed and `code` items
  both count; code items whose toolchain is missing are skipped, and a concept
  left with no item is skipped too.
- **Grading**: each answer is graded and scheduled (FSRS) exactly like a normal
  review and stored in the answer history.
- **Pass**: at least 80% correct (`100 × correct ≥ 80 × total`, so 79% fails and
  80% passes). Every attempt is stored; a passed section shows in Tracks and
  Stats. An exam can be retaken at any time once available.

In the app: on Tracks, `x` starts the exam of the selected concept's section.

## Validation (load errors)

Each error names the file and the track, concept or item ID. All errors are
reported at once.

- track `id` missing or different from its directory; duplicate track; no title; no concepts
- concept `id` missing or duplicated; no title; bad `level`; no items
- unknown prerequisite; prerequisite cycle
- item `id` missing or duplicated; unknown `type`; empty `prompt`
- `mcq` (and choice `output`) without exactly one correct option; fewer than 2 options
- `multi` without a correct option
- wrong option without `misconception` or `explanation`
- `output` without `code`, or with both/neither of `options` and `answers`
- `cloze` without a `___` blank, without answers, or with an answer whose `|`
  count does not match the blanks
- `parsons` with fewer than 2 lines
- `bug` without `code` or with `bug_line` outside it
- `command` without answers
- `code` with a missing or unknown `language`, or without `starter` or
  `solution`; a non-sql item without `expected` or with `setup`, `rows` or
  `ordered`; a sql item without `rows` or with `expected`; rows of different
  widths
- code-only fields (`language`, `starter`, `solution`, `expected`, `setup`,
  `rows`, `ordered`) on an item that is not `code`
- `lesson` without `intro`, `takeaway` or steps; a step without `text`; an
  `example` without `code` or with more than 15 lines of code
- lesson `predict` without a prompt, with fewer than 2 or more than 9 options, an
  option without text, or not exactly one correct option
- a lesson step with `error` but no `predict`, or in a lesson without an `example`
- section without `id` or `title`, a duplicate section ID (across all tracks), a
  section with no concepts, an unknown concept or one of another track, a
  concept listed in two sections

## ID conventions

- Concept: `<track>.<concept>` — `git.branches`
- Item: `<concept>.<type>-<slug>` — `git.branches.cloze-switch-create`
- Misconception: kebab-case, reusable — `commit-takes-worktree`
- Section: `<track>/<slug>` — `git/basics`
