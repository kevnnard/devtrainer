#!/usr/bin/env bash
# Hermetic tests for scripts/release.sh. Every scenario runs in its own
# temporary directory with local bare repositories standing in for GitHub
# and fake gh, go and curl commands on PATH. Remotes keep their real
# git@github.com: URLs; a fake ssh (GIT_SSH_COMMAND) serves them from the
# local bare repositories, so git's effective URLs are the genuine ones. Nothing touches the network, the real remotes or your git config.
#
# Usage: scripts/release_test.sh   (needs bash, git, python3, tar, gzip)
# The workspace is created under ${TMPDIR:-/tmp} and left for inspection;
# the path is printed first and last.
set -uo pipefail

SCRIPTS="$(cd "$(dirname "$0")" && pwd -P)"
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/devtrainer-release-test.XXXXXX")"
TMP_ROOT="$(cd "$TMP_ROOT" && pwd -P)"
PASS=0
FAIL=0
echo "test workspace: $TMP_ROOT"

# ---------- fixture ----------

write_shims() { # dir
  local bin="$1/bin"
  mkdir -p "$bin"
  cat >"$bin/gh" <<'EOF'
#!/usr/bin/env bash
echo "gh $*" >>"$MOCK_LOG"
case "$1 $2" in
  "auth status") exit "${MOCK_GH_AUTH:-0}" ;;
  "repo view")
    [[ "${MOCK_GH_REPO:-ok}" == ok ]] || { echo "GraphQL: Could not resolve to a Repository" >&2; exit 1; }
    echo "kevnnard/devtrainer"; exit 0 ;;
  "api repos/kevnnard/devtrainer/releases/tags/"*)
    case "${MOCK_GH_RELEASE:-absent}" in
      absent) echo "gh: Not Found (HTTP 404)" >&2; exit 1 ;;
      present) echo '{"tag_name":"x"}'; exit 0 ;;
      *) echo "error connecting to api.github.com" >&2; exit 1 ;;
    esac ;;
  "release create") exit "${MOCK_GH_CREATE:-0}" ;;
esac
echo "unexpected gh call: $*" >&2
exit 99
EOF
  cat >"$bin/go" <<'EOF'
#!/usr/bin/env bash
echo "go $*" >>"$MOCK_LOG"
case "$1" in
  env) [[ "$2" == GOVERSION ]] && { echo "${MOCK_GO_VERSION:-go1.27.4}"; exit 0; } ;;
  test)
    # MOCK_GO_TEST_HOOK simulates something changing while the tests run.
    if [[ -n "${MOCK_GO_TEST_HOOK:-}" ]]; then eval "$MOCK_GO_TEST_HOOK"; fi
    exit "${MOCK_GO_TEST:-0}" ;;
  vet) exit 0 ;;
  build)
    out="" ver="dev"
    while [[ $# -gt 0 ]]; do
      case "$1" in
        -o) out="$2"; shift ;;
        -ldflags) ver="${2##*-X main.version=}"; ver="${ver%% *}"; shift ;;
      esac
      shift
    done
    [[ -n "$out" ]] || exit 3
    [[ "$out" == /dev/null ]] && exit 0
    printf '#!/usr/bin/env bash\ncase "$1" in --version) echo "devtrainer %s" ;; validate) echo "Content OK: 1 track" ;; *) exit 1 ;; esac\n' "$ver" >"$out"
    chmod +x "$out"; exit 0 ;;
esac
echo "unexpected go call: $*" >&2
exit 99
EOF
  cat >"$bin/curl" <<'EOF'
