package store

import (
	"errors"
	"strings"
	"testing"
	"time"
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

// completedOf reads back whether each id is complete, by id.
func completedOf(t *testing.T, s *Store, ids []string) map[string]bool {
	t.Helper()
	tasks, err := s.Tasks(Query{IncludeCompleted: true, IncludeDeleted: true, IncludeSnoozed: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	done := make(map[string]bool, len(ids))
	for _, task := range tasks {
		done[task.ID] = !task.CompletedAt.IsZero()
	}
	return done
}

// TestReopeningASubtaskReopensWhatIsAboveIt is the other direction of "a
// parent cannot complete with open children". Completing refuses; reopening
// cannot refuse without stranding the person, so the parent gives way, one
// level at a time all the way to the root.
func TestReopeningASubtaskReopensWhatIsAboveIt(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 3)

	for i := len(ids) - 1; i >= 0; i-- {
		if err := s.CompleteTask("alice", ids[i]); err != nil {
			t.Fatalf("CompleteTask at level %d: %v", i+1, err)
		}
	}
	if err := s.ReopenTask("alice", ids[2]); err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}

	done := completedOf(t, s, ids)
	for i, id := range ids {
		if done[id] {
			t.Errorf("level %d stayed complete over an open child", i+1)
		}
	}
}

// TestAnOpenSubtaskUnderACompletedParentReopensIt closes the other way in.
// The completion rule was a BEFORE UPDATE trigger only, so an INSERT walked
// straight past it and left a completed parent holding an open child.
func TestAnOpenSubtaskUnderACompletedParentReopensIt(t *testing.T) {
	s := openTemp(t)
	root := leased(t, s, "alice", Attributes{Title: Set("Fix the roof")})
	if err := s.CompleteTask("alice", root); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	kid, err := s.AddSubtask("alice", root, Attributes{Title: Set("Buy tiles")})
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	done := completedOf(t, s, []string{root, kid})
	if done[root] {
		t.Error("the parent stayed complete while carrying an open child")
	}
}

// TestADeletedTaskTakesItsSubtreeOutOfSight is the aggregate seen from the
// read side. A subtree is reached through its root or not at all: a child left
// behind by a deleted parent draws as a row indented under nothing.
func TestADeletedTaskTakesItsSubtreeOutOfSight(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 3)

	if err := s.DeleteTask("alice", ids[0]); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("deleting the root left %d rows behind it, want none", len(tasks))
	}

	// Asking for deleted Tasks brings the whole tree back, root first.
	all, err := s.Tasks(Query{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(all) != len(ids) {
		t.Errorf("the tree came back as %d rows, want %d", len(all), len(ids))
	}
}

// TestASnoozedTaskTakesItsSubtreeWithIt is the same rule for the other way a
// row leaves the everyday view.
func TestASnoozedTaskTakesItsSubtreeWithIt(t *testing.T) {
	s := openTemp(t)
	ids := chain(t, s, "alice", 2)

	if err := s.EditTask("alice", ids[0],
		Attributes{SnoozedUntil: Set(time.Now().UTC().Add(24 * time.Hour))}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("snoozing the root left %d rows behind it, want none", len(tasks))
	}
}
