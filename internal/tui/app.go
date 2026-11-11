// Package tui is devtrainer's Bubble Tea app: one root model that owns the
// Home, Study, Tracks and Stats screens, the page layout and the global
// keys. Time comes from an injected clock and progress from an injected
// Progress, so every screen is deterministic in tests.
package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/devtrainer/internal/check"
	"github.com/kevnnard/devtrainer/internal/domain"
	"github.com/kevnnard/devtrainer/internal/srs"
)

// Progress is where answers and schedules live; *store.Store implements it.
type Progress interface {
	Schedules(ctx context.Context) (map[string]domain.Schedule, error)
	History(ctx context.Context) (map[string][]domain.Answer, error)
	Stats(ctx context.Context, cat domain.Catalog, now time.Time) (domain.Stats, error)
	RecordReview(ctx context.Context, a domain.Answer, sc domain.Schedule) error
	LessonsSeen(ctx context.Context) (map[string]bool, error)
	MarkLessonSeen(ctx context.Context, conceptID string, at time.Time) error
	RecordExam(ctx context.Context, r domain.ExamResult) error
	Exams(ctx context.Context) (map[string][]domain.ExamResult, error)
}

// Explainer gives a deeper explanation of an answered item;
// *tutor.Tutor implements it.
type Explainer interface {
	Explain(ctx context.Context, it domain.Item, c domain.Concept, r check.Response, res check.Result) (string, error)
}

// Screen identifies one page of the app.
type Screen int

const (
	Home Screen = iota
	Study
	Tracks
	Stats
)

// screens is the order of the tabs, of tab / shift+tab and of the number keys.
var screens = []Screen{Home, Study, Tracks, Stats}

func (s Screen) String() string {
	return [...]string{"Home", "Study", "Tracks", "Stats"}[s]
}

// Config is what the app needs from the outside world.
type Config struct {
	Catalog  domain.Catalog
	Progress Progress
	Now      func() time.Time // injected clock; its location sets the study day
	MaxNew   int              // new concepts per day (srs.DefaultMaxNew)
	Tutor    Explainer        // nil: the e key explains that the tutor is off
	Runner   CodeRunner       // nil: code items are not served
	Start    Screen
}

// App is the root Bubble Tea model.
type App struct {
	cat      domain.Catalog
	progress Progress
	now      func() time.Time
	sched    srs.Scheduler
	tutor    Explainer
	runner   CodeRunner
	maxNew   int

	missing   map[string]error // code languages without a toolchain
	toolNotes []string         // toolchain warnings, such as an old JDK

	schedules map[string]domain.Schedule
	history   map[string][]domain.Answer
	states    map[string]domain.State
	stats     domain.Stats
	seen      map[string]bool                // concepts whose lesson was gone through
	exams     map[string][]domain.ExamResult // exam attempts by section ID, oldest first

	session session
	reqs    int         // last async request number; replies carry theirs so stale ones are dropped
	exam    *examRun    // the section exam in Study, if any; today's session waits in it
	note    string      // a one-off message on Tracks, such as why an exam is locked
	lesson  lessonState // the lesson on screen, if any
	screen  Screen
	help    bool
	track   int // cursor on the Tracks screen
	concept int // concept cursor inside the selected track

	width, height int
	st            styles
	err           error // a failed save stops the app
	errWhat       string
}

// NewApp loads the learner's progress and plans today's session.
func NewApp(cfg Config) (App, error) {
	a := App{
		cat:      cfg.Catalog,
		progress: cfg.Progress,
		now:      cfg.Now,
		sched:    srs.NewScheduler(),
		tutor:    cfg.Tutor,
		runner:   cfg.Runner,
		maxNew:   cfg.MaxNew,
		screen:   cfg.Start,
		st:       newStyles(true),
	}
	ctx := context.Background()
	var err error
	if a.schedules, err = a.progress.Schedules(ctx); err != nil {
		return App{}, err
	}
	if a.history, err = a.progress.History(ctx); err != nil {
		return App{}, err
	}
	if a.seen, err = a.progress.LessonsSeen(ctx); err != nil {
		return App{}, err
	}
	if a.exams, err = a.progress.Exams(ctx); err != nil {
		return App{}, err
	}
	if err := a.refresh(); err != nil {
		return App{}, err
	}
	a.checkToolchains()
	now := a.now()
	picks := srs.PlanSession(srs.PlanInput{
		Catalog:   a.servable(),
		Schedules: a.schedules,
		States:    a.states,
		MaxNew:    max(0, cfg.MaxNew-a.newToday(now)),
	}, now)
	a.session = newSession(picks)
	a.startQuestion()
	return a, nil
}