#!/usr/bin/env bash
echo "curl $*" >>"$MOCK_LOG"
out="" url=""
while [[ $# -gt 0 ]]; do
  case "$1" in -o) out="$2"; shift ;; https://*) url="$1" ;; esac
  shift
done
[[ "${MOCK_CURL:-ok}" == ok ]] || exit 22
[[ "$url" == "https://codeload.github.com/kevnnard/devtrainer/tar.gz/refs/tags/v"* ]] || exit 6
cp "$MOCK_ARCHIVE" "$out"
EOF
  cat >"$bin/fake-ssh" <<'EOF'
#!/usr/bin/env bash
# GIT_SSH_VARIANT=simple: args are <host> <command>.
host="$1" cmd="$2"
echo "ssh $host $cmd" >>"$MOCK_LOG"
[[ "$host" == git@github.com ]] || { echo "fake-ssh: unknown host $host" >&2; exit 255; }
prog="${cmd%% *}" path="${cmd#* }"
path="${path//\'/}"
path="${path#/}"
case "$prog $path" in
  "git-upload-pack kevnnard/"*.git | "git-receive-pack kevnnard/"*.git) ;;
  *) echo "fake-ssh: refused $cmd" >&2; exit 128 ;;
esac
exec git "${prog#git-}" "$MOCK_REMOTES/${path#kevnnard/}"
EOF
  chmod +x "$bin/gh" "$bin/go" "$bin/curl" "$bin/fake-ssh"
}

# new_fixture: sets T, REPO, TAP, REMOTE, TAP_REMOTE and the environment.
new_fixture() {
  T="$(mktemp -d "$TMP_ROOT/case.XXXXXX")"
  REPO="$T/repo"
  REMOTE="$T/remotes/devtrainer.git"
  TAP_REMOTE="$T/remotes/homebrew-tap.git"
  TAP="$REPO/.release/homebrew-tap"
  export HOME="$T/home" GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$T/gitconfig"
  export MOCK_LOG="$T/calls.log" MOCK_ARCHIVE="$T/archive.tar.gz"
  export PATH="$T/bin:$BASE_PATH" RELEASE_DOWNLOAD_WAIT=0
  export MOCK_REMOTES="$T/remotes" GIT_SSH_COMMAND="$T/bin/fake-ssh" GIT_SSH_VARIANT=simple
  unset MOCK_GO_TEST_HOOK MOCK_GH_AUTH MOCK_GH_REPO MOCK_GH_RELEASE MOCK_GH_CREATE MOCK_GO_VERSION MOCK_GO_TEST MOCK_CURL TAP_DIR
  mkdir -p "$HOME" "$T/remotes"
  : >"$MOCK_LOG"
  git config --global user.name "Release Test"
  git config --global user.email "release-test@example.invalid"
  git config --global init.defaultBranch main
  git config --global commit.gpgsign false
  git config --global tag.gpgsign false
  write_shims "$T"

  git init -q --bare "$REMOTE"
  git init -q --bare "$TAP_REMOTE"

  local seed="$T/tap-seed"
  git init -q "$seed"
  mkdir -p "$seed/Formula"
  printf 'class Watts < Formula\nend\n' >"$seed/Formula/watts.rb"
  printf '# tap\n' >"$seed/README.md"
  git -C "$seed" add -A && git -C "$seed" commit -qm "seed tap"
  git -C "$seed" push -q "$TAP_REMOTE" main

  git init -q "$REPO"
  mkdir -p "$REPO/scripts" "$REPO/cmd/devtrainer"
  cp "$SCRIPTS/release.sh" "$SCRIPTS/release_tap.py" "$REPO/scripts/" 2>/dev/null
  printf 'module github.com/kevnnard/devtrainer\n\ngo 1.27.3\n' >"$REPO/go.mod"
  printf 'package main\n\nfunc main() {}\n' >"$REPO/cmd/devtrainer/main.go"
  printf '/.release/\n*.db\n' >"$REPO/.gitignore"
  git -C "$REPO" add -A && git -C "$REPO" commit -qm "public snapshot"
  git -C "$REPO" remote add origin git@github.com:kevnnard/devtrainer.git
  git clone -q git@github.com:kevnnard/homebrew-tap.git "$TAP"
  build_archive
}

