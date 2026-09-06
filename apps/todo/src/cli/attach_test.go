package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `todo attach` is the same in-process call the add screen's file selector
// makes: a pointer on, a pointer off, and a list of what is held.
func TestAttachAddsListsAndRemovesPointers(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")

	_, out, errs := run(t, "add", "File the accounts")
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatalf("todo add printed no id: %s", errs)
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "receipts.pdf")
	if err := os.WriteFile(file, []byte("%PDF"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	for _, target := range []string{file, "https://example.com/policy"} {
		if code, _, errs := run(t, "attach", id, target); code != 0 {
			t.Fatalf("todo attach %s exited %d: %s", target, code, errs)
		}
	}

	code, out, errs := run(t, "attach", id)
	if code != 0 {
		t.Fatalf("todo attach exited %d: %s", code, errs)
	}
	want := file + "\nhttps://example.com/policy\n"
	if out != want {
		t.Errorf("todo attach printed %q, want %q", out, want)
	}

	// The target going away changes nothing: nothing checks, by design.
	if err := os.Remove(file); err != nil {
		t.Fatalf("removing the file: %v", err)
	}
	if _, out, _ := run(t, "attach", id); out != want {
		t.Errorf("a dead link read back as %q", out)
	}

	if code, _, errs := run(t, "attach", "-off", id, file); code != 0 {
		t.Fatalf("todo attach -off exited %d: %s", code, errs)
	}
	if _, out, _ := run(t, "attach", id); out != "https://example.com/policy\n" {
		t.Errorf("after -off the task holds %q", out)
	}
}

func TestAttachRefusesNonsense(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")

	if code, _, _ := run(t, "attach"); code != 2 {
		t.Error("todo attach with no id was not a usage error")
	}
	if code, _, _ := run(t, "attach", "nosuchtask"); code != 2 {
		t.Error("todo attach on a task that is not there was not a usage error")
	}
}
