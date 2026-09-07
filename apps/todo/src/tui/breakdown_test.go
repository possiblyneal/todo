package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// standIn is the box, played by an httptest server. Each call answers with the
// next reply in turn, which is how a two-turn conversation is written down.
func standIn(t *testing.T, replies ...string) *ai.Client {
	t.Helper()
	turn := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "{}"
		if turn < len(replies) {
			content = replies[turn]
		}
		turn++
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
		})
	}))
	t.Cleanup(srv.Close)
	return &ai.Client{BaseURL: srv.URL + "/v1", Model: "stand-in", HTTP: srv.Client()}
}

// onTask puts the cursor on a Task by title.
func onTask(t *testing.T, m Model, title string) Model {
	t.Helper()
	for range len(m.tasks.Items()) {
		if got, ok := m.selected(); ok && got.Title == title {
			return m
		}
		m = press(m, "j")
	}
	t.Fatalf("no Task titled %q in view", title)
	return m
}

func subtasksOf(t *testing.T, s *store.Store, parent string) []store.Task {
	t.Helper()
	all, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	var out []store.Task
	for _, got := range all {
		if got.Parent == parent {
			out = append(out, got)
		}
	}
	return out
}

const twoProposals = `{"proposals":[
	{"title":"Order felt","estimate":"30m","priority":"high"},
	{"title":"Strip the old felt","estimate":"2h","impact":"med"}]}`

// TestTheBoxAsksBeforeItProposes is the whole interaction: it asks for what it
// needs, the answer goes back with the next turn, and what comes back is
// proposals waiting on somebody.
func TestTheBoxAsksBeforeItProposes(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, `{"questions":["Which side of the roof?"]}`, twoProposals)
	m = onTask(t, m, "Buy apples")

	m, cmd := m.run("/breakdown")
	m = send(m, cmd())
	if m.bd == nil {
		t.Fatalf("the breakdown did not open: %v", m.err)
	}
	if len(m.bd.asking) != 1 || m.bd.asking[0] != "Which side of the roof?" {
		t.Fatalf("the box asked %v, want its one question", m.bd.asking)
	}

	for _, key := range []string{"N", "o", "r", "t", "h", "enter"} {
		m = press(m, key)
	}
	if len(m.bd.answers) != 1 || m.bd.answers[0].Answer != "North" {
		t.Fatalf("the turn carried %v, want the answer that was typed", m.bd.answers)
	}
	if len(m.bd.proposals) != 2 {
		t.Fatalf("the box proposed %v, want the two", m.bd.proposals)
	}
	if !strings.Contains(m.View().Content, "Order felt") {
		t.Error("the proposals were not put on screen to be approved")
	}
}

// TestNothingIsWrittenWithoutApproval. Proposals reach the screen and the
// interaction is abandoned; nothing durable survives it.
func TestNothingIsWrittenWithoutApproval(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, twoProposals)
	m = onTask(t, m, "Buy apples")
	apples, _ := m.selected()

	before := historyLength(t, s)
	m, cmd := m.run("/breakdown")
	m = send(m, cmd())
	if len(m.bd.proposals) != 2 {
		t.Fatalf("the proposals never arrived: %v", m.err)
	}

	m = press(m, "esc")
	if m.bd != nil {
		t.Error("escape left the breakdown open")
	}
	if got := subtasksOf(t, s, apples.ID); len(got) != 0 {
		t.Errorf("abandoning the breakdown wrote %d Subtasks", len(got))
	}
	// Lease Taken and Lease Released, and nothing else: a proposal nobody
	// approved is not in the Change History at all.
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, e := range entries[before:] {
		if e.Kind != store.KindLeaseTaken && e.Kind != store.KindLeaseReleased {
			t.Errorf("an abandoned breakdown appended %s", e.Kind)
		}
	}
}

// TestApprovalWritesOnlyWhatWasTicked. Unticking is how a proposal is
// declined, and a declined one leaves nothing behind either.
func TestApprovalWritesOnlyWhatWasTicked(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, twoProposals)
	m = onTask(t, m, "Buy apples")
	apples, _ := m.selected()

	m, cmd := m.run("/breakdown")
	m = send(m, cmd())
	if len(m.bd.proposals) != 2 {
		t.Fatalf("the proposals never arrived: %v", m.err)
	}

	// The cursor starts on the first proposal; space unticks it.
	m = press(m, " ")
	m = press(m, "enter")
	if m.err != nil {
		t.Fatalf("approving: %v", m.err)
	}
	if m.bd != nil {
		t.Error("approving left the breakdown open")
	}

	got := subtasksOf(t, s, apples.ID)
	if len(got) != 1 || got[0].Title != "Strip the old felt" {
		t.Fatalf("approval wrote %v, want only the ticked proposal", titlesIn(got))
	}
	if got[0].Estimate.String() != "2h0m0s" || got[0].Impact != store.LevelMed {
		t.Errorf("the Subtask came back %+v, want the attributes proposed with it", got[0])
	}
}

