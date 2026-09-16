package api

import (
	"net/http"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// kind is which of the two collections a route acts on. A List and a Tag are
// the same three writes against different aggregates, so the routes are written
// once and handed the pair of calls they act through, the way `todo lists` and
// `todo tags` are one verb given two sets of calls.
type kind struct {
	add      func(actor, name, color string) (string, error)
	describe func(actor, id string, name, color *string) error
	drop     func(actor, id string) error
}

func lists(s *store.Store) kind {
	return kind{add: s.AddList, describe: s.DescribeList, drop: s.DeleteList}
}

func tags(s *store.Store) kind {
	return kind{add: s.AddTag, describe: s.DescribeTag, drop: s.DeleteTag}
}

// collectionBody is a List or a Tag as the client sends one. Both attributes
// are pointers for the reason every attribute of a Task is: absent leaves it
// alone and present changes it, which is the difference between renaming a List
// and recoloring it in one body that carries the other field empty.
type collectionBody struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// addCollection is POST /api/lists and POST /api/tags. Creating is its own
// route because the store mints the id, so there is nothing to PUT to.
func addCollection(k kind, actor string, w http.ResponseWriter, r *http.Request) {
	in, err := decode[collectionBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	var name, color string
	if in.Name != nil {
		name = *in.Name
	}
	if in.Color != nil {
		color = *in.Color
	}
	id, err := k.add(actor, name, color)
	if err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusCreated, map[string]string{"id": id})
}

// describeCollection is PATCH /api/lists/{id} and PATCH /api/tags/{id}: the
// rename and the recolor, which are one write because they are one entry.
func describeCollection(k kind, actor string, w http.ResponseWriter, r *http.Request) {
	in, err := decode[collectionBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	id := r.PathValue("id")
	if err := k.describe(actor, id, in.Name, in.Color); err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}

// dropCollection is DELETE /api/lists/{id} and DELETE /api/tags/{id}. Every
// Task that carried it goes on existing and loses the membership, which is the
// store's transaction rather than anything this route arranges.
func dropCollection(k kind, actor string, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := k.drop(actor, id); err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}
