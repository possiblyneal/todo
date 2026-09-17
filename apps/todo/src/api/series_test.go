package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func repeating(t *testing.T, s *store.Store, rule string) string {
	t.Helper()
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Water the plants"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}
	id := said(t, w)["id"]
	if w := do(t, s, http.MethodPut, "/api/tasks/"+id+"/series", `{"rule": "`+rule+`"}`); w.Code != http.StatusOK {
		t.Fatalf("PUT series answered %d: %s", w.Code, w.Body.String())
	}
	return id
}

func repeat(t *testing.T, s *store.Store, target string) seriesBody {
	t.Helper()
	w := do(t, s, http.MethodGet, target, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d: %s", target, w.Code, w.Body.String())
	}
	var out seriesBody
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return out
}

// The Series screen reads the rule and the dates it produces next. Asking is a
// read: it computes the dates rather than storing them, so asking twice costs
// the log nothing.
func TestTheSeriesIsTheRuleAndTheDatesItProduces(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every week on mon,thu")

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	out := repeat(t, s, "/api/tasks/"+id+"/series")
	if !out.Repeats {
		t.Fatalf("the task does not repeat, want the rule just set")
	}
	// As the store wrote it back, anchor and all: a Series is stored as the
	// text schedule.String produces, and that text is what the screen draws.
	if !strings.HasPrefix(out.Rule, "every week on mon,thu") {
		t.Errorf("the rule came back as %q, want it as the store wrote it", out.Rule)
	}
	if len(out.Occurrences) != occurrencePage {
		t.Errorf("it answered %d dates, want a page of %d", len(out.Occurrences), occurrencePage)
	}
	for _, o := range out.Occurrences {
		on, err := time.ParseInLocation(time.DateOnly, o.Date, time.Local)
		if err != nil {
			t.Fatalf("the date %q is not 2006-01-02: %v", o.Date, err)
		}
		if day := on.Weekday(); day != time.Monday && day != time.Thursday {
			t.Errorf("%s is a %s, want a day the rule names", o.Date, day)
		}
		if o.State != "" {
			t.Errorf("%s came back %q, want a date nobody has touched", o.Date, o.State)
		}
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after != before {
		t.Errorf("reading the series appended %d entries, want none", after-before)
	}
}

// A Task that does not repeat is not an error: most do not. The answer says so
// and carries no rule and no dates.
func TestATaskThatDoesNotRepeatAnswersThatItDoesNot(t *testing.T) {
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint the fence"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST answered %d: %s", w.Code, w.Body.String())
	}

	out := repeat(t, s, "/api/tasks/"+said(t, w)["id"]+"/series")
	if out.Repeats || out.Rule != "" {
		t.Errorf("it answered %+v, want a task carrying no series", out)
	}
	// Empty, never null: a client that must tell the two apart before it can
	// count is being asked a question the store never asks.
	if out.Occurrences == nil {
		t.Errorf("the dates came back null, want []")
	}
}

