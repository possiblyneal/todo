package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
)

// maxBody is as much as any route here reads. Every body is a Task somebody
// typed or a sentence they said, so a megabyte is already far more than one,
// and a client sending more than that is refused rather than held in memory.
const maxBody = 1 << 20

// taskBody is a Task's attributes and its memberships as the client sends
// them, and the same shape POST /api/capture answers in: what the Broker read
// comes back as the body the client submits once somebody has corrected it.
//
// Every attribute is a pointer because absent and empty mean different things.
// Absent leaves it alone, empty clears it, and that is write.Given's rule
// rather than a second one written here, which is why a typed `-title ""` and
// a sent `"title": ""` do the same thing.
type taskBody struct {
	Title       *string           `json:"title,omitempty"`
	Description *string           `json:"description,omitempty"`
	Why         *string           `json:"why,omitempty"`
	Color       *string           `json:"color,omitempty"`
	Deadline    *string           `json:"deadline,omitempty"`
	Estimate    *string           `json:"estimate,omitempty"`
	Priority    *string           `json:"priority,omitempty"`
	Impact      *string           `json:"impact,omitempty"`
	Snooze      *string           `json:"snooze,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`

	// Parent makes the new Task a Subtask of that one. A PATCH naming one is
	// refused: a Task is not moved by editing it.
	Parent string `json:"parent,omitempty"`

	// The memberships, by id, named the way write.Membership names them.
	IntoLists  []string `json:"intoLists,omitempty"`
	OutOfLists []string `json:"outOfLists,omitempty"`
	AddTags    []string `json:"addTags,omitempty"`
	DropTags   []string `json:"dropTags,omitempty"`
}

func (b taskBody) given() write.Given {
	return write.Given{
		Title:       b.Title,
		Description: b.Description,
		Why:         b.Why,
		Color:       b.Color,
		Deadline:    b.Deadline,
		Estimate:    b.Estimate,
		Priority:    b.Priority,
		Impact:      b.Impact,
		Snooze:      b.Snooze,
		Fields:      b.Fields,
	}
}

// saying is the other direction: what POST /api/capture answers with. The two
// are the same ten attributes because the body the client submits is the body
// it was given, corrected.
func saying(g write.Given, m write.Membership) taskBody {
	return taskBody{
		Title:       g.Title,
		Description: g.Description,
		Why:         g.Why,
		Color:       g.Color,
		Deadline:    g.Deadline,
		Estimate:    g.Estimate,
		Priority:    g.Priority,
		Impact:      g.Impact,
		Snooze:      g.Snooze,
		Fields:      g.Fields,
		IntoLists:   m.IntoLists,
		AddTags:     m.AddTags,
	}
}

func (b taskBody) membership() write.Membership {
	return write.Membership{
		IntoLists:  b.IntoLists,
		OutOfLists: b.OutOfLists,
		AddTags:    b.AddTags,
		DropTags:   b.DropTags,
	}
}

// addTask is POST /api/tasks: one Task written and filed under the Lease
// write.Add takes, and its id back. 201, because the id names something that
// did not exist before the request.
func addTask(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	in, err := decode[taskBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	given, err := in.given().Attributes()
	if err != nil {
		// A value read wrongly is the caller's mistake to fix, which is 400
		// here and exit status 2 at a terminal, and the sentence is the one
		// the verb would have printed.
		fail(w, usage{err})
		return
	}
	var a store.Attributes
	if given != nil {
		a = *given
	}
	// The store refuses an empty Title too, and this says so in the same
	// sentence one status earlier: a Task with no title is the caller asking
	// wrongly rather than the store refusing, which is what `todo add` says
	// with exit status 2 about the same mistake. The trimming that decides
	// whether a Title is empty is write.Given's, not this route's.
	if a.Title == nil || *a.Title == "" {
		fail(w, usage{errors.New("a task needs a title")})
		return
	}

	// The id comes back from write.Add whether or not the filing went
	// through, and it is dropped here where the filing failed: the client
	// polls the whole state a second later, so a Task written but not filed
	// arrives on the list by itself rather than needing to be named in an
	// error body that has room only for the sentence.
	id, err := write.Add(s, actor, in.Parent, a, in.membership())
	if err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusCreated, map[string]string{"id": id})
}

// editTask is PATCH /api/tasks/{id}: the attributes and every List and Tag the
// Task joins or leaves, under one Lease, which is write.Edit's shape. A body
// naming no attribute is a membership edit and nothing else.
func editTask(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	in, err := decode[taskBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	// A Task is not moved by editing it, so a body naming a parent is refused
	// rather than answered 200 with the parent quietly ignored. That is the
	// same answer decode gives a field this package does not know: a client
	// told its write went through is entitled to assume the whole of it did.
	if in.Parent != "" {
		fail(w, usage{errors.New("a task is not moved by editing it: leave out parent")})
		return
	}
	a, err := in.given().Attributes()
	if err != nil {
		fail(w, usage{err})
		return
	}
	// A Title sent empty is a Task with no title, which the store refuses and
	// which is the caller asking wrongly rather than anything going wrong
	// here. Saying so at 400 is the same mapping addTask makes one attribute
	// earlier; without it the store's plain error falls through to 500 for a
	// sentence a person can act on.
	if a != nil && a.Title != nil && *a.Title == "" {
		fail(w, usage{errors.New("a task needs a title")})
		return
	}

	id := r.PathValue("id")
	if err := write.Edit(s, actor, id, a, in.membership()); err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}

// decode reads one request body. A body that is not the JSON the route takes
// is the caller asking wrongly rather than anything going wrong here, so every
// caller wraps what comes back as usage. A field this package does not know is
// refused rather than ignored: a client with a typo in a name would otherwise
// be told its write went through with the attribute silently dropped.
func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var into T
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	d.DisallowUnknownFields()
	if err := d.Decode(&into); err != nil {
		return into, fmt.Errorf("the body is not the JSON this route takes: %w", err)
	}
	return into, nil
}
