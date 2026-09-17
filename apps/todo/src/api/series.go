package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
)

// occurrenceWindow is how far ahead GET /api/tasks/{id}/series looks. Dates
// are computed as they are answered and nothing is stored by asking, so the
// window is two years because that is what `todo repeat` shows and the two
// surfaces should not disagree about which dates are next.
//
// It is a window and not a page: a rule sparser than it, `every 5 years`, has
// fewer dates in two years than a page holds, and the route answers the few it
// produces rather than reaching further to fill the page. There is no offset,
// so what falls outside the window is not reachable by asking differently.
const occurrenceWindow = 2

// occurrencePage is how many dates the route answers with when the caller does
// not say, and occurrenceLimit is as many as one request can ask for. As in
// GET /api/history, asking for more than the limit gets the limit rather than
// a refusal: it is a page size and not an assertion about anything.
const (
	occurrencePage  = 20
	occurrenceLimit = 200
)

// seriesBody is what a Task's Series looks like to the client: the rule as the
// store wrote it back, and the dates it produces next with what anybody has
// done to each.
//
// Repeats is false for a Task that carries no Series, which is most of them
// and is not an error. The rule is then empty and there are no dates, rather
// than the route answering 404 for a Task that is perfectly present.
type seriesBody struct {
	Repeats     bool         `json:"repeats"`
	Rule        string       `json:"rule,omitempty"`
	Occurrences []occurrence `json:"occurrences"`
}

// occurrence is one date and its state. The state is the store's own word, and
// a date nobody has touched carries none.
type occurrence struct {
	Date  string `json:"date"`
	State string `json:"state,omitempty"`
}

// series is GET /api/tasks/{id}/series: the rule and the dates it produces
// next. It is a read that computes rather than stores, so asking for a page of
// dates costs nothing and changes nothing.
func series(s *store.Store, w http.ResponseWriter, r *http.Request) {
	count, err := page(r, "dates", occurrencePage, occurrenceLimit)
	if err != nil {
		fail(w, err)
		return
	}
	id := r.PathValue("id")
	rule, repeats, err := s.Rule(id)
	if err != nil {
		fail(w, err)
		return
	}
	body := seriesBody{Repeats: repeats, Occurrences: []occurrence{}}
	if !repeats {
		send(w, http.StatusOK, body)
		return
	}
	body.Rule = rule.String()

	// Local rather than UTC: the rule arithmetic reads a date in the local
	// zone, so a UTC instant whose calendar date is not the local one shifts
	// the window a day and answers with the wrong date first.
	from := time.Now()
	dates, err := s.Occurrences(id, from, from.AddDate(occurrenceWindow, 0, 0))
	if err != nil {
		fail(w, err)
		return
	}
	for _, o := range dates {
		if len(body.Occurrences) == count {
			break
		}
		body.Occurrences = append(body.Occurrences, occurrence{
			Date:  o.Date.Format(time.DateOnly),
			State: string(o.State),
		})
	}
	send(w, http.StatusOK, body)
}

