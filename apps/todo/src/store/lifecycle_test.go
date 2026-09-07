package store

import (
	"errors"
	"testing"
	"time"
)

// leased adds a top-level Task and holds a Lease on it for the test.
func leased(t *testing.T, s *Store, actor string, a Attributes) string {
	t.Helper()
	id, err := s.AddTask(actor, a)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if _, err := s.TakeLease(actor, id, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	return id
}

func only(t *testing.T, s *Store, q Query) Task {
	t.Helper()
	tasks, err := s.Tasks(q)
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("Tasks returned %d tasks, want 1", len(tasks))
	}
	return tasks[0]
}

func TestCompleteReopenAndDeleteEachAppendTheirOwnEvent(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Buy milk")})

	if err := s.CompleteTask("alice", id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if got := only(t, s, Query{IncludeCompleted: true}); got.CompletedAt.IsZero() {
		t.Error("completing left no completion time")
	}
	if tasks, _ := s.Tasks(Query{}); len(tasks) != 0 {
		t.Errorf("a completed Task is still in the everyday view: %d tasks", len(tasks))
	}

	if err := s.ReopenTask("alice", id); err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}
	if got := only(t, s, Query{}); !got.CompletedAt.IsZero() {
		t.Error("reopening did not undo the completion")
	}

	if err := s.DeleteTask("alice", id); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if tasks, _ := s.Tasks(Query{}); len(tasks) != 0 {
		t.Errorf("a deleted Task is still in the everyday view: %d tasks", len(tasks))
	}

	// Reopen is its own entry, not a second Task Described.
	want := []string{
		KindTaskAdded, KindLeaseTaken,
		KindTaskCompleted, KindTaskReopened, KindTaskDeleted,
	}
	got := historyKinds(t, s)
	if len(got) != len(want) {
		t.Fatalf("the Change History reads %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the Change History reads %v, want %v", got, want)
		}
	}
}

// A deletion is an appended entry, so everything the Task ever was is still
// readable in the Change History.
func TestDeletingKeepsTheChangeHistory(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Buy milk")})
	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if err := s.DeleteTask("alice", id); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after != before+1 {
		t.Errorf("the Change History went from %d to %d entries; a deletion appends one and erases none", before, after)
	}
	if got := only(t, s, Query{IncludeDeleted: true}); got.Title != "Buy milk" {
		t.Errorf("the deleted Task reads %q, want it kept", got.Title)
	}
}

