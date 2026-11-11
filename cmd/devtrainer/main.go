// Command devtrainer is a terminal trainer for programming, architecture
// and tooling with spaced repetition and closed questions.
//
// Usage:
//
//	devtrainer [flags]                       open the app
//	devtrainer [flags] stats                 print a progress summary
//	devtrainer [flags] validate              check the embedded content
//	devtrainer [flags] draft <track> <topic> draft new items with the AI into drafts/
//	devtrainer version                       print the version
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/content"
	contentload "github.com/kevnnard/devtrainer/internal/content"
	"github.com/kevnnard/devtrainer/internal/runner"
	"github.com/kevnnard/devtrainer/internal/srs"
	"github.com/kevnnard/devtrainer/internal/store"
	"github.com/kevnnard/devtrainer/internal/tui"
)

const usage = `devtrainer — practice programming, architecture and tooling in the terminal.

usage:
  devtrainer [flags]                        open the app (Home, Study, Tracks, Stats)
  devtrainer [flags] stats                  print a plain-text progress summary
  devtrainer [flags] validate               load the embedded content; print counts or the errors
  devtrainer [flags] draft <track> <topic>  ask the AI for candidate items about <topic> and write
                                            them, validated, to drafts/<track>-<topic>.yaml for review
  devtrainer version                        print the version

flags (before or after the command):
  --db path      SQLite progress database
                 (default $XDG_DATA_HOME/devtrainer/devtrainer.db, else ~/.local/share/...)
  --new n        new concepts per day (default 3)
  --config path  AI config (default $XDG_CONFIG_HOME/devtrainer/config.json, else ~/.config/...):
                 {"pi":"pi","models":{"explain":"provider/model","draft":"provider/model"},"timeout_seconds":90}
  --version      print the version and exit
  -h, --help     show this help

The AI runs through the pi CLI (Gentle Shell); everything except "e" and draft works without it.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "devtrainer:", err)
		os.Exit(1)
	}
}

// version is the build version; release builds set it with
// -ldflags "-X main.version=X.Y.Z".
var version = "dev"

// contentFS is the study content; tests replace it.
var contentFS fs.FS = content.FS

// newRunner builds the code exercise runner; tests replace it.
var newRunner = runner.New

// appOptions is how the app was asked to open.
type appOptions struct {
	dbPath     string
	configPath string
	maxNew     int
}

// run parses args and executes one command, writing messages to stdout.
func run(args []string, stdout io.Writer) error {
	home, _ := os.UserHomeDir()
	defaultDB, err := store.DefaultPath()
	if err != nil {
		return err
	}

	fset := flag.NewFlagSet("devtrainer", flag.ContinueOnError)
	fset.SetOutput(io.Discard) // errors are returned, not printed twice
	opts := appOptions{}
	fset.StringVar(&opts.dbPath, "db", defaultDB, "path to the SQLite database")
	fset.IntVar(&opts.maxNew, "new", srs.DefaultMaxNew, "new concepts per day")
	fset.StringVar(&opts.configPath, "config", defaultConfigPath(os.Getenv("XDG_CONFIG_HOME"), home), "path to config.json")
	showVersion := fset.Bool("version", false, "print the version and exit")

	pos, err := parseInterleaved(fset, args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(stdout, usage)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, usage)
	}
	if opts.maxNew < 0 {
		return fmt.Errorf("--new must be 0 or more, got %d", opts.maxNew)
	}
	if *showVersion {
		return printVersion(stdout)
	}

	if len(pos) == 0 {
		return startApp(opts)
	}
	cmd, rest := pos[0], pos[1:]
	switch cmd {
	case "stats":
		if len(rest) != 0 {
			return errors.New("stats takes no arguments\n\n" + usage)
		}
		return runStats(opts, stdout)
	case "validate":
		if len(rest) != 0 {
			return errors.New("validate takes no arguments\n\n" + usage)
		}
		return runValidate(stdout)
	case "draft":
		if len(rest) < 2 {
			return errors.New("draft needs a track and a topic\n\n" + usage)
		}
		return runDraft(opts, rest[0], strings.Join(rest[1:], " "), stdout)
	case "version":
		if len(rest) != 0 {
			return errors.New("version takes no arguments\n\n" + usage)
		}
		return printVersion(stdout)
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

// printVersion writes the build version; it needs no content or database.
func printVersion(stdout io.Writer) error {
	_, err := fmt.Fprintf(stdout, "devtrainer %s\n", version)
	return err
}

// parseInterleaved parses flags anywhere in args and returns the other
// arguments in order; "--" ends flag parsing.
func parseInterleaved(fset *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fset.Parse(args); err != nil {
			return nil, err
		}
		args = fset.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// startApp runs the TUI; tests replace it so no terminal is opened.
var startApp = runApp

func runApp(opts appOptions) error {
	cat, err := contentload.Load(contentFS)
	if err != nil {
		return err
	}
	conf, err := loadConfig(opts.configPath)
	if err != nil {
		return err
	}
	st, err := store.Open(opts.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	app, err := tui.NewApp(tui.Config{
		Catalog:  cat,
		Progress: st,
		Now:      time.Now,
		MaxNew:   opts.maxNew,
		Tutor:    newTutor(conf),
		Runner:   newRunner(),
		Start:    tui.Home,
	})
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(app).Run()
	if err != nil {
		return err
	}
	return final.(tui.App).Err()
}
