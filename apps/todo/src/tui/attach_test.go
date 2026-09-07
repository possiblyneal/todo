package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// The add screen brings up the file selector, and what it picks lands on the
// draft as a pointer. Nothing is read and nothing is copied: the path is the
// whole of it.
func TestTheAddScreenPicksAFileToPointAt(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	dir := t.TempDir()
	file := filepath.Join(dir, "quote.pdf")
	if err := os.WriteFile(file, []byte("%PDF"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}

	m, _ = m.run("/add")
	m.draft.Title = "Fix the gutter"
	m = press(m, "ctrl+a")
	if m.files == nil {
		t.Fatal("ctrl+a opened no file selector")
	}

	// Walk to the file the way a person does: into the directory, then onto
	// the file itself.
	m.files.picker.CurrentDirectory = dir
	m = send(m, m.files.picker.Init()())
	m = press(m, "enter")

	if m.files != nil {
		t.Error("the selector stayed open after a file was picked")
	}
	if want := []string{file}; len(m.draft.Attachments) != 1 || m.draft.Attachments[0] != want[0] {
		t.Fatalf("the draft carries %v, want %v", m.draft.Attachments, want)
	}
	if err := m.save(m.draft); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	saved, ok := byTitle(m, "Fix the gutter")
	if !ok {
		t.Fatal("the Task was not saved")
	}
	if len(saved.Attachments) != 1 || saved.Attachments[0] != file {
		t.Errorf("the Task carries %v", saved.Attachments)
	}
}

// A web address is a pointer too, and it is typed rather than picked: the file
// selector only walks the filesystem.
func TestATypedAddressIsAttachedAndUntickingTakesOneOff(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	roof, ok := byTitle(m, "Fix the roof")
	if !ok {
		t.Fatal("no Fix the roof in the fixture")
	}
	d := draftOf(roof)
	d.Attach = "https://example.com/felt-spec"
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	roof, _ = byTitle(m, "Fix the roof")
	if len(roof.Attachments) != 2 {
		t.Fatalf("the Task carries %v, want both pointers", roof.Attachments)
	}

	// The form shows every pointer ticked, so unticking one is how it comes
	// off. A draft saved with one of them dropped detaches exactly that one.
	d = draftOf(roof)
	d.Attachments = []string{roof.Attachments[1]}
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	roof, _ = byTitle(m, "Fix the roof")
	if len(roof.Attachments) != 1 || roof.Attachments[0] != d.Attachments[0] {
		t.Errorf("the Task carries %v, want just %v", roof.Attachments, d.Attachments)
	}
}

// The paperclip says a Task holds pointers, and nothing else does.
func TestThePaperclipFollowsThePointers(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	draw := func(title string) string {
		t.Helper()
		for _, item := range m.tasks.Items() {
			if r := item.(row); r.task.Title == title {
				return rowDelegate{zones: m.zones, width: 80}.render(r, false)
			}
		}
		t.Fatalf("no row titled %q", title)
		return ""
	}
	if !strings.Contains(draw("Fix the roof"), paperclip) {
		t.Error("a Task holding a pointer drew no paperclip")
	}
	if strings.Contains(draw("Buy apples"), paperclip) {
		t.Error("a Task holding none drew a paperclip")
	}

	// Saving a Task whose pointers were not touched touches no pointer: the
	// write path appends only the differences.
	before := historyLength(t, s)
	roof, _ := byTitle(m, "Fix the roof")
	if err := m.save(draftOf(roof)); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, e := range entries[before:] {
		if strings.HasPrefix(e.Kind, "attachment_") {
			t.Errorf("saving an untouched Task appended %s", e.Kind)
		}
	}
}

// Escape leaves the selector without pointing at anything.
func TestEscapeClosesTheSelector(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	m, _ = m.run("/add")
	m = press(m, "ctrl+a")
	if m.files == nil {
		t.Fatal("ctrl+a opened no file selector")
	}
	m = send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.files != nil {
		t.Error("escape left the selector open")
	}
	if len(m.draft.Attachments) != 0 {
		t.Errorf("escaping attached %v", m.draft.Attachments)
	}
	if m.editor == nil {
		t.Error("escaping the selector closed the form behind it")
	}
}

func byTitle(m Model, title string) (store.Task, bool) {
	for _, item := range m.tasks.Items() {
		if r := item.(row); r.task.Title == title {
			return r.task, true
		}
	}
	return store.Task{}, false
}
