#!/usr/bin/env bash
# Cut a devtrainer release from main: check everything, test and build, tag
# and push, publish the GitHub release with the exact source archive, and
# point the Homebrew tap formula at it. The version is injected at build time
# with -ldflags "-X main.version=X.Y.Z", so no version bump commit is needed.
#
# Usage: scripts/release.sh [--dry-run] [--notes FILE] X.Y.Z
#   --dry-run     run every check, test and build; publish nothing
#   --notes FILE  release notes (Markdown); default: GitHub-generated notes
# Env:
#   TAP_DIR                 kevnnard/homebrew-tap checkout (default .release/homebrew-tap)
#   RELEASE_DOWNLOAD_WAIT   seconds between archive download attempts (default 5)
#
# Exit codes: 0 done, 1 a check or step failed, 2 bad usage.
# See docs/RELEASING.md for prerequisites and failure recovery.
set -euo pipefail

REPO_SLUG="kevnnard/devtrainer"
TAP_SLUG="kevnnard/homebrew-tap"
FORMULA="Formula/devtrainer.rb"
USAGE="usage: scripts/release.sh [--dry-run] [--notes FILE] X.Y.Z"

# Paths that must never reach the public repository, at any commit.
PRIVATE_PATHS='^(odd|design|designs|drafts)/|(^|/)\.(agents|claude|codex|atl|pi|backup|codegraph|release)/|(^|/)(AGENTS|CLAUDE)\.md$|\.db(-wal|-shm)?$'

say() { printf '%s\n' "$*"; }
err() { printf 'release: error: %s\n' "$*" >&2; }
die() { err "$@"; exit 1; }
usage() { [[ $# -gt 0 ]] && err "$*"; printf '%s\n' "$USAGE" >&2; exit 2; }

# ---------- arguments ----------

DRY_RUN=0
NOTES=""
VERSION=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --notes) [[ $# -ge 2 ]] || usage "--notes needs a file"; NOTES="$2"; shift ;;
    --notes=*) NOTES="${1#--notes=}" ;;
    -h | --help) printf '%s\n' "$USAGE"; exit 0 ;;
    -*) usage "unknown option: $1" ;;
    *) [[ -z "$VERSION" ]] || usage "only one version may be given"; VERSION="$1" ;;
  esac
  shift
