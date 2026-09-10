package tui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// typeIn puts a line into whatever has the keyboard, a key at a time.
func typeIn(m Model, text string) Model {
	for _, r := range text {
		m = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// done is ctrl+d, which is what hands a dump to the broker. Enter is a new
// line in that box, so it cannot be the key that submits it.
func done(m Model) Model {
	return send(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
}

// addScreen is the add form on its own: the box /add opens, left empty, which
// is the way past the broker for a Task somebody would rather type in.
func addScreen(m Model) Model {
	m, cmd := m.run("/add")
	for _, msg := range run(cmd) {
		m = send(m, msg)
	}
	return done(m)
}

const dentist = `{"title":"Call the dentist","description":"about the crown",
	"deadline":"2026-11-02 11:00","estimate":"20m","impact":"high",
	"lists":["Home"],"tags":["urgent","made up"]}`

// The whole of the add screen: one box, the broker, and the form filled in
// waiting on somebody. Nothing is written until that form is submitted.
func TestADumpFillsTheFormInAndWritesNothingOnItsOwn(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, dentist)
	before := historyLength(t, s)

	m, cmd := m.run("/add")
	m = send(m, cmd())
	if m.capturing == nil {
		t.Fatalf("the box did not open: %v", m.err)
	}
	m = typeIn(m, "dentist about the crown")
	m = done(m)

	if m.capturing != nil {
		t.Fatalf("the box is still open after the broker read it: %v", m.err)
	}
	if m.draft == nil || m.editor == nil {
		t.Fatalf("the form did not open on what the broker read: %v", m.err)
	}
	if m.draft.Title != "Call the dentist" || m.draft.Description != "about the crown" {
		t.Errorf("the draft reads %+v, want the title and description the broker filled in", m.draft)
	}
	if m.draft.Deadline != "2026-11-02 11:00" || m.draft.Estimate != "20m0s" {
		t.Errorf("the draft carries deadline %q and estimate %q, want the ones read out of the dump",
			m.draft.Deadline, m.draft.Estimate)
	}
	if m.draft.Impact != store.LevelHigh {
		t.Errorf("the draft carries impact %q, want high", m.draft.Impact)
	}

	// A List and a Tag come back by name and are carried by id. A name that
	// is not one of the offered ones is dropped rather than created.
	home := idOf(t, "Home", m.lists, m.tags)
	urgent := idOf(t, "urgent", m.lists, m.tags)
	if !slices.Equal(m.draft.Lists, []string{home}) {
		t.Errorf("the draft is in Lists %v, want Home by id", m.draft.Lists)
	}
	if !slices.Equal(m.draft.Tags, []string{urgent}) {
		t.Errorf("the draft carries Tags %v, want only the Tag that exists", m.draft.Tags)
	}

	if after := historyLength(t, s); after != before {
		t.Errorf("reading a dump appended %d entries, want none until the form is submitted", after-before)
	}
}

// The box is the way in, not a wall: an empty one opens the form itself, which
// is how a Task is typed in when there is nothing to say about it.
func TestAnEmptyBoxOpensTheFormItself(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, dentist)

	m, cmd := m.run("/add")
	m = send(m, cmd())
	m = done(m)

	if m.capturing != nil || m.editor == nil {
		t.Fatalf("an empty box did not open the form: %v", m.err)
	}
	if m.draft == nil || m.draft.Title != "" {
		t.Errorf("the draft reads %+v, want an empty one", m.draft)
	}
}

// A dump onto an existing Task changes what it mentions and leaves the rest of
// the Task alone. The broker is shown the Task and answers with the whole of
// it, so the form opens on a Task, not on a difference.
func TestADumpOntoATaskLeavesWhatItDoesNotMention(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, `{"title":"Fix the roof","why":"Water is getting in",
		"description":"The felt on the north side lifted in the storm and the batten under it is soft.",
		"priority":"low"}`)
	m = onTask(t, m, "Fix the roof")
	was, _ := m.selected()

	m, cmd := m.run("/edit")
	m = send(m, cmd())
	if m.capturing == nil || m.capturing.onto == nil {
		t.Fatalf("the box did not open on the Task under the cursor: %v", m.err)
	}
	m = typeIn(m, "drop it to low priority")
	m = done(m)

	if m.draft == nil {
		t.Fatalf("the form did not open: %v", m.err)
	}
	if m.draft.taskID != was.ID {
		t.Errorf("the draft edits %q, want the Task the box was opened on", m.draft.taskID)
	}
	if m.draft.Priority != store.LevelLow {
		t.Errorf("the draft carries priority %q, want the one the dump asked for", m.draft.Priority)
	}
	// The estimate and the deadline were never mentioned, so they are the
	// Task's own: an answer left empty leaves what was already there.
	if m.draft.Estimate != was.Estimate.String() {
		t.Errorf("the draft carries estimate %q, want the Task's %q", m.draft.Estimate, was.Estimate)
	}
	if m.draft.Deadline == "" {
		t.Error("the draft lost the deadline the Task already had")
	}
	if !slices.Equal(m.draft.Tags, was.Tags) {
		t.Errorf("the draft carries Tags %v, want the ones the Task already had", m.draft.Tags)
	}
}