build_archive() { # the bytes the fake codeload returns
  git -C "$REPO" archive --format=tar --prefix=devtrainer-0.1.0/ HEAD | gzip -n >"$MOCK_ARCHIVE"
}

release() { # args...; sets OUT, ERR, CODE
  CODE=0
  (cd "$REPO" && bash scripts/release.sh "$@") >"$T/out" 2>"$T/err" || CODE=$?
  OUT="$(cat "$T/out")"
  ERR="$(cat "$T/err")"
}

# ---------- assertions ----------

CASE=""
case_ok=1
check() { # description, command...
  if ! "${@:2}"; then
    case_ok=0
    echo "    FAIL: $1"
  fi
}
begin() { CASE="$1"; case_ok=1; new_fixture; }
end() {
  if [[ $case_ok -eq 1 ]]; then
    PASS=$((PASS + 1)); echo "ok   $CASE"
  else
    FAIL=$((FAIL + 1)); echo "FAIL $CASE"
    echo "    --- exit $CODE; stdout tail:"; tail -n 8 "$T/out" | sed 's/^/    | /'
    echo "    --- stderr:"; sed 's/^/    | /' "$T/err" | tail -n 12
  fi
}
code_is() { [[ "$CODE" == "$1" ]]; }
err_has() { grep -qF -- "$1" "$T/err"; }
out_has() { grep -qF -- "$1" "$T/out"; }
log_has() { grep -qF -- "$1" "$MOCK_LOG"; }
log_lacks() { ! grep -qF -- "$1" "$MOCK_LOG"; }
no_local_tag() { ! git -C "$REPO" rev-parse -q --verify "refs/tags/$1" >/dev/null; }
remote_empty() { [[ -z "$(git --git-dir="$REMOTE" for-each-ref)" ]]; }
tap_remote_unchanged() { [[ "$(git --git-dir="$TAP_REMOTE" rev-list --count main)" == 1 ]]; }
no_formula() { [[ ! -e "$TAP/Formula/devtrainer.rb" ]]; }
no_state() { [[ ! -e "$REPO/.release/state" ]]; }
nothing_published() { no_local_tag v0.1.0 && remote_empty && tap_remote_unchanged && no_formula && log_lacks "gh release create"; }

expect_preflight_failure() { # message
  check "exit 1" code_is 1
  check "stderr explains: $1" err_has "$1"
  check "nothing published" nothing_published
  check "no state file" no_state
}

BASE_PATH="$PATH"

# ---------- argument validation ----------

begin "rejects invalid versions with usage (exit 2)"
for v in "" 1.2 v0.1.0 01.2.3 1.02.3 1.2.3-rc1 1.2.3.4 " 0.1.0" "0.1.0 " a.b.c; do
  release --dry-run "$v"
  check "version '$v' exit 2" code_is 2
  check "version '$v' usage" err_has "usage: scripts/release.sh [--dry-run] [--notes FILE] X.Y.Z"
done
release --dry-run 0.1.0 0.2.0
check "extra argument exit 2" code_is 2
release --bogus 0.1.0
check "unknown option exit 2" code_is 2
check "nothing published" nothing_published
end

begin "rejects a missing or empty notes file"
release --dry-run --notes "$T/nope.md" 0.1.0
expect_preflight_failure "release notes file not found or empty"
: >"$T/empty.md"
release --dry-run --notes "$T/empty.md" 0.1.0
expect_preflight_failure "release notes file not found or empty"
end

# ---------- local repository ----------

begin "refuses to release from a branch other than main"
git -C "$REPO" switch -q -c topic
release --dry-run 0.1.0
expect_preflight_failure "release from main only (current: topic)"
end

begin "refuses tracked changes"
echo "// edit" >>"$REPO/cmd/devtrainer/main.go"
release --dry-run 0.1.0
expect_preflight_failure "working tree is not clean"
end

