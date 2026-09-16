package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// broker stands in for inference-runtime-broker and answers one turn with
// whatever the test hands it. What it was told is kept so a test can say what
// crossed the wire; the broker itself is somebody else's deployable and is not
// started by a test.
func broker(t *testing.T, answer string) *string {
	t.Helper()
	var told string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("the request body was not JSON: %v", err)
		}
		turns, _ := json.Marshal(sent["messages"])
		told = string(turns)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": answer}}},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TODO_AI_URL", srv.URL+"/v1")
	t.Setenv("TODO_AI_MODEL", "a-model")
	return &told
}

func TestCaptureAnswersInTheShapeTheAddRouteTakes(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("tester", "House", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	told := broker(t, `{"title":"Paint the fence","why":"it is peeling",
		"deadline":"2026-03-04","estimate":"90m","priority":"high",
		"lists":["house"],"tags":["nothing by that name"]}`)

	w := do(t, s, http.MethodPost, "/api/capture", `{"text": "paint the fence before march"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	var out taskBody
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if out.Title == nil || *out.Title != "Paint the fence" {
		t.Errorf("the title came back as %v, want what the broker read", out.Title)
	}
	// The attributes come back as the broker said them, not parsed: the add
	// sheet is where somebody corrects one, so a duration this program cannot
	// read has to reach the field rather than be dropped on the way.
	if out.Estimate == nil || *out.Estimate != "90m" {
		t.Errorf("the estimate came back as %v, want the broker's own words", out.Estimate)
	}
	// A List it chose is answered by id, and a name it invented is dropped.
	if len(out.IntoLists) != 1 || out.IntoLists[0] != list {
		t.Errorf("it would be filed in %v, want the id of the list it named", out.IntoLists)
	}
	if len(out.AddTags) != 0 {
		t.Errorf("it would carry %v, want a name nobody offered dropped", out.AddTags)
	}
	if !strings.Contains(*told, "paint the fence before march") {
		t.Errorf("the broker was told %q, want the dump in it", *told)
	}
	if !strings.Contains(*told, "House") {
		t.Errorf("the broker was told %q, want the list names it may choose from", *told)
	}
}

func TestCaptureWritesNothing(t *testing.T) {
	// The form is the gate: a dump read and then abandoned leaves nothing
	// behind, which is what makes handing one over safe.
	s := openTemp(t)
	broker(t, `{"title":"Paint the fence"}`)

	if w := do(t, s, http.MethodPost, "/api/capture", `{"text": "paint the fence"}`); w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("the store holds %d tasks, want a read to have written nothing", len(tasks))
	}
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the log holds %d entries, want a read to have appended nothing", len(entries))
	}
}

func TestCaptureRefusesADumpWithNothingInIt(t *testing.T) {
	s := openTemp(t)
	broker(t, `{"title":"Paint the fence"}`)

	for _, body := range []string{`{}`, `{"text": "   "}`} {
		w := do(t, s, http.MethodPost, "/api/capture", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400 (%s)", body, w.Code, w.Body.String())
		}
	}
}

func TestAskSendsTheTasksInViewAndAnswersAsProse(t *testing.T) {
	s := openTemp(t)
	add(t, s, "Paint the fence")
	done := add(t, s, "Buy paint")
	if err := s.WithLease("tester", done, store.WriteTTL, func() error {
		return s.CompleteTask("tester", done)
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	told := broker(t, "Start with the fence.")

	w := do(t, s, http.MethodPost, "/api/ask", `{"question": "what first?"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if said(t, w)["answer"] != "Start with the fence." {
		t.Errorf("the api answered %q, want the broker's prose", said(t, w)["answer"])
	}
	if !strings.Contains(*told, "Paint the fence") || !strings.Contains(*told, "what first?") {
		t.Errorf("the broker was told %q, want the list and the question", *told)
	}
	// The narrowing is the one the list is drawn under, so a question asked
	// over the default view is not asked about a completed Task.
	if strings.Contains(*told, "Buy paint") {
		t.Errorf("the broker was told %q, want only the tasks in view", *told)
	}
}

func TestAskTakesTheSameNarrowingsTheListDoes(t *testing.T) {
	s := openTemp(t)
	done := add(t, s, "Buy paint")
	if err := s.WithLease("tester", done, store.WriteTTL, func() error {
		return s.CompleteTask("tester", done)
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	told := broker(t, "Nothing left.")

	if w := do(t, s, http.MethodPost, "/api/ask?all=true", `{"question": "what first?"}`); w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(*told, "Buy paint") {
		t.Errorf("the broker was told %q, want the wider view the query asked for", *told)
	}
}

func TestAskWritesNothingAndRefusesAnEmptyQuestion(t *testing.T) {
	s := openTemp(t)
	add(t, s, "Paint the fence")
	broker(t, "Start with the fence.")

	for _, body := range []string{`{}`, `{"question": "   "}`} {
		w := do(t, s, http.MethodPost, "/api/ask", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400 (%s)", body, w.Code, w.Body.String())
		}
	}
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the log holds %d entries, want only the task that was added", len(entries))
	}
}
