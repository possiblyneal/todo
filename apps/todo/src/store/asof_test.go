package store

import "testing"

// The point of the whole feature: a Task nobody can reach in a list is still
// readable at the entry that took it away, with the words it had.
func TestATaskIsReadableAtTheEntryThatDeletedIt(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Buy paint")})
	if err := s.DeleteTask("alice", id); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	entries, err := s.HistoryOf(id)
	if err != nil {
		t.Fatalf("HistoryOf: %v", err)
	}
	gone := entries[len(entries)-1]
	if gone.Kind != KindTaskDeleted {
		t.Fatalf("last entry is %s, want %s", gone.Kind, KindTaskDeleted)
	}

	was, err := s.TaskAsOf(id, gone.Seq)
	if err != nil {
		t.Fatalf("TaskAsOf: %v", err)
	}
	if was.Title != "Buy paint" {
		t.Errorf("the deleted Task read as %q, want %q", was.Title, "Buy paint")
	}
	if was.DeletedAt.IsZero() {
		t.Error("the Task read at its own deletion is not marked deleted")
	}
}

// An edit is a partial payload, so a position before one has to show the words
// that were replaced rather than the words that replaced them.
func TestAnEarlierPositionShowsTheWordsThatWereThereThen(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Buy paint")})

	entries, err := s.HistoryOf(id)
	if err != nil {
		t.Fatalf("HistoryOf: %v", err)
	}
	added := entries[len(entries)-1].Seq

	if err := s.WithLease("alice", id, WriteTTL, func() error {
		return s.EditTask("alice", id, Attributes{Title: Set("Buy blue paint")})
	}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}

	before, err := s.TaskAsOf(id, added)
	if err != nil {
		t.Fatalf("TaskAsOf: %v", err)
	}
	if before.Title != "Buy paint" {
		t.Errorf("before the edit the Task read as %q, want %q", before.Title, "Buy paint")
	}
}