begin "refuses non-ignored untracked files but allows ignored ones"
echo scratch >"$REPO/notes.txt"
release --dry-run 0.1.0
expect_preflight_failure "working tree is not clean"
check "names the file" err_has "notes.txt"
mv "$REPO/notes.txt" "$REPO/progress.db"
release --dry-run 0.1.0
check "ignored file is fine (exit 0)" code_is 0
end

begin "refuses an origin that is not kevnnard/devtrainer"
git -C "$REPO" remote set-url origin https://github.com/someone/devtrainer.git
release --dry-run 0.1.0
expect_preflight_failure "origin must be github.com/kevnnard/devtrainer"
end

begin "refuses a push URL that points elsewhere"
git -C "$REPO" remote set-url --push origin https://github.com/kevnnard/other.git
release --dry-run 0.1.0
expect_preflight_failure "origin push URL must be github.com/kevnnard/devtrainer"
end

begin "refuses an existing local tag"
git -C "$REPO" tag -a v0.1.0 -m x
release --dry-run 0.1.0
check "exit 1" code_is 1
check "explains" err_has "tag v0.1.0 already exists locally"
check "remote untouched" remote_empty
end

begin "refuses history that ever contained agent files"
mkdir -p "$REPO/odd/tasks"
echo plan >"$REPO/odd/tasks/plan.md"
git -C "$REPO" add -f odd && git -C "$REPO" commit -qm "add plan"
git -C "$REPO" rm -rq --cached odd && git -C "$REPO" commit -qm "drop plan"
mv "$REPO/odd" "$REPO/.release/odd-moved"
release --dry-run 0.1.0
expect_preflight_failure "history contains private paths"
check "names the path" err_has "odd/tasks/plan.md"
end

begin "refuses agent files at any depth or root markers"
for p in AGENTS.md CLAUDE.md .claude/settings.json design/x.html .pi/state drafts/git.yaml; do
  new_fixture
  mkdir -p "$REPO/$(dirname "$p")"
  echo x >"$REPO/$p"
  git -C "$REPO" add -f "$p" && git -C "$REPO" commit -qm "add $p"
  release --dry-run 0.1.0
  check "$p rejected" err_has "history contains private paths"
  check "$p exit 1" code_is 1
done
end

# ---------- tools and GitHub ----------

begin "refuses an unauthenticated gh"
export MOCK_GH_AUTH=1
release --dry-run 0.1.0
expect_preflight_failure "gh is not authenticated"
end

begin "refuses when the GitHub repository is not reachable"
export MOCK_GH_REPO=missing
release --dry-run 0.1.0
expect_preflight_failure "cannot read GitHub repository kevnnard/devtrainer"
end

begin "refuses a Go older than go.mod requires"
export MOCK_GO_VERSION=go1.27.2
release --dry-run 0.1.0
expect_preflight_failure "go1.27.2 is older than go 1.27.3 required by go.mod"
end

begin "refuses an existing remote tag"
git -C "$REPO" push -q origin main
git -C "$REPO" tag -a v0.1.0 -m x && git -C "$REPO" push -q origin v0.1.0 && git -C "$REPO" tag -d v0.1.0 >/dev/null
release --dry-run 0.1.0
check "exit 1" code_is 1
check "explains" err_has "tag v0.1.0 already exists on origin"
end

begin "treats an unreachable origin as an error, not as absence"
git --git-dir="$REMOTE" config core.bare true
mv "$REMOTE" "$T/remotes/moved.git"
release --dry-run 0.1.0
check "exit 1" code_is 1
check "explains" err_has "cannot query origin"
check "no gh release create" log_lacks "gh release create"
end

begin "refuses a remote main that is not an ancestor of HEAD"
other="$T/other"
git init -q "$other" && echo x >"$other/x" && git -C "$other" add x && git -C "$other" commit -qm other
git -C "$other" push -q "$REMOTE" main
git -C "$REPO" fetch -q origin
release --dry-run 0.1.0
check "exit 1" code_is 1
check "explains" err_has "origin/main is not an ancestor of HEAD"
end

