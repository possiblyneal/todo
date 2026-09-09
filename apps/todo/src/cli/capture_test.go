package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// standIn is the broker, played by an httptest server, and TODO_AI_URL is how
// the verb is pointed at it. No test here reaches the LAN.
func standIn(t *testing.T, content string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TODO_AI_URL", srv.URL+"/v1")
	t.Setenv("TODO_AI_MODEL", "stand-in")
}

const dentist = `{"title":"Call the dentist","description":"about the crown",
	"deadline":"2026-11-02 11:00","estimate":"20m","priority":"high",
	"lists":["Errands"],"tags":["made up"]}`

// `todo capture` is the verb shape of the same box: one dump in, one Task
// written, and its id on stdout the way `todo add` says one.
func TestCaptureWritesOneTaskFiledWhereTheBrokerSaid(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")
	standIn(t, dentist)

	code, out, errs := run(t, "lists", "new", "Errands")
	if code != 0 {
		t.Fatalf("todo lists new exited %d: %s", code, errs)
	}
	list := strings.TrimSpace(out)

	code, out, errs = run(t, "capture", "dentist about the crown, before noon on the 2nd")
	if code != 0 {
		t.Fatalf("todo capture exited %d: %s", code, errs)
	}
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("todo capture wrote no id")
	}

	shown := listed(t)
	if !strings.Contains(shown, "Call the dentist") {
		t.Errorf("the task is not listed under the title the broker read: %q", shown)
	}
	// A List the broker chose is one the store already had. A Tag it made up
	// is dropped rather than created.
	if in := listed(t, "-list", list); !strings.Contains(in, id) {
		t.Errorf("the task is not in the List the broker filed it under: %q", in)
	}
	code, out, errs = run(t, "tags")
	if code != 0 {
		t.Fatalf("todo tags exited %d: %s", code, errs)
	}
	if strings.Contains(out, "made up") {
		t.Errorf("a Tag the broker invented was created: %q", out)
	}
}

// -dry is the approval a verb cannot ask for: what was read, printed, and
// nothing written.
func TestCaptureDryWritesNothing(t *testing.T) {
	storeInTemp(t)
	standIn(t, dentist)

	code, out, errs := run(t, "capture", "-dry", "dentist about the crown")
	if code != 0 {
		t.Fatalf("todo capture -dry exited %d: %s", code, errs)
	}
	for _, want := range []string{"Call the dentist", "2026-11-02 11:00", "20m", "high"} {
		if !strings.Contains(out, want) {
			t.Errorf("todo capture -dry did not print %q: %q", want, out)
		}
	}
	if shown := listed(t); strings.TrimSpace(shown) != "" {
		t.Errorf("todo capture -dry wrote a task: %q", shown)
	}
}

func TestCaptureWithNothingToReadIsAUsageError(t *testing.T) {
	storeInTemp(t)
	standIn(t, dentist)

	if code, _, _ := run(t, "capture"); code != 2 {
		t.Errorf("todo capture with no words exited %d, want 2", code)
	}
}

// The broker answering with something that is not a task is a failure, not a
// Task with no title.
func TestCaptureRefusesWhatIsNotATask(t *testing.T) {
	storeInTemp(t)
	standIn(t, "I could not read that.")

	code, _, errs := run(t, "capture", "something")
	if code != 1 {
		t.Errorf("todo capture exited %d, want 1", code)
	}
	if !strings.Contains(errs, "not a task") {
		t.Errorf("todo capture said %q, want what the broker did wrong", errs)
	}
	if shown := listed(t); strings.TrimSpace(shown) != "" {
		t.Errorf("a task was written from an answer that was not one: %q", shown)
	}
}
