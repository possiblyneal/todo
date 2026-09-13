package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// A date is typed the way a person writes one: slashes, either year length,
// either clock, and the offered snoozes by name.
func TestADateIsTypedTheWayAPersonWritesOne(t *testing.T) {
	want := time.Date(2025, 4, 4, 9, 34, 0, 0, time.Local)
	for _, typed := range []string{
		"4/4/25 9:34 AM", "04/04/2025 09:34 am", "4/4/2025 9:34 AM", "4/4/25 09:34",
	} {
		got, err := parseDate(typed)
		if err != nil {
			t.Errorf("%q was refused as a date: %v", typed, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("%q read as %v, want %v", typed, got, want)
		}
	}

	// The layouts the form writes back are still read back.
	for _, typed := range []string{"", "2026-01-02", "2026-01-02 15:04", "4/4/25"} {
		if _, err := parseDate(typed); err != nil {
			t.Errorf("%q was refused as a date: %v", typed, err)
		}
	}
	for _, bad := range []string{"4-4-25", "13/1/25", "tuesday"} {
		if _, err := parseDate(bad); err == nil {
			t.Errorf("%q was accepted as a date", bad)
		}
	}
}

// The offered snoozes are typed by name, which is what the calendar's
// shortcut keys were.
func TestTheOfferedSnoozesAreTypedByName(t *testing.T) {
	now := time.Now()
	for _, s := range store.SnoozeDefaults {
		got, err := parseDate(s.Label)
		if err != nil {
			t.Errorf("%q was refused as a date: %v", s.Label, err)
			continue
		}
		if diff := got.Sub(s.Until(now)); diff > time.Minute || diff < -time.Minute {
			t.Errorf("%q read as %v, want about %v", s.Label, got, s.Until(now))
		}
	}
}

// /snooze opens the one field and hides the Task until what is typed into it.
func TestSnoozeHidesTheTaskUntilWhatIsTyped(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	task, _ := m.selected()

	m, cmd := m.run("/snooze")
	if m.popup == nil || m.pop == nil || m.pop.Task != task.ID {
		t.Fatalf("/snooze opened no field for %s", task.ID)
	}
	m = send(m, cmd())

	m = typeIn(m, "1 week")
	m = press(m, "enter")

	if m.err != nil {
		t.Fatalf("snoozing: %v", m.err)
	}
	if m.popup != nil || m.pop != nil {
		t.Error("the field stayed open after it was taken")
	}
	for _, item := range m.tasks.Items() {
		if item.(row).task.ID == task.ID {
			t.Error("a snoozed Task is still in the view")
		}
	}

	want := store.SnoozeDefaults[2].Until(time.Now())
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

// The add screen asks for a deadline in the field a person types into.
func TestTheAddScreenAsksForATypedDeadline(t *testing.T) {
	m := addScreen(newModel(t, fixture(t)))
	if m.editor == nil {
		t.Fatal("/add opened no screen")
	}
	view := m.editor.View()
	if !strings.Contains(view, "Deadline") || !strings.Contains(view, "9:34 AM, 2026-01-02") {
		t.Errorf("the add screen does not ask for a typed deadline:\n%s", view)
	}
	if strings.Contains(view, time.Now().Format("January 2006")) {
		t.Errorf("the add screen still draws a calendar:\n%s", view)
	}
	// Nothing is hiding a Task that does not exist yet.
	if strings.Contains(view, "Snooze until") {
		t.Errorf("the add screen asks a new Task when to hide:\n%s", view)
	}
}

// The edit screen keeps the snooze, because a Task already in the way is the
// one somebody wants out of it.
func TestTheEditScreenStillAsksForASnooze(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	task, _ := m.selected()

	form := m.form(draftOf(task))
	form.Init()
	view := form.View()
	if !strings.Contains(view, "Snooze until") {
		t.Errorf("the edit screen dropped the snooze:\n%s", view)
	}
}
