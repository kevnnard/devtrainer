# Releasing devtrainer

One command cuts a release: it checks everything, tests and builds, tags and
pushes `main`, publishes the GitHub release with the exact source archive, and
updates the Homebrew formula in `kevnnard/homebrew-tap`. Run a dry run first.

## Quick path

```sh
# 0. Once: clone the tap where the script expects it (ignored by git).
git clone https://github.com/kevnnard/homebrew-tap.git .release/homebrew-tap

# 1. Check everything without publishing anything.
scripts/release.sh --dry-run 0.1.0

# 2. Publish.
scripts/release.sh 0.1.0                         # GitHub-generated notes
scripts/release.sh --notes notes.md 0.1.0        # or your own notes file

# 3. Install or upgrade like a user.
brew install kevnnard/tap/devtrainer
devtrainer --version                             # devtrainer 0.1.0
```

## Prerequisites

| Need | Why |
|---|---|
| bash, git, curl, tar, sha256sum or shasum | the script itself |
| python3 3.8+ | renders the formula (`scripts/release_tap.py`) |
| Go at least the `go` version in `go.mod` | tests and builds run with `GOTOOLCHAIN=local`, as Homebrew does |
| `gh`, logged in (`gh auth login`) with push rights | creates the GitHub release |
| `origin` = `github.com/kevnnard/devtrainer` | the script refuses any other remote |
| tap checkout on `main`, clean, equal to its origin | the release adds only `Formula/devtrainer.rb` |

`TAP_DIR=/path/to/homebrew-tap` points the script at a tap checkout elsewhere.

## Command

```text
scripts/release.sh [--dry-run] [--notes FILE] X.Y.Z
```

| Part | Meaning |
|---|---|
| `X.Y.Z` | numeric version, no `v`, no leading zeros, no suffix; the tag is `vX.Y.Z` |
| `--dry-run` | every check, test and build; prints the plan and the formula; publishes nothing |
| `--notes FILE` | release notes in Markdown; without it GitHub generates them |
| exit codes | `0` done, `1` a check or step failed, `2` bad usage |

The version is set at build time with `-ldflags "-X main.version=X.Y.Z"`; there
is no version-bump commit. Untagged builds report `devtrainer dev`.

## What it checks before changing anything

1. Tools are installed and `python3` is new enough.
2. You are on `main`; there are no tracked changes and no untracked files that
   are not ignored. `.release/` must be ignored.
3. `origin` resolves, **as git resolves it** (after `url.*.insteadOf`,
   `url.*.pushInsteadOf` and `pushurl`), to exactly one fetch URL and exactly
   one push URL, both `github.com/kevnnard/devtrainer` over HTTPS or SSH; it is
   not a mirror (`remote.origin.mirror`). Tag `vX.Y.Z` does not exist locally.
4. **No commit reachable from `main` contains private paths**: agent and
   planning files (`odd/`, `design/`, `designs/`, `drafts/`, `.agents/`,
   `.claude/`, `.codex/`, `.atl/`, `.pi/`, `AGENTS.md`, `CLAUDE.md`, …) and
   databases (`*.db`). Every commit is checked, not only the latest one.
5. `gh` is logged in and can read the repository; the tag is not on `origin`;
   `origin/main` is absent or an ancestor of `HEAD` (no force-push is ever
   needed); no GitHub release exists for the tag. A failed query stops the
   release; it is never read as "absent".
6. The tap checkout is a git root on `main`, its origin resolves the same way
   to exactly one fetch and one push URL for `kevnnard/homebrew-tap`, it has no
   changes or untracked files, and it equals its origin.
7. `go test ./...`, `go vet ./...`, a `CGO_ENABLED=0` linux/amd64 build, and a
   host build whose `--version` and `validate` are smoke-tested in a throwaway
   home (no database may be created). The formula is rendered as a preview.

Builds and the preview go to `.release/build/vX.Y.Z/` (ignored).

Just before publishing, the script checks both origins' destinations again and
records which branches and tags each one holds.

## What a real run publishes, in order

| Step | Effect |
|---|---|
| create local tag | annotated `vX.Y.Z` on `HEAD` |
| push main | `refs/heads/main:refs/heads/main` only |
| push tag | `refs/tags/vX.Y.Z:refs/tags/vX.Y.Z` only |
| download archive | GitHub's codeload `.tar.gz` for the tag, into `.release/archive.*/` (up to 3 tries; a download changes nothing) |
| verify archive | every entry under `devtrainer-X.Y.Z/`, has `go.mod`, no private paths; computes its sha256 |
| create GitHub release | `gh release create --verify-tag`, the archive attached |
| write formula | `Formula/devtrainer.rb` only; any other tap change stops the run |
| commit tap formula | commits only that file |
| push tap | `refs/heads/main:refs/heads/main` only, in the tap |

Every push sends exactly one ref:

- it re-checks the destination first;
- it runs `git -c push.followTags=false push --porcelain --no-follow-tags
  --recurse-submodules=no origin REF:REF`, so `push.followTags`, configured
  push refspecs, `push.default` and submodules cannot add other refs or tags;
- the `--porcelain` report must list only that ref, and afterwards the origin's
  branches and tags must be exactly the recorded set plus that ref.

Anything else is a boundary violation: the script prints **STOP**.

The hash comes from codeload's archive because that is the file Homebrew
downloads; a local `git archive` has different bytes.

The formula builds from source with Homebrew's Go (`depends_on "go" => :build`,
`std_go_args` with the version ldflag). There are no precompiled bottles. The
licence is declared as `:cannot_represent` because PolyForm Strict 1.0.0 is not
on the SPDX list; see `LICENSE`.

## When a step fails

Nothing is retried, undone or force-pushed. The script:

- appends each step to `.release/state/vX.Y.Z.log` (`done …`, `failed …`, the
  archive sha256);
- prints what is already published, the step that failed, and for each
  remaining step a **check** command and a **run** command;
- refuses to start again for that version while the state log exists.

Recover by hand:

1. Read the printed list. For the failed step, run its **check** first: a push
   or `gh release create` can fail after it took effect.
2. Run the remaining **run** commands in order, skipping any whose check shows
   it is done.
3. When everything is published, rename the state log (for example to
   `vX.Y.Z.recovered.log`) to keep it as a record.

**STOP** replaces the recovery list when continuing would be unsafe. No
release, formula or tap commands are printed. This happens when:

- any archive check fails: the file is not a readable `.tar.gz`, it has entries
  outside `devtrainer-X.Y.Z/`, it has no `go.mod`, it has private paths, or it
  cannot be hashed;
- a push sent, or an origin gained, refs this release did not ask for.

Do not create the release or the formula. Inspect what is public
(`git ls-tree -r --name-only vX.Y.Z`, `git ls-remote origin`) and decide by hand
how to handle the public repository.

## Testing the release tooling

```sh
bash -n scripts/release.sh scripts/release_test.sh
python3 -I -B -m unittest discover -s scripts -p 'test_*.py'
scripts/release_test.sh
```

`scripts/release_test.sh` runs every check and a full release against local
stand-in repositories with fake `gh`, `go`, `curl` and `ssh`. The remotes keep
their real `git@github.com:` URLs, so git resolves destinations as it would in
a real release. It never touches the network or the real remotes. Its temporary workspace path is printed so you can
inspect or delete it.
