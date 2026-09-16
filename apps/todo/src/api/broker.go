package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
)

// capture is POST /api/capture: somebody's words read into a Task, and nothing
// written. What comes back is what the Broker said, in the shape POST
// /api/tasks takes, so the client fills the add sheet with it and submitting
// is the only thing that writes. A dump read and then abandoned leaves nothing
// behind, which is what keeps the form the gate on a surface a person is at.
//
// The attributes come back as the Broker said them rather than parsed. A
// duration it wrote in a way this program cannot read belongs in the field for
// somebody to correct, not dropped on the way to a screen they are looking at:
// dropping it is `todo capture`'s rule, and that verb has nobody left to ask.
func capture(s *store.Store, c *ai.Client, w http.ResponseWriter, r *http.Request) {
	in, err := decode[captureBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		fail(w, usage{errors.New("say what the task is")})
		return
	}

	dump, err := write.Gather(s, text)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), ai.Patience)
	defer cancel()
	read, err := c.Read(ctx, dump.Shown)
	if err != nil {
		fail(w, err)
		return
	}

	// The Lists and Tags it chose are answered by id rather than by the names
	// it chose them under, because an id is what the write takes and matching
	// a name back to one is this side's job either way.
	filed := dump.Filed(read)
	out := taskBody{IntoLists: filed.IntoLists, AddTags: filed.AddTags}
	for _, said := range []struct {
		value string
		to    **string
	}{
		{read.Title, &out.Title},
		{read.Description, &out.Description},
		{read.Why, &out.Why},
		{read.Deadline, &out.Deadline},
		{read.Estimate, &out.Estimate},
		{read.Priority, &out.Priority},
		{read.Impact, &out.Impact},
	} {
		if said.value != "" {
			*said.to = &said.value
		}
	}
	send(w, http.StatusOK, out)
}

// ask is POST /api/ask: a question about the Tasks in view, answered as prose.
// It is a read like the list it is about: no Lease, nothing appended, and
// nothing kept between calls, which is why the whole list goes with every
// question.
//
// Which Tasks are in view is the query string, read the way GET /api/state
// reads it, so a question asked under a filter is asked about what the filter
// left on the screen.
func ask(s *store.Store, c *ai.Client, w http.ResponseWriter, r *http.Request) {
	in, err := decode[askBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	question := strings.TrimSpace(in.Question)
	if question == "" {
		fail(w, usage{errors.New("say what the question is")})
		return
	}

	tasks, err := s.Tasks(query(r))
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), ai.Patience)
	defer cancel()
	answer, err := c.Ask(ctx, question, write.Briefs(tasks))
	if err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"answer": answer})
}

// captureBody is the dump: the words somebody typed, and nothing else. What
// the Broker may file the Task under is the store's answer rather than the
// client's, so it is gathered here.
type captureBody struct {
	Text string `json:"text"`
}

// askBody is the question. The Tasks it is about are the query string's.
type askBody struct {
	Question string `json:"question"`
}
