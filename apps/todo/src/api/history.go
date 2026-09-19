package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// historyPage is how many entries GET /api/history answers with when the
// caller does not say. The whole log is decoded by store.History and this route
// exists so a screen does not have to do that on every load, so there is a
// default rather than "all of it" to fall back to.
const historyPage = 200

// historyLimit is as many as one request can ask for. A caller asking for more
// than this gets this, rather than a refusal: the number is a page size and not
// an assertion about anything, so there is no mistake to report.
const historyLimit = 1000

// entry is one appended fact as the client reads it. The Actor is verbatim: the
// convention that an Agent names itself `<harness>/<model>` is read by whoever
// draws it, and this side recognises no model by name.
type entry struct {
	Seq     int64  `json:"seq"`
	At      string `json:"at"`
	Actor   string `json:"actor"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`

	// Payload is the entry's own body, passed through as the JSON it is
	// stored as. An entry whose payload is not JSON carries none rather than
	// breaking the encoding of the whole page around it.
	Payload json.RawMessage `json:"payload,omitempty"`
}

type historyBody struct {
	Entries []entry `json:"entries"`
}

// taskHistory is GET /api/tasks/{id}/history: what the Change History holds
// about one Task, newest first, which is the order the detail screen reads it
// in. Every entry the log holds for that subject is here, Lease bookkeeping
// included: the store narrows and a screen decides what is worth drawing.
func taskHistory(s *store.Store, w http.ResponseWriter, r *http.Request) {
	entries, err := s.HistoryOf(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	// The store reads a log as a log, which is oldest first. Newest first is
	// what both screens draw, so the reversing happens once, here, rather than
	// in each of them.
	slices.Reverse(entries)
	send(w, http.StatusOK, newHistory(entries))
}

// taskAsOf is GET /api/tasks/{id}/at/{seq}: the Task as it stood when that
// entry was appended, which is how a deleted Task is looked at now that no
// list read offers one. It is not only for a deletion: any entry answers, so
// "what did this say before that edit" is a question with an answer.
//
// The replay is the store's, through the store's own triggers. Nothing here
// knows what a kind does to a Task.
func taskAsOf(s *store.Store, w http.ResponseWriter, r *http.Request) {
	asked := r.PathValue("seq")
	seq, err := strconv.ParseInt(asked, 10, 64)
	if err != nil || seq < 1 {
		fail(w, usage{fmt.Errorf("cannot read %q as a position in the Change History: want a whole number of at least 1", asked)})
		return
	}
	was, err := s.TaskAsOf(r.PathValue("id"), seq)
	switch {
	case errors.Is(err, store.ErrAbsent):
		fail(w, missing{err})
		return
	case err != nil:
		fail(w, err)
		return
	}
	send(w, http.StatusOK, newTask(was))
}

// history is GET /api/history: the same rows across every Task, newest first
// and bounded to a page, which is what the activity screen reads.
//
// `?search=` narrows the whole log rather than the page, which is why it is a
// parameter here and not something the screen does to what it already holds: a
// screen reaches further back by asking for more, so a match it made itself
// could only find what had already arrived. It is the store that matches, the
// same way it is the store that matches the search over the list.
func history(s *store.Store, w http.ResponseWriter, r *http.Request) {
	limit, err := page(r, "entries", historyPage, historyLimit)
	if err != nil {
		fail(w, err)
		return
	}
	entries, err := s.LatestHistory(limit, r.URL.Query().Get("search"))
	if err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, newHistory(entries))
}

// newHistory writes entries out in the order they were given in. Nothing is
// dropped and nothing is folded: both routes answer with the rows as they are.
func newHistory(entries []store.Entry) historyBody {
	out := historyBody{Entries: make([]entry, 0, len(entries))}
	for _, e := range entries {
		one := entry{
			Seq:     e.Seq,
			At:      rfc3339(e.At),
			Actor:   e.Actor,
			Kind:    e.Kind,
			Subject: e.Subject,
		}
		if json.Valid([]byte(e.Payload)) {
			one.Payload = json.RawMessage(e.Payload)
		}
		out.Entries = append(out.Entries, one)
	}
	return out
}
