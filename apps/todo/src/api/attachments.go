package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
)

// attachmentBody is the one thing an Attachment is: the text naming somewhere
// else. It is not a pointer field, because there is no absent-leaves-it-alone
// here -- adding and removing both act on a target, and a body without one is
// a request that says nothing.
type attachmentBody struct {
	Target string `json:"target"`
}

// attach is POST /api/tasks/{id}/attachments, and detach is DELETE on the same
// path. The target is in the body rather than the path because a pointer is a
// file path or a web address and carries its own slashes, which a path segment
// cannot hold without being escaped into something no one can read in a log.
//
// A literal segment beats the wildcard `{verb}` route in this mux, so a
// `POST` to `attachments` never reaches the lifecycle handler. The `DELETE`
// shares the path and not that hazard: the lifecycle route is `POST` only.
func attach(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	pointed(s, actor, w, r, write.Attach)
}

func detach(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	pointed(s, actor, w, r, write.Detach)
}

// pointed reads the target and answers the write. Adding and removing differ
// only in which of the two write calls they are given, the way a List and a Tag
// differ only in theirs.
func pointed(
	s *store.Store,
	actor string,
	w http.ResponseWriter,
	r *http.Request,
	point func(*store.Store, string, string, string) error,
) {
	in, err := decode[attachmentBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	// A target with nothing in it is a Task pointed nowhere, which the store
	// refuses and which is the caller asking wrongly rather than anything
	// going wrong here. Saying so at 400 is the mapping editTask makes for a
	// Title sent empty; without it the store's plain error falls through to
	// 500 for a sentence a person can act on. The sentence is the store's own,
	// so the browser and the terminal say the same thing about this mistake,
	// and the route's test asks the store for it rather than repeating it.
	if strings.TrimSpace(in.Target) == "" {
		fail(w, usage{errors.New("an Attachment needs somewhere to point")})
		return
	}
	id := r.PathValue("id")
	if err := point(s, actor, id, in.Target); err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}
