package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// state is GET /api/state: everything one screen needs in one response, which
// is the tree the list draws, the Lists and Tags it can be narrowed by, and
// the sorts it can be ordered through.
//
// It is a read, so nothing is attributed and no Lease is taken.
func state(s *store.Store, w http.ResponseWriter, r *http.Request) {
	// The ETag is the write-ahead log's token hashed with the query that
	// produced the response. The token alone is store-global while this
	// response is not: a client changing its filter or its sort with no
	// write in between would be answered 304 for a different representation.
	//
	// An empty token means the log could not be stat'd rather than that
	// nothing has changed, so there is nothing to compare and the response
	// carries no ETag at all.
	tag := ""
	if token := s.WALToken(); token != "" {
		sum := sha256.Sum256([]byte(token + "\x00" + r.URL.RawQuery))
		tag = `"` + hex.EncodeToString(sum[:]) + `"`
		if matches(r.Header.Values("If-None-Match"), tag) {
			w.Header().Set("ETag", tag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	tasks, err := narrowed(s, r)
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
		Sorts: store.SortNames(),

		Colors:  store.ColorNames(),
		Snoozes: store.SnoozeNames(),
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

	// The ETag goes on the response that was actually sent. Setting it before
	// the reads would put a tag on an error body too, and a client handing
	// that one back would be answered 304 for a screen it never received.
	if tag != "" {
		w.Header().Set("ETag", tag)
	}
	send(w, http.StatusOK, out)
}

// matches reads If-None-Match the way RFC 9110 writes it: several tags to one
// header line separated by commas, a weak tag marked `W/`, and `*` for any
// representation at all. Comparison is weak, which is what a GET of an
// unchanged response wants; only a range request needs the strong kind.
func matches(values []string, tag string) bool {
	for _, value := range values {
		for _, one := range strings.Split(value, ",") {
			one = strings.TrimSpace(one)
			if one == "*" || strings.TrimPrefix(one, "W/") == tag {
				return true
			}
		}
	}
	return false
}

// query reads a Query out of the request, on the same narrowings and the same
// names `todo list` takes them under. An unknown sort is not refused
// here: the store keeps the list of sorts, so the store is what refuses one.
func query(r *http.Request) store.Query {
	all := r.URL.Query().Get("all") == "true"
	return store.Query{
		IncludeCompleted: all,
		IncludeDeclined:  all,
		IncludeSnoozed:   all,
		IncludeDeleted:   all,
		List:             r.URL.Query().Get("list"),
		Tag:              r.URL.Query().Get("tag"),
		Search:           r.URL.Query().Get("search"),
		Sort:             store.Sort(r.URL.Query().Get("sort")),
	}
}

// narrowed reads the Tasks a request asked for, and turns the store's refusal
// of an unknown sort into a caller's error.
//
// The store holds the only list of sorts there is, so it is the store that
// turns an unknown one away and the store's sentence that says so. Reaching
// that refusal means the caller asked wrongly, which is 400 here and exit
// status 2 at a terminal; anything else went wrong.
//
// Every route that narrows by the query string reads it through this, so a
// sort the store does not have is refused the same way wherever it is sent. An
// Agent asking a question under a bad sort gets the answer a person's list
// gets, rather than a 500 for the same mistake.
func narrowed(s *store.Store, r *http.Request) ([]store.Task, error) {
	q := query(r)
	tasks, err := s.Tasks(q)
	if err != nil && q.Sort != "" && !slices.Contains(store.Sorts, q.Sort) {
		return nil, usage{err}
	}
	return tasks, err
}

type stateBody struct {
	Tasks []task       `json:"tasks"`
	Lists []collection `json:"lists"`
	Tags  []collection `json:"tags"`

	// Sorts is what ?sort= accepts, from store.Sorts, so a surface offering
	// the choice does not keep its own list of it. It is the same set the
	// store refuses an unknown sort against, which is what stops a picker
	// offering one the store would turn away.
	Sorts []string `json:"sorts"`

	// Colors is store.Colors by name, for the same reason Sorts is here: a
	// color is one of nine or it is refused, so a surface offering the choice
	// would otherwise keep a second copy of the nine and offer a tenth the day
	// one is added here and not there. The ANSI code each carries does not
	// cross: it is what a terminal paints with, and the name is the whole of
	// what is stored.
	Colors []string `json:"colors"`

	// Snoozes is store.SnoozeDefaults by label, which is what the offered
	// snoozes are called rather than all a snooze can be: write.Snooze reads a
	// plain duration too, so a surface may send one of these or a duration of
	// its own.
	Snoozes []string `json:"snoozes"`
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
		CreatedAt:       rfc3339(t.CreatedAt),
		Deadline:        rfc3339(t.Deadline),
		SnoozedUntil:    rfc3339(t.SnoozedUntil),
		CompletedAt:     rfc3339(t.CompletedAt),
		DeclinedAt:      rfc3339(t.DeclinedAt),
		DeletedAt:       rfc3339(t.DeletedAt),
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

// rfc3339 writes a time as RFC 3339, and a zero time as nothing at all: a Task
// with no deadline has no deadline rather than one in year one.
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