// repeatSeries is PUT /api/tasks/{id}/series: the rule the Task repeats on,
// set or replaced. A Series is one value edited as one thing, which is why
// this is a PUT of the whole rule and not a PATCH of part of one.
func repeatSeries(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	in, err := decode[ruleBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	rule := strings.TrimSpace(in.Rule)
	// Stopping is DELETE, so an empty rule here is a caller that meant one and
	// sent none rather than one asking for the Task to stop repeating. Saying
	// so is the same mapping an empty title gets: the store refuses it either
	// way, one status earlier and in the sentence the parser would print.
	if rule == "" {
		fail(w, usage{errors.New("say what the rule is, or DELETE the series to stop repeating")})
		return
	}
	id := r.PathValue("id")
	if err := write.Repeat(s, actor, id, rule); err != nil {
		// A rule the parser cannot read is the caller asking wrongly, and the
		// sentence is the parser's rather than one written here.
		fail(w, usage{err})
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}

// unrepeatSeries is DELETE /api/tasks/{id}/series: the Task stops repeating.
// The Series and every mark on its dates stay in the record, so this deletes
// nothing despite the method: it is the rule coming off the Task.
func unrepeatSeries(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := write.Unrepeat(s, actor, id); err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}

// markOccurrence is POST /api/tasks/{id}/series/{mark}: one date ticked,
// skipped or lifted out, which is write.Mark's list rather than a second copy
// of it. The date is the whole of the body, because a mark says nothing else.
func markOccurrence(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	mark := r.PathValue("mark")
	act, ok := write.Mark(mark)
	if !ok {
		// The store never sees this one, so there is no sentence of its own to
		// use: a URL naming something a date is not is the caller asking
		// wrongly, and the answer says which three it could have named.
		fail(w, usage{fmt.Errorf("a date is not %sed: want %s", mark, strings.Join(write.MarkNames(), ", "))})
		return
	}
	in, err := decode[markBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	// UTC, because a date has no time of day and the store keys a mark by the
	// date's text. Reading it in a zone would move the key by a day.
	on, err := time.ParseInLocation(time.DateOnly, in.On, time.UTC)
	if err != nil {
		fail(w, usage{fmt.Errorf("cannot read %q as a date: want 2006-01-02", in.On)})
		return
	}

	id := r.PathValue("id")
	written, err := act(s, actor, id, on)
	if err != nil {
		fail(w, err)
		return
	}
	// Detaching answers the id of the Task the date became; the other two have
	// nothing new to name and answer the Task the mark was against.
	if written != "" {
		send(w, http.StatusCreated, map[string]string{"id": written})
		return
	}
	send(w, http.StatusOK, map[string]string{"id": id})
}

// ruleBody is the recurrence, as the text the parser reads and the store
// stores. There is no second shape of columns that could disagree with it.
type ruleBody struct {
	Rule string `json:"rule"`
}

// markBody is the date a mark is against, and nothing else.
type markBody struct {
	On string `json:"on"`
}

// detachEdited is POST /api/tasks/{id}/series/edit: one date lifted out as the
// Task it was corrected into. It is the fourth thing a surface does to a date
// and the TUI's `e edit` on the Scheduling screen, which is one store call and
// so one entry rather than a detach followed by an edit of what it became.
//
// It is a route of its own rather than a fourth name under {mark} because it
// carries a whole Task where the three carry only the date, and because only
// this one lets a surface show the corrected copy before anything is written:
// a date the person backed out of is still an Occurrence.
//
// The literal segment wins over {mark}, so `POST .../series/edit` reaches here
// and `POST .../series/tick` does not.
func detachEdited(s *store.Store, actor string, w http.ResponseWriter, r *http.Request) {
	in, err := decode[detachBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	on, err := time.ParseInLocation(time.DateOnly, in.On, time.UTC)
	if err != nil {
		fail(w, usage{fmt.Errorf("cannot read %q as a date: want 2006-01-02", in.On)})
		return
	}
	given, err := in.taskBody.given().Attributes()
	if err != nil {
		fail(w, usage{err})
		return
	}
	// The lifted copy is an ordinary Task from the moment it exists, so it
	// needs a title for the same reason POST /api/tasks does, and saying so
	// here is the sentence the store would refuse it with one status later.
	if given == nil || given.Title == nil || *given.Title == "" {
		fail(w, usage{errors.New("a task needs a title")})
		return
	}
	// Nothing is moved by lifting a date out: the copy's parent is the store's
	// to decide, the same way an edit refuses one.
	if in.Parent != "" {
		fail(w, usage{errors.New("a detached date's parent is the store's to set: leave out parent")})
		return
	}
	id := r.PathValue("id")
	written, err := write.DetachEdited(s, actor, id, on, *given, in.membership())
	if err != nil {
		fail(w, err)
		return
	}
	// 201, because the id names a Task that did not exist before the request,
	// which is what POST .../series/detach answers with too.
	send(w, http.StatusCreated, map[string]string{"id": written})
}

// detachBody is the date being lifted out and the Task it is being lifted out
// as. The attributes are the ten every other write takes, because the copy is
// an ordinary Task and is corrected on the same form.
type detachBody struct {
	taskBody
	On string `json:"on"`
}