begin "refuses an existing GitHub release"
export MOCK_GH_RELEASE=present
release --dry-run 0.1.0
expect_preflight_failure "GitHub release v0.1.0 already exists"
end

begin "treats a GitHub API error as an error, not as absence"
export MOCK_GH_RELEASE=error
release --dry-run 0.1.0
expect_preflight_failure "cannot check GitHub release v0.1.0"
end

begin "stops when tests fail"
export MOCK_GO_TEST=1
release --dry-run 0.1.0
expect_preflight_failure "go test failed"
end

# ---------- tap checkout ----------

begin "refuses a missing tap checkout"
export TAP_DIR="$T/no-tap"
release --dry-run 0.1.0
expect_preflight_failure "tap checkout not found: $T/no-tap"
end

begin "refuses a tap whose origin is not kevnnard/homebrew-tap"
git -C "$TAP" remote set-url origin https://github.com/someone/homebrew-tap.git
release --dry-run 0.1.0
expect_preflight_failure "tap origin must be github.com/kevnnard/homebrew-tap"
end

begin "refuses a tap with unrelated changes"
echo "# local" >>"$TAP/README.md"
release --dry-run 0.1.0
expect_preflight_failure "tap checkout is not clean"
end

begin "refuses a tap with untracked files"
echo x >"$TAP/Formula/other.rb"
release --dry-run 0.1.0
expect_preflight_failure "tap checkout is not clean"
end

begin "refuses a tap that is behind its origin"
seed="$T/tap-seed"
echo more >>"$seed/README.md" && git -C "$seed" commit -qam more && git -C "$seed" push -q "$TAP_REMOTE" main
release --dry-run 0.1.0
check "exit 1" code_is 1
check "explains" err_has "tap main is not up to date with origin"
check "local repo untagged" no_local_tag v0.1.0
check "origin untouched" remote_empty
check "no release" log_lacks "gh release create"
check "tap origin keeps its 2 commits" [ "$(git --git-dir="$TAP_REMOTE" rev-list --count main)" == 2 ]
end

begin "refuses a tap with unpushed commits"
echo more >>"$TAP/README.md" && git -C "$TAP" commit -qam local
release --dry-run 0.1.0
check "exit 1" code_is 1
check "explains" err_has "tap main is not up to date with origin"
end

# ---------- dry run ----------

begin "dry run checks everything and changes nothing"
before_head="$(git -C "$REPO" rev-parse HEAD)"
release --dry-run 0.1.0
check "exit 0" code_is 0
check "nothing published" nothing_published
check "no state file" no_state
check "HEAD unchanged" [ "$(git -C "$REPO" rev-parse HEAD)" == "$before_head" ]
check "tests ran" log_has "go test ./... -count=1"
check "vet ran" log_has "go vet ./..."
check "linux/amd64 build ran" log_has "go build -trimpath -ldflags -X main.version=0.1.0 -o"
check "no download" log_lacks "curl"
check "prints formula preview" out_has 'url "https://codeload.github.com/kevnnard/devtrainer/tar.gz/refs/tags/v0.1.0"'
check "prints planned push" out_has "[dry-run] git push origin refs/heads/main:refs/heads/main"
check "prints planned release" out_has "[dry-run] gh release create v0.1.0"
check "generated notes by default" out_has "--generate-notes"
check "says dry run passed" out_has "dry run passed"
check "repo still clean" [ -z "$(git -C "$REPO" status --porcelain)" ]
check "tap still clean" [ -z "$(git -C "$TAP" status --porcelain)" ]
end

# ---------- real run against local stand-ins ----------

