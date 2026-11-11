<p align="center">
  <img src="docs/assets/logo.png" alt="devtrainer logo" width="200">
</p>

<h1 align="center">devtrainer</h1>

<p align="center">
  Short daily practice sessions in your terminal, for programming, architecture and tooling.<br>
  Spaced repetition, auto-graded questions, and code exercises that really run.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-PolyForm%20Strict%201.0.0-7dd3fc?style=flat-square" alt="License: PolyForm Strict 1.0.0"></a>
</p>

```bash
brew install kevnnard/tap/devtrainer
devtrainer
```

## Why devtrainer

You forget most of what you read within days unless you go back to it, and coming back *just before* you forget
is what makes it stick. devtrainer plans those returns for you: every day it brings back the concepts that are about
to fade, mixes them across subjects, and adds a few new ones. Instead of rereading, you answer: pick an option,
predict what code prints, fill a blank, find the bug, type the command, or write code that devtrainer runs and checks.

It is built for developers who want to stay sharp or prepare for interviews and certifications:

- **Interview preparation** — algorithms, language semantics, SQL, system design trade-offs.
- **Learning a new language** — Go, Python, TypeScript, JavaScript, Java, C# or Rust, with real code exercises.
- **Tooling and cloud** — Git, Linux, Docker and AWS (aligned with the Solutions Architect Associate, SAA-C03, domains).

About 30 minutes a day is enough. The [study plan](docs/STUDY-PLAN.md) gives a full 26-week route, and
[pedagogy](docs/PEDAGOGY.md) explains the research behind the design.

## What you get

| | |
|:--|:--|
| **14 tracks** | 106 concepts and 492 questions, embedded in the app. No account, no download of content. |
| **82 code exercises** | Write real code in 7 languages plus SQL; devtrainer runs it on your machine and compares its output. |
| **Spaced repetition** | FSRS scheduling per concept. Each review uses a different question of the concept. |
| **Grades from what you did** | Wrong, used a hint or slow, correct, fast: you never grade yourself. |
| **Lessons** | A short worked example before a concept's first question, step by step. |
| **Section exams** | Mixed exams that pass at 80%, in the Go, Python, JavaScript and SQL tracks. |
| **Progress** | Streak, mastery per track, your most frequent misconceptions and weakest concepts. |
| **Works offline** | Everything except the optional AI tutor runs locally. |

## Install

```bash
brew install kevnnard/tap/devtrainer
devtrainer --version   # devtrainer 0.1.0
```

