package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// A deadline is picked. The field writes the calendar's answer straight into
// the draft, in the one layout the rest of the form reads back.
func TestTheDateFieldWritesWhatThePickerChose(t *testing.T) {
	var value string
	f := newDateField("Deadline", &value, snoozeShortcuts())
	f.Update(tea.KeyPressMsg{Code: 'l', Text: "right"})

	want := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if value != want {
		t.Errorf("the draft carries %q, want %q", value, want)
	}
	if _, err := parseDate(value); err != nil {
		t.Errorf("the form cannot read back what the picker wrote: %v", err)
	}

	// Space is "no deadline", which is how a date already set is taken off.
	f.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if value != "" {
		t.Errorf("clearing left %q behind", value)
	}
}

// An existing deadline opens the calendar on itself rather than on today, so
// editing a Task shows the date it already has.
func TestEditingOpensThePickerOnTheDateAlreadySet(t *testing.T) {
	value := "2026-11-01"
	f := newDateField("Deadline", &value, nil)
	if got := f.picked().Format("2006-01-02"); got != value {
		t.Errorf("the picker opened on %s, want %s", got, value)
	}
}

// The offered snoozes are the picker's shortcut keys, in the order the store
// offers them.
func TestTheSnoozesAreShortcutsOnTheCalendar(t *testing.T) {
	shortcuts := snoozeShortcuts()
	if len(shortcuts) != len(store.SnoozeDefaults) {
		t.Fatalf("%d shortcuts for %d snoozes", len(shortcuts), len(store.SnoozeDefaults))
	}
	now := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	for i, s := range shortcuts {
		if s.Key != string(rune('1'+i)) || s.Label != store.SnoozeDefaults[i].Label {
			t.Errorf("shortcut %d is %q %q", i, s.Key, s.Label)
		}
		if !s.When(now).Equal(store.SnoozeDefaults[i].Until(now)) {
			t.Errorf("%s jumped to %v", s.Label, s.When(now))
		}
	}
}

// The add screen shows a calendar where the typed date input used to be.
func TestTheAddScreenPicksTheDeadline(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m, _ = m.run("/add")
	if m.editor == nil {
		t.Fatal("/add opened no screen")
	}
	view := m.editor.View()
	if !strings.Contains(view, time.Now().Format("January 2006")) {
		t.Errorf("the add screen shows no calendar:\n%s", view)
	}
}

// /snooze opens the same calendar and hides the Task until what it settles on.
func TestSnoozeFromTheCalendarHidesTheTask(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	task, _ := m.selected()

	m, cmd := m.run("/snooze")
	if m.popup == nil || m.pop == nil || m.pop.Task != task.ID {
		t.Fatalf("/snooze opened no picker for %s", task.ID)
	}
	m = send(m, cmd())

	// "2" is the second offered snooze, and enter takes the one-field form.
	m = press(m, "2")
	m = press(m, "enter")

	if m.err != nil {
		t.Fatalf("snoozing: %v", m.err)
	}
	if m.popup != nil || m.pop != nil {
		t.Error("the picker stayed open after it was taken")
	}
	for _, item := range m.tasks.Items() {
		if item.(row).task.ID == task.ID {
			t.Error("a snoozed Task is still in the view")
		}
	}
	want := store.SnoozeDefaults[1].Until(time.Now())
	tasks, err := s.Tasks(store.Query{IncludeSnoozed: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	for _, got := range tasks {
		if got.ID != task.ID {
			continue
		}
		if diff := got.SnoozedUntil.Sub(want); diff > time.Minute || diff < -time.Minute {
			t.Errorf("snoozed until %v, want about %v", got.SnoozedUntil, want)
		}
	}
}