// refresh derives the states from the history and reloads the stats.
func (a *App) refresh() error {
	ids := make([]string, len(a.cat.Concepts))
	for i, c := range a.cat.Concepts {
		ids[i] = c.ID
	}
	now := a.now()
	a.states = srs.States(ids, a.history, now.Location())
	st, err := a.progress.Stats(context.Background(), a.cat, now)
	if err != nil {
		return err
	}
	a.stats = st
	return nil
}

// newToday counts the concepts first studied today, so the daily cap on
// new concepts holds across several runs of the app on one day.
func (a App) newToday(now time.Time) int {
	today := domain.DateOf(now, now.Location())
	n := 0
	for _, answers := range a.history {
		if len(answers) > 0 && domain.DateOf(answers[0].At, now.Location()) == today {
			n++
		}
	}
	return n
}

// Init asks the terminal for its background color so the palette can follow it.
func (a App) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Screen returns the screen on display.
func (a App) Screen() Screen { return a.screen }

// Err returns the save error that ended the app, if any.
func (a App) Err() error { return a.err }

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		a.st = newStyles(msg.IsDark())
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	case tea.PasteMsg:
		return a.paste(msg), nil
	case explainMsg:
		return a.gotExplanation(msg), nil
	case codeRunMsg:
		return a.gotRun(msg), nil
	}
	return a, nil
}

// handleKey routes a key press: the error and help overlays first, then a
// focused text input, then the active screen, then the global keys.
func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == "ctrl+c":
		return a, tea.Quit
	case a.err != nil:
		return a, tea.Quit // any key leaves the error screen
	case a.help:
		if k == "?" || k == "esc" || k == "q" {
			a.help = false
		}
		return a, nil
	case a.screen == Study && a.typing():
		return a.updateStudy(msg)
	}

	inLesson := a.lessonOn()
	switch {
	case inLesson:
		if next, cmd, ok := a.lessonKey(msg); ok {
			return next, cmd
		}
	case a.screen == Study:
		if next, cmd, ok := a.studyKey(msg); ok {
			return next, cmd
		}
	}
	switch k {
	case "tab":
		a = a.show(screens[(int(a.screen)+1)%len(screens)])
	case "shift+tab":
		a = a.show(screens[(int(a.screen)+len(screens)-1)%len(screens)])
	case "1", "2", "3", "4":
		a = a.show(screens[k[0]-'1'])
	case "?":
		a.help = true
	case "esc":
		a = a.show(Home)
	case "q":
		return a, tea.Quit
	default:
		if inLesson {
			return a, nil
		}
		return a.updateScreen(msg)
	}
	return a, nil
}

// show switches to screen s.
func (a App) show(s Screen) App {
	if a.lesson.on && a.lesson.screen == Tracks && s != Tracks {
		a.lesson = lessonState{} // a review lesson closes when you leave Tracks
	}
	if a.exam != nil && a.exam.result != nil && s != Study {
		a = a.endExam() // a finished exam gives Study back to today's session
	}
	if s != Tracks {
		a.note = ""
	}
	a.screen = s
	if s == Study {
		a.startQuestion()
	}
	return a
}

// startQuestion starts the clock of the current question the first time
// it is on screen. A new concept with an unseen lesson shows the lesson
// first; the clock starts when the lesson ends.
func (a *App) startQuestion() {
	if a.screen != Study || a.session.done() {
		return
	}
	if a.lesson.on && a.lesson.screen == Study {
		return
	}
	q := &a.session.q
	if !q.shown.IsZero() || q.answered {
		return
	}
	if c := q.pick.Concept; a.lessonDue(c) {
		a.lesson = newLesson(c, Study)
		return
	}
	q.shown = a.now()
}

func (a App) updateScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case Home:
		if msg.String() == "enter" {
			a = a.show(a.homeAction().target)
		}
	case Tracks:
		return a.tracksKey(msg), nil
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	return v
}