The formula lives in the [kevnnard/homebrew-tap](https://github.com/kevnnard/homebrew-tap) tap. Homebrew builds
devtrainer from source with Go (installed for you as a build dependency); there are no prebuilt bottles. It builds
from the same source on macOS and Linux, but version 0.1.0 has only been run and tested on Linux.

Upgrade or remove it with Homebrew. Removing the app does not touch your progress (see [Your data](#your-data)).

```bash
brew upgrade devtrainer
brew uninstall devtrainer
```

<details>
<summary>Build from source</summary>

You need Go 1.27.3 or newer.

```bash
git clone https://github.com/kevnnard/devtrainer
cd devtrainer
go install ./cmd/devtrainer   # to $GOBIN, or $(go env GOPATH)/bin; that directory must be on PATH
```

A plain source build reports its version as `devtrainer dev`. Testing, release builds and the Homebrew formula
are described in [docs/RELEASING.md](docs/RELEASING.md).
</details>

## Quick start

```bash
devtrainer validate   # 1. see which code toolchains you have (optional, but useful)
devtrainer            # 2. open the app
```

3. On **Home**, press `enter` to start today's session.
4. Answer, read the explanation, press `enter` for the next question. Press `?` at any time for every key.

### Your first session

- **Reviews come first.** Concepts that are due, mixed across tracks. On day one there are none.
- **Then up to 3 new concepts a day** (`--new 5` to change it). A new concept with a lesson opens with a short
  worked example: press `enter` to reveal each step; some steps ask you to predict what comes next (never graded).
  `s` skips the lesson. You can open it again later from **Tracks**.
- **Concepts unlock in order.** A concept appears once its prerequisites have been answered correctly.
- **Mastery** means 4 of your last 5 answers were correct, on 2 or more different days.

Your grade comes from what you did:

| Grade | Questions | Code exercises |
|:--|:--|:--|
| Again | wrong answer | wrong output, or it does not compile or run |
| Hard | correct after a hint, or slower than 45 s | passed after a hint |
| Good | correct, without a hint | passed without a hint, in more than 90 s |
| Easy | correct, without a hint, under 8 s | passed without a hint, within 90 s |

Time spent waiting for a code run does not count.

## Tracks

Tracks are listed in the order of the study plan.

| Track | What it covers | Code exercises |
|:--|:--|:-:|
| Logic & algorithms | Complexity, hashing, two pointers, recursion, binary search, sorting | 6 (Python, Go) |
| Git | From the first commit to rebasing, remotes and undoing mistakes safely | — |
| Linux & terminal | Files, permissions, pipes, processes and networking (bash) | — |
| Go | Modern Go: zero values, slices, errors, interfaces, generics, concurrency | 5 |
| Python | Python 3.12+: semantics, collections, classes, typing, generators, asyncio | 5 |
| JavaScript | Node 22+: coercion, closures, `this`, prototypes, the event loop, modules | 13 |
| TypeScript | Strict TypeScript 5.x: narrowing, generics, utility types, async | 5 |
| Java | Java 21: records, sealed types, pattern matching, generics, collections | 12 |
| C# | C# 14 on .NET 10: nullability, records, pattern matching, LINQ, async, Span | 13 |
| Rust | Ownership, borrowing, traits, lifetimes, iterators, smart pointers, errors | 14 |
| Docker | Images, containers, Dockerfiles, volumes, networks, Compose v2 | — |
| SQL | SELECT to joins, aggregation, indexes and transactions | 9 |
| Software architecture | SOLID, coupling and cohesion, hexagonal, caching, CAP, idempotency, queues | — |
| AWS | SAA-C03 domains: IAM, VPC, EC2 and scaling, S3, databases, Well-Architected | — |

Question types: multiple choice, multiple answer, predict the output, fill in the blanks, reorder lines, find the
faulty line, type the command, and write code.

## Code exercises

You edit the starter code, press `ctrl+r`, and devtrainer runs it in a fresh temporary directory and compares what
it prints with the expected output. When it fails you see the real compiler or runtime error.

devtrainer uses the toolchains already on your machine and never installs or changes them. If a toolchain is
missing, that language's code exercises are simply not served; everything else keeps working. Run
`devtrainer validate` to see what was found.

| Language | Runs with |
|:--|:--|
| SQL | Built in: SQLite, in memory. Nothing to install. |
| Python | `python3` (or `python`) |
| Go | `go` |
| JavaScript | `node` |
| TypeScript | `node`, recent enough to run `.ts` files directly (Node 22.18 or newer). Node only strips the types: it does not type-check. `deno` is used only when there is no `node`. |
| Java 21 | `$DEVTRAINER_JAVA` if set, else Homebrew's `openjdk@21`, else `java` on PATH (with a warning when it is older than 21) |
| C# 14 / .NET 10 | `dotnet run app.cs` (the first run is slow) |
| Rust | `rustc` |

> [!NOTE]
> The SQL track's questions and lessons use the PostgreSQL dialect, but SQL code exercises run and are graded on
> SQLite. Where the two differ, the SQLite result is the one that counts.

> [!WARNING]
> **Code runs unsandboxed, as you.** The timeout and output limit stop runaway programs; they are not a sandbox.
> Your code has your permissions, your files and your environment. Only run code you wrote or reviewed.

## Section exams

Some tracks group their concepts into sections: Go, Python, JavaScript and SQL. A section exam mixes questions from
every concept of the section.

1. On **Tracks**, select a concept and press `x` to start its section's exam.
2. The exam stays locked until every concept of the section is at least "learning".
3. It passes at **80%** or more. Exam answers are scheduled like any other review.
4. After the result, `enter` returns to Tracks.

Passed sections show in **Tracks** and in the app's **Stats** screen. (`devtrainer stats` does not list exams.)

## Keys

Press `?` in the app for this list.

| Where | Key | Action |
|:--|:--|:--|
| Everywhere | `tab` / `shift+tab` | next / previous screen |
| | `1` `2` `3` `4` | Home, Study, Tracks, Stats (in Study, numbers answer first) |
| | `?` | show / hide help |
| | `esc` | back to Home |
| | `q` · `ctrl+c` | quit |
| Study | `1–9` · `j/k` `enter` | choose an option |
| | `space` · `enter` | toggle an option · submit (multiple answer) |
| | type · `enter` | write the answer (output, command) |
| | `tab` · `enter` | next blank · submit on the last (fill in the blanks) |
| | `j/k` · `J/K` | select a line · move it up or down (reorder lines) |
| | `h` · `ctrl+t` | show the hint (`ctrl+t` while typing); a hint grades Hard |
| | `e` | after answering: ask the AI tutor to explain more (optional) |
| | `enter` | after answering: next question |
| Code editor | `enter` · `tab` | new line · indent |
| | `ctrl+r` · `ctrl+s` | run and submit |
| Lesson | `enter` | reveal the next step; after the takeaway, start the questions |
| | `s` | skip the lesson |
| Tracks | `j/k` · `J/K` | choose a track · choose a concept |
| | `l` · `enter` | open the concept's lesson (not graded) |
| | `x` | start the section exam |

## Commands

| Command | What it does |
|:--|:--|
| `devtrainer` | Opens the app: Home, Study, Tracks, Stats. |
| `devtrainer stats` | Prints a plain-text progress summary: streak, accuracy, mastery per track, misconceptions, weakest concepts. |
| `devtrainer validate` | Checks the embedded content and lists the code toolchains found on this machine. |
| `devtrainer version` | Prints the version (same as `--version`). |
| `devtrainer draft <track> <topic>` | Asks the AI for new candidate questions; see [Optional AI tutor](#optional-ai-tutor). |
| `devtrainer -h` | Shows every command and flag. |

| Flag | Default |
|:--|:--|
| `--db path` | `$XDG_DATA_HOME/devtrainer/devtrainer.db`, else `~/.local/share/devtrainer/devtrainer.db` |
| `--new n` | `3` new concepts per day (`0` means reviews only) |
| `--config path` | `$XDG_CONFIG_HOME/devtrainer/config.json`, else `~/.config/devtrainer/config.json` |

Flags go before or after the command.

## Your data

Everything stays on your machine.

| What | Where |
|:--|:--|
| Progress (one SQLite file) | `$XDG_DATA_HOME/devtrainer/devtrainer.db`, else `~/.local/share/devtrainer/devtrainer.db` |
| Optional AI config | `$XDG_CONFIG_HOME/devtrainer/config.json`, else `~/.config/devtrainer/config.json` |

The database and its folder are created on first run. Neither file is inside the Homebrew prefix, so upgrading or
uninstalling devtrainer keeps your progress.

**Back up your progress** by copying the file while devtrainer is closed:

```bash
cp ~/.local/share/devtrainer/devtrainer.db ~/devtrainer-backup-$(date +%F).db
```

To check a backup, open it without touching your real progress: `devtrainer --db ~/devtrainer-backup-2026-11-11.db stats`.
To try devtrainer with a clean slate, point `--db` at a new file instead of deleting your database.

## Optional AI tutor

devtrainer works fully offline. Two features use AI, and only when you ask for them:

- **`e` after an answer** — a deeper explanation of that question and of the misconception behind your answer.
- **`devtrainer draft <track> <topic>`** — writes validated candidate questions to `drafts/<track>-<topic>.yaml`
  in the current directory, for you to review. Content is compiled into the app, so drafts are only useful if you
  build devtrainer from source and add them to `content/`.

The AI runs through the `pi` command-line agent (the engine of Gentle Shell), which you install and log in to
separately. devtrainer runs `pi` with no tools, no session, no skills, no MCP servers and no project files, so
the model can only answer the question. devtrainer never reads your credentials; `pi` uses your own login and
provider, and any cost or usage limits are those of your provider account. Nothing is free or bundled.

**What is sent** when you press `e`: the concept's title and summary, the question, its code and options, your
answer (for code exercises, the code you wrote), the correct answer, and the explanation already shown. `draft`
sends the topic and the IDs and titles of the existing concepts. Nothing else, and nothing until you press the key
or run the command.

**Configure it** in `~/.config/devtrainer/config.json`. Every field is optional; missing ones keep their default.

```json
{
  "pi": "pi",
  "models": {
    "explain": "provider/model",
    "draft": "provider/model"
  },
  "timeout_seconds": 90
}
```

| Field | Meaning | Default |
|:--|:--|:--|
| `pi` | The `pi` command, or a full path to it | `pi` from PATH |
| `models.explain` | Model for `e`, as pi's `provider/model` | `claude-bridge/claude-sonnet-5-5` |
| `models.draft` | Model for `draft` | `claude-bridge/claude-sonnet-5-5` |
| `timeout_seconds` | How long one answer may take | `90` |

The default model only works if your `pi` has that provider set up; set `models` to a model you can use.

## Troubleshooting

<details>
<summary><b>A language's code exercises never show up</b></summary>

Its toolchain was not found. Run `devtrainer validate`: the "Code exercises" list shows, per language, the tool it
uses or what is missing. Install the toolchain with your usual package manager and open devtrainer again.
</details>

<details>
<summary><b>TypeScript exercises fail with an error about the <code>.ts</code> extension</b></summary>

Your `node` is too old to run TypeScript files directly. Upgrade Node.js to 22.18 or newer (`node --version`).
devtrainer uses `deno` only when there is no `node` on PATH, so installing Deno alone does not help.
</details>

<details>
<summary><b>Java warns that the JDK is older than 21</b></summary>

Install JDK 21 (for example `brew install openjdk@21`, which devtrainer finds on its own) or point devtrainer at
one: `export DEVTRAINER_JAVA=/path/to/jdk-21/bin/java`.
</details>

<details>
<summary><b>The first C# exercise is very slow</b></summary>

`dotnet run app.cs` builds a project on the first run. Later runs are faster. C# runs get up to 120 seconds.
</details>

<details>
<summary><b><code>devtrainer --version</code> shows <code>dev</code> or an old version after <code>brew install</code></b></summary>

Another `devtrainer` comes first on your PATH, usually one you built with `go install`. See every copy, in the
order your shell uses them:

```bash
which -a devtrainer
```

Then either put Homebrew's `bin` directory before Go's in your PATH, or, if you no longer want your source build,
remove that one file you installed: `$(go env GOPATH)/bin/devtrainer` (or `$GOBIN/devtrainer`). Leave any other
copy alone. Run `hash -r` (or open a new terminal) and check `devtrainer --version` again. Your progress is not
affected: every copy uses the same database.
</details>

<details>
<summary><b><code>brew install</code> fails while building</b></summary>

Homebrew compiles devtrainer with Go. Run `brew update` and try again; `brew doctor` reports common setup problems.
</details>

<details>
<summary><b><code>e</code> says pi was not found, timed out, or failed</b></summary>

- *not found* — install `pi`, or set its full path in `"pi"` in the config.
- *model not found* or a login error — set `models` to a `provider/model` that works in your `pi`.
- *timed out* — raise `timeout_seconds`.

Studying never depends on this: close the message and keep going.
</details>

<details>
<summary><b>Long compiler errors are cut off</b></summary>

On a small terminal (80×24), long errors and solutions are shortened in the preview, and it says how much was cut.
A larger terminal window shows more.
</details>

## License

**devtrainer is source-available, not open source.** You can read the code, build it and use it for personal,
noncommercial purposes.

You may **not**:

- share or redistribute it, in source or binary form;
- publish modified versions or new works based on it;
- use it commercially.

The full terms are in the [PolyForm Strict License 1.0.0](LICENSE). Copyright © 2026 Kevnnard.