// TestTheTreeIsUnwritableForTheDuration is ADR 0002's cost, paid where the
// ticket says it is paid: the whole top-level tree, for as long as the
// breakdown is open.
func TestTheTreeIsUnwritableForTheDuration(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, twoProposals)
	m = onTask(t, m, "Fix the roof")
	roof, _ := m.selected()

	m, cmd := m.run("/breakdown")
	m = send(m, cmd())
	if m.bd == nil {
		t.Fatalf("the breakdown did not open: %v", m.err)
	}

	// Every Task in the tree, not only the one being broken down.
	for _, child := range subtasksOf(t, s, roof.ID) {
		if err := s.CompleteTask("bob", child.ID); err == nil {
			t.Errorf("bob wrote to %q while a breakdown held the tree", child.Title)
		}
	}
	if _, err := s.TakeLease("bob", roof.ID, ttl); err == nil {
		t.Error("bob took the Lease while a breakdown held it")
	}

	m = press(m, "esc")
	if _, err := s.TakeLease("bob", roof.ID, ttl); err != nil {
		t.Errorf("the Lease was still held after the breakdown ended: %v", err)
	}
}

// TestAQuestionAboutTheListWritesNothing. `/ask` is a read: the list goes to
// the box with the question and the prose comes back on screen.
func TestAQuestionAboutTheListWritesNothing(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, "The roof, then the invoice.")

	before := historyLength(t, s)
	m, cmd := m.run("/ask")
	m = send(m, cmd())
	for _, key := range []string{"W", "h", "a", "t", "enter"} {
		m = press(m, key)
	}
	if m.err != nil {
		t.Fatalf("asking: %v", m.err)
	}
	if m.ask == nil || m.ask.Answer != "The roof, then the invoice." {
		t.Fatalf("the answer came back %+v", m.ask)
	}
	if !strings.Contains(m.View().Content, "The roof, then the invoice.") {
		t.Error("the answer was not drawn")
	}
	if got := historyLength(t, s) - before; got != 0 {
		t.Errorf("asking a question appended %d entries", got)
	}

	m = press(m, "enter")
	if m.ask != nil {
		t.Error("a key did not close the answer")
	}
}

func titlesIn(tasks []store.Task) []string {
	var out []string
	for _, t := range tasks {
		out = append(out, t.Title)
	}
	return out
}

// sameTitleTwice is what a box that repeats itself sends back. Nothing stops
// it, and the two are different proposals with different attributes.
const sameTitleTwice = `{"proposals":[
	{"title":"Order felt","estimate":"30m"},
	{"title":"Order felt","estimate":"4h"}]}`

// TestDecliningOneOfTwoWithTheSameTitleWritesOnlyTheOther is the gate holding
// where a title cannot tell two proposals apart. Approval is by position:
// matching on the title wrote both when one was declined, which is the one
// thing the approval step exists to prevent.
func TestDecliningOneOfTwoWithTheSameTitleWritesOnlyTheOther(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	m.ai = standIn(t, sameTitleTwice)
	m = onTask(t, m, "Buy apples")
	apples, _ := m.selected()

	m, cmd := m.run("/breakdown")
	m = send(m, cmd())
	if len(m.bd.proposals) != 2 {
		t.Fatalf("the proposals never arrived: %v", m.err)
	}

	// The cursor starts on the first; space unticks it.
	m = press(m, " ")
	m = press(m, "enter")
	if m.err != nil {
		t.Fatalf("approving: %v", m.err)
	}

	got := subtasksOf(t, s, apples.ID)
	if len(got) != 1 {
		t.Fatalf("approval wrote %d Subtasks, want the one that stayed ticked", len(got))
	}
	if got[0].Estimate.String() != "4h0m0s" {
		t.Errorf("the Subtask came back with a %v estimate, want the second proposal's 4h",
			got[0].Estimate)
	}
}
