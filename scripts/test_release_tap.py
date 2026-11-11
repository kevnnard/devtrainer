"""Tests for release_tap.py. Run: python3 -I -m unittest discover -s scripts -p 'test_*.py'"""

import contextlib
import io
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import release_tap  # noqa: E402

SHA = "0123456789abcdef" * 4

EXPECTED = '''class Devtrainer < Formula
  desc "Terminal trainer for programming, architecture and tooling with spaced repetition"
  homepage "https://github.com/kevnnard/devtrainer"
  url "https://codeload.github.com/kevnnard/devtrainer/tar.gz/refs/tags/v0.1.0"
  version "0.1.0"
  sha256 "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  # PolyForm Strict License 1.0.0 (not on the SPDX list); see LICENSE and
  # https://polyformproject.org/licenses/strict/1.0.0
  license :cannot_represent

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}"), "./cmd/devtrainer"
  end

  test do
    assert_equal "devtrainer #{version}\\n", shell_output("#{bin}/devtrainer --version")

    # validate reads only the embedded content; it must not create a database.
    ENV["XDG_DATA_HOME"] = (testpath/"data").to_s
    ENV["XDG_CONFIG_HOME"] = (testpath/"config").to_s
    assert_match "Content OK", shell_output("#{bin}/devtrainer validate")
    refute_path_exists testpath/"data/devtrainer/devtrainer.db"
  end
end
'''


def run_main(*argv):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = release_tap.main(list(argv))
    return code, out.getvalue(), err.getvalue()


def make_tap(root):
    os.mkdir(os.path.join(root, ".git"))
    os.mkdir(os.path.join(root, "Formula"))
    for name, body in {"Formula/watts.rb": "class Watts\nend\n", "README.md": "# tap\n"}.items():
        with open(os.path.join(root, name), "w") as f:
            f.write(body)


def snapshot(root):
    files = {}
    for d, _, names in os.walk(root):
        for n in names:
            p = os.path.join(d, n)
            with open(p, "rb") as f:
                files[os.path.relpath(p, root)] = f.read()
    return files


class RenderTest(unittest.TestCase):
    def test_renders_the_exact_formula(self):
        self.assertEqual(release_tap.render("0.1.0", SHA), EXPECTED)

    def test_rendering_is_deterministic(self):
        self.assertEqual(release_tap.render("1.20.3", SHA), release_tap.render("1.20.3", SHA))

    def test_formula_builds_from_source_without_bottles(self):
        body = release_tap.render("2.0.0", SHA)
        for want in ('depends_on "go" => :build', "std_go_args(", "-X main.version=#{version}",
                     'url "https://codeload.github.com/kevnnard/devtrainer/tar.gz/refs/tags/v2.0.0"',
                     "devtrainer --version", "devtrainer validate"):
            self.assertIn(want, body)
        for unwanted in ("bottle", "on_macos", "on_linux", "depends_on :macos", "head "):
            self.assertNotIn(unwanted, body)

    def test_license_uses_the_non_spdx_symbol(self):
        body = release_tap.render("0.1.0", SHA)
        license_lines = [l.strip() for l in body.splitlines() if l.strip().startswith("license")]
        self.assertEqual(license_lines, ["license :cannot_represent"])
        self.assertIn("# PolyForm Strict License 1.0.0", body)
        self.assertIn("https://polyformproject.org/licenses/strict/1.0.0", body)
        self.assertNotIn("PolyForm-Strict", body)  # not a real SPDX identifier

    def test_rejects_invalid_versions(self):
        for v in ("", "1", "1.2", "v1.2.3", "01.2.3", "1.02.3", "1.2.03", "1.2.3-rc1",
                  "1.2.3+b", " 1.2.3", "1.2.3\n", "1.2.3.4", "1..3", "a.b.c", None):
            with self.subTest(v=v), self.assertRaises(release_tap.InputError):
                release_tap.render(v, SHA)

    def test_rejects_invalid_checksums(self):
        for s in ("", "abc", SHA.upper(), SHA[:-1], SHA + "0", "g" * 64, SHA[:-1] + "\n", None):
            with self.subTest(s=s), self.assertRaises(release_tap.InputError):
                release_tap.render("0.1.0", s)

    def test_accepts_zero_components(self):
        self.assertIn('version "0.0.0"', release_tap.render("0.0.0", SHA))


