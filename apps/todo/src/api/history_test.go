package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func log(t *testing.T, s *store.Store, target string) historyBody {
	t.Helper()
	w := do(t, s, http.MethodGet, target, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d: %s", target, w.Code, w.Body.String())
	}
	var out historyBody
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return out
}

// The detail screen's log is one Task's entries, newest first, and it is the
// rows as they are: the Actor is verbatim and the Lease bookkeeping around a
// guarded write is in the answer, because what is worth drawing is the screen's
// question.
func TestATasksHistoryIsItsOwnEntriesNewestFirst(t *testing.T) {
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint the fence"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}
	mine := said(t, w)["id"]
	if w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Buy bread"}`); w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}
	if w := do(t, s, http.MethodPost, "/api/tasks/"+mine+"/complete", ""); w.Code != http.StatusOK {
		t.Fatalf("POST complete answered %d: %s", w.Code, w.Body.String())
	}

	out := log(t, s, "/api/tasks/"+mine+"/history")
	if len(out.Entries) == 0 {
		t.Fatalf("the task's history is empty")
	}
	for _, e := range out.Entries {
		if e.Subject != mine {
			t.Errorf("an entry about %q is in %q's history", e.Subject, mine)
		}
		if e.Actor != "tester" {
			t.Errorf("entry actor = %q, want the listener's actor verbatim", e.Actor)
		}
	}
	if first, last := out.Entries[0], out.Entries[len(out.Entries)-1]; first.Seq <= last.Seq {
		t.Errorf("the history runs %d..%d, want newest first", first.Seq, last.Seq)
	}
	if out.Entries[len(out.Entries)-1].Kind != store.KindTaskAdded {
		t.Errorf("the oldest entry is %q, want the task being added", out.Entries[len(out.Entries)-1].Kind)
	}

	// The payload is passed through as the JSON it is stored as, so a screen
	// reads an attribute out of it rather than a string it has to parse again.
	var payload map[string]any
	if err := json.Unmarshal(out.Entries[len(out.Entries)-1].Payload, &payload); err != nil {
		t.Fatalf("the added entry's payload is not JSON: %v", err)
	}
	if payload["title"] != "Paint the fence" {
		t.Errorf("the added entry's payload is %v, want the title in it", payload)
	}
}

// The activity screen reads across every Task, newest first and a page at a
// time: the whole log is what this route exists not to decode.
func TestHistoryIsEveryTaskNewestFirstAndAPage(t *testing.T) {
	s := openTemp(t)
	for _, title := range []string{"first", "second", "third"} {
		if w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "`+title+`"}`); w.Code != http.StatusCreated {
			t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
		}
	}

	out := log(t, s, "/api/history")
	if len(out.Entries) != 3 {
		t.Fatalf("the log holds %d entries, want the three writes", len(out.Entries))
	}
	if out.Entries[0].Seq <= out.Entries[2].Seq {
		t.Errorf("the log runs %d..%d, want newest first", out.Entries[0].Seq, out.Entries[2].Seq)
	}

	if page := log(t, s, "/api/history?limit=2"); len(page.Entries) != 2 {
		t.Errorf("a page of 2 holds %d entries", len(page.Entries))
	}
}

// A limit that is not a number of entries is the caller asking wrongly, and it
// is said one status before the store is reached.
func TestAnUnreadableLimitIsTheCallerAskingWrongly(t *testing.T) {
	s := openTemp(t)
	for _, asked := range []string{"soon", "0", "-4"} {
		w := do(t, s, http.MethodGet, "/api/history?limit="+asked, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("limit=%s answered %d, want 400: %s", asked, w.Code, w.Body.String())
		}
	}
}

// Reads write nothing, which is the store's rule and has to survive the two
// routes that read the log itself.
func TestReadingTheHistoryAppendsNothing(t *testing.T) {
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint the fence"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}
	id := said(t, w)["id"]

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	for range 3 {
		log(t, s, "/api/history")
		log(t, s, "/api/tasks/"+id+"/history")
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if before != after {
		t.Errorf("reading the history appended %d entries", after-before)
	}
}
