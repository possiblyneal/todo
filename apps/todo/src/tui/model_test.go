package tui

import (
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

const ttl = time.Minute

// fixture is a store with two Lists, two Tags and four Tasks, enough for
// narrowing, sorting and a tree to have something to show.
func fixture(t *testing.T) *store.Store {
	t.Helper()

	s, err := store.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	add := func(id string, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("fixture: %v", err)
		}
		return id
	}

	home := add(s.AddList("alice", "Home", "blue"))
	work := add(s.AddList("alice", "Work", "green"))
	urgent := add(s.AddTag("alice", "urgent", "red"))
	slow := add(s.AddTag("alice", "slow", "grey"))

	roof := add(s.AddTask("alice", store.Attributes{
		Title:       store.Set("Fix the roof"),
		Description: store.Set("The felt on the north side lifted in the storm and the batten under it is soft."),
		Why:         store.Set("Water is getting in"),
		Deadline:    store.Set(time.Now().Add(48 * time.Hour)),
		Estimate:    store.Set(3 * time.Hour),
		Priority:    store.Set(store.LevelHigh),
		Impact:      store.Set(store.LevelHigh),
	}))
	invoice := add(s.AddTask("alice", store.Attributes{
		Title:    store.Set("Send the invoice"),
		Estimate: store.Set(20 * time.Minute),
		Deadline: store.Set(time.Now().Add(24 * time.Hour)),
	}))
	apples := add(s.AddTask("alice", store.Attributes{Title: store.Set("Buy apples")}))

	carry(t, s, roof, func() error {
		if _, err := s.AddSubtask("alice", roof, store.Attributes{Title: store.Set("Order felt")}); err != nil {
			return err
		}
		if err := s.AddToList("alice", roof, home); err != nil {
			return err
		}
		if err := s.AttachTag("alice", roof, urgent); err != nil {
			return err
		}
		return s.AttachTag("alice", roof, slow)
	})
	carry(t, s, invoice, func() error {
		if err := s.AddToList("alice", invoice, work); err != nil {
			return err
		}
		return s.AttachTag("alice", invoice, urgent)
	})
	carry(t, s, apples, func() error { return s.AddToList("alice", apples, home) })

	return s
}

func carry(t *testing.T, s *store.Store, taskID string, fn func() error) {
	t.Helper()
	if err := s.WithLease("alice", taskID, ttl, fn); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

func newModel(t *testing.T, s *store.Store) Model {
	t.Helper()
	m, err := New(s, "alice", rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func press(m Model, key string) Model {
	msg := tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	switch key {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "L":
		msg = tea.KeyPressMsg{Code: 'l', Text: "L", Mod: tea.ModShift}
	}
	return send(m, msg)
}

// send delivers a message and then runs whatever commands come back, feeding
// their messages in too. The searchbox matches through a command, so a test
// that drops commands never sees a filter take effect.
func send(m Model, msg tea.Msg) Model {
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		if batch, ok := head.(tea.BatchMsg); ok {
			for _, cmd := range batch {
				queue = append(queue, run(cmd)...)
			}
			continue
		}
		next, cmd := m.Update(head)
		m = next.(Model)
		queue = append(queue, run(cmd)...)
	}
	return m
}

// run gives a command a moment to produce its message. Some of them never do
// -- the cursor's blink waits on a channel the program owns -- so a test that
// waited on every command would hang instead of failing.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	produced := make(chan tea.Msg, 1)
	go func() { produced <- cmd() }()
	select {
	case msg := <-produced:
		if msg != nil {
			return []tea.Msg{msg}
		}
	case <-time.After(100 * time.Millisecond):
	}
	return nil
}

// clickOn renders, waits for the zone manager to catch up with the render, and
// clicks the middle of the named zone. Scan hands its marks to a worker, so a
// Get straight after a render can still be empty.
func clickOn(t *testing.T, m Model, id string) Model {
	t.Helper()
	m.View()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if z := m.zones.Get(id); z != nil {
			return send(m, tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
		}
		if time.Now().After(deadline) {
			t.Fatalf("zone %q was never drawn", id)
		}
		time.Sleep(time.Millisecond)
	}
}

func titles(m Model) []string {
	var out []string
	for _, item := range m.tasks.Items() {
		out = append(out, item.(row).task.Title)
	}
	return out
}

func idOf(t *testing.T, name string, lists []store.List, tags []store.Tag) string {
	t.Helper()
	for _, l := range lists {
		if l.Name == name {
			return l.ID
		}
	}
	for _, g := range tags {
		if g.Name == name {
			return g.ID
		}
	}
	t.Fatalf("no List or Tag named %q", name)
	return ""
}

// TestTheMainViewWritesNothing is the ticket's own assertion: reading and
// clicking around the main view leaves the Change History exactly as long as
// it was. Asserted, not assumed.
func TestTheMainViewWritesNothing(t *testing.T) {
	s := fixture(t)
	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}

	m := newModel(t, s)
	lists, tags := m.lists, m.tags
	home := idOf(t, "Home", lists, tags)
	urgent := idOf(t, "urgent", lists, tags)

	m = press(m, "s")
	m = press(m, "j")
	m = press(m, "enter")
	m = press(m, "esc")
	m = clickOn(t, m, "tag:"+urgent)
	m = clickOn(t, m, "tag:"+urgent)
	m = press(m, "L")
	m = clickOn(t, m, "list:"+home)
	m = press(m, "f")
	m = press(m, "r")
	m = press(m, "esc")
	m = press(m, "/")
	m = press(m, "esc")
	m.View()

	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after != before {
		t.Errorf("the Change History grew from %d to %d; a read wrote something", before, after)
	}
}