func TestEveryLifecycleVerbHonoursTheLease(t *testing.T) {
	s := openTemp(t)
	id, err := s.AddTask("alice", Attributes{Title: Set("Buy milk")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	verbs := map[string]func() error{
		"EditTask":     func() error { return s.EditTask("alice", id, Attributes{Why: Set("we are out")}) },
		"CompleteTask": func() error { return s.CompleteTask("alice", id) },
		"ReopenTask":   func() error { return s.ReopenTask("alice", id) },
		"DeleteTask":   func() error { return s.DeleteTask("alice", id) },
	}
	for name, verb := range verbs {
		if err := verb(); !errors.Is(err, ErrRefused) {
			t.Errorf("%s with no Lease = %v, want ErrRefused", name, err)
		}
	}
}

func TestTheFullAttributeSetRoundTrips(t *testing.T) {
	s := openTemp(t)
	deadline := time.Date(2027, 3, 4, 9, 0, 0, 0, time.UTC)

	id := leased(t, s, "alice", Attributes{
		Title:       Set("Ship the thing"),
		Description: Set("The whole thing, not half of it"),
		Why:         Set("It is what the quarter was for"),
		Deadline:    Set(deadline),
		Estimate:    Set(90 * time.Minute),
		Priority:    Set(LevelHigh),
		Impact:      Set(LevelMed),
		Colour:      Set("#ff8800"),
		Fields:      map[string]string{"repo": "todo", "pr": "42"},
	})

	got := only(t, s, Query{})
	switch {
	case got.Title != "Ship the thing":
		t.Errorf("title = %q", got.Title)
	case got.Description != "The whole thing, not half of it":
		t.Errorf("description = %q", got.Description)
	case got.Why != "It is what the quarter was for":
		t.Errorf("why = %q", got.Why)
	case !got.Deadline.Equal(deadline):
		t.Errorf("deadline = %v, want %v", got.Deadline, deadline)
	case got.Estimate != 90*time.Minute:
		t.Errorf("estimate = %v", got.Estimate)
	case got.Priority != LevelHigh:
		t.Errorf("priority = %q", got.Priority)
	case got.Impact != LevelMed:
		t.Errorf("impact = %q", got.Impact)
	case got.Colour != "#ff8800":
		t.Errorf("colour = %q", got.Colour)
	case got.CreatedAt.IsZero():
		t.Error("creation date is unset")
	case got.Fields["repo"] != "todo" || got.Fields["pr"] != "42":
		t.Errorf("fields = %v", got.Fields)
	}

	// An edit touches only what it names, and a zero value clears.
	if err := s.EditTask("alice", id, Attributes{
		Priority: Set(Level("")),
		Fields:   map[string]string{"pr": "", "branch": "main"},
	}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	got = only(t, s, Query{})
	switch {
	case got.Priority != "":
		t.Errorf("priority = %q, want it cleared", got.Priority)
	case got.Impact != LevelMed:
		t.Errorf("impact = %q, want it untouched by an edit that did not name it", got.Impact)
	case got.Title != "Ship the thing":
		t.Errorf("title = %q, want it untouched", got.Title)
	case len(got.Fields) != 2 || got.Fields["repo"] != "todo" || got.Fields["branch"] != "main":
		t.Errorf("fields = %v, want pr removed and branch added", got.Fields)
	}
}

func TestALevelOutsideTheThreeIsRefused(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Buy milk")})
	if err := s.EditTask("alice", id, Attributes{Priority: Set(Level("urgent"))}); err == nil {
		t.Error("priority \"urgent\" was accepted; the three are low, med, high")
	}
	for _, level := range Levels {
		if PriorityExamples[level] == "" || ImpactExamples[level] == "" {
			t.Errorf("level %q has no example", level)
		}
	}
}

// Overdue is the expression deadline < now, worked out by the read.
func TestOverdueIsReadNotRecorded(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{
		Title:    Set("Renew the passport"),
		Deadline: Set(time.Now().UTC().Add(time.Hour)),
	})
	if only(t, s, Query{}).Overdue {
		t.Error("a Task due in an hour reads as Overdue")
	}

	before, _ := s.HistoryLength()
	if err := s.EditTask("alice", id, Attributes{Deadline: Set(time.Now().UTC().Add(-time.Hour))}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	if !only(t, s, Query{}).Overdue {
		t.Error("a Task an hour past its deadline does not read as Overdue")
	}
	// Reading it did not record anything, and the edit appended exactly one.
	after, _ := s.HistoryLength()
	if after != before+1 {
		t.Errorf("the Change History went from %d to %d; only the edit should have appended", before, after)
	}
	for _, k := range historyKinds(t, s) {
		if k == "task_became_overdue" {
			t.Error("becoming Overdue was recorded as an event")
		}
	}

	// Clearing the deadline clears the condition; nothing was stored to undo.
	if err := s.EditTask("alice", id, Attributes{Deadline: Set(time.Time{})}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	got := only(t, s, Query{})
	if got.Overdue || !got.Deadline.IsZero() {
		t.Errorf("clearing the deadline left overdue=%v deadline=%v", got.Overdue, got.Deadline)
	}
}

func TestSnoozeHidesATaskUntilItsTime(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Chase the invoice")})

	if err := s.EditTask("alice", id, Attributes{SnoozedUntil: Set(time.Now().UTC().Add(time.Hour))}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	if tasks, _ := s.Tasks(Query{}); len(tasks) != 0 {
		t.Errorf("a snoozed Task is still in the everyday view: %d tasks", len(tasks))
	}
	if tasks, _ := s.Tasks(Query{IncludeSnoozed: true}); len(tasks) != 1 {
		t.Errorf("a snoozed Task is hidden even when asked for: %d tasks", len(tasks))
	}

	// Once the snooze has passed the Task is back, with nothing having run.
	if err := s.EditTask("alice", id, Attributes{SnoozedUntil: Set(time.Now().UTC().Add(-time.Minute))}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	if tasks, _ := s.Tasks(Query{}); len(tasks) != 1 {
		t.Errorf("a Task whose snooze has passed is still hidden: %d tasks", len(tasks))
	}
}

func TestSnoozeDefaultsAreTheFourOffered(t *testing.T) {
	from := time.Date(2027, 1, 31, 12, 0, 0, 0, time.UTC)
	want := []struct {
		label string
		until time.Time
	}{
		{"1 hour", from.Add(time.Hour)},
		{"1 day", time.Date(2027, 2, 1, 12, 0, 0, 0, time.UTC)},
		{"1 week", time.Date(2027, 2, 7, 12, 0, 0, 0, time.UTC)},
		{"1 month", time.Date(2027, 2, 28, 12, 0, 0, 0, time.UTC)},
	}
	if len(SnoozeDefaults) != len(want) {
		t.Fatalf("%d snooze defaults, want %d", len(SnoozeDefaults), len(want))
	}
	for i, w := range want {
		got := SnoozeDefaults[i]
		if got.Label != w.label {
			t.Errorf("default %d is %q, want %q", i, got.Label, w.label)
		}
		if until := got.Until(from); !until.Equal(w.until) {
			t.Errorf("%s from %v = %v, want %v", got.Label, from, until, w.until)
		}
	}
}

func TestWithLeaseTakesAndGivesBack(t *testing.T) {
	s := openTemp(t)
	root, err := s.AddTask("alice", Attributes{Title: Set("Ship the thing")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if _, err := s.TakeLease("alice", root, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	child, err := s.AddSubtask("alice", root, Attributes{Title: Set("Write the code")})
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	if err := s.ReleaseLease("alice", root); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}

	// A Subtask names its root, so a verb aimed at a Subtask leases the tree.
	err = s.WithLease("agent-7", child, time.Minute, func() error {
		return s.CompleteTask("agent-7", child)
	})
	if err != nil {
		t.Fatalf("WithLease: %v", err)
	}
	if err := s.CompleteTask("agent-7", child); !errors.Is(err, ErrRefused) {
		t.Errorf("the Lease outlived WithLease: a later write = %v, want ErrRefused", err)
	}
}
