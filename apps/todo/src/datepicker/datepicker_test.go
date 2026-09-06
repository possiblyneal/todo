package datepicker

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// fixed puts the picker on a known day, so a test asserting on "today" is not
// asserting on the day it runs.
func fixed(t *testing.T, date string) Model {
	t.Helper()
	when, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatalf("parse %q: %v", date, err)
	}
	m := New()
	m.Now = func() time.Time { return when }
	return m.SetValue(time.Time{})
}

// press builds the message a terminal builds. Space is the one key whose
// text is not its name: ultraviolet drops a " " text and falls back to the
// code, so a test that sends Text " " alone is not sending space at all.
func press(m Model, keys ...string) Model {
	for _, k := range keys {
		msg := tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		if k == " " {
			msg = tea.KeyPressMsg{Code: tea.KeySpace}
		}
		m, _ = m.Update(msg)
	}
	return m
}

// The picker reads its own clock, so a test can hold the calendar still.
func TestItStartsOnTodayWithNoDateChosen(t *testing.T) {
	m := fixed(t, "2026-09-04")
	if !m.Value().IsZero() {
		t.Errorf("a new picker had %v chosen, want no date", m.Value())
	}
	if m.Text() != "no date" {
		t.Errorf("Text() = %q, want %q", m.Text(), "no date")
	}
	if !strings.Contains(m.View(), "September 2026") {
		t.Errorf("View() does not name the month:\n%s", m.View())
	}
}

// Moving the cursor is choosing: there is no separate confirm, because a
// calendar with the cursor on a day and no date chosen is lying about itself.
func TestMovingTheCursorChoosesTheDate(t *testing.T) {
	m := press(fixed(t, "2026-09-04"), "right")
	if got := m.Value().Format("2006-01-02"); got != "2026-09-05" {
		t.Errorf("right = %s, want 2026-09-05", got)
	}
	m = press(m, "down")
	if got := m.Value().Format("2006-01-02"); got != "2026-09-12" {
		t.Errorf("down = %s, want a week on", got)
	}
	m = press(m, "up", "up", "left")
	if got := m.Value().Format("2006-01-02"); got != "2026-08-28" {
		t.Errorf("up up left = %s, want 2026-08-28", got)
	}
}

// A month step from the 31st lands on the last day of a shorter month, the
// same clamp a monthly Series makes.
func TestAMonthStepClampsToTheShorterMonth(t *testing.T) {
	m := fixed(t, "2026-01-31")
	m = m.SetValue(m.Now())
	for _, want := range []string{"2026-02-28", "2026-03-28"} {
		m = press(m, "]")
		if got := m.Value().Format("2006-01-02"); got != want {
			t.Fatalf("a month on = %s, want %s", got, want)
		}
	}
}

// Space is the one toggle: a chosen date becomes no date, and no date becomes
// the day under the cursor.
func TestSpaceClearsAndChoosesAgain(t *testing.T) {
	m := press(fixed(t, "2026-09-04"), "right")
	m = press(m, " ")
	if !m.Value().IsZero() {
		t.Errorf("space left %v chosen, want no date", m.Value())
	}
	m = press(m, " ")
	if got := m.Value().Format("2006-01-02"); got != "2026-09-05" {
		t.Errorf("space again = %s, want the day under the cursor", got)
	}
}

// The offered snoozes are shortcut keys on the same calendar, so "in an hour"
// and "the 14th" are one screen rather than two.
func TestAShortcutJumpsAndKeepsItsTimeOfDay(t *testing.T) {
	m := fixed(t, "2026-09-04")
	m.Shortcuts = []Shortcut{
		{Key: "1", Label: "1 hour", When: func(now time.Time) time.Time { return now.Add(90 * time.Minute) }},
	}
	m = press(m, "1")
	if got := m.Text(); got != "2026-09-04 01:30" {
		t.Errorf("the shortcut chose %q", got)
	}
	// Moving on from a moment keeps the time of day rather than rounding it
	// quietly to midnight.
	m = press(m, "right")
	if got := m.Text(); got != "2026-09-05 01:30" {
		t.Errorf("after moving = %q, want the same time of day", got)
	}
	if !strings.Contains(m.Keys(), "1 hour") {
		t.Errorf("Keys() = %q, want the shortcut named", m.Keys())
	}
}

// t goes back to today from wherever the cursor wandered.
func TestTodayGoesBack(t *testing.T) {
	m := press(fixed(t, "2026-09-04"), "]", "]", "right")
	m = press(m, "t")
	if got := m.Value().Format("2006-01-02"); got != "2026-09-04" {
		t.Errorf("t = %s, want today", got)
	}
}

// The picker consumes only its own keys, so the form around it keeps tab,
// enter and escape.
func TestItLeavesTheFormsKeysAlone(t *testing.T) {
	m := fixed(t, "2026-09-04")
	for _, key := range []string{"tab", "enter", "esc", "a"} {
		if _, handled := m.Update(tea.KeyPressMsg{Text: key}); handled {
			t.Errorf("the picker swallowed %q", key)
		}
	}
}