begin "release publishes tag, main, release asset and only the formula"
printf 'First release.\n' >"$T/notes.md"
release --notes "$T/notes.md" 0.1.0
check "exit 0" code_is 0
head="$(git -C "$REPO" rev-parse HEAD)"
check "remote main is HEAD" [ "$(git --git-dir="$REMOTE" rev-parse main)" == "$head" ]
check "remote tag points at HEAD" [ "$(git --git-dir="$REMOTE" rev-parse 'v0.1.0^{commit}')" == "$head" ]
check "remote has only main and the tag" [ "$(git --git-dir="$REMOTE" for-each-ref --format='%(refname)' | tr '\n' ' ')" == "refs/heads/main refs/tags/v0.1.0 " ]
check "tag is annotated" [ "$(git --git-dir="$REMOTE" cat-file -t v0.1.0)" == tag ]
sha="$( (sha256sum "$MOCK_ARCHIVE" 2>/dev/null || shasum -a 256 "$MOCK_ARCHIVE") | cut -d' ' -f1)"
check "release created with verify-tag, asset and notes file" \
  log_has "gh release create v0.1.0 --repo kevnnard/devtrainer --verify-tag --title devtrainer v0.1.0 --notes-file $T/notes.md"
check "release asset is the archive" grep -qE "gh release create .*devtrainer-0\.1\.0\.tar\.gz" "$MOCK_LOG"
check "tap got one commit" [ "$(git --git-dir="$TAP_REMOTE" rev-list --count main)" == 2 ]
check "tap commit touches only the formula" \
  [ "$(git --git-dir="$TAP_REMOTE" diff --name-only main~1 main)" == "Formula/devtrainer.rb" ]
check "formula carries the archive sha" grep -qF "sha256 \"$sha\"" <(git --git-dir="$TAP_REMOTE" show main:Formula/devtrainer.rb)
check "unrelated tap files intact" [ "$(git --git-dir="$TAP_REMOTE" show main:Formula/watts.rb)" == "$(printf 'class Watts < Formula\nend')" ]
check "state log records completion" grep -q "^done release complete" "$REPO/.release/state/v0.1.0.log"
check "prints install command" out_has "brew install kevnnard/tap/devtrainer"
check "repo clean afterwards" [ -z "$(git -C "$REPO" status --porcelain)" ]
end

begin "release stops on an archive with private paths before creating the release"
git -C "$REPO" archive --format=tar --prefix=devtrainer-0.1.0/ HEAD >"$T/a.tar"
mkdir -p "$T/x/devtrainer-0.1.0/.claude" && echo x >"$T/x/devtrainer-0.1.0/.claude/s.json"
tar -rf "$T/a.tar" -C "$T/x" devtrainer-0.1.0/.claude/s.json && gzip -nc "$T/a.tar" >"$MOCK_ARCHIVE"
release 0.1.0
check "exit 1" code_is 1
check "explains" err_has "archive contains private paths"
check "no release created" log_lacks "gh release create"
check "tap untouched" tap_remote_unchanged
check "recovery printed" err_has "tag v0.1.0 and main are already on origin"
check "says to stop instead of continuing" err_has "Do not continue publishing"
check "offers no publish commands" bash -c "! grep -qF 'run:   gh release create' '$T/err'"
end

begin "a failed release step records partial state and refuses to replay"
export MOCK_GH_CREATE=1
release 0.1.0
check "exit 1" code_is 1
check "tag was pushed" [ -n "$(git --git-dir="$REMOTE" rev-parse -q --verify v0.1.0)" ]
check "tap untouched" tap_remote_unchanged
check "no formula written" no_formula
check "state lists pushed tag" grep -q "^done push tag v0.1.0" "$REPO/.release/state/v0.1.0.log"
check "state lists failed step" grep -q "^failed create GitHub release v0.1.0" "$REPO/.release/state/v0.1.0.log"
check "recovery explains how to check" err_has "gh release view v0.1.0 --repo kevnnard/devtrainer"
check "recovery never forces" bash -c "! grep -qiE -- '--force|push -f|reset --hard' '$T/err'"
unset MOCK_GH_CREATE
: >"$MOCK_LOG"
release 0.1.0
check "rerun refuses (exit 1)" code_is 1
check "rerun points at state" err_has "a previous release attempt for v0.1.0 left state"
check "rerun calls no gh mutation" log_lacks "gh release create"
end