class CliTest(unittest.TestCase):
    def test_stdout_mode_prints_the_formula(self):
        code, out, err = run_main("--version", "0.1.0", "--sha256", SHA, "--stdout")
        self.assertEqual((code, out, err), (0, EXPECTED, ""))

    def test_bad_input_exits_2_without_writing(self):
        with tempfile.TemporaryDirectory() as tap:
            make_tap(tap)
            before = snapshot(tap)
            code, out, err = run_main("--version", "1.2", "--sha256", SHA, "--tap", tap)
            self.assertEqual(code, 2)
            self.assertIn("version must be numeric", err)
            self.assertEqual(snapshot(tap), before)

    def test_requires_exactly_one_output(self):
        for argv in ([], ["--stdout", "--tap", "x"]):
            with self.subTest(argv=argv), self.assertRaises(SystemExit) as cm, \
                    contextlib.redirect_stderr(io.StringIO()):
                release_tap.main(["--version", "0.1.0", "--sha256", SHA, *argv])
            self.assertEqual(cm.exception.code, 2)


class WriteTest(unittest.TestCase):
    def test_writes_only_the_devtrainer_formula(self):
        with tempfile.TemporaryDirectory() as tap:
            make_tap(tap)
            before = snapshot(tap)
            code, out, _ = run_main("--version", "0.1.0", "--sha256", SHA, "--tap", tap)
            self.assertEqual(code, 0)
            after = snapshot(tap)
            self.assertEqual(after.pop(os.path.join("Formula", "devtrainer.rb")), EXPECTED.encode())
            self.assertEqual(after, before)
            self.assertTrue(out.strip().endswith(os.path.join("Formula", "devtrainer.rb")))

    def test_rewrites_an_existing_formula(self):
        with tempfile.TemporaryDirectory() as tap:
            make_tap(tap)
            with open(os.path.join(tap, "Formula", "devtrainer.rb"), "w") as f:
                f.write("old\n")
            self.assertEqual(run_main("--version", "0.1.0", "--sha256", SHA, "--tap", tap)[0], 0)
            with open(os.path.join(tap, "Formula", "devtrainer.rb")) as f:
                self.assertEqual(f.read(), EXPECTED)
            self.assertEqual(sorted(os.listdir(os.path.join(tap, "Formula"))), ["devtrainer.rb", "watts.rb"])

    def test_rejects_unusable_taps(self):
        with tempfile.TemporaryDirectory() as root:
            missing = os.path.join(root, "missing")
            no_git = os.path.join(root, "nogit")
            os.makedirs(os.path.join(no_git, "Formula"))
            no_formula = os.path.join(root, "noformula")
            os.makedirs(os.path.join(no_formula, ".git"))
            link_target = os.path.join(root, "linked")
            os.mkdir(os.path.join(root, "tap"))
            make_tap(os.path.join(root, "tap"))
            os.symlink(os.path.join(root, "elsewhere.rb"), os.path.join(root, "tap", "Formula", "devtrainer.rb"))
            os.symlink(os.path.join(root, "tap"), link_target)
            for tap, msg in ((missing, "not found"), (no_git, "not a git checkout"),
                             (no_formula, "no Formula/"), (os.path.join(root, "tap"), "non-regular"),
                             (link_target, "symlink")):
                with self.subTest(tap=tap):
                    code, _, err = run_main("--version", "0.1.0", "--sha256", SHA, "--tap", tap)
                    self.assertEqual(code, 2)
                    self.assertIn(msg, err)
            self.assertFalse(os.path.lexists(os.path.join(root, "elsewhere.rb")))


if __name__ == "__main__":
    unittest.main()
