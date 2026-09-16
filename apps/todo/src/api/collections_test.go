package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// screen reads the whole screen back the way the client does, for a test that
// cares what a write looks like from the outside rather than in the store.
func screen(t *testing.T, s *store.Store) stateBody {
	t.Helper()
	w := do(t, s, http.MethodGet, "/api/state", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/state answered %d: %s", w.Code, w.Body.String())
	}
	var out stateBody
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return out
}

// A List and a Tag are the same three writes against different aggregates, and
// the routes are registered from one table, so both are exercised the same way
// here: whichever pair of store calls a route was handed is the one it made.
func TestCollectionsAreCreatedRenamedAndRemoved(t *testing.T) {
	for _, noun := range []string{"lists", "tags"} {
		t.Run(noun, func(t *testing.T) {
			s := openTemp(t)
			// The pair is read back through GET /api/state, which is where
			// the client sees them, so the rename this makes is the rename the
			// screen would draw.
			carried := func() []collection {
				t.Helper()
				out := screen(t, s)
				if noun == "lists" {
					return out.Lists
				}
				return out.Tags
			}

			w := do(t, s, http.MethodPost, "/api/"+noun, `{"name": "House", "color": "blue"}`)
			if w.Code != http.StatusCreated {
				t.Fatalf("POST answered %d, want 201: %s", w.Code, w.Body.String())
			}
			id := said(t, w)["id"]

			// A rename and a recolor are one write because they are one entry,
			// and a body naming one attribute leaves the other alone.
			if w := do(t, s, http.MethodPatch, "/api/"+noun+"/"+id, `{"name": "Home"}`); w.Code != http.StatusOK {
				t.Fatalf("PATCH answered %d, want 200: %s", w.Code, w.Body.String())
			}
			rows := carried()
			if len(rows) != 1 || rows[0].Name != "Home" || rows[0].Color != "blue" {
				t.Errorf("after the rename the store holds %+v, want one named Home still blue", rows)
			}

			if w := do(t, s, http.MethodDelete, "/api/"+noun+"/"+id, ""); w.Code != http.StatusOK {
				t.Fatalf("DELETE answered %d, want 200: %s", w.Code, w.Body.String())
			}
			if rows := carried(); len(rows) != 0 {
				t.Errorf("after the delete the store holds %+v, want none", rows)
			}
		})
	}
}

// Deleting a List takes it off the Tasks that were in it and leaves them, which
// is the store's transaction. The route arranges none of it, and this says so
// from the outside.
func TestDeletingAListLeavesItsTasks(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("tester", "House", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint the fence", "intoLists": ["`+list+`"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}

	if w := do(t, s, http.MethodDelete, "/api/lists/"+list, ""); w.Code != http.StatusOK {
		t.Fatalf("DELETE answered %d: %s", w.Code, w.Body.String())
	}
	task := only(t, s)
	if len(task.Lists) != 0 {
		t.Errorf("the task is still in %v, want the membership gone with the list", task.Lists)
	}
}