done
if [[ ! "$VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  usage "version must be numeric X.Y.Z without leading zeros or a v prefix"
fi

TAG="v$VERSION"
URL="https://codeload.github.com/$REPO_SLUG/tar.gz/refs/tags/$TAG"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd -P)"
TAP_DIR="${TAP_DIR:-$REPO_DIR/.release/homebrew-tap}"
BUILD_DIR="$REPO_DIR/.release/build/$TAG"
STATE_FILE="$REPO_DIR/.release/state/$TAG.log"
DOWNLOAD_WAIT="${RELEASE_DOWNLOAD_WAIT:-5}"
cd "$REPO_DIR"

if [[ -n "$NOTES" ]]; then
  [[ -f "$NOTES" && -s "$NOTES" ]] || die "release notes file not found or empty: $NOTES"
  NOTES="$(cd "$(dirname "$NOTES")" && pwd -P)/$(basename "$NOTES")"
  NOTES_ARGS=(--notes-file "$NOTES")
else
  NOTES_ARGS=(--generate-notes)
fi

# github_slug URL prints owner/name for a github.com remote URL, or nothing.
github_slug() {
  local u="$1"
  if [[ "$u" =~ ^(https://github\.com/|ssh://git@github\.com/|git@github\.com:)([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)$ ]]; then
    u="${BASH_REMATCH[2]}"
    printf '%s\n' "${u%.git}"
  fi
}

# check_origin DIR SLUG LABEL: origin's EFFECTIVE fetch and push URLs (after
# url.*.insteadOf, url.*.pushInsteadOf and remote.origin.pushurl, as git
# itself resolves them) are exactly one URL each, both naming SLUG, and
# origin is not a mirror. Called again before every push.
check_origin() {
  local dir="$1" slug="$2" label="$3" kind urls n
  [[ -n "$(git -C "$dir" config --get-all remote.origin.url || true)" ]] ||
    die "$label has no origin remote (want github.com/$slug)"
  [[ "$(git -C "$dir" config --bool --get remote.origin.mirror 2>/dev/null || true)" != true ]] ||
    die "$label is configured as a mirror (remote.origin.mirror); refusing to push"
  for kind in fetch push; do
    if [[ $kind == fetch ]]; then
      urls="$(git -C "$dir" remote get-url --all origin 2>/dev/null || true)"
    else
      urls="$(git -C "$dir" remote get-url --push --all origin 2>/dev/null || true)"
    fi
    n="$(printf '%s\n' "$urls" | grep -c . || true)"
    if [[ "$n" -ne 1 ]]; then
      [[ $kind == fetch ]] && die "$label must have exactly one URL (effective: $(printf '%s' "$urls" | tr '\n' ' '))"
      die "$label must have exactly one push URL (effective: $(printf '%s' "$urls" | tr '\n' ' '))"
    fi
    if [[ "$(github_slug "$urls")" != "$slug" ]]; then
      [[ $kind == fetch ]] && die "$label must be github.com/$slug (effective URL: $urls)"
      die "$label push URL must be github.com/$slug (effective push URL: $urls)"
    fi
  done
}

# push_ref DIR SLUG LABEL REF pushes exactly REF:REF to origin and nothing
# else: no followed tags, no submodules, no mirror. An explicit refspec
# already replaces remote.origin.push and push.default. Every ref line of
# the --porcelain report must be REF:REF; anything else is a boundary
# violation and stops the release.
push_ref() {
  local dir="$1" slug="$2" label="$3" ref="$4" out code=0 extra
  check_origin "$dir" "$slug" "$label"
  out="$(git -C "$dir" -c push.followTags=false -c push.recurseSubmodules=no -c remote.origin.mirror=false \
    push --porcelain --no-follow-tags --recurse-submodules=no origin "$ref:$ref" 2>&1)" || code=$?
  printf '%s\n' "$out"
  extra="$(printf '%s\n' "$out" | awk -F'\t' -v want="$ref:$ref" '/^[ +*!=-]\t/ && $2 != want { print $2 }')"
  if [[ -n "$extra" ]]; then
    HALT="git pushed refs this release did not ask for ($extra). Do not continue publishing;
inspect $label (git -C $dir ls-remote origin) and decide by hand what to do."
    die "push to $label sent unexpected refs: $extra"
  fi
  return "$code"
}

# remote_refset DIR prints origin's branch and tag names, sorted.
remote_refset() {
  git -C "$1" ls-remote origin 'refs/heads/*' 'refs/tags/*' | awk '$2 !~ /\^\{\}$/ { print $2 }' | LC_ALL=C sort -u
}

# expect_refset DIR LABEL EXPECTED: origin holds exactly the expected names.
expect_refset() {
  local now
  now="$(remote_refset "$1")" || die "cannot list the refs on $2 after pushing"
  if [[ "$now" != "$3" ]]; then
    HALT="$2 does not hold exactly the refs this release expected. Do not continue publishing;
compare 'git -C $1 ls-remote origin' with the state log and decide by hand what to do."
    die "$2 refs changed unexpectedly: $(LC_ALL=C comm -3 <(printf '%s\n' "$3") <(printf '%s\n' "$now") | tr -s ' \t\n' ' ')"
  fi
}

# with_ref LIST REF adds REF to a sorted newline list of ref names.
with_ref() { printf '%s\n%s\n' "$1" "$2" | grep . | LC_ALL=C sort -u; }

# remote_ref DIR REF prints the sha of REF on origin, nothing when absent,
# and fails on any other error: an unreachable remote is never "absent".
remote_ref() {
  local out code=0
  out="$(git -C "$1" ls-remote --exit-code origin "$2" 2>&1)" || code=$?
  case "$code" in
    0) printf '%s\n' "$out" | awk -v r="$2" '$2 == r { print $1 }' ;;
    2) ;;
    *) err "cannot query origin of $1 for $2 (git ls-remote exit $code):"; printf '%s\n' "$out" >&2; return 1 ;;
  esac
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# version_lt A B: numeric dotted comparison, A < B.
version_lt() {
  local IFS=. i a b
  read -r -a a <<<"$1"
  read -r -a b <<<"$2"
  for i in 0 1 2; do
    [[ "${a[i]:-0}" =~ ^[0-9]+$ ]] || return 0
    if ((10#${a[i]:-0} < 10#${b[i]:-0})); then return 0; fi
    if ((10#${a[i]:-0} > 10#${b[i]:-0})); then return 1; fi
  done
  return 1
}

# ---------- preflight: nothing below changes any repository ----------

say "==> [1/7] tools"
for tool in git gh go python3 curl tar; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required but not on PATH"
done
command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 || die "sha256sum or shasum is required"
python3 -I -c 'import sys; sys.exit(sys.version_info < (3, 8))' || die "python3 3.8 or newer is required"

say "==> [2/7] local repository"
[[ "$(git rev-parse --show-toplevel 2>/dev/null)" == "$REPO_DIR" ]] || die "$REPO_DIR is not the root of a git checkout"
branch="$(git symbolic-ref --quiet --short HEAD || echo "detached HEAD")"
[[ "$branch" == main ]] || die "release from main only (current: $branch)"
dirty="$(git status --porcelain=v1 --untracked-files=all)"
if [[ -n "$dirty" ]]; then
  err "working tree is not clean (commit, stash or ignore these first):"
  printf '%s\n' "$dirty" >&2
  exit 1
fi
git check-ignore -q "$BUILD_DIR" || die ".release/ must be ignored by git (scratch files live there)"
[[ ! -e "$STATE_FILE" ]] || die "a previous release attempt for $TAG left state in $STATE_FILE; finish its recovery (docs/RELEASING.md) before trying again"
check_origin "$REPO_DIR" "$REPO_SLUG" "origin"
! git rev-parse -q --verify "refs/tags/$TAG" >/dev/null || die "tag $TAG already exists locally"
HEAD_SHA="$(git rev-parse HEAD)"

say "==> [3/7] public history (every commit reachable from main)"
leaked="$(git rev-list HEAD | while read -r c; do git ls-tree -r --name-only "$c"; done | sort -u | grep -E "$PRIVATE_PATHS" || true)"
if [[ -n "$leaked" ]]; then
  err "history contains private paths; publish a clean history instead (see docs/RELEASING.md):"
  printf '%s\n' "$leaked" | sed 's/^/  /' >&2
  exit 1
fi

say "==> [4/7] GitHub"
gh auth status >/dev/null 2>&1 || die "gh is not authenticated (run: gh auth login)"
seen="$(gh repo view "$REPO_SLUG" --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true)"
[[ "$seen" == "$REPO_SLUG" ]] || die "cannot read GitHub repository $REPO_SLUG with gh"
remote_main="$(remote_ref "$REPO_DIR" refs/heads/main)" || die "cannot query origin; refusing to guess what is published"
remote_tag="$(remote_ref "$REPO_DIR" "refs/tags/$TAG")" || die "cannot query origin; refusing to guess what is published"
[[ -z "$remote_tag" ]] || die "tag $TAG already exists on origin"
if [[ -n "$remote_main" ]] && ! git merge-base --is-ancestor "$remote_main" HEAD 2>/dev/null; then
  die "origin/main is not an ancestor of HEAD ($remote_main); fetch and rebase your work first; this script never force-pushes"
fi
code=0
rel_err="$(gh api "repos/$REPO_SLUG/releases/tags/$TAG" 2>&1 >/dev/null)" || code=$?
if [[ $code -eq 0 ]]; then
  die "GitHub release $TAG already exists"
elif [[ "$rel_err" != *"HTTP 404"* ]]; then
  die "cannot check GitHub release $TAG: $rel_err"
fi

say "==> [5/7] tap checkout ($TAP_DIR)"
[[ -d "$TAP_DIR" ]] || die "tap checkout not found: $TAP_DIR (clone github.com/$TAP_SLUG there or set TAP_DIR)"
TAP_DIR="$(cd "$TAP_DIR" && pwd -P)"
[[ "$(git -C "$TAP_DIR" rev-parse --show-toplevel 2>/dev/null)" == "$TAP_DIR" ]] || die "$TAP_DIR is not the root of a git checkout"
check_origin "$TAP_DIR" "$TAP_SLUG" "tap origin"
[[ "$(git -C "$TAP_DIR" symbolic-ref --quiet --short HEAD || true)" == main ]] || die "tap checkout must be on main"
tap_dirty="$(git -C "$TAP_DIR" status --porcelain=v1 --untracked-files=all)"
if [[ -n "$tap_dirty" ]]; then
  err "tap checkout is not clean; the release only adds $FORMULA:"
  printf '%s\n' "$tap_dirty" >&2
  exit 1
fi
[[ -d "$TAP_DIR/Formula" ]] || die "tap checkout has no Formula/ directory"
tap_remote="$(remote_ref "$TAP_DIR" refs/heads/main)" || die "cannot query the tap origin"
[[ -n "$tap_remote" && "$tap_remote" == "$(git -C "$TAP_DIR" rev-parse HEAD)" ]] ||
  die "tap main is not up to date with origin (local $(git -C "$TAP_DIR" rev-parse --short HEAD), origin ${tap_remote:-none}); pull or push it first"

say "==> [6/7] tests and builds"
export GOTOOLCHAIN=local # like Homebrew: use the installed Go, never download one
want_go="$(awk '$1 == "go" { print $2; exit }' go.mod)"
have_go="$(go env GOVERSION)"
have_num="${have_go#go}"
have_num="${have_num%%[!0-9.]*}"
[[ -n "$want_go" ]] || die "go.mod has no go directive"
! version_lt "$have_num" "$want_go" || die "$have_go is older than go $want_go required by go.mod"
go test ./... -count=1 || die "go test failed"
go vet ./... || die "go vet failed"
mkdir -p "$BUILD_DIR"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X main.version=$VERSION" \
  -o "$BUILD_DIR/devtrainer-linux-amd64" ./cmd/devtrainer || die "linux/amd64 build (CGO_ENABLED=0) failed"
CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/devtrainer" ./cmd/devtrainer ||
  die "host build failed"
smoke="$(mktemp -d "$BUILD_DIR/smoke.XXXXXX")"
got="$(env HOME="$smoke" XDG_DATA_HOME="$smoke/data" XDG_CONFIG_HOME="$smoke/config" "$BUILD_DIR/devtrainer" --version)" ||
  die "built binary failed to print its version"
[[ "$got" == "devtrainer $VERSION" ]] || die "built binary reports '$got', want 'devtrainer $VERSION'"
env HOME="$smoke" XDG_DATA_HOME="$smoke/data" XDG_CONFIG_HOME="$smoke/config" "$BUILD_DIR/devtrainer" validate >/dev/null ||
  die "built binary failed 'devtrainer validate'"
[[ ! -e "$smoke/data/devtrainer/devtrainer.db" ]] || die "'devtrainer validate' created a database; it must not"
python3 -I -B scripts/release_tap.py --version "$VERSION" --sha256 "$(printf '0%.0s' {1..64})" --stdout >"$BUILD_DIR/formula-preview.rb" ||
  die "formula renderer failed"

if [[ $DRY_RUN -eq 1 ]]; then
  say "==> [7/7] publish plan (dry run: nothing below is executed)"
  say "[dry-run] git tag -a $TAG -m \"devtrainer $TAG\" $HEAD_SHA"
  say "[dry-run] git push origin refs/heads/main:refs/heads/main   (origin/main now: ${remote_main:-absent})"
  say "[dry-run] git push origin refs/tags/$TAG:refs/tags/$TAG"
  say "[dry-run]   each push: --porcelain --no-follow-tags --recurse-submodules=no, only that ref"
  say "[dry-run] curl -fsSL --proto =https -o .release/archive.*/devtrainer-$VERSION.tar.gz $URL"
  say "[dry-run] gh release create $TAG --repo $REPO_SLUG --verify-tag --title \"devtrainer $TAG\" ${NOTES_ARGS[*]} <archive>"
  say "[dry-run] write $TAP_DIR/$FORMULA, then commit only that file and push the tap main"
  say "---- formula preview (sha256 is a placeholder) ----"
  cat "$BUILD_DIR/formula-preview.rb"
  say "----"
  say "dry run passed for $TAG at $HEAD_SHA"
  exit 0
fi

# ---------- publish: every step is recorded; nothing is retried or undone ----------

STEP=""
DONE_STEPS=()
ARCHIVE=""
SHA=""
HALT=""
state() { printf '%s %s\n' "$1" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$STATE_FILE"; }
begin() { STEP="$1"; say "==> $1"; }
finish() { DONE_STEPS+=("$STEP"); printf 'done %s %s\n' "$STEP" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$STATE_FILE"; STEP=""; }

recovery() {
  local s
  {
    say ""
    say "Release $TAG stopped at: ${STEP:-unknown step}"
    say "State log: $STATE_FILE"
    say "Already done:"
    if [[ ${#DONE_STEPS[@]} -eq 0 ]]; then say "  (nothing)"; else for s in "${DONE_STEPS[@]}"; do say "  - $s"; done; fi
    case " ${DONE_STEPS[*]-} " in
      *"push tag $TAG"*) say "Note: tag $TAG and main are already on origin; this script never deletes or moves them." ;;
    esac
    if [[ -n "$HALT" ]]; then
      say ""
      say "STOP: $HALT"
      return
    fi
    say ""
    say "Safe recovery: run these by hand, in order, from $REPO_DIR. First check whether"
    say "the failed step took effect anyway; skip any step whose check shows it is done."
    say "Never force-push and never move a published tag."
    local started=0
    for s in "create local tag $TAG" "push main" "push tag $TAG" "download archive" "verify archive" \
      "create GitHub release $TAG" "write formula" "commit tap formula" "push tap"; do
      [[ "$s" == "$STEP" ]] && started=1
      [[ $started -eq 1 ]] || continue
      case "$s" in
        "create local tag $TAG")
          say "  check: git rev-parse -q --verify refs/tags/$TAG"
          say "  run:   git tag -a $TAG -m \"devtrainer $TAG\" $HEAD_SHA" ;;
        "push main")
          say "  check: git ls-remote origin refs/heads/main   (want $HEAD_SHA)"
          say "  run:   git -c push.followTags=false push --no-follow-tags --recurse-submodules=no origin refs/heads/main:refs/heads/main" ;;
        "push tag $TAG")
          say "  check: git ls-remote origin refs/tags/$TAG"
          say "  run:   git -c push.followTags=false push --no-follow-tags --recurse-submodules=no origin refs/tags/$TAG:refs/tags/$TAG" ;;
        "download archive")
          say "  run:   curl -fsSL --proto =https -o ${ARCHIVE:-<archive>} $URL" ;;
        "verify archive")
          say "  check: tar -tzf ${ARCHIVE:-<archive>}   (every path under devtrainer-$VERSION/, none private)"
          say "  run:   sha256sum ${ARCHIVE:-<archive>}   (or: shasum -a 256)" ;;
        "create GitHub release $TAG")
          say "  check: gh release view $TAG --repo $REPO_SLUG"
          say "  run:   gh release create $TAG --repo $REPO_SLUG --verify-tag --title \"devtrainer $TAG\" ${NOTES_ARGS[*]} ${ARCHIVE:-<archive>}"
          say "         (if the release exists without the asset: gh release upload $TAG ${ARCHIVE:-<archive>} --repo $REPO_SLUG)" ;;
        "write formula")
          say "  run:   python3 -I -B scripts/release_tap.py --version $VERSION --sha256 ${SHA:-<sha256 of the archive>} --tap $TAP_DIR" ;;
        "commit tap formula")
          say "  check: git -C $TAP_DIR status --short   (only $FORMULA may change)"
          say "  run:   git -C $TAP_DIR add -- $FORMULA && git -C $TAP_DIR commit -m \"devtrainer $TAG\" -- $FORMULA" ;;
        "push tap")
          say "  check: git -C $TAP_DIR ls-remote origin refs/heads/main"
          say "  run:   git -C $TAP_DIR -c push.followTags=false push --no-follow-tags --recurse-submodules=no origin refs/heads/main:refs/heads/main" ;;
      esac
    done
    say ""
    say "When everything is published, keep $STATE_FILE as a record by renaming it"
    say "(for example to $TAG.recovered.log); the script refuses to start while it exists."
  } >&2
}

on_exit() {
  local code=$?
  if [[ $code -ne 0 && -n "$STEP" ]]; then
    printf 'failed %s %s\n' "$STEP" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$STATE_FILE"
    recovery
  fi
}
trap on_exit EXIT

# The tests and builds took a while: check the destinations again and record
# which branches and tags both origins hold before changing anything.
check_origin "$REPO_DIR" "$REPO_SLUG" "origin"
check_origin "$TAP_DIR" "$TAP_SLUG" "tap origin"
EXPECT_REPO="$(remote_refset "$REPO_DIR")" || die "cannot list the refs on origin"
EXPECT_TAP="$(remote_refset "$TAP_DIR")" || die "cannot list the refs on the tap origin"

mkdir -p "$(dirname "$STATE_FILE")"
state "start $TAG head=$HEAD_SHA tap=$TAP_DIR"

begin "create local tag $TAG"
git tag -a "$TAG" -m "devtrainer $TAG" "$HEAD_SHA" || die "git tag failed"
finish

begin "push main"
push_ref "$REPO_DIR" "$REPO_SLUG" "origin" refs/heads/main || die "pushing main failed; it may or may not have reached origin"
EXPECT_REPO="$(with_ref "$EXPECT_REPO" refs/heads/main)"
expect_refset "$REPO_DIR" "origin" "$EXPECT_REPO"
finish

begin "push tag $TAG"
push_ref "$REPO_DIR" "$REPO_SLUG" "origin" "refs/tags/$TAG" || die "pushing $TAG failed; it may or may not have reached origin"
EXPECT_REPO="$(with_ref "$EXPECT_REPO" "refs/tags/$TAG")"
expect_refset "$REPO_DIR" "origin" "$EXPECT_REPO"
finish

begin "download archive"
# Hash the bytes Homebrew will download: codeload's archive for the tag. A
# local git archive differs byte for byte, so its hash would not match.
ARCHIVE="$(mktemp -d "$REPO_DIR/.release/archive.XXXXXX")/devtrainer-$VERSION.tar.gz"
ok=0
for attempt in 1 2 3; do # a GET changes nothing, so retrying it is safe
  if curl -fsSL --proto =https --max-time 120 -o "$ARCHIVE" "$URL"; then ok=1; break; fi
  err "archive download attempt $attempt failed"
  if [[ $attempt -lt 3 ]]; then sleep "$DOWNLOAD_WAIT"; fi
done
[[ $ok -eq 1 && -s "$ARCHIVE" ]] || die "could not download $URL"
finish

begin "verify archive"
# Any failed check below means the published archive cannot be trusted: stop
# the whole chain instead of offering release or formula commands.
HALT="the archive GitHub serves for $TAG failed an integrity check. Do not continue publishing:
no release or formula may point at it. Inspect the public tag (git ls-tree -r --name-only $TAG)
and the downloaded file, and decide by hand how to handle the public repository."
listing="$(tar -tzf "$ARCHIVE")" || die "archive is not a readable tar.gz: $ARCHIVE"
prefix="devtrainer-$VERSION/"
outside="$(printf '%s\n' "$listing" | grep -v "^$prefix" || true)"
[[ -z "$outside" ]] || die "archive has entries outside $prefix: $(printf '%s' "$outside" | head -n 3 | tr '\n' ' ')"
printf '%s\n' "$listing" | grep -qx "${prefix}go.mod" || die "archive has no ${prefix}go.mod"
private="$(printf '%s\n' "$listing" | sed "s|^$prefix||" | grep -E "$PRIVATE_PATHS" || true)"
if [[ -n "$private" ]]; then
  HALT="the public tag archive exposes private paths. Do not continue publishing: no release
or formula may point at it. Inspect what is public (git ls-tree -r --name-only $TAG) and decide
by hand how to handle the public repository."
  die "archive contains private paths: $(printf '%s' "$private" | head -n 5 | tr '\n' ' ')"
fi
SHA="$(sha256_of "$ARCHIVE")"
[[ "$SHA" =~ ^[0-9a-f]{64}$ ]] || die "could not hash $ARCHIVE"
say "sha256 $SHA  $ARCHIVE"
state "sha256 $SHA"
HALT=""
finish

begin "create GitHub release $TAG"
gh release create "$TAG" --repo "$REPO_SLUG" --verify-tag --title "devtrainer $TAG" "${NOTES_ARGS[@]}" "$ARCHIVE" ||
  die "gh release create failed; the release may exist partially"
finish

begin "write formula"
python3 -I -B scripts/release_tap.py --version "$VERSION" --sha256 "$SHA" --tap "$TAP_DIR" || die "formula renderer failed"
changed="$(git -C "$TAP_DIR" status --porcelain=v1 --untracked-files=all)"
case "$changed" in
  " M $FORMULA" | "?? $FORMULA") ;;
  *) die "unexpected tap changes after writing the formula:"$'\n'"$changed" ;;
esac
finish

begin "commit tap formula"
git -C "$TAP_DIR" add -- "$FORMULA" || die "git add failed in the tap"
staged="$(git -C "$TAP_DIR" diff --cached --name-only)"
[[ "$staged" == "$FORMULA" ]] || die "tap index holds more than $FORMULA: $staged"
git -C "$TAP_DIR" commit -q -m "devtrainer $TAG" -- "$FORMULA" || die "tap commit failed"
finish

begin "push tap"
push_ref "$TAP_DIR" "$TAP_SLUG" "tap origin" refs/heads/main || die "pushing the tap failed; it may or may not have reached origin"
expect_refset "$TAP_DIR" "tap origin" "$(with_ref "$EXPECT_TAP" refs/heads/main)"
finish

STEP="release complete"
finish
trap - EXIT
say "==> done: $TAG"
say "   brew install kevnnard/tap/devtrainer   (or: brew update && brew upgrade devtrainer)"
