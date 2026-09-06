package store

import (
	"errors"
	"strings"
	"testing"
)

// chain builds a root and nests subtasks under it, returning every id from the
// root down.
func chain(t *testing.T, s *Store, actor string, levels int) []string {
	t.Helper()
	ids := []string{leased(t, s, actor, Attributes{Title: Set("level 1")})}
	for i := 2; i <= levels; i++ {
		id, err := s.AddSubtask(actor, ids[len(ids)-1], Attributes{Title: Set("level " + string(rune('0'+i)))})
		if err != nil {
			t.Fatalf("AddSubtask at level %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestSubtasksNestToFiveLevelsAndNoFurther(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 5)

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	_, err = s.AddSubtask("alice", ids[4], Attributes{Title: Set("level 6")})
	if err == nil {
		t.Fatal("a sixth level was accepted")
	}
	if !strings.Contains(err.Error(), "a Subtask nests to five levels, no deeper") {
		t.Errorf("the refusal reads %v, want the domain's own words", err)
	}

	// The append and the fold commit together, so a refused fold leaves no
	// entry behind it.
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after != before {
		t.Errorf("the refused Subtask appended %d entries", after-before)
	}
}

func TestASubtaskNeverMoves(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 3)
	other := leased(t, s, "alice", Attributes{Title: Set("Another tree")})

	// Nothing in the API asks for this, which is the point: the store
	// refuses it at the table, so no future surface can offer it by accident.
	_, err := s.db.Exec(`UPDATE task SET parent_id = ? WHERE id = ?`, other, ids[2])
	if err == nil {
		t.Fatal("a Subtask was re-parented")
	}
	if !strings.Contains(err.Error(), "a Subtask never moves") {
		t.Errorf("the refusal reads %v, want the domain's own words", err)
	}
}

func TestAParentCannotCompleteWhileAChildIsOpen(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 3)
	root, middle, leaf := ids[0], ids[1], ids[2]

	for _, id := range []string{root, middle} {
		err := s.CompleteTask("alice", id)
		if err == nil {
			t.Fatalf("%s completed with an open child", id)
		}
		if !strings.Contains(err.Error(), "a parent cannot complete while a child is open") {
			t.Errorf("the refusal reads %v, want the domain's own words", err)
		}
	}

	// Closed from the bottom, each one goes through.
	for _, id := range []string{leaf, middle, root} {
		if err := s.CompleteTask("alice", id); err != nil {
			t.Fatalf("CompleteTask %s: %v", id, err)
		}
	}
}

func TestADeletedChildDoesNotHoldItsParentOpen(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 2)

	if err := s.DeleteTask("alice", ids[1]); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if err := s.CompleteTask("alice", ids[0]); err != nil {
		t.Errorf("a deleted child held its parent open: %v", err)
	}
}

// The stated cost of ADR 0002, asserted rather than worked around.
func TestTheRootsLeaseReachesFiveLevelsDown(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 5)
	leaf := ids[4]

	edit := Attributes{Title: Set("edited")}
	if err := s.EditTask("bob", leaf, edit); !errors.Is(err, ErrRefused) {
		t.Errorf("a write five levels down without the root's Lease failed with %v, want a refusal", err)
	}
	if err := s.EditTask("alice", leaf, edit); err != nil {
		t.Errorf("the root's holder was refused its own tree: %v", err)
	}
}

func TestATreeComesBackDepthFirst(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 5)
	second := leased(t, s, "alice", Attributes{Title: Set("Another tree")})

	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	var got []string
	for i, task := range tasks {
		got = append(got, task.ID)
		if i < len(ids) && task.Depth != i+1 {
			t.Errorf("%s came back at depth %d, want %d", task.ID, task.Depth, i+1)
		}
	}
	want := append(append([]string{}, ids...), second)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the read came back in the order %v, want the tree depth first then the next root %v", got, want)
	}
}