// Escaping is the whole of taking it back. The broker may still answer, and
// the answer is dropped rather than filling in a screen nobody is on.
func TestEscapingTheBoxDropsWhatComesBack(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, dentist)

	m, cmd := m.run("/add")
	m = send(m, cmd())
	asked := m.capturing
	m = press(m, "esc")
	if m.capturing != nil {
		t.Fatal("escaping left the box open")
	}

	m = send(m, readMsg{cap: asked, read: ai.Capture{Title: "Call the dentist"}})
	if m.editor != nil || m.draft != nil {
		t.Error("an answer to an abandoned dump opened the form")
	}
}

// A broker that cannot be reached is a sentence in the footer, not a Task
// half-written.
func TestABrokerThatFailsLeavesNothingOpen(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, "not a task at all")

	m, cmd := m.run("/add")
	m = send(m, cmd())
	m = typeIn(m, "something")
	m = done(m)

	if m.capturing != nil || m.editor != nil {
		t.Fatal("a broker that answered with prose left a screen open")
	}
	if m.err == nil || !strings.Contains(m.err.Error(), "not a task") {
		t.Errorf("the footer says %v, want what the broker did wrong", m.err)
	}
}

// ctrl+d is a submit here and a delete-forward in the textarea underneath, so
// the key cannot be allowed to reach the form: handing a dump over with the
// cursor anywhere but the end would otherwise eat the character under it.
func TestHandingADumpOverKeepsEveryCharacterOfIt(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent = string(body)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": dentist}}},
		})
	}))
	t.Cleanup(srv.Close)
	m.ai = &ai.Client{BaseURL: srv.URL + "/v1", Model: "stand-in", HTTP: srv.Client()}

	m, cmd := m.run("/add")
	m = send(m, cmd())
	m = typeIn(m, "dentist about the crown")
	m = send(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = send(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = done(m)

	if !strings.Contains(sent, "dentist about the crown") {
		t.Errorf("the broker was sent %q, want every character that was typed", sent)
	}
}

// The same name said twice is one id. The broker choosing "Home" and "home"
// would otherwise carry the List twice and append the membership twice.
func TestANameSaidTwiceIsCarriedOnce(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, `{"title":"Call the dentist","lists":["Home","home"],"tags":["urgent","Urgent"]}`)

	m, cmd := m.run("/add")
	m = send(m, cmd())
	m = typeIn(m, "dentist")
	m = done(m)

	if m.draft == nil {
		t.Fatalf("the form did not open: %v", m.err)
	}
	if !slices.Equal(m.draft.Lists, []string{idOf(t, "Home", m.lists, m.tags)}) {
		t.Errorf("the draft is in Lists %v, want Home once", m.draft.Lists)
	}
	if !slices.Equal(m.draft.Tags, []string{idOf(t, "urgent", m.lists, m.tags)}) {
		t.Errorf("the draft carries Tags %v, want urgent once", m.draft.Tags)
	}
}