func TestChoosingATagNarrowsTheView(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	urgent := idOf(t, "urgent", m.lists, m.tags)

	all := len(m.tasks.Items())
	m = clickOn(t, m, "tag:"+urgent)
	if !m.chosen[urgent] {
		t.Fatal("clicking a Tag did not choose it")
	}
	narrowed := titles(m)
	if len(narrowed) >= all {
		t.Errorf("choosing a Tag showed %d of %d Tasks; it narrowed nothing", len(narrowed), all)
	}
	for _, want := range []string{"Fix the roof", "Send the invoice"} {
		if !contains(narrowed, want) {
			t.Errorf("narrowed to %v, want it to hold %q", narrowed, want)
		}
	}
	if contains(narrowed, "Buy apples") {
		t.Errorf("narrowed to %v, want the untagged Task gone", narrowed)
	}

	m = clickOn(t, m, "tag:"+urgent)
	if len(m.tasks.Items()) != all {
		t.Errorf("clicking the Tag again left %d Tasks, want the %d from before", len(m.tasks.Items()), all)
	}
}

func TestChoosingAListNarrowsTheView(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	work := idOf(t, "Work", m.lists, m.tags)

	m = press(m, "L")
	if !m.dropdown {
		t.Fatal("L did not open the List dropdown")
	}
	m = clickOn(t, m, "list:"+work)
	if m.dropdown {
		t.Error("choosing a List left the dropdown open")
	}
	if got := titles(m); len(got) != 1 || got[0] != "Send the invoice" {
		t.Errorf("the Work List showed %v, want just the invoice", got)
	}

	m = press(m, "L")
	m = clickOn(t, m, "list:every")
	if len(m.tasks.Items()) < 3 {
		t.Errorf("every List showed %d Tasks, want them all back", len(m.tasks.Items()))
	}
}

func TestSortCyclesTheFourOrders(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	seen := map[store.Sort]bool{m.sort: true}
	for range len(sorts) {
		m = press(m, "s")
		seen[m.sort] = true
	}
	if len(seen) != len(sorts) {
		t.Errorf("cycling reached %v, want all four of %v", seen, sorts)
	}

	for m.sort != store.SortTitle {
		m = press(m, "s")
	}
	got := titles(m)
	if len(got) < 3 || got[0] != "Buy apples" {
		t.Errorf("alphabetical showed %v, want it to start at Buy apples", got)
	}
}

func TestARowShowsWhatTheOperatorAskedFor(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	var roof row
	for _, item := range m.tasks.Items() {
		if r := item.(row); r.task.Title == "Fix the roof" {
			roof = r
		}
	}
	roof.attachments = 2

	drawn := rowDelegate{zones: m.zones, width: 80}.render(roof, true)
	if lines := strings.Count(drawn, "\n") + 1; lines != rowHeight {
		t.Errorf("a row drew %d lines, want %d", lines, rowHeight)
	}
	for _, want := range []string{"Fix the roof", "felt", "created", "due", "Home", paperclip} {
		if !strings.Contains(drawn, want) {
			t.Errorf("the row is missing %q:\n%s", want, drawn)
		}
	}
}

func TestSelectingATaskShowsEveryAttribute(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	for titles(m)[m.tasks.Index()] != "Fix the roof" {
		m = press(m, "j")
	}
	m = press(m, "enter")
	if !m.expanded {
		t.Fatal("enter did not expand the Task")
	}

	shown := m.View().Content
	for _, want := range []string{"Water is getting in", "high", "urgent", "slow", "Home", "3h0m0s"} {
		if !strings.Contains(shown, want) {
			t.Errorf("the expanded Task is missing %q:\n%s", want, shown)
		}
	}

	m = press(m, "esc")
	if m.expanded {
		t.Error("esc did not close the expanded Task")
	}
}

func TestTheSearchboxFiltersLive(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	m = press(m, "f")
	for _, key := range []string{"a", "p", "p"} {
		m = press(m, key)
	}
	got := m.tasks.VisibleItems()
	if len(got) != 1 || got[0].(row).task.Title != "Buy apples" {
		t.Errorf("searching \"app\" showed %d items, want just Buy apples", len(got))
	}
}

func TestATreeComesBackIndented(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	for _, item := range m.tasks.Items() {
		r := item.(row)
		if r.task.Title != "Order felt" {
			continue
		}
		if r.task.Depth != 2 {
			t.Fatalf("the Subtask came back at depth %d, want 2", r.task.Depth)
		}
		drawn := rowDelegate{zones: m.zones, width: 80}.render(r, false)
		if !strings.HasPrefix(drawn, "    ") {
			t.Errorf("the Subtask drew without an indent:\n%q", drawn)
		}
		return
	}
	t.Fatal("the Subtask never came back")
}
