package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// storeInTemp points the verbs at a store of this test's own.
func storeInTemp(t *testing.T) {
	t.Helper()
	t.Setenv("TODO_DB", filepath.Join(t.TempDir(), "todo.db"))
}

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = Run(args, &out, &errs)
	return code, out.String(), errs.String()
}

func TestAddThenListEndToEnd(t *testing.T) {
	storeInTemp(t)

	code, out, errs := run(t, "add", "Buy", "milk")
	if code != 0 {
		t.Fatalf("todo add exited %d: %s", code, errs)
	}
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("todo add printed no id")
	}

	code, out, errs = run(t, "list")
	if code != 0 {
		t.Fatalf("todo list exited %d: %s", code, errs)
	}
	if !strings.Contains(out, id) || !strings.Contains(out, "Buy milk") {
		t.Errorf("todo list printed %q, want the added task %s", out, id)
	}
}

// Reads are pure, all the way out to the verb.
func TestListWritesNothing(t *testing.T) {
	storeInTemp(t)
	if code, _, errs := run(t, "add", "Buy milk"); code != 0 {
		t.Fatalf("todo add exited %d: %s", code, errs)
	}

	before := historyLength(t)
	for range 3 {
		if code, _, errs := run(t, "list"); code != 0 {
			t.Fatalf("todo list exited %d: %s", code, errs)
		}
	}
	if after := historyLength(t); before != after {
		t.Errorf("the Change History grew across `todo list`: %d -> %d", before, after)
	}
}

func historyLength(t *testing.T) int64 {
	t.Helper()
	s, err := open()
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	defer func() { _ = s.Close() }()
	n, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	return n
}

func TestAddRefusesAnEmptyTitle(t *testing.T) {
	storeInTemp(t)
	if code, _, _ := run(t, "add", "   "); code != 2 {
		t.Errorf("todo add with no title exited %d, want 2", code)
	}
}

func TestUnknownVerbIsRefused(t *testing.T) {
	storeInTemp(t)
	code, _, errs := run(t, "frobnicate")
	if code != 2 {
		t.Errorf("an unknown verb exited %d, want 2", code)
	}
	if !strings.Contains(errs, "frobnicate") {
		t.Errorf("stderr = %q, want it to name the unknown verb", errs)
	}
}

// The bare invocation says what the binary is for. It used to open the TUI;
// the person's surface is the browser client now, so there is nothing left for
// it to open and nothing it should guess at.
func TestBareTodoSaysWhatItIsFor(t *testing.T) {
	storeInTemp(t)
	code, out, errs := run(t)
	if code != 2 {
		t.Fatalf("bare todo exited %d, want 2: %s%s", code, out, errs)
	}
	for _, want := range []string{"todo <verb>", "todo api"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr = %q, want it to name %q", errs, want)
		}
	}
	// Every verb the dispatch holds is named, because the two read one list
	// and a binary that offered a verb it does not take would be lying.
	for _, one := range verbs {
		if !strings.Contains(errs, one.verb) {
			t.Errorf("stderr = %q, want it to name the verb %q", errs, one.verb)
		}
	}
}

// `todo serve` is gone with the TUI it served. The word is an unknown verb now,
// which is what every other word the binary does not know gets, rather than a
// mode that starts a listener nothing reaches.
func TestServeIsGone(t *testing.T) {
	storeInTemp(t)
	code, _, errs := run(t, "serve")
	if code != 2 {
		t.Fatalf("todo serve exited %d, want 2", code)
	}
	if !strings.Contains(errs, "unknown verb") {
		t.Errorf("stderr = %q, want it to call serve an unknown verb", errs)
	}
}