// How many dates is the caller's, capped. Asking for more than the cap gets the
// cap rather than a refusal, and the word is `limit` because it is the same
// question GET /api/history is asked: both routes read it through api.page.
func TestTheSeriesAnswersAsManyDatesAsAsked(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")

	if out := repeat(t, s, "/api/tasks/"+id+"/series?limit=3"); len(out.Occurrences) != 3 {
		t.Errorf("it answered %d dates, want the 3 asked for", len(out.Occurrences))
	}
	if out := repeat(t, s, fmt.Sprintf("/api/tasks/%s/series?limit=%d", id, occurrenceLimit*2)); len(out.Occurrences) != occurrenceLimit {
		t.Errorf("it answered %d dates, want the cap of %d", len(out.Occurrences), occurrenceLimit)
	}
	w := do(t, s, http.MethodGet, "/api/tasks/"+id+"/series?limit=none", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("a count that is not a number answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

// A rule the parser cannot read is the caller asking wrongly, and the sentence
// is the parser's own rather than one written in the route.
func TestARuleNobodyCanReadIsAUsageError(t *testing.T) {
	s := openTemp(t)
	w := do(t, s, http.MethodPost, "/api/tasks", `{"title": "Paint the fence"}`)
	id := said(t, w)["id"]

	w = do(t, s, http.MethodPut, "/api/tasks/"+id+"/series", `{"rule": "every blue moon"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
	// Stopping is DELETE, so an empty rule is a caller that meant one and sent
	// none rather than one asking for the task to stop repeating.
	w = do(t, s, http.MethodPut, "/api/tasks/"+id+"/series", `{"rule": "  "}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("an empty rule answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if out := repeat(t, s, "/api/tasks/"+id+"/series"); out.Repeats {
		t.Errorf("the task repeats after two refusals, want neither to have written")
	}
}

// DELETE takes the rule off. It deletes nothing despite the method: the Series
// and every mark on its dates stay in the record, because the Task stopped
// repeating rather than the repeating never having happened.
func TestDeletingTheSeriesStopsTheTaskRepeating(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")
	on := repeat(t, s, "/api/tasks/"+id+"/series").Occurrences[0].Date
	if w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/tick", `{"on": "`+on+`"}`); w.Code != http.StatusOK {
		t.Fatalf("POST tick answered %d: %s", w.Code, w.Body.String())
	}

	if w := do(t, s, http.MethodDelete, "/api/tasks/"+id+"/series", ""); w.Code != http.StatusOK {
		t.Fatalf("DELETE answered %d: %s", w.Code, w.Body.String())
	}
	if out := repeat(t, s, "/api/tasks/"+id+"/series"); out.Repeats {
		t.Errorf("it still repeats after the rule came off")
	}
	// The mark is still in the log: the Change History is append-only, and a
	// rule coming off is not an erasure.
	var ticked bool
	for _, e := range log(t, s, "/api/history").Entries {
		ticked = ticked || e.Kind == store.KindOccurrenceTicked
	}
	if !ticked {
		t.Errorf("the tick is not in the history, want the record kept")
	}
}

// Ticking and skipping leave the Series alone and name the Task they were
// against; detaching lifts the date out into an ordinary Task and names that
// one, which is the only thing that afterwards names it at all.
func TestTheThreeMarksAgainstOneDate(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")
	dates := repeat(t, s, "/api/tasks/"+id+"/series?limit=3").Occurrences

	for i, mark := range []string{"tick", "skip"} {
		w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/"+mark, `{"on": "`+dates[i].Date+`"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("POST %s answered %d: %s", mark, w.Code, w.Body.String())
		}
		if got := said(t, w)["id"]; got != id {
			t.Errorf("%s answered %q, want the task it was against", mark, got)
		}
	}
	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/detach", `{"on": "`+dates[2].Date+`"}`)
	// 201, because the id names something that did not exist before the
	// request, which is what POST /api/tasks answers for the same reason.
	if w.Code != http.StatusCreated {
		t.Fatalf("POST detach answered %d, want 201 (%s)", w.Code, w.Body.String())
	}
	if lifted := said(t, w)["id"]; lifted == id || lifted == "" {
		t.Errorf("detach answered %q, want the id of the task the date became", lifted)
	}

	out := repeat(t, s, "/api/tasks/"+id+"/series?limit=3")
	if out.Occurrences[0].State != "ticked" || out.Occurrences[1].State != "skipped" {
		t.Errorf("the dates came back %+v, want the two marks on them", out.Occurrences[:2])
	}
	// A detached date stops being an Occurrence: it became a Task, and the rule
	// no longer produces it.
	for _, o := range out.Occurrences {
		if o.Date == dates[2].Date {
			t.Errorf("%s is still a date of the series after being detached", o.Date)
		}
	}
}

// A mark the store does not have is the caller asking wrongly, and the answer
// says which three it could have named. The store never sees it.
func TestAMarkNobodyRecognisesIsAUsageError(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")
	on := repeat(t, s, "/api/tasks/"+id+"/series").Occurrences[0].Date

	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/postpone", `{"on": "`+on+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown mark answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
	for _, mark := range []string{"tick", "skip", "detach"} {
		if !strings.Contains(w.Body.String(), mark) {
			t.Errorf("the refusal does not name %q: %s", mark, w.Body.String())
		}
	}
	// A date the rule does not produce is the store's refusal rather than this
	// side's, and it arrives as one.
	w = do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/tick", `{"on": "not a date"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("a date nobody can read answered %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

// The fourth thing done to a date: lifted out as the Task it was corrected
// into, which is one write and so one new Task, and the date stops being an
// Occurrence because the rule no longer produces it.
func TestADateIsLiftedOutAsTheTaskItWasCorrectedInto(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")
	on := repeat(t, s, "/api/tasks/"+id+"/series").Occurrences[0].Date

	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/edit",
		`{"on": "`+on+`", "title": "Water the plants twice", "why": "it is hot"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("it answered %d, want 201: %s", w.Code, w.Body.String())
	}
	lifted := said(t, w)["id"]
	if lifted == "" || lifted == id {
		t.Fatalf("it answered %q, want the id of the task the date became", lifted)
	}

	// The corrections went on in the same write, so the lifted Task says what
	// it was corrected into and not what the recurring one says.
	out := repeat(t, s, "/api/tasks/"+id+"/series")
	for _, o := range out.Occurrences {
		if o.Date == on {
			t.Errorf("%s is still an occurrence, want it lifted out", on)
		}
	}
}

// A lifted date is an ordinary Task from the moment it exists, so it is
// refused without a title the way POST /api/tasks is, and at the same status.
func TestLiftingADateOutNeedsATitle(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")
	on := repeat(t, s, "/api/tasks/"+id+"/series").Occurrences[0].Date

	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/edit", `{"on": "`+on+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("it answered %d, want 400: %s", w.Code, w.Body.String())
	}
	// And the date is untouched: nothing is written by being refused.
	if len(repeat(t, s, "/api/tasks/"+id+"/series").Occurrences) == 0 {
		t.Errorf("the refusal took the dates away, want them left alone")
	}
}

// `edit` is a route and not a mark, so the three marks still reach the marks
// and an unknown one is still turned away naming them.
func TestEditIsNotOneOfTheThreeMarks(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "daily")
	on := repeat(t, s, "/api/tasks/"+id+"/series").Occurrences[0].Date

	if w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/tick", `{"on": "`+on+`"}`); w.Code != http.StatusOK {
		t.Errorf("tick answered %d, want 200: %s", w.Code, w.Body.String())
	}
	w := do(t, s, http.MethodPost, "/api/tasks/"+id+"/series/polish", `{"on": "`+on+`"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "detach") {
		t.Errorf("an unknown mark answered %d (%s), want 400 naming the three", w.Code, w.Body.String())
	}
}
