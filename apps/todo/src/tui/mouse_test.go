package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// wheel scrolls the view the way a mouse does, one notch.
func wheel(m Model, button tea.MouseButton) Model {
	return send(m, tea.MouseWheelMsg{Button: button})
}

func TestTheWheelMovesTheCursor(t *testing.T) {
	m := newModel(t, fixture(t))
	if len(m.tasks.Items()) < 2 {
		t.Fatalf("the fixture has %d Tasks; the wheel needs two to move between", len(m.tasks.Items()))
	}

	m = wheel(m, tea.MouseWheelDown)
	if m.tasks.Index() != 1 {
		t.Errorf("a notch down left the cursor at %d, want 1", m.tasks.Index())
	}
	m = wheel(m, tea.MouseWheelUp)
	if m.tasks.Index() != 0 {
		t.Errorf("a notch back up left the cursor at %d, want 0", m.tasks.Index())
	}
}

// A click puts the cursor on a Task. It takes a second one on the same Task to
// blow it up, which is what leaves a Task reachable by mouse for a slash
// command without the detail pane opening over the list first.
func TestAClickChoosesATaskAndASecondOpensIt(t *testing.T) {
	m := newModel(t, fixture(t))
	var second row
	for i, item := range m.tasks.Items() {
		if i == 1 {
			second = item.(row)
		}
	}

	m = clickOn(t, m, "task:"+second.task.ID)
	if m.tasks.Index() != 1 {
		t.Errorf("a click left the cursor at %d, want it on the Task clicked", m.tasks.Index())
	}
	if m.expanded {
		t.Error("one click blew the Task up; it should take a second")
	}

	m = clickOn(t, m, "task:"+second.task.ID)
	if !m.expanded {
		t.Error("a second click on the same Task did not blow it up")
	}

	m = clickOn(t, m, "task:"+second.task.ID)
	if m.expanded {
		t.Error("a click with the Task blown up did not go back to the list")
	}
}

// Every key named in the footer is also a hit box, and clicking it does what
// the key does. Nothing there writes, so this stays a read.
func TestTheFooterKeysAreClickable(t *testing.T) {
	m := newModel(t, fixture(t))

	was := m.sort
	m = clickOn(t, m, "hint:sort")
	if m.sort == was {
		t.Errorf("clicking sort left the order at %q", m.sort)
	}

	m = clickOn(t, m, "hint:snoozed")
	if !m.snoozed {
		t.Error("clicking snoozed did not show the Tasks that are away")
	}

	m = clickOn(t, m, "hint:lists")
	if !m.dropdown {
		t.Error("clicking lists did not open the dropdown")
	}
	m = clickOn(t, m, "hint:lists")
	if m.dropdown {
		t.Error("clicking lists again did not close the dropdown")
	}

	m = clickOn(t, m, "hint:commands")
	if !m.paletteOpen {
		t.Error("clicking commands did not open the palette")
	}
}

// The sort in the header is the same hit box as the footer's key.
func TestTheSortInTheHeaderIsClickable(t *testing.T) {
	m := newModel(t, fixture(t))
	was := m.sort
	m = clickOn(t, m, "sortbox")
	if m.sort == was {
		t.Errorf("clicking the header's sort left the order at %q", m.sort)
	}
}

// A palette entry runs on the click that lands on it, with no second one:
// a verb is not something a cursor is parked on.
func TestAClickRunsAPaletteCommand(t *testing.T) {
	m := newModel(t, fixture(t))
	was := m.sort

	m = press(m, "/")
	m = clickOn(t, m, "command:/sort")
	if m.paletteOpen {
		t.Error("running a command by click left the palette open")
	}
	if m.sort == was {
		t.Errorf("clicking /sort left the order at %q", m.sort)
	}
	if m.sort != nextSort(was) {
		t.Errorf("clicking /sort ordered by %q, want %q", m.sort, nextSort(was))
	}
}

// The wheel moves the palette's cursor too, and moving it writes nothing.
func TestTheWheelMovesThePaletteCursor(t *testing.T) {
	s := fixture(t)
	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	m := newModel(t, s)

	m = press(m, "/")
	m = wheel(m, tea.MouseWheelDown)
	if m.palette.Index() != 1 {
		t.Errorf("a notch down left the palette's cursor at %d, want 1", m.palette.Index())
	}

	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after != before {
		t.Errorf("the Change History grew from %d to %d; scrolling wrote something", before, after)
	}
}

// The sidebar's rule runs the height of the Tasks' pane rather than stopping
// under the last Tag, so the two columns read as two columns.
func TestTheSidebarIsDrawnTheHeightOfThePane(t *testing.T) {
	m := newModel(t, fixture(t))
	m = m.resize(tea.WindowSizeMsg{Width: 100, Height: 30})

	drawn := m.View().Content
	if got := lines(drawn); got != 30 {
		t.Errorf("the screen drew %d lines into a terminal of 30", got)
	}
}
