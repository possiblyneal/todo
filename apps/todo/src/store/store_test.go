package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAddThenList(t *testing.T) {
	s := openTemp(t)

	id, err := s.AddTask("alice", "Buy milk")
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	tasks, err := s.Tasks()
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("Tasks() returned %d tasks, want 1", len(tasks))
	}
	if tasks[0].ID != id || tasks[0].Title != "Buy milk" {
		t.Errorf("Tasks() = %+v, want id %q titled %q", tasks[0], id, "Buy milk")
	}
}

// Every write is attributed; reads are not. The Actor reaching the Change
// History is an identity there and an opaque id everywhere else.
func TestEveryWriteCarriesAnActor(t *testing.T) {
	s := openTemp(t)
	if _, err := s.AddTask("agent-7", "Ship it"); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("History() returned %d entries, want 1", len(entries))
	}
	if entries[0].Actor != "agent-7" {
		t.Errorf("entry actor = %q, want %q", entries[0].Actor, "agent-7")
	}
	if entries[0].Kind != KindTaskAdded {
		t.Errorf("entry kind = %q, want %q", entries[0].Kind, KindTaskAdded)
	}
}

// Reads are pure. Seam contracts (#5) dissolved "materialize on read" rather
// than hoping every reader discharges it, so a read that appends anything at
// all is a defect.
func TestReadsWriteNothing(t *testing.T) {
	s := openTemp(t)
	if _, err := s.AddTask("alice", "Buy milk"); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	for range 5 {
		if _, err := s.Tasks(); err != nil {
			t.Fatalf("Tasks: %v", err)
		}
		if _, err := s.History(); err != nil {
			t.Fatalf("History: %v", err)
		}
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if before != after {
		t.Errorf("the Change History grew across reads: %d -> %d", before, after)
	}
}

// Append-only is enforced by the store, not hoped. A Task Deleted is an
// appended entry rather than an erasure, which only holds if erasure is refused.
func TestChangeHistoryRefusesUpdateAndDelete(t *testing.T) {
	s := openTemp(t)
	if _, err := s.AddTask("alice", "Buy milk"); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	for _, stmt := range []string{
		`UPDATE change_history SET actor = 'mallory'`,
		`DELETE FROM change_history`,
	} {
		if _, err := s.db.Exec(stmt); err == nil {
			t.Errorf("%s was allowed; the Change History must refuse it", stmt)
		} else if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s failed with %v, want the append-only abort", stmt, err)
		}
	}
}

// database/sql issues a plain deferred BEGIN, and the deferred path silently
// loses rows under contention: 228/250 created against 250/250 with the flag.
func TestDSNTakesTheWriteLockUpFront(t *testing.T) {
	dsn := dsn("/tmp/todo.db")
	if !strings.Contains(dsn, "_txlock=immediate") {
		t.Errorf("dsn = %q, want it to carry _txlock=immediate", dsn)
	}
}
