// Package datepicker is a calendar for Bubble Tea v2.
//
// It exists because nothing else does. The only Go date picker,
// EthanEFung/bubble-datepicker, pins bubbletea v0.24.2 and does not compile
// against charm.land/bubbletea/v2; Textual and ratatui both ship a current
// one. That gap was found before any code was written and accepted as a cost
// of choosing Go, and this is the cost being paid.
//
// The component is a value, updated the way every bubbles component is: hand
// it a message, take back a new Model. It reads no clock but its own Now, so a
// test can put it on any day it likes.
package datepicker

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Shortcut is a named jump the picker offers, such as the four snoozes. The
// key is what takes it, and When is given the picker's idea of now.
type Shortcut struct {
	Key   string
	Label string
	When  func(time.Time) time.Time
}

// Styles are the calendar's own. They are set from whatever the surrounding
// form is themed with, so the picker carries no colours of its own.
type Styles struct {
	Header   lipgloss.Style
	Weekday  lipgloss.Style
	Day      lipgloss.Style
	Cursor   lipgloss.Style
	Today    lipgloss.Style
	Dim      lipgloss.Style
	Selected lipgloss.Style
}

// DefaultStyles are enough to read the calendar without a theme.
func DefaultStyles() Styles {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Weekday:  dim,
		Day:      lipgloss.NewStyle(),
		Cursor:   lipgloss.NewStyle().Reverse(true),
		Today:    lipgloss.NewStyle().Underline(true),
		Dim:      dim,
		Selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
	}
}

// Model is a month of days with a cursor on one of them.
//
// A date is chosen by putting the cursor on it: moving sets the value, and
// space takes it back off, which is what "no deadline" is. There is no
// separate confirm, because a calendar with a cursor on the 14th and no date
// chosen is a calendar lying about itself.
type Model struct {
	Styles    Styles
	Shortcuts []Shortcut

	// Now is the picker's clock. It is a field so a test can fix today.
	Now func() time.Time

	cursor time.Time
	value  time.Time
	chosen bool
}

// New returns a picker sitting on today with no date chosen.
func New() Model {
	m := Model{Styles: DefaultStyles(), Now: time.Now}
	m.cursor = day(m.Now())
	return m
}

// Value is the chosen date, zero when none is chosen.
func (m Model) Value() time.Time {
	if !m.chosen {
		return time.Time{}
	}
	return m.value
}

// SetValue puts the cursor on a date and chooses it. The zero time chooses
// nothing and leaves the cursor on today, which is where a person looking for
// a date wants to start.
func (m Model) SetValue(t time.Time) Model {
	if t.IsZero() {
		m.chosen, m.value = false, time.Time{}
		m.cursor = day(m.Now())
		return m
	}
	m.value, m.chosen = t, true
	m.cursor = day(t)
	return m
}

// Update moves the cursor. The second result says whether the message was
// one of the picker's, so a form keeps tab, enter and escape for itself.
//
// It returns no command. A calendar is arithmetic on a date: there is nothing
// for it to go and do.
func (m Model) Update(msg tea.Msg) (Model, bool) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, false
	}
	switch key := press.String(); key {
	case "left", "h":
		return m.move(0, 0, -1), true
	case "right", "l":
		return m.move(0, 0, 1), true
	case "up", "k":
		return m.move(0, 0, -7), true
	case "down", "j":
		return m.move(0, 0, 7), true
	case "pgup", "[", "shift+left":
		return m.move(0, -1, 0), true
	case "pgdown", "]", "shift+right":
		return m.move(0, 1, 0), true
	case "home", "t":
		return m.SetValue(day(m.Now())), true
	case "space":
		// The one toggle: a date chosen becomes no date, and no date becomes
		// the day under the cursor.
		if m.chosen {
			m.chosen, m.value = false, time.Time{}
			return m, true
		}
		return m.SetValue(m.cursor), true
	default:
		for _, s := range m.Shortcuts {
			if s.Key != key {
				continue
			}
			return m.SetValue(s.When(m.Now())), true
		}
	}
	return m, false
}

// move walks the cursor and takes the value with it. A month step from the
// 31st lands on the last day of a shorter month rather than skipping it,
// which is the same clamp a monthly Series makes.
func (m Model) move(years, months, days int) Model {
	next := m.cursor.AddDate(years, 0, days)
	if months != 0 {
		next = clamp(m.cursor, years, months)
	}
	return m.SetValue(withTime(next, m.value))
}

// clamp adds months to a date, keeping it inside the month it lands in.
func clamp(from time.Time, years, months int) time.Time {
	first := time.Date(from.Year()+years, from.Month(), 1, 0, 0, 0, 0, from.Location()).
		AddDate(0, months, 0)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(from.Day(), last), 0, 0, 0, 0, from.Location())
}

// withTime keeps the time of day a shortcut put on the value. A calendar picks
// a date; "in 1 hour" picks a moment, and moving the cursor afterwards should
// not quietly round it to midnight.
func withTime(date, from time.Time) time.Time {
	if from.IsZero() || (from.Hour() == 0 && from.Minute() == 0) {
		return date
	}
	return time.Date(date.Year(), date.Month(), date.Day(),
		from.Hour(), from.Minute(), 0, 0, date.Location())
}

// Keys is the help line, in the order the keys are laid out.
func (m Model) Keys() string {
	keys := []string{"←→↑↓ move", "[ ] month", "t today", "space clear"}
	for _, s := range m.Shortcuts {
		keys = append(keys, s.Key+" "+s.Label)
	}
	return strings.Join(keys, " · ")
}

// View draws the month the cursor is in, Monday first.
func (m Model) View() string {
	s := m.Styles
	first := time.Date(m.cursor.Year(), m.cursor.Month(), 1, 0, 0, 0, 0, m.cursor.Location())
	lead := (int(first.Weekday()) + 6) % 7 // Monday first, not Sunday
	days := first.AddDate(0, 1, -1).Day()
	today := day(m.Now())

	rows := []string{
		s.Header.Render(first.Format("January 2006")),
		s.Weekday.Render("Mo Tu We Th Fr Sa Su"),
	}
	var week []string
	for range lead {
		week = append(week, "  ")
	}
	for date := 1; date <= days; date++ {
		on := time.Date(first.Year(), first.Month(), date, 0, 0, 0, 0, first.Location())
		cell := fmt.Sprintf("%2d", date)
		switch {
		case on.Equal(m.cursor):
			cell = s.Cursor.Render(cell)
		case on.Equal(today):
			cell = s.Today.Render(cell)
		default:
			cell = s.Day.Render(cell)
		}
		week = append(week, cell)
		if len(week) == 7 {
			rows = append(rows, strings.Join(week, " "))
			week = week[:0]
		}
	}
	if len(week) > 0 {
		rows = append(rows, strings.Join(week, " "))
	}
	rows = append(rows, "", s.Selected.Render(m.Text()))
	return strings.Join(rows, "\n")
}

// Text is the chosen date as a person reads it, and "no date" when none is.
func (m Model) Text() string {
	switch {
	case !m.chosen:
		return "no date"
	case m.value.Hour() != 0 || m.value.Minute() != 0:
		return m.value.Format("2006-01-02 15:04")
	default:
		return m.value.Format("2006-01-02")
	}
}

func day(t time.Time) time.Time {
	year, month, date := t.Date()
	return time.Date(year, month, date, 0, 0, 0, 0, t.Location())
}
