package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func openTemp(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func add(t *testing.T, s *store.Store, title string) string {
	t.Helper()
	id, err := s.AddTask("tester", store.Attributes{Title: &title})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	return id
}

// get runs one request against the whole handler, which is what a client
// reaches: routing included, so a route that moved is a failing test.
func get(t *testing.T, s *store.Store, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	for k, values := range header {
		for _, v := range values {
			r.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	Handler(s, Options{}).ServeHTTP(w, r)
	return w
}

func decodeState(t *testing.T, w *httptest.ResponseRecorder) stateBody {
	t.Helper()
	var body stateBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return body
}

func TestStateReturnsTheTreeAndItsCollections(t *testing.T) {
	s := openTemp(t)
	parent := add(t, s, "Paint the fence")
	if err := s.WithLease("tester", parent, store.WriteTTL, func() error {
		_, err := s.AddSubtask("tester", parent, store.Attributes{Title: ptr("Buy paint")})
		return err
	}); err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	if _, err := s.AddList("tester", "Home", "blue"); err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if _, err := s.AddTag("tester", "errand", "green"); err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	w := get(t, s, "/api/state", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeState(t, w)

	if len(body.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(body.Tasks))
	}
	// Tasks comes back depth first, so the Subtask follows its parent and
	// carries the depth the client indents by.
	if body.Tasks[0].Depth != 1 || body.Tasks[1].Depth != 2 {
		t.Errorf("depths = %d, %d, want 1, 2", body.Tasks[0].Depth, body.Tasks[1].Depth)
	}
	if body.Tasks[1].Parent != parent {
		t.Errorf("parent = %q, want %q", body.Tasks[1].Parent, parent)
	}
	if len(body.Lists) != 1 || len(body.Tags) != 1 {
		t.Errorf("lists = %d, tags = %d, want 1 and 1", len(body.Lists), len(body.Tags))
	}
}

func TestStateSaysWhatAReadWorkedOut(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Ship it")
	if err := s.WithLease("tester", id, store.WriteTTL, func() error {
		return s.CompleteTask("tester", id)
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// A completed Task is out of the everyday view, so it takes all=true to
	// see it at all, and it arrives carrying the mark rather than a field the
	// client would have to work the mark out from.
	body := decodeState(t, get(t, s, "/api/state?all=true", nil))
	if len(body.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(body.Tasks))
	}
	if got := body.Tasks[0].Marks; len(got) != 1 || got[0] != "done" {
		t.Errorf("marks = %v, want [done]", got)
	}
	if body.Tasks[0].CompletedAt == "" {
		t.Error("completedAt is empty on a completed Task")
	}
	// A Task with no deadline has no deadline, rather than one in year one.
	if body.Tasks[0].Deadline != "" {
		t.Errorf("deadline = %q, want empty", body.Tasks[0].Deadline)
	}
}

func TestStateAnswers304OnlyForTheSameRepresentation(t *testing.T) {
	s := openTemp(t)
	add(t, s, "Water the plants")

	first := get(t, s, "/api/state", nil)
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on a store that has been written to")
	}

	again := get(t, s, "/api/state", http.Header{"If-None-Match": {tag}})
	if again.Code != http.StatusNotModified {
		t.Errorf("unchanged store = %d, want 304", again.Code)
	}

	// The token is store-global and the response is not: a different query is
	// a different representation, whether or not anything has been written.
	other := get(t, s, "/api/state?all=true", http.Header{"If-None-Match": {tag}})
	if other.Code != http.StatusOK {
		t.Errorf("different query = %d, want 200", other.Code)
	}
	if other.Header().Get("ETag") == tag {
		t.Error("a different query carries the same ETag")
	}

	add(t, s, "Feed the cat")
	written := get(t, s, "/api/state", http.Header{"If-None-Match": {tag}})
	if written.Code != http.StatusOK {
		t.Errorf("after a write = %d, want 200", written.Code)
	}
}

func TestStateRefusesASortTheStoreDoesNotHave(t *testing.T) {
	s := openTemp(t)
	w := get(t, s, "/api/state?sort=whenever", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] == "" {
		t.Errorf("body = %v, want an error sentence", body)
	}
}

func TestStateTakesEverySortTheStoreHas(t *testing.T) {
	s := openTemp(t)
	add(t, s, "Sortable")
	for _, sort := range store.Sorts {
		w := get(t, s, "/api/state?sort="+string(sort), nil)
		if w.Code != http.StatusOK {
			t.Errorf("sort=%s = %d, want 200", sort, w.Code)
		}
	}
}

// The sorts the response offers are the sorts it accepts. A surface drawing a
// picker from this must not be able to offer one the store would refuse, which
// is the whole reason the list is on the wire rather than copied client-side.
func TestStateOffersExactlyTheSortsItAccepts(t *testing.T) {
	s := openTemp(t)
	state := decodeState(t, get(t, s, "/api/state", nil))

	if len(state.Sorts) != len(store.Sorts) {
		t.Fatalf("sorts = %v, want %v", state.Sorts, store.SortNames())
	}
	for i, sort := range store.Sorts {
		if state.Sorts[i] != string(sort) {
			t.Errorf("sorts[%d] = %q, want %q", i, state.Sorts[i], sort)
		}
		if w := get(t, s, "/api/state?sort="+state.Sorts[i], nil); w.Code != http.StatusOK {
			t.Errorf("sort=%s = %d, want 200", state.Sorts[i], w.Code)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// An empty list on the wire is `[]` and never `null`, in every place one can
// appear. The client counts these without checking them first, which a `null`
// would make a crash rather than a zero.
func TestStateWritesAnEmptyListAsAList(t *testing.T) {
	s := openTemp(t)
	add(t, s, "Nothing hangs off this one")

	w := get(t, s, "/api/state", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); strings.Contains(body, "null") {
		t.Errorf("the response carries a null: %s", body)
	}

	state := decodeState(t, w)
	if state.Lists == nil || state.Tags == nil {
		t.Errorf("Lists = %v, Tags = %v, want both empty rather than nil", state.Lists, state.Tags)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].Marks == nil {
		t.Errorf("Marks = %v, want empty rather than nil", state.Tasks[0].Marks)
	}
}

// The store keeps the only list of sorts there is, so the sentence a bad one
// gets is the store's own, word for word, rather than a second phrasing this
// package keeps beside it.
func TestStateRefusesASortInTheStoresOwnWords(t *testing.T) {
	s := openTemp(t)
	w := get(t, s, "/api/state?sort=nope", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	_, err := s.Tasks(store.Query{Sort: "nope"})
	if err == nil {
		t.Fatal("the store took a sort it does not have")
	}
	var body map[string]string
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if body["error"] != err.Error() {
		t.Errorf("body = %q, want the store's own %q", body["error"], err.Error())
	}
}

// An If-None-Match is read the way RFC 9110 writes one: several tags to a
// line, and a weak one still matching. A client whose tags arrive joined by a
// proxy would otherwise re-read the whole list on every poll.
func TestStateReads304FromEveryShapeOfIfNoneMatch(t *testing.T) {
	s := openTemp(t)
	add(t, s, "Water the plants")
	tag := get(t, s, "/api/state", nil).Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on a store that has been written to")
	}

	for _, sent := range []string{tag, `"other", ` + tag, "W/" + tag, "*"} {
		w := get(t, s, "/api/state", http.Header{"If-None-Match": {sent}})
		if w.Code != http.StatusNotModified {
			t.Errorf("If-None-Match: %s = %d, want 304", sent, w.Code)
		}
	}

	w := get(t, s, "/api/state", http.Header{"If-None-Match": {`"nothing like it"`}})
	if w.Code != http.StatusOK {
		t.Errorf("a tag naming another representation = %d, want 200", w.Code)
	}
}

// A route this package does not serve is a usage error with a sentence in it,
// even with the client's files being served underneath. The fallback answers any
// path it is given, so an /api/ typo would otherwise be index.html at 200 and
// the client would fail parsing HTML as JSON instead of saying what happened.
func TestAnAPIRouteThatIsNotOneIsNotTheClient(t *testing.T) {
	s := openTemp(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>todo</title>"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	serve := func(method, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		Handler(s, Options{Web: dir}).ServeHTTP(w, httptest.NewRequest(method, target, nil))
		return w
	}

	for _, r := range []struct{ method, target string }{
		{http.MethodGet, "/api/stat"},
		{http.MethodPost, "/api/state"},
		{http.MethodGet, "/api/tasks/nope"},
	} {
		w := serve(r.method, r.target)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400 rather than the client", r.method, r.target, w.Code)
		}
		if strings.Contains(w.Body.String(), "<title>") {
			t.Errorf("%s %s was answered with the client's index.html", r.method, r.target)
		}
	}

	// A path the client routes in the browser still reaches the client.
	if w := serve(http.MethodGet, "/activity"); w.Code != http.StatusOK {
		t.Errorf("/activity = %d, want the client at 200", w.Code)
	}

	// Serving the JSON alone says the same thing about the same bad route, so
	// a client on its own dev server is not told something different from one
	// reading the files this process serves.
	alone := get(t, s, "/api/typo", nil)
	if alone.Code != http.StatusBadRequest {
		t.Errorf("/api/typo with no client served = %d, want 400", alone.Code)
	}
	if !strings.Contains(alone.Body.String(), "is not a route") {
		t.Errorf("body = %s, want a sentence saying so", alone.Body.String())
	}
}

// A path that climbs out of the served directory reaches the client's
// index.html, never the file it named. http.Dir refuses the name before
// anything is opened, and this test asks client directly because ServeMux
// cleans a path of its own accord and would never hand one like this over.
func TestAPathThatClimbsOutOfTheServedDirectoryDoesNot(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "web")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>todo</title>"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret"), []byte("not for the browser"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for _, target := range []string{"/../secret", "/..%2fsecret", "/web/../../secret"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "http://todo.test", nil)
		r.URL.Path = target
		client(dir).ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), "not for the browser") {
			t.Errorf("%s was served the file above the directory", target)
		}
	}
}

// The Tag and the search reach the read the same way the List does, under the
// names `todo list` takes them under. An Agent narrows by query string and a
// person's client narrows by query string, so what a client can ask for is what
// this route accepts and no less.
func TestStateNarrowsByTagAndBySearch(t *testing.T) {
	s := openTemp(t)
	fence := add(t, s, "Paint the fence")
	shop := add(t, s, "Buy paint")
	tag, err := s.AddTag("tester", "errand", "green")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	if err := s.WithLease("tester", shop, store.WriteTTL, func() error {
		return s.AttachTag("tester", shop, tag)
	}); err != nil {
		t.Fatalf("AttachTag: %v", err)
	}

	body := decodeState(t, get(t, s, "/api/state?tag="+tag, nil))
	if len(body.Tasks) != 1 || body.Tasks[0].ID != shop {
		t.Errorf("tag narrowing gave %d tasks, want only the tagged one", len(body.Tasks))
	}

	body = decodeState(t, get(t, s, "/api/state?search=fence", nil))
	if len(body.Tasks) != 1 || body.Tasks[0].ID != fence {
		t.Errorf("search gave %d tasks, want only the one whose words hold it", len(body.Tasks))
	}
}

// The colors and the snoozes are the store's own lists, served for the same
// reason the sorts are: a surface offering either would otherwise keep a second
// copy and go on offering what the store stopped taking. Each is checked
// against the store's list rather than against nine and four written out here,
// which would be this test keeping the copy instead.
func TestStateOffersTheStoresColorsAndSnoozes(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Paint the fence")
	state := decodeState(t, get(t, s, "/api/state", nil))

	if got, want := state.Colors, store.ColorNames(); !slices.Equal(got, want) {
		t.Errorf("colors = %v, want %v", got, want)
	}
	if got, want := state.Snoozes, store.SnoozeNames(); !slices.Equal(got, want) {
		t.Errorf("snoozes = %v, want %v", got, want)
	}

	// Every one offered is one a write takes, which is what stops the lists
	// being a menu with entries the store turns away.
	for _, color := range state.Colors {
		w := do(t, s, http.MethodPatch, "/api/tasks/"+id, `{"color": "`+color+`"}`)
		if w.Code != http.StatusOK {
			t.Errorf("color %s = %d, want 200 (%s)", color, w.Code, w.Body.String())
		}
	}
	for _, snooze := range state.Snoozes {
		w := do(t, s, http.MethodPatch, "/api/tasks/"+id, `{"snooze": "`+snooze+`"}`)
		if w.Code != http.StatusOK {
			t.Errorf("snooze %s = %d, want 200 (%s)", snooze, w.Code, w.Body.String())
		}
	}
}
