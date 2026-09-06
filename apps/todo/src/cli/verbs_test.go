package cli

import (
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

func TestBadAttributeValuesAreUsageErrors(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	for _, args := range [][]string{
		{"add", "-priority", "urgent", "Buy milk"},
		{"add", "-deadline", "next tuesday", "Buy milk"},
		{"add", "-estimate", "ninety minutes", "Buy milk"},
		{"add", "-field", "novalue", "Buy milk"},
		{"complete"},
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
