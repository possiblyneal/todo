package cli

import (
	"strings"
	"testing"
)

func newCollection(t *testing.T, noun, name string, args ...string) string {
	t.Helper()
	code, out, errs := run(t, append([]string{noun, "new"}, append(args, name)...)...)
	if code != 0 {
		t.Fatalf("todo %s new exited %d: %s", noun, code, errs)
	}
	return strings.TrimSpace(out)
}

func TestListsAndTagsAreCreatedRenamedAndCounted(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")

	home := newCollection(t, "lists", "Home", "-colour", "#88cc88")
	deep := newCollection(t, "tags", "deep-work")
	task := added(t, "Fix the sink")

	if code, _, errs := run(t, "edit", "-list", home, "-tag", deep, task); code != 0 {
		t.Fatalf("todo edit exited %d: %s", code, errs)
	}

	code, out, errs := run(t, "lists")
	if code != 0 {
		t.Fatalf("todo lists exited %d: %s", code, errs)
	}
	if !strings.Contains(out, "Home  (1)") {
		t.Errorf("the list does not carry its one task: %q", out)
	}
	if code, out, _ = run(t, "tags"); !strings.Contains(out, "deep-work  (1)") {
		t.Errorf("the tag is not counted: %q", out)
	}

	// One rename, and the task carrying it was not written to.
	before := historyLength(t)
	if code, _, errs := run(t, "lists", "rename", home, "House"); code != 0 {
		t.Fatalf("todo lists rename exited %d: %s", code, errs)
	}
	if after := historyLength(t); after-before != 1 {
		t.Errorf("renaming appended %d entries, want 1", after-before)
	}
	if _, out, _ = run(t, "lists"); !strings.Contains(out, "House  (1)") {
		t.Errorf("the rename is not visible: %q", out)
	}

	// Out of the list, which stays.
	if code, _, errs := run(t, "edit", "-unlist", home, task); code != 0 {
		t.Fatalf("todo edit -unlist exited %d: %s", code, errs)
	}
	if _, out, _ = run(t, "lists"); !strings.Contains(out, "House  (0)") {
		t.Errorf("the list did not survive its last task: %q", out)
	}
}

func TestListNarrowsToAListAndSorts(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	home := newCollection(t, "lists", "Home")

	zebra := added(t, "-deadline", "2027-03-09", "Zebra")
	anvil := added(t, "-deadline", "2027-03-01", "Anvil")
	if code, _, errs := run(t, "edit", "-list", home, zebra); code != 0 {
		t.Fatalf("todo edit exited %d: %s", code, errs)
	}

	out := listed(t, "-list", home)
	if !strings.Contains(out, zebra) || strings.Contains(out, anvil) {
		t.Errorf("narrowing to a list returned %q", out)
	}

	byDeadline := listed(t, "-sort", "deadline")
	if strings.Index(byDeadline, anvil) > strings.Index(byDeadline, zebra) {
		t.Errorf("sorted by deadline the order is wrong:\n%s", byDeadline)
	}
	byTitle := listed(t, "-sort", "title")
	if strings.Index(byTitle, anvil) > strings.Index(byTitle, zebra) {
		t.Errorf("sorted by title the order is wrong:\n%s", byTitle)
	}
	if code, _, _ := run(t, "list", "-sort", "colour"); code != 2 {
		t.Errorf("an unknown sort exited %d, want 2", code)
	}
}

func TestACollectionFormIsRefusedWhenItIsNotOne(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	for _, args := range [][]string{
		{"lists", "new"},
		{"tags", "new", "  "},
		{"lists", "rename", "abc"},
		{"tags", "sharpen", "abc", "x"},
	} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("todo %v exited %d, want 2", args, code)
		}
	}
}
