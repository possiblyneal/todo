package tui

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// The screen's whole palette, in one place. Every style below is built from
// these four roles rather than from a code typed where it is used, so a change
// of look is a change here and the surfaces stay in agreement.
//
// The numbers are ANSI 256 rather than hex because the TUI is served over SSH
// to whatever terminal is on the other end, and 256 is what every one of them
// has. store.Colors carries the nine a Task can be painted in; these are the
// screen's own furniture and are deliberately none of them.
var (
	accent = lipgloss.Color("212") // the cursor, and anything chosen
	muted  = lipgloss.Color("245") // supporting text
	faint  = lipgloss.Color("240") // rules and borders, seen but not read
	danger = lipgloss.Color("203") // overdue, and an error
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	chosenStyle   = selectedStyle
	dimStyle      = lipgloss.NewStyle().Foreground(muted)
	overdueStyle  = lipgloss.NewStyle().Foreground(danger)
	ruleStyle     = lipgloss.NewStyle().Foreground(faint)

	// headerStyle names a region: the List dropdown, the Tags heading, and
	// the title of a Task blown up to fill the pane.
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)

	// boxStyle is every screen that opens over the main view, so they all
	// sit in the same frame however different their contents are.
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(faint).
			Padding(0, 1)

	// sidebarStyle rules the Tags off from the Tasks. The border is a
	// column of its own, so the widths add up to sidebarWidth.
	sidebarStyle = lipgloss.NewStyle().
			Width(sidebarWidth-3).
			PaddingRight(1).
			MarginRight(1).
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(faint)

	// hintStyle is a footer key, and hintKeyStyle the key itself: the key
	// is what the eye is looking for, so it is the part that is not dim.
	hintStyle    = dimStyle
	hintKeyStyle = lipgloss.NewStyle().Foreground(accent)

	// faintStyle is a footer key with nothing to act on: still there, so
	// the row does not move as the cursor does, and plainly not offered.
	faintStyle = lipgloss.NewStyle().Foreground(faint)
)

// formTheme is the palette above, handed to huh. Every form the TUI opens
// wears it, so a Task being written looks like the list it came from rather
// than like a different program borrowed for the occasion.
func formTheme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)

	t.Focused.Base = t.Focused.Base.BorderForeground(accent)
	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(accent).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(muted)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(accent)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(accent)
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(faint)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(danger)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(danger)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(faint)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(accent)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(lipgloss.Color("0")).Background(accent)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(muted).Background(lipgloss.Color("0"))

	// A field nobody is in keeps the same shape and loses the color, so
	// the eye has one place to be.
	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderForeground(faint)
	t.Blurred.Title = t.Blurred.Title.Foreground(muted)
	t.Blurred.NoteTitle = t.Blurred.NoteTitle.Foreground(muted)
	t.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}
