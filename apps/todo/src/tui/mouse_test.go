package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
// blow it up, which is what leaves a Task reachable by mouse for a verb
// without the detail pane opening over the list first.
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
	m = clickOn(t, m, "key:s")
	if m.sort == was {
		t.Errorf("clicking sort left the order at %q", m.sort)
	}

	m = clickOn(t, m, "key:h")
	if !m.snoozed {
		t.Error("clicking hidden did not show the Tasks that are away")
	}

	m = clickOn(t, m, "key:l")
	if !m.dropdown {
		t.Error("clicking lists did not open the dropdown")
	}
	m = clickOn(t, m, "key:l")
	if m.dropdown {
		t.Error("clicking lists again did not close the dropdown")
	}

	// A Task verb is a hit box too, and the snooze it opens is a form: it
	// asks before it writes, so clicking it keeps this a read.
	m = clickOn(t, m, "key:z")
	if m.popup == nil {
		t.Error("clicking snooze opened no field")
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

// The searchbox narrows the list, and a click has to mean the Task drawn under
// it rather than whatever sits at that index in the unfiltered list.
func TestAClickChoosesTheRightTaskUnderASearch(t *testing.T) {
	m := newModel(t, fixture(t))

	m = press(m, "/")
	for _, r := range "apples" {
		m = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = press(m, "enter")

	visible := m.tasks.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("the search left %d Tasks in view, want the one", len(visible))
	}
	wanted := visible[0].(row)
	if len(m.tasks.Items()) == len(visible) {
		t.Fatal("the search narrowed nothing, so the click cannot go wrong either way")
	}

	m = clickOn(t, m, "task:"+wanted.task.ID)
	got, ok := m.tasks.SelectedItem().(row)
	if !ok || got.task.ID != wanted.task.ID {
		t.Errorf("the click chose %q, want %q", got.task.Title, wanted.task.Title)
	}
}

// A Task blown up covers the rows, so there is nothing there for the wheel to
// scroll and it leaves the detail on the Task it was opened on.
func TestTheWheelLeavesABlownUpTaskAlone(t *testing.T) {
	m := newModel(t, fixture(t))

	m = press(m, "enter")
	if !m.expanded {
		t.Fatal("enter did not blow the Task up")
	}

	m = wheel(m, tea.MouseWheelDown)
	if m.tasks.Index() != 0 {
		t.Errorf("the wheel moved the cursor to %d behind the detail pane, want 0", m.tasks.Index())
	}
	if !m.expanded {
		t.Error("the wheel closed the detail pane")
	}
}

// The dropdown is drawn over a blown-up Task, so it is the dropdown that gets
// the click rather than the way back to the list.
func TestTheDropdownTakesItsOwnClicksOverABlownUpTask(t *testing.T) {
	m := newModel(t, fixture(t))

	m = press(m, "enter")
	m = clickOn(t, m, "key:l")
	if !m.expanded || !m.dropdown {
		t.Fatalf("wanted the dropdown open over the blown-up Task; expanded=%v dropdown=%v", m.expanded, m.dropdown)
	}

	var home string
	for _, l := range m.lists {
		if l.Name == "Home" {
			home = l.ID
		}
	}
	m = clickOn(t, m, "list:"+home)
	if m.list != home {
		t.Errorf("the click chose List %q, want Home", m.list)
	}
	if m.dropdown {
		t.Error("choosing a List left the dropdown open")
	}
}

// A search matching nothing is the list's own line to say, because it is the
// only one that can say how to clear the search. bubbles clears a search that
// accepts with no matches, so the state this guards is mid-typing.
func TestASearchMatchingNothingIsNotTheEmptyState(t *testing.T) {
	m := newModel(t, fixture(t))

	m = press(m, "/")
	for _, r := range "zzzz" {
		m = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	if len(m.tasks.VisibleItems()) != 0 {
		t.Fatalf("the search matched %d Tasks; it was meant to match none", len(m.tasks.VisibleItems()))
	}
	if drawn := m.View().Content; strings.Contains(drawn, "Nothing to do") {
		t.Error("a search matching nothing drew the empty state, which says to add the first Task")
	}
}

// The sidebar's state rows are hit boxes like the Tags under them.
func TestTheSidebarStatesAreClickable(t *testing.T) {
	m := newModel(t, fixture(t))

	m = clickOn(t, m, "state:done")
	if !m.done {
		t.Error("clicking done did not bring the completed Tasks into view")
	}
	m = clickOn(t, m, "state:snoozed")
	if !m.snoozed || !m.done {
		t.Errorf("clicking snoozed left snoozed=%v done=%v, want both", m.snoozed, m.done)
	}
	m = clickOn(t, m, "state:done")
	if m.done {
		t.Error("clicking done again left it on")
	}
}

// Nothing is drawn past the terminal's last column, however narrow the window
// is: a row that overran its pane would push the sidebar's rule off the edge.
func TestNothingIsDrawnPastTheEdgeOfANarrowTerminal(t *testing.T) {
	m := newModel(t, fixture(t))
	m = m.resize(tea.WindowSizeMsg{Width: 40, Height: 20})

	for i, line := range strings.Split(m.View().Content, "\n") {
		if got := lipgloss.Width(line); got > 40 {
			t.Errorf("line %d is %d cells wide in a terminal of 40: %q", i, got, line)
		}
	}
}
