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

// TestTheTUINeedsATerminal is the bare invocation with its output piped, which
// is how a test and an Agent reach it. The TUI takes a terminal, so without one
// it says so and exits 2 rather than failing inside the renderer.
func TestTheTUINeedsATerminal(t *testing.T) {
	storeInTemp(t)
	code, out, errs := run(t)
	if code != 2 {
		t.Fatalf("bare todo without a terminal exited %d, want 2: %s%s", code, out, errs)
	}
	if !strings.Contains(errs, "terminal") {
		t.Errorf("stderr = %q, want it to say the tui needs a terminal", errs)
	}
}

// `todo serve` will not start without an allowlist. A public key is the only
// way in, so a missing authorized_keys is a refusal to listen at all rather
// than a server anyone can reach.
func TestServeWillNotListenWithoutAnAllowlist(t *testing.T) {
	storeInTemp(t)
	missing := filepath.Join(t.TempDir(), "authorized_keys")

	code, _, errs := run(t, "serve", "-addr", "127.0.0.1:0", "-authorized-keys", missing)
	if code != 1 {
		t.Fatalf("todo serve exited %d, want 1", code)
	}
	if !strings.Contains(errs, "public key") {
		t.Errorf("stderr = %q, want it to say a public key is the only way in", errs)
	}
}