begin "a failed download records state and does not retry mutations"
export MOCK_CURL=fail
release 0.1.0
check "exit 1" code_is 1
check "download attempted a bounded number of times" [ "$(grep -c '^curl' "$MOCK_LOG")" -le 3 ]
check "no release created" log_lacks "gh release create"
check "tag stays published" [ -n "$(git --git-dir="$REMOTE" rev-parse -q --verify v0.1.0)" ]
check "recovery gives the manual download" err_has "curl -fsSL --proto =https"
check "state records failure" grep -q "^failed download" "$REPO/.release/state/v0.1.0.log"
end

# ---------- effective destinations ----------

begin "refuses an origin that url.insteadOf rewrites elsewhere"
git config --global url."file://$T/remotes/".insteadOf "git@github.com:kevnnard/"
release --dry-run 0.1.0
expect_preflight_failure "origin must be github.com/kevnnard/devtrainer"
check "names the effective URL" err_has "file://$T/remotes/devtrainer.git"
end

begin "refuses a push destination that url.pushInsteadOf rewrites elsewhere"
git config --global url."file://$T/remotes/".pushInsteadOf "git@github.com:kevnnard/"
release --dry-run 0.1.0
expect_preflight_failure "origin push URL must be github.com/kevnnard/devtrainer"
end

begin "refuses several push URLs even when the last one is approved"
git -C "$REPO" remote set-url --add --push origin git@github.com:kevnnard/devtrainer-mirror.git
git -C "$REPO" remote set-url --add --push origin git@github.com:kevnnard/devtrainer.git
release --dry-run 0.1.0
expect_preflight_failure "origin must have exactly one push URL"
end

begin "refuses several fetch URLs"
git -C "$REPO" config remote.origin.url git@github.com:kevnnard/devtrainer-old.git
git -C "$REPO" config --add remote.origin.url git@github.com:kevnnard/devtrainer.git
release --dry-run 0.1.0
expect_preflight_failure "origin must have exactly one URL"
end

begin "refuses an origin configured as a mirror"
git -C "$REPO" config remote.origin.mirror true
release --dry-run 0.1.0
expect_preflight_failure "origin is configured as a mirror"
end

begin "refuses a tap destination that url.insteadOf rewrites elsewhere"
git config --global url."file://$T/remotes/homebrew-tap.git".insteadOf "git@github.com:kevnnard/homebrew-tap.git"
release --dry-run 0.1.0
expect_preflight_failure "tap origin must be github.com/kevnnard/homebrew-tap"
end

begin "refuses a tap with several push URLs"
git -C "$TAP" remote set-url --add --push origin git@github.com:kevnnard/homebrew-other.git
git -C "$TAP" remote set-url --add --push origin git@github.com:kevnnard/homebrew-tap.git
release --dry-run 0.1.0
expect_preflight_failure "tap origin must have exactly one push URL"
end

begin "rechecks destinations right before publishing"
export MOCK_GO_TEST_HOOK="git -C '$REPO' config remote.origin.pushurl git@github.com:kevnnard/devtrainer-fork.git"
release 0.1.0
expect_preflight_failure "origin push URL must be github.com/kevnnard/devtrainer"
end

# ---------- only the intended refs are pushed ----------

begin "push.followTags never publishes other local tags"
git -C "$REPO" config push.followTags true
git -C "$REPO" tag -a private-note -m "local only"
git -C "$TAP" config push.followTags true
git -C "$TAP" tag -a tap-note -m "local only"
release 0.1.0
check "exit 0" code_is 0
check "origin has only main and the release tag" \
  [ "$(git --git-dir="$REMOTE" for-each-ref --format='%(refname)' | tr '\n' ' ')" == "refs/heads/main refs/tags/v0.1.0 " ]
check "tap origin has only main" \
  [ "$(git --git-dir="$TAP_REMOTE" for-each-ref --format='%(refname)' | tr '\n' ' ')" == "refs/heads/main " ]
end

