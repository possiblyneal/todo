package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func added(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errs := run(t, append([]string{"add"}, args...)...)
	if code != 0 {
		t.Fatalf("todo add exited %d: %s", code, errs)
	}
	return strings.TrimSpace(out)
}

func listed(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errs := run(t, append([]string{"list"}, args...)...)
	if code != 0 {
		t.Fatalf("todo list exited %d: %s", code, errs)
	}
	return out
}

func TestTheLifecycleVerbsEachTakeTheirOwnLease(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	id := added(t, "Buy milk")

	if code, _, errs := run(t, "edit", "-why", "we are out", id); code != 0 {
		t.Fatalf("todo edit exited %d: %s", code, errs)
	}
	if code, _, errs := run(t, "complete", id); code != 0 {
		t.Fatalf("todo complete exited %d: %s", code, errs)
	}
	if out := listed(t); strings.Contains(out, id) {
		t.Errorf("a completed task is still listed: %q", out)
	}
	if out := listed(t, "-all"); !strings.Contains(out, "done") {
		t.Errorf("todo list -all does not mark it done: %q", out)
	}

	if code, _, errs := run(t, "reopen", id); code != 0 {
		t.Fatalf("todo reopen exited %d: %s", code, errs)
	}
	if out := listed(t); !strings.Contains(out, id) {
		t.Errorf("a reopened task is not listed: %q", out)
	}
	if code, _, errs := run(t, "delete", id); code != 0 {
		t.Fatalf("todo delete exited %d: %s", code, errs)
	}
	if out := listed(t); strings.Contains(out, id) {
		t.Errorf("a deleted task is still listed: %q", out)
	}

	// Every verb appended, and each one gave its Lease back.
	kinds := kindsFromStore(t)
	for _, want := range []string{
		store.KindTaskDescribed, store.KindTaskCompleted,
		store.KindTaskReopened, store.KindTaskDeleted,
	} {
		if !contains(kinds, want) {
			t.Errorf("the Change History has no %s entry: %v", want, kinds)
		}
	}
	if n := count(kinds, store.KindLeaseReleased); n != 4 {
		t.Errorf("%d Lease Released entries against 4 writing verbs: %v", n, kinds)
	}
}

func TestAVerbIsRefusedWhileAnotherActorHoldsTheLease(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	id := added(t, "Buy milk")

	s, err := open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.TakeLease("agent-7", id, time.Hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}

	code, _, errs := run(t, "edit", "-title", "Buy oat milk", id)
	if code != 3 {
		t.Errorf("todo edit against a held Lease exited %d, want 3", code)
	}
	if strings.Contains(errs, "agent-7") {
		t.Errorf("the refusal named the holder: %q", errs)
	}
	if tasks, _ := s.Tasks(store.Query{}); tasks[0].Title != "Buy milk" {
		t.Errorf("the refused edit was applied anyway: %q", tasks[0].Title)
	}
}

func TestAddTakesTheWholeAttributeSet(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	id := added(t,
		"-description", "The whole thing",
		"-why", "It is what the quarter was for",
		"-deadline", "2027-03-04",
		"-estimate", "90m",
		"-priority", "high",
		"-impact", "med",
		"-colour", "#ff8800",
		"-field", "repo=todo",
		"-field", "pr=42",
		"Ship the thing",
	)

	s, err := open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()
	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	got := tasks[0]
	switch {
	case got.ID != id:
		t.Errorf("id = %q, want %q", got.ID, id)
	case got.Description != "The whole thing":
		t.Errorf("description = %q", got.Description)
	case got.Estimate != 90*time.Minute:
		t.Errorf("estimate = %v", got.Estimate)
	case got.Priority != store.LevelHigh || got.Impact != store.LevelMed:
		t.Errorf("priority/impact = %q/%q", got.Priority, got.Impact)
	case got.Colour != "#ff8800":
		t.Errorf("colour = %q", got.Colour)
	case got.Fields["repo"] != "todo" || got.Fields["pr"] != "42":
		t.Errorf("fields = %v", got.Fields)
	case got.Deadline.Format("2006-01-02") != "2027-03-04":
		t.Errorf("deadline = %v", got.Deadline)
	}
}

