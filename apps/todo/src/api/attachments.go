package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/store"
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
// A literal segment beats the wildcard `{verb}` route in this mux, so
// `attachments` never reaches the lifecycle handler.
func attach(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	pointed(s, actor, w, r, func(target string) error {
		return s.Attach(actor, r.PathValue("id"), target)
	})
}

func detach(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	pointed(s, actor, w, r, func(target string) error {
		return s.Detach(actor, r.PathValue("id"), target)
	})
}

// pointed reads the target and answers the write, under the Lease every write
// to a Task needs. Adding and removing differ only in the store call, the way a
// List and a Tag differ only in theirs.
func pointed(s *store.Store, actor string, w http.ResponseWriter, r *http.Request, write func(target string) error) {
	in, err := decode[attachmentBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	// A body carrying no target is the caller asking wrongly, which is the
	// wire shape's rule rather than the store's: `store.pointer` decides what
	// a pointer is, and what it decides comes back as the store's own refusal.
	if strings.TrimSpace(in.Target) == "" {
		fail(w, usage{fmt.Errorf("an attachment needs a target: somewhere to point")})
		return
	}
	id := r.PathValue("id")
	if err := s.WithLease(actor, id, store.WriteTTL, func() error {
		return write(in.Target)
	}); err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}
