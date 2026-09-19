package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func deleted(t *testing.T, s *store.Store, title string) (string, int64) {
	t.Helper()
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": `+quoted(title)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}
	id := said(t, w)["id"]
	if w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/delete", ""); w.Code != http.StatusOK {
		t.Fatalf("POST delete answered %d: %s", w.Code, w.Body.String())
	}
	out := log(t, s, "/api/tasks/"+id+"/history")
	for _, e := range out.Entries {
		if e.Kind == store.KindTaskDeleted {
			return id, e.Seq
		}
	}
	t.Fatalf("no deletion in the Task's history")
	return "", 0
}

func quoted(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// The deletion leaves no way to reach the Task through a list, which is the
// whole point: `?all=true` is for the snoozed, the completed and the declined,
// all of which were meant to be there.
func TestShowingEverythingDoesNotShowTheDeleted(t *testing.T) {
	s := openTemp(t)
	gone, _ := deleted(t, s, "Buy paint")

	w := do(t, s, http.MethodGet, "/api/state?all=true", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET state answered %d: %s", w.Code, w.Body.String())
	}
	var out stateBody
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, task := range out.Tasks {
		if task.ID == gone {
			t.Errorf("?all=true offered the deleted Task %s back", gone)
		}
	}
}

// And the entry that deleted it is where it is read instead, with the words it
// had when it went.
func TestTheDeletingEntryAnswersWithTheTask(t *testing.T) {
	s := openTemp(t)
	gone, seq := deleted(t, s, "Buy paint")

	w := do(t, s, http.MethodGet, "/api/tasks/"+gone+"/at/"+strconv.FormatInt(seq, 10), "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET at answered %d: %s", w.Code, w.Body.String())
	}
	var was task
	if err := json.Unmarshal(w.Body.Bytes(), &was); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if was.Title != "Buy paint" {
		t.Errorf("the entry answered with %q, want %q", was.Title, "Buy paint")
	}
	if was.DeletedAt == "" {
		t.Error("the Task read at its own deletion does not say it was deleted")
	}
}

// A position before the Task existed is nothing to answer with rather than an
// empty Task, and a position that is not a number is the caller's mistake.
func TestAPositionBeforeTheTaskIsNotFound(t *testing.T) {
	s := openTemp(t)
	// Something else first, so position 1 is an entry about a different Task
	// and the one asked about genuinely was not there yet.
	if w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Earlier"}`); w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d", w.Code)
	}
	gone, _ := deleted(t, s, "Buy paint")

	if w := do(t, s, http.MethodGet, "/api/tasks/"+gone+"/at/1", ""); w.Code != http.StatusNotFound {
		t.Errorf("a position before the Task answered %d, want 404: %s", w.Code, w.Body.String())
	}
	if w := do(t, s, http.MethodGet, "/api/tasks/"+gone+"/at/soon", ""); w.Code != http.StatusBadRequest {
		t.Errorf("a position that is not a number answered %d, want 400", w.Code)
	}
	// Past the end of the log is not the Task as it stands now. The read is of
	// every entry at or before the position, so a number the log is merely
	// shorter than takes in all of it, and a route that says "as it stood
	// then" would be answering "as it stands".
	if w := do(t, s, http.MethodGet, "/api/tasks/"+gone+"/at/999999", ""); w.Code != http.StatusNotFound {
		t.Errorf("a position past the end of the log answered %d, want 404: %s", w.Code, w.Body.String())
	}
}

// The search reaches the whole log rather than the page, which is what makes a
// deletion findable after enough has happened to push it off the first page.
func TestSearchingTheLogReachesPastThePage(t *testing.T) {
	s := openTemp(t)
	gone, _ := deleted(t, s, "Buy cerulean paint")
	for range 5 {
		if w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Something else"}`); w.Code != http.StatusCreated {
			t.Fatalf("POST answered %d", w.Code)
		}
	}

	out := log(t, s, "/api/history?search=cerulean&limit=1")
	if len(out.Entries) != 1 {
		t.Fatalf("the search answered %d entries, want 1", len(out.Entries))
	}
	if out.Entries[0].Subject != gone {
		t.Errorf("the search found %s, want %s", out.Entries[0].Subject, gone)
	}
}
