package store

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDecliningEndsATaskWithoutCompletingIt(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Review the deck")})

	if err := s.DeclineTask("alice", id); err != nil {
		t.Fatalf("DeclineTask: %v", err)
	}
	if tasks, _ := s.Tasks(Query{}); len(tasks) != 0 {
		t.Errorf("a declined Task is still in the everyday view: %d tasks", len(tasks))
	}

	got := only(t, s, Query{IncludeDeclined: true})
	if got.DeclinedAt.IsZero() {
		t.Error("declining left no time on the Task")
	}
	if !got.CompletedAt.IsZero() {
		t.Error("declining completed the Task")
	}
	if !got.DeletedAt.IsZero() {
		t.Error("declining deleted the Task")
	}
	if marks := got.Marks(); !slices.Contains(marks, "declined") || slices.Contains(marks, "done") {
		t.Errorf("a declined Task reads as %v, want declined and not done", marks)
	}
	if kinds := historyKinds(t, s); !slices.Contains(kinds, KindTaskDeclined) {
		t.Errorf("the Change History reads %v, want a %s in it", kinds, KindTaskDeclined)
	}
}

// The two terminal states are different answers to what happened, so asking
// for one does not bring back the other.
func TestCompletedAndDeclinedAreAskedForSeparately(t *testing.T) {
	s := openTemp(t)
	done := leased(t, s, "alice", Attributes{Title: Set("Ship it")})
	refused := leased(t, s, "alice", Attributes{Title: Set("Rewrite it in Rust")})

	if err := s.CompleteTask("alice", done); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if err := s.DeclineTask("alice", refused); err != nil {
		t.Fatalf("DeclineTask: %v", err)
	}

	if got := only(t, s, Query{IncludeCompleted: true}); got.ID != done {
		t.Error("IncludeCompleted returned the declined Task")
	}
	if got := only(t, s, Query{IncludeDeclined: true}); got.ID != refused {
		t.Error("IncludeDeclined returned the completed Task")
	}
}

// The two endings are mutually exclusive, and the store is what keeps them so:
// a Task carrying both marks could not answer which ending it got, which is
// the only reason Declined exists apart from Completed.
func TestATaskEndsOnce(t *testing.T) {
	s := openTemp(t)
	for _, ending := range []struct {
		first, second func(string, string) error
		name          string
	}{
		{s.DeclineTask, s.CompleteTask, "completing a declined Task"},
		{s.CompleteTask, s.DeclineTask, "declining a completed Task"},
	} {
		id := leased(t, s, "alice", Attributes{Title: Set("Review the deck")})
		if err := ending.first("alice", id); err != nil {
			t.Fatalf("the first ending: %v", err)
		}

		err := ending.second("alice", id)
		if err == nil {
			t.Errorf("%s went through", ending.name)
		} else if !strings.Contains(err.Error(), "a Task that has ended is reopened before it ends the other way") {
			t.Errorf("the refusal reads %v, want the domain's own words", err)
		}

		got := only(t, s, Query{IncludeCompleted: true, IncludeDeclined: true})
		if len(got.Marks()) != 1 {
			t.Errorf("after %s the Task reads %v, want one ending", ending.name, got.Marks())
		}
		if err := s.DeleteTask("alice", id); err != nil {
			t.Fatalf("DeleteTask: %v", err)
		}
	}
}

func TestReopeningUndoesADecline(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Review the deck")})

	if err := s.DeclineTask("alice", id); err != nil {
		t.Fatalf("DeclineTask: %v", err)
	}
	if err := s.ReopenTask("alice", id); err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}
	if got := only(t, s, Query{}); !got.DeclinedAt.IsZero() {
		t.Error("reopening did not undo the decline")
	}
}

func TestADeclinedChildDoesNotHoldItsParentOpen(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 2)

	if err := s.DeclineTask("alice", ids[1]); err != nil {
		t.Fatalf("DeclineTask: %v", err)
	}
	if err := s.CompleteTask("alice", ids[0]); err != nil {
		t.Errorf("a declined child held its parent open: %v", err)
	}
}

// Declining is a direction a person drives, so it refuses over an open child
// the way completing does.
func TestAParentCannotDeclineWhileAChildIsOpen(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 2)

	err := s.DeclineTask("alice", ids[0])
	if err == nil {
		t.Fatal("a parent was declined with an open child")
	}
	if !strings.Contains(err.Error(), "a parent cannot complete or decline while a child is open") {
		t.Errorf("the refusal reads %v, want the domain's own words", err)
	}

	if err := s.DeclineTask("alice", ids[1]); err != nil {
		t.Fatalf("DeclineTask the child: %v", err)
	}
	if err := s.DeclineTask("alice", ids[0]); err != nil {
		t.Errorf("DeclineTask the parent, its child declined: %v", err)
	}
}

// Reopening cannot strand a person under an ended parent, whichever of the two
// terminal states that parent is in.
func TestReopeningAChildReopensTheDeclinedTasksAboveIt(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 3)

	for _, id := range []string{ids[2], ids[1], ids[0]} {
		if err := s.DeclineTask("alice", id); err != nil {
			t.Fatalf("DeclineTask %s: %v", id, err)
		}
	}
	if err := s.ReopenTask("alice", ids[2]); err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}

	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("reopening the leaf left %d Tasks in view, want the whole chain", len(tasks))
	}
	for _, got := range tasks {
		if !got.DeclinedAt.IsZero() {
			t.Errorf("%s is still declined above the reopened leaf", got.ID)
		}
	}
}

// A store first opened before Declined existed has a task table that CREATE
// TABLE IF NOT EXISTS will not widen, so the column is asked for by name.
func TestAStoreOlderThanDeclinedGainsTheColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.db")
	old, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("open the older store: %v", err)
	}
	_, err = old.Exec(`
CREATE TABLE task (
	id               TEXT PRIMARY KEY,
	parent_id        TEXT REFERENCES task(id),
	depth            INTEGER NOT NULL,
	title            TEXT NOT NULL,
	description      TEXT,
	why              TEXT,
	created_at       TEXT NOT NULL,
	deadline         TEXT,
	estimate_seconds INTEGER,
	priority         TEXT,
	impact           TEXT,
	snoozed_until    TEXT,
	colour           TEXT,
	completed_at     TEXT,
	deleted_at       TEXT,
	series_id        TEXT
)`)
	if err != nil {
		t.Fatalf("write the older task table: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close the older store: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open a store older than Declined: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	id := leased(t, s, "alice", Attributes{Title: Set("Review the deck")})
	if err := s.DeclineTask("alice", id); err != nil {
		t.Fatalf("DeclineTask against the widened store: %v", err)
	}
	if got := only(t, s, Query{IncludeDeclined: true}); got.DeclinedAt.IsZero() {
		t.Error("the decline did not fold into the added column")
	}
}

// Detached is the one statement of what a detached copy carries, read by both
// the screen that previews the copy and the append that writes it. A copy is
// an ordinary open Task, so it carries neither ending.
func TestADetachedCopyCarriesNeitherEnding(t *testing.T) {
	source := Task{
		Title:       "Water the plants",
		CompletedAt: time.Now().UTC(),
		DeclinedAt:  time.Now().UTC(),
		DeletedAt:   time.Now().UTC(),
	}
	copied := Detached(source, time.Now())
	if !copied.CompletedAt.IsZero() || !copied.DeclinedAt.IsZero() || !copied.DeletedAt.IsZero() {
		t.Errorf("a detached copy reads %v, want an open Task", copied.Marks())
	}
}
