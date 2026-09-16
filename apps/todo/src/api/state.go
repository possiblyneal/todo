package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// state is GET /api/state: everything one screen needs in one response, which
// is the tree the list draws plus the Lists and Tags its sidebar ranks.
//
// It is a read, so nothing is attributed and no Lease is taken.
func state(s *store.Store, w http.ResponseWriter, r *http.Request) {
	q, err := query(r)
	if err != nil {
		fail(w, err)
		return
	}

	// The ETag is the write-ahead log's token hashed with the query that
	// produced the response. The token alone is store-global while this
	// response is not: a client changing its filter or its sort with no
	// write in between would be answered 304 for a different representation.
	//
	// An empty token means the log could not be stat'd rather than that
	// nothing has changed, so there is nothing to compare and the response
	// carries no ETag at all.
	if token := s.WALToken(); token != "" {
		sum := sha256.Sum256([]byte(token + "\x00" + r.URL.RawQuery))
		tag := `"` + hex.EncodeToString(sum[:]) + `"`
		w.Header().Set("ETag", tag)
		if slices.Contains(r.Header.Values("If-None-Match"), tag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	tasks, err := s.Tasks(q)
	if err != nil {
		fail(w, err)
		return
	}
	lists, err := s.Lists()
	if err != nil {
		fail(w, err)
		return
	}
	tags, err := s.Tags()
	if err != nil {
		fail(w, err)
		return
	}

	// Every collection is allocated empty rather than left nil: a nil slice
	// encodes as `null`, and a client that has to tell `null` from `[]` before
	// it can count is being asked a question the store never asks.
	out := stateBody{
		Tasks: make([]task, 0, len(tasks)),
		Lists: make([]collection, 0, len(lists)),
		Tags:  make([]collection, 0, len(tags)),
	}
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, newTask(t))
	}
	for _, l := range lists {
		out.Lists = append(out.Lists, collection{ID: l.ID, Name: l.Name, Color: l.Color, Count: l.Count})
	}
	for _, g := range tags {
		out.Tags = append(out.Tags, collection{ID: g.ID, Name: g.Name, Color: g.Color, Count: g.Count})
	}
	write(w, http.StatusOK, out)
}

// query reads a Query out of the request, on the same four narrowings and the
// same names `todo list` takes them under. An unknown sort is a usage error
// here because it is one there: the store's set is the only set.
func query(r *http.Request) (store.Query, error) {
	all := r.URL.Query().Get("all") == "true"
	q := store.Query{
		IncludeCompleted: all,
		IncludeDeclined:  all,
		IncludeSnoozed:   all,
		IncludeDeleted:   all,
		List:             r.URL.Query().Get("list"),
	}
	if sort := r.URL.Query().Get("sort"); sort != "" {
		if !slices.Contains(store.Sorts, store.Sort(sort)) {
			return q, fmt.Errorf("%w: sort is one of %v, not %q", errUsage, store.SortNames(), sort)
		}
		q.Sort = store.Sort(sort)
	}
	return q, nil
}

type stateBody struct {
	Tasks []task       `json:"tasks"`
	Lists []collection `json:"lists"`
	Tags  []collection `json:"tags"`
}

// collection is a List or a Tag as the client reads it. The two are the same
// shape on the wire because they are the same shape in the store, and which
// one this is, is said by the field it arrives under.
type collection struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

// task is a store.Task as the client reads it. It is written out here rather
// than tagged onto the store's own type so that the wire shape is this
// package's to keep: Marks is a read's conclusion rather than a field, and a
// zero time is absent rather than "0001-01-01T00:00:00Z".
type task struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty"`
	Depth  int    `json:"depth"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Why         string `json:"why,omitempty"`
	Color       string `json:"color,omitempty"`

	CreatedAt    string `json:"createdAt"`
	Deadline     string `json:"deadline,omitempty"`
	SnoozedUntil string `json:"snoozedUntil,omitempty"`
	CompletedAt  string `json:"completedAt,omitempty"`
	DeclinedAt   string `json:"declinedAt,omitempty"`
	DeletedAt    string `json:"deletedAt,omitempty"`

	// EstimateSeconds is a number rather than a duration string because
	// JavaScript has no duration and the client formats it itself.
	EstimateSeconds int64 `json:"estimateSeconds,omitempty"`

	Priority string `json:"priority,omitempty"`
	Impact   string `json:"impact,omitempty"`

	Lists       []string          `json:"lists,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Attachments []string          `json:"attachments,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	Series      string            `json:"series,omitempty"`

	// Marks is what the read worked out, in the words store.Task.Marks says
	// them in, so a Task cannot read as snoozed on one surface and plain on
	// another.
	Marks []string `json:"marks"`
}

func newTask(t store.Task) task {
	return task{
		ID:              t.ID,
		Parent:          t.Parent,
		Depth:           t.Depth,
		Title:           t.Title,
		Description:     t.Description,
		Why:             t.Why,
		Color:           t.Color,
		CreatedAt:       stamp(t.CreatedAt),
		Deadline:        stamp(t.Deadline),
		SnoozedUntil:    stamp(t.SnoozedUntil),
		CompletedAt:     stamp(t.CompletedAt),
		DeclinedAt:      stamp(t.DeclinedAt),
		DeletedAt:       stamp(t.DeletedAt),
		EstimateSeconds: int64(t.Estimate / time.Second),
		Priority:        string(t.Priority),
		Impact:          string(t.Impact),
		Lists:           t.Lists,
		Tags:            t.Tags,
		Attachments:     t.Attachments,
		Fields:          t.Fields,
		Series:          t.Series,
		Marks:           marks(t),
	}
}

// marks is Task.Marks with nothing to say written as an empty list rather than
// as `null`, for the same reason the collections above are.
func marks(t store.Task) []string {
	if m := t.Marks(); m != nil {
		return m
	}
	return []string{}
}

// stamp writes a time as RFC 3339, and a zero time as nothing at all: a Task
// with no deadline has no deadline rather than one in year one.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
