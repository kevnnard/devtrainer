package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setVersion overrides the build version for one test, as -ldflags
// "-X main.version=..." does for a release build.
func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	t.Cleanup(func() { version = old })
	version = v
}

func TestVersionDefaultsToDev(t *testing.T) {
	setup(t, nil)
	out, err := runOut(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if out != "devtrainer dev\n" {
		t.Errorf("version output = %q, want %q", out, "devtrainer dev\n")
	}
}

func TestVersionCommandAndFlagPrintTheBuildVersion(t *testing.T) {
	for _, args := range [][]string{
		{"version"},
		{"--version"},
		{"-version"},
		{"--db", "x.db", "version"},
		{"version", "--new", "2"},
		{"--version", "stats"},
		{"validate", "--version"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setup(t, nil)
			setVersion(t, "1.2.3")
			out, err := runOut(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			if out != "devtrainer 1.2.3\n" {
				t.Errorf("output = %q, want %q", out, "devtrainer 1.2.3\n")
			}
			// Printing the version never opens the app, content or database.
			for _, p := range []string{filepath.Join(dir, "data"), filepath.Join(dir, "x.db")} {
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Errorf("%s was created by a version request (err=%v)", p, err)
				}
			}
		})
	}
}

func TestVersionRejectsArguments(t *testing.T) {
	setup(t, nil)
	_, err := runOut(t, "version", "now")
	if err == nil || !strings.HasPrefix(err.Error(), "version takes no arguments\n\n") || !strings.Contains(err.Error(), usage) {
		t.Errorf("err = %v", err)
	}
}

func TestVersionDoesNotHideUsageErrors(t *testing.T) {
	setup(t, nil)
	for _, args := range [][]string{{"--version", "--bogus"}, {"--version", "--new", "-1"}} {
		if _, err := runOut(t, args...); err == nil {
			t.Errorf("%q: want an error", args)
		}
	}
}

func TestHelpDocumentsVersion(t *testing.T) {
	setup(t, nil)
	out, err := runOut(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"devtrainer version", "--version"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
}
