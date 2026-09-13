package tui

import (
	"strings"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// A color is chosen from the ten, not typed, and "none" is on the same list.
func TestTheFormOffersTheColors(t *testing.T) {
	options := colorOptions()
	if len(options) != len(store.Colors)+1 {
		t.Fatalf("%d options for %d colors and none", len(options), len(store.Colors))
	}
	if options[0].Value != "" {
		t.Errorf("the first option is %q, want none", options[0].Value)
	}
	for i, c := range store.Colors {
		if got := options[i+1].Value; got != c.Name {
			t.Errorf("option %d is %q, want %q", i+1, got, c.Name)
		}
	}
}

// A Task's color is on its title in the list, which is the whole point of
// carrying one.
func TestARowIsPaintedInItsColor(t *testing.T) {
	painted := rowDelegate{width: 60}.render(row{task: store.Task{
		Title: "Paint the shed", Depth: 1, Color: "green",
	}}, false)
	plain := rowDelegate{width: 60}.render(row{task: store.Task{
		Title: "Paint the shed", Depth: 1,
	}}, false)

	green, ok := colorStyle("green")
	if !ok {
		t.Fatal("green is not one of the offered colors")
	}
	if !strings.Contains(painted, green.Bold(true).Render("Paint the shed")) {
		t.Errorf("the row is not painted green:\n%q", painted)
	}
	if painted == plain {
		t.Errorf("a colored row draws the same as an uncolored one:\n%q", painted)
	}
}

// The cursor's own style wins, so selection stays readable on any color.
func TestTheSelectedRowKeepsTheCursorColor(t *testing.T) {
	selected := rowDelegate{width: 60}.render(row{task: store.Task{
		Title: "Paint the shed", Depth: 1, Color: "green",
	}}, true)
	if !strings.Contains(selected, selectedStyle.Render("Paint the shed")) {
		t.Errorf("the selected row lost the cursor's style:\n%q", selected)
	}
}
