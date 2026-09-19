package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

// The four lifecycle verbs are one route, and the write each makes is the one
// the verb of the same name makes at a terminal: guarded, under the Lease over
// the Task's tree, which is what write.Lifecycle is.
func TestLifecycleVerbsMakeTheWriteTheVerbMakes(t *testing.T) {
	s := openTemp(t)
	for _, act := range []struct {
		verb string
		mark string
	}{
		{"complete", "done"},
		{"decline", "declined"},
		{"delete", "deleted"},
	} {
		id, err := s.AddTask("tester", store.Attributes{Title: store.Set("Paint the fence")})
		if err != nil {
			t.Fatalf("AddTask: %v", err)
		}
		w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/"+act.verb, "")
		if w.Code != http.StatusOK {
			t.Fatalf("POST %s answered %d: %s", act.verb, w.Code, w.Body.String())
		}
		tasks, err := s.Tasks(store.Query{IncludeCompleted: true, IncludeDeclined: true, IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Tasks: %v", err)
		}
		var marks []string
		for _, task := range tasks {
			if task.ID == id {
				marks = task.Marks()
			}
		}
		if !slices.Contains(marks, act.mark) {
			t.Errorf("after %s the task's marks are %v, want %q among them", act.verb, marks, act.mark)
		}
	}
}

// A verb a Task does not do is the caller asking wrongly, and the store never
// sees it: there is no sentence of the store's to borrow, so the route says
// which four it could have named.
func TestAnUnknownVerbIsRefusedBeforeTheStore(t *testing.T) {
	s := openTemp(t)
	id, err := s.AddTask("tester", store.Attributes{Title: store.Set("Paint the fence")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}

	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/archive", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST archive answered %d, want 400: %s", w.Code, w.Body.String())
	}
	if sentence := said(t, w)["error"]; !strings.Contains(sentence, "complete") {
		t.Errorf("the refusal reads %q, want the four verbs named in it", sentence)
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if before != after {
		t.Errorf("a verb nobody recognises appended %d entries", after-before)
	}
}

// A Subtask's parent is the Task in the path, and the write is the one
// POST /api/tasks makes with a parent: the same store call, under the parent's
// Lease, so a Subtask written here and one written by `todo add -parent` are
// the same entry.
func TestSubtasksAreWrittenUnderTheTaskInThePath(t *testing.T) {
	s := openTemp(t)
	parent, err := s.AddTask("tester", store.Attributes{Title: store.Set("Paint the fence")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	w := do(t, s, http.MethodPost, "/api/tasks/"+parent+"/subtasks", `{"title": "Buy the paint"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST subtasks answered %d, want 201: %s", w.Code, w.Body.String())
	}
	child := said(t, w)["id"]

	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	for _, task := range tasks {
		if task.ID != child {
			continue
		}
		if task.Parent != parent {
			t.Errorf("the subtask's parent is %q, want %q", task.Parent, parent)
		}
		if task.Depth != 2 {
			t.Errorf("the subtask's depth is %d, want 2", task.Depth)
		}
		return
	}
	t.Errorf("the subtask %q is not in the store", child)
}

// The path says which Task this goes under, so a body saying it again is the
// caller asking wrongly rather than one of the two quietly losing.
func TestSubtasksRefuseABodyNamingAnotherParent(t *testing.T) {
	s := openTemp(t)
	parent, err := s.AddTask("tester", store.Attributes{Title: store.Set("Paint the fence")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	w := do(t, s, http.MethodPost, "/api/tasks/"+parent+"/subtasks",
		`{"title": "Buy the paint", "parent": "somewhere-else"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST subtasks with a parent answered %d, want 400: %s", w.Code, w.Body.String())
	}
}

// The refusal the terminal exits 3 on is 409 here, in the same sentence. It is
// the one lifecycle verb a Task can be in a state to be refused for, and the
// status comes off store.Refused rather than off a list of errors written out
// beside api.fail: store.ErrGone is neither ErrRefused nor ErrHeld, so a
// second copy here would have answered 500 for a write that was turned away.
//
// No Task Reopened is appended, which is the whole point of refusing. The
// Change History is append-only, so one written against a Task that stayed
// deleted is a reopening the record claims happened and nothing later can take
// back.
func TestReopeningADeletedTaskIsRefusedWithAConflict(t *testing.T) {
	s := openTemp(t)
	id, err := s.AddTask("tester", store.Attributes{Title: store.Set("Paint the fence")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/delete", ""); w.Code != http.StatusOK {
		t.Fatalf("POST delete answered %d: %s", w.Code, w.Body.String())
	}
	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/reopen", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("POST reopen on a deleted Task answered %d, want 409: %s", w.Code, w.Body.String())
	}
	// Asked of the store rather than repeated here, so the two cannot drift
	// apart -- the same reason attachments_test.go asks for its sentence.
	if sentence := said(t, w)["error"]; sentence != store.ErrGone.Error() {
		t.Errorf("the refusal reads %q, want the store's own %q", sentence, store.ErrGone)
	}

	// Not the log's length: write.Lifecycle takes the Lease before the store is
	// asked, so lease_taken and lease_released land on a refusal too. What must
	// not be there is the entry itself.
	history, err := s.HistoryOf(id)
	if err != nil {
		t.Fatalf("HistoryOf: %v", err)
	}
	for _, e := range history {
		if e.Kind == store.KindTaskReopened {
			t.Error("a refused reopening was written into the Change History")
		}
	}

	tasks, err := s.Tasks(store.Query{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	for _, task := range tasks {
		if task.ID == id && task.DeletedAt.IsZero() {
			t.Error("the refused reopening undeleted the Task anyway")
		}
	}
}