func TestListMarksOverdueWithoutRecordingIt(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	added(t, "-deadline", yesterday, "Renew the passport")

	before := historyLength(t)
	if out := listed(t); !strings.Contains(out, "overdue") {
		t.Errorf("todo list does not mark a task past its deadline: %q", out)
	}
	if after := historyLength(t); after != before {
		t.Errorf("reading an overdue task appended to the Change History: %d -> %d", before, after)
	}
}

func TestSnoozeHidesATaskFromTheList(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	id := added(t, "Chase the invoice")

	if code, _, errs := run(t, "edit", "-snooze", "1 week", id); code != 0 {
		t.Fatalf("todo edit -snooze exited %d: %s", code, errs)
	}
	if out := listed(t); strings.Contains(out, id) {
		t.Errorf("a snoozed task is still listed: %q", out)
	}
	if out := listed(t, "-all"); !strings.Contains(out, "snoozed") {
		t.Errorf("todo list -all does not mark it snoozed: %q", out)
	}
}

// Declining is the lifecycle's other ending: the task leaves the everyday
// list marked as refused rather than done, and reopening brings it back.
func TestDecliningATaskEndsItWithoutCompletingIt(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	id := added(t, "Rewrite it in Rust")

	if code, _, errs := run(t, "decline", id); code != 0 {
		t.Fatalf("todo decline exited %d: %s", code, errs)
	}
	if out := listed(t); strings.Contains(out, id) {
		t.Errorf("a declined task is still listed: %q", out)
	}
	out := listed(t, "-all")
	if !strings.Contains(out, "declined") {
		t.Errorf("todo list -all does not mark it declined: %q", out)
	}
	if strings.Contains(out, "done") {
		t.Errorf("todo list -all reads a declined task as done: %q", out)
	}

	if code, _, errs := run(t, "reopen", id); code != 0 {
		t.Fatalf("todo reopen exited %d: %s", code, errs)
	}
	if out := listed(t); !strings.Contains(out, id) {
		t.Errorf("a reopened task is not listed: %q", out)
	}
}

func TestBadAttributeValuesAreUsageErrors(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	for _, args := range [][]string{
		{"add", "-priority", "urgent", "Buy milk"},
		{"add", "-deadline", "next tuesday", "Buy milk"},
		{"add", "-estimate", "ninety minutes", "Buy milk"},
		{"add", "-field", "novalue", "Buy milk"},
		{"complete"},
		{"decline"},
		{"edit", "-title", "x"},
	} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("todo %v exited %d, want 2", args, code)
		}
	}
}

func kindsFromStore(t *testing.T) []string {
	t.Helper()
	s, err := open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	kinds := make([]string, len(entries))
	for i, e := range entries {
		kinds[i] = e.Kind
	}
	return kinds
}

func contains(all []string, want string) bool { return count(all, want) > 0 }

func count(all []string, want string) int {
	n := 0
	for _, v := range all {
		if v == want {
			n++
		}
	}
	return n
}

func TestAddNestsSubtasksAndListDrawsTheTree(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	root := added(t, "Ship the thing")

	id := root
	for i := 2; i <= 5; i++ {
		id = added(t, "-parent", id, "level "+strconv.Itoa(i))
	}
	if code, _, errs := run(t, "add", "-parent", id, "level 6"); code != 1 {
		t.Errorf("a sixth level exited %d, want 1: %s", code, errs)
	}

	out := listed(t)
	if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines != 5 {
		t.Errorf("the list has %d lines, want the five nested tasks:\n%s", lines, out)
	}
	if !strings.Contains(out, "        level 5") {
		t.Errorf("the deepest task is not drawn at its depth:\n%s", out)
	}

	// The parent cannot close while anything under it is open, and the
	// deepest task is the only one that can.
	if code, _, _ := run(t, "complete", root); code != 1 {
		t.Errorf("the root completed with open children")
	}
	if code, _, errs := run(t, "complete", id); code != 0 {
		t.Fatalf("completing the deepest task exited %d: %s", code, errs)
	}
}
