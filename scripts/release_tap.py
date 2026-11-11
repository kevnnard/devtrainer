#!/usr/bin/env python3
"""Render the Homebrew formula for a devtrainer release.

The formula builds devtrainer from the exact GitHub source archive of the
release tag with the Go toolchain; there are no precompiled bottles. Only
Formula/devtrainer.rb is written; every other file in the tap is left alone.
This helper never runs git, commits or pushes: scripts/release.sh does that.

Usage:
  release_tap.py --version X.Y.Z --sha256 HEX --stdout
  release_tap.py --version X.Y.Z --sha256 HEX --tap DIR

Exit codes: 0 written, 2 invalid input.
"""

import argparse
import os
import re
import sys
import tempfile

REPO = "kevnnard/devtrainer"
FORMULA_PATH = os.path.join("Formula", "devtrainer.rb")

VERSION_RE = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
SHA256_RE = re.compile(r"[0-9a-f]{64}")

TEMPLATE = '''class Devtrainer < Formula
  desc "Terminal trainer for programming, architecture and tooling with spaced repetition"
  homepage "https://github.com/{repo}"
  url "{url}"
  version "{version}"
  sha256 "{sha256}"
  # PolyForm Strict License 1.0.0 (not on the SPDX list); see LICENSE and
  # https://polyformproject.org/licenses/strict/1.0.0
  license :cannot_represent

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{{version}}"), "./cmd/devtrainer"
  end

  test do
    assert_equal "devtrainer #{{version}}\\n", shell_output("#{{bin}}/devtrainer --version")

    # validate reads only the embedded content; it must not create a database.
    ENV["XDG_DATA_HOME"] = (testpath/"data").to_s
    ENV["XDG_CONFIG_HOME"] = (testpath/"config").to_s
    assert_match "Content OK", shell_output("#{{bin}}/devtrainer validate")
    refute_path_exists testpath/"data/devtrainer/devtrainer.db"
  end
end
'''


class InputError(Exception):
    """An argument or the tap checkout is not usable."""


def source_url(version):
    """The codeload archive Homebrew downloads for tag vVERSION."""
    return f"https://codeload.github.com/{REPO}/tar.gz/refs/tags/v{version}"


def render(version, sha256):
    """Return the formula text; raise InputError on invalid input."""
    if not isinstance(version, str) or not VERSION_RE.fullmatch(version):
        raise InputError(f"version must be numeric X.Y.Z without leading zeros, got {version!r}")
    if not isinstance(sha256, str) or not SHA256_RE.fullmatch(sha256):
        raise InputError(f"sha256 must be 64 lowercase hex characters, got {sha256!r}")
    return TEMPLATE.format(repo=REPO, url=source_url(version), version=version, sha256=sha256)


def write_formula(tap_dir, body):
    """Atomically replace Formula/devtrainer.rb inside tap_dir; return its path."""
    if not os.path.isdir(tap_dir) or os.path.islink(tap_dir):
        raise InputError(f"tap directory not found (or is a symlink): {tap_dir}")
    if not os.path.isdir(os.path.join(tap_dir, ".git")):
        raise InputError(f"tap directory is not a git checkout root: {tap_dir}")
    formula_dir = os.path.join(tap_dir, "Formula")
    if not os.path.isdir(formula_dir) or os.path.islink(formula_dir):
        raise InputError(f"tap has no Formula/ directory: {tap_dir}")
    target = os.path.join(tap_dir, FORMULA_PATH)
    if os.path.lexists(target) and (os.path.islink(target) or not os.path.isfile(target)):
        raise InputError(f"refusing to replace a non-regular file: {target}")
    fd, tmp = tempfile.mkstemp(prefix=".devtrainer.rb.", dir=formula_dir)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as f:
            f.write(body)
        os.chmod(tmp, 0o644)
        os.replace(tmp, target)
    except BaseException:
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise
    return target


def main(argv=None):
    p = argparse.ArgumentParser(description="Render the devtrainer Homebrew formula.")
    p.add_argument("--version", required=True, help="release version X.Y.Z (no leading v)")
    p.add_argument("--sha256", required=True, help="sha256 of the codeload tag archive")
    out = p.add_mutually_exclusive_group(required=True)
    out.add_argument("--stdout", action="store_true", help="print the formula")
    out.add_argument("--tap", metavar="DIR", help="write DIR/Formula/devtrainer.rb")
    args = p.parse_args(argv)
    try:
        body = render(args.version, args.sha256)
        if args.stdout:
            sys.stdout.write(body)
        else:
            print(write_formula(args.tap, body))
    except InputError as e:
        print(f"release_tap.py: {e}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
