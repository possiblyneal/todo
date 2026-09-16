package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// do runs one request against the whole handler, routing included, which is
// what a client reaches. The Actor is the listener's, because there is nobody
// to name per request.
func do(t *testing.T, s *store.Store, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	Handler(s, Options{Actor: "tester"}).ServeHTTP(w, r)
	return w
}

func said(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return body
}

func only(t *testing.T, s *store.Store) store.Task {
	t.Helper()
	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("the store holds %d tasks, want one", len(tasks))
	}
	return tasks[0]
}

func TestAddWritesTheTaskAndFilesItTogether(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("tester", "House", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	tag, err := s.AddTag("tester", "outdoors", "green")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	w := do(t, s, http.MethodPost, "/api/tasks", `{
		"title": "Paint the fence",
		"deadline": "2026-03-04",
		"estimate": "90m",
		"priority": "high",
		"intoLists": ["`+list+`"],
		"addTags": ["`+tag+`"]}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("the api answered %d, want 201 (%s)", w.Code, w.Body.String())
	}
	task := only(t, s)
	if said(t, w)["id"] != task.ID {
		t.Errorf("the api named %q, want the task it wrote", said(t, w)["id"])
	}
	if task.Title != "Paint the fence" || task.Estimate.Minutes() != 90 {
		t.Errorf("the task reads %q at %s, want the attributes that were sent", task.Title, task.Estimate)
	}
	if len(task.Lists) != 1 || len(task.Tags) != 1 {
		t.Errorf("the task is in %v and carries %v, want one of each", task.Lists, task.Tags)
	}
	// Attribution is the listener's Actor, because the browser names nobody.
	if task.Deadline.IsZero() {
		t.Error("the deadline was dropped")
	}
}

func TestAddAttributesTheWriteToTheListenersActor(t *testing.T) {
	s := openTemp(t)
	do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint the fence"}`)

	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, e := range entries {
		if e.Actor != "tester" {
			t.Errorf("entry %s was written by %q, want the listener's actor", e.Kind, e.Actor)
		}
	}
}

func TestAddNestsUnderAParent(t *testing.T) {
	s := openTemp(t)
	parent := add(t, s, "Paint the fence")

	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Buy paint", "parent": "`+parent+`"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("the api answered %d, want 201 (%s)", w.Code, w.Body.String())
	}
	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 || tasks[1].Parent != parent {
		t.Errorf("the subtask hangs off %q, want %q", tasks[1].Parent, parent)
	}
}

func TestAddRefusesAValueItCannotReadInTheSameWordsTheVerbWould(t *testing.T) {
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint it", "deadline": "next tuesday"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("the api answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(said(t, w)["error"], `"next tuesday"`) {
		t.Errorf("the api said %q, want the sentence naming what it could not read", said(t, w)["error"])
	}
}

func TestAddRefusesATaskWithNoTitle(t *testing.T) {
	s := openTemp(t)
	for _, body := range []string{`{}`, `{"why": "the fence is peeling"}`, `{"title": "   "}`} {
		w := do(t, s, http.MethodPost, "/api/tasks", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400 (%s)", body, w.Code, w.Body.String())
		}
	}
	if tasks, _ := s.Tasks(store.Query{}); len(tasks) != 0 {
		t.Errorf("the store holds %d tasks, want nothing written", len(tasks))
	}
}

func TestABodyWithAFieldTheRouteDoesNotKnowIsRefused(t *testing.T) {
	// A client with a typo in an attribute name is told so, rather than told
	// its write went through with that attribute quietly dropped.
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint it", "titel": "Paint it"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("the api answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(said(t, w)["error"], "titel") {
		t.Errorf("the api said %q, want the sentence naming the field", said(t, w)["error"])
	}
}

func TestEditChangesAttributesAndMembershipTogether(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Paint the fence")
	list, err := s.AddList("tester", "House", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}

	w := do(t, s, http.MethodPatch, "/api/tasks/"+id, `{
		"why": "the fence is peeling",
		"intoLists": ["`+list+`"]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	task := only(t, s)
	if task.Why != "the fence is peeling" || len(task.Lists) != 1 {
		t.Errorf("the task reads %q in %v, want both halves of the edit", task.Why, task.Lists)
	}
}

func TestEditWithNoAttributesAppendsNoEntrySayingNothingChanged(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Paint the fence")
	list, err := s.AddList("tester", "House", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}

	w := do(t, s, http.MethodPatch, "/api/tasks/"+id, `{"intoLists": ["`+list+`"]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, e := range entries {
		if e.Kind == store.KindTaskDescribed {
			t.Errorf("a membership edit appended %s, want nothing saying the attributes changed", e.Kind)
		}
	}
}

func TestEditClearsWhatIsSentEmptyAndLeavesWhatIsNotSent(t *testing.T) {
	s := openTemp(t)
	why := "the fence is peeling"
	title := "Paint the fence"
	id, err := s.AddTask("tester", store.Attributes{Title: &title, Why: &why})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	w := do(t, s, http.MethodPatch, "/api/tasks/"+id, `{"why": ""}`)

	if w.Code != http.StatusOK {
		t.Fatalf("the api answered %d, want 200 (%s)", w.Code, w.Body.String())
	}
	task := only(t, s)
	if task.Why != "" {
		t.Errorf("why reads %q, want an empty value to have cleared it", task.Why)
	}
	if task.Title != title {
		t.Errorf("title reads %q, want an attribute nobody sent left alone", task.Title)
	}
}

func TestAddStoresTheTitleTheVerbWouldHaveStored(t *testing.T) {
	// `todo add   Paint the shed  ` stores "Paint the shed", so a sent title
	// padded the same way stores the same Task rather than one titled with
	// the padding still on it.
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "  Paint the shed  "}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("the api answered %d, want 201 (%s)", w.Code, w.Body.String())
	}
	if got := only(t, s).Title; got != "Paint the shed" {
		t.Errorf("the store holds %q, want the space around it gone", got)
	}
}

func TestEditRefusesToMoveATaskRatherThanIgnoringTheParent(t *testing.T) {
	// A known field dropped in silence is the thing DisallowUnknownFields is
	// there to prevent, so a PATCH naming a parent is refused rather than
	// answered 200 with the Task left where it was.
	s := openTemp(t)
	id, err := s.AddTask("tester", store.Attributes{Title: ptr("Paint the shed")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	other, err := s.AddTask("tester", store.Attributes{Title: ptr("Do the garden")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	w := do(t, s, http.MethodPatch, "/api/tasks/"+id, `{"parent": "`+other+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("the api answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(said(t, w)["error"], "parent") {
		t.Errorf("the api said %q, want the sentence naming what to leave out", said(t, w)["error"])
	}
}

func TestEditRefusesAnEmptyTitleAsTheMistakeItIs(t *testing.T) {
	// A Task with no title is not one, and saying so is the caller's mistake
	// to fix rather than a failure here, which is 400 and not 500.
	s := openTemp(t)
	id, err := s.AddTask("tester", store.Attributes{Title: ptr("Paint the shed")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	for _, body := range []string{`{"title": ""}`, `{"title": "   "}`} {
		w := do(t, s, http.MethodPatch, "/api/tasks/"+id, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400 (%s)", body, w.Code, w.Body.String())
		}
	}
	if got := only(t, s).Title; got != "Paint the shed" {
		t.Errorf("the store holds %q, want the title untouched", got)
	}
}
