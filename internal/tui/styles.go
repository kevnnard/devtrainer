package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// colors holds one color per role: a calm slate base with a cyan accent.
// Light values are darker shades of the same hues so text stays readable
// on a light terminal. Primary text uses the terminal's own foreground.
type colors struct {
	accent, secondary, muted       color.Color
	success, warning, danger, info color.Color
	rule                           color.Color // pane separators and card borders
	chrome                         color.Color // background of the bottom bar
	sel                            color.Color // background of the focused row
	raised                         color.Color // background of code and Parsons tiles
	errBg, okBg                    color.Color // backgrounds of the -/+ answer diff
}

func newColors(dark bool) colors {
	ld := lipgloss.LightDark(dark)
	c := lipgloss.Color
	return colors{
		accent:    ld(c("#0B7A8F"), c("#5CCFE6")),
		secondary: ld(c("#4B5563"), c("#B8C0CC")),
		muted:     ld(c("#7B8494"), c("#8B949E")),
		success:   ld(c("#2E7D32"), c("#8BD49C")),
		warning:   ld(c("#A15C00"), c("#F2C27A")),
		danger:    ld(c("#C62828"), c("#F28B82")),
		info:      ld(c("#3F51B5"), c("#A5B4FC")),
		rule:      ld(c("#C3CAD3"), c("#3A414B")),
		chrome:    ld(c("#E6EAF0"), c("#161B22")),
		sel:       ld(c("#D3EEF3"), c("#12333B")),
		raised:    ld(c("#EEF1F5"), c("#1C2430")),
		errBg:     ld(c("#FBE9E7"), c("#2A1719")),
		okBg:      ld(c("#E8F5E9"), c("#13241A")),
	}
}

// roles are the text styles. The same roles exist once per background
// (canvas, bar, focused row, code, diff rows), because a style must carry
// its background for it to survive the reset at the end of each segment.
type roles struct {
	base      lipgloss.Style
	title     lipgloss.Style // bold accent
	strong    lipgloss.Style // bold primary
	value     lipgloss.Style
	label     lipgloss.Style // dim
	meta      lipgloss.Style // dim italic
	secondary lipgloss.Style
	accent    lipgloss.Style
	selected  lipgloss.Style // the row under the cursor
	success   lipgloss.Style
	warning   lipgloss.Style
	danger    lipgloss.Style
	info      lipgloss.Style
	rule      lipgloss.Style
	code      lipgloss.Style // code
	input     lipgloss.Style // what the learner types
	reverse   lipgloss.Style // reverse video: the one primary action, the active tab
	faint     lipgloss.Style // empty meter cells
}

func newRoles(p colors, bg color.Color) roles {
	base := lipgloss.NewStyle()
	if bg != nil {
		base = base.Background(bg)
	}
	fg := func(c color.Color) lipgloss.Style { return base.Foreground(c) }
	return roles{
		base:      base,
		title:     fg(p.accent).Bold(true),
		strong:    base.Bold(true),
		value:     base,
		label:     fg(p.muted),
		meta:      fg(p.muted).Italic(true),
		secondary: fg(p.secondary),
		accent:    fg(p.accent),
		selected:  fg(p.accent).Bold(true),
		success:   fg(p.success),
		warning:   fg(p.warning),
		danger:    fg(p.danger),
		info:      fg(p.info),
		rule:      fg(p.rule),
		code:      fg(p.info),
		input:     fg(p.accent).Underline(true),
		reverse:   lipgloss.NewStyle().Foreground(p.accent).Bold(true).Reverse(true),
		faint:     fg(p.rule),
	}
}

// styles is everything View needs to paint the app.
type styles struct {
	main   roles // the panes
	chrome roles // the bottom bar
	sel    roles // the focused option, line or track
	raised roles // code blocks and Parsons tiles
	errRow roles // the "-" row of the answer diff
	okRow  roles // the "+" row of the answer diff
}

func newStyles(dark bool) styles {
	p := newColors(dark)
	return styles{
		main:   newRoles(p, nil),
		chrome: newRoles(p, p.chrome),
		sel:    newRoles(p, p.sel),
		raised: newRoles(p, p.raised),
		errRow: newRoles(p, p.errBg),
		okRow:  newRoles(p, p.okBg),
	}
}