begin "a configured push refspec or push.default does not widen what is sent"
git -C "$REPO" config remote.origin.push 'refs/heads/*:refs/heads/*'
git -C "$REPO" config --add remote.origin.push 'refs/tags/*:refs/tags/*'
git -C "$REPO" config push.default matching
git -C "$REPO" branch side
git -C "$REPO" tag -a private-note -m "local only"
release 0.1.0
check "exit 0" code_is 0
check "origin has only main and the release tag" \
  [ "$(git --git-dir="$REMOTE" for-each-ref --format='%(refname)' | tr '\n' ' ')" == "refs/heads/main refs/tags/v0.1.0 " ]
end

begin "an origin that gains refs it was not sent stops the release"
printf '#!/bin/sh\ngit update-ref refs/heads/surprise "$(git rev-parse refs/heads/main)" 2>/dev/null || true\n' >"$REMOTE/hooks/post-receive"
chmod +x "$REMOTE/hooks/post-receive"
release 0.1.0
check "exit 1" code_is 1
check "explains" err_has "origin refs changed unexpectedly"
check "names the ref" err_has "refs/heads/surprise"
check "says STOP" err_has "STOP:"
check "tag not pushed" [ -z "$(git --git-dir="$REMOTE" rev-parse -q --verify refs/tags/v0.1.0)" ]
check "no release" log_lacks "gh release create"
check "tap untouched" tap_remote_unchanged
check "state records the failed push" grep -q "^failed push main" "$REPO/.release/state/v0.1.0.log"
end

# ---------- every archive integrity failure stops publication ----------

expect_integrity_stop() { # message
  check "exit 1" code_is 1
  check "explains: $1" err_has "$1"
  check "says STOP" err_has "STOP:"
  check "says not to continue" err_has "Do not continue publishing"
  check "offers no release command" bash -c "! grep -qF 'gh release create' '$T/err'"
  check "offers no formula command" bash -c "! grep -qF 'release_tap.py' '$T/err'"
  check "offers no tap push" bash -c "! grep -qF 'homebrew-tap push' '$T/err'"
  check "no release created" log_lacks "gh release create"
  check "tap untouched" tap_remote_unchanged
  check "no formula" no_formula
  check "state records the failed check" grep -q "^failed verify archive" "$REPO/.release/state/v0.1.0.log"
}

begin "an unreadable archive stops publication"
printf 'not a tarball\n' >"$MOCK_ARCHIVE"
release 0.1.0
expect_integrity_stop "archive is not a readable tar.gz"
end

begin "an archive with the wrong top directory stops publication"
git -C "$REPO" archive --format=tar --prefix=other-0.1.0/ HEAD | gzip -n >"$MOCK_ARCHIVE"
release 0.1.0
expect_integrity_stop "archive has entries outside devtrainer-0.1.0/"
end

begin "an archive without go.mod stops publication"
mkdir -p "$T/x/devtrainer-0.1.0" && echo hi >"$T/x/devtrainer-0.1.0/README.md"
tar -cf - -C "$T/x" devtrainer-0.1.0 | gzip -n >"$MOCK_ARCHIVE"
release 0.1.0
expect_integrity_stop "archive has no devtrainer-0.1.0/go.mod"
end

begin "an archive with private paths stops publication"
git -C "$REPO" archive --format=tar --prefix=devtrainer-0.1.0/ HEAD >"$T/a.tar"
mkdir -p "$T/x/devtrainer-0.1.0/odd" && echo x >"$T/x/devtrainer-0.1.0/odd/plan.md"
tar -rf "$T/a.tar" -C "$T/x" devtrainer-0.1.0/odd/plan.md && gzip -nc "$T/a.tar" >"$MOCK_ARCHIVE"
release 0.1.0
expect_integrity_stop "archive contains private paths"
end

# ---------- summary ----------

echo
echo "release_test: $PASS passed, $FAIL failed (workspace $TMP_ROOT)"
[[ $FAIL -eq 0 ]]
