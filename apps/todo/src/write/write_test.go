package write

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func openTemp(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func kinds(t *testing.T, s *store.Store) []string {
	t.Helper()
	log, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	out := make([]string, 0, len(log))
	for _, e := range log {
		out = append(out, e.Kind)
	}
	return out
}

func count(kinds []string, kind string) int {
	n := 0
	for _, k := range kinds {
		if k == kind {
			n++
		}
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func TestAddFilesANewTaskUnderOneLease(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("alice", "Home", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	tag, err := s.AddTag("alice", "errand", "green")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	id, err := Add(s, "alice", "", store.Attributes{Title: ptr("Buy paint")},
		Membership{IntoLists: []string{list}, AddTags: []string{tag}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != id {
		t.Fatalf("tasks = %v, want the one just added", tasks)
	}
	if len(tasks[0].Lists) != 1 || len(tasks[0].Tags) != 1 {
		t.Errorf("lists = %v, tags = %v, want one of each", tasks[0].Lists, tasks[0].Tags)
	}
	// The Task and both memberships are one visit to the tree, not three.
	if n := count(kinds(t, s), store.KindLeaseReleased); n != 1 {
		t.Errorf("%d Leases released, want 1 for one add", n)
	}
}

// A top-level Task with nothing to file takes no Lease at all: there is no
// tree to visit, and a Lease taken for nothing is bookkeeping in the Change
// History that says a person did something they did not do.
func TestAddTakesNoLeaseWhenThereIsNothingToFile(t *testing.T) {
	s := openTemp(t)
	if _, err := Add(s, "alice", "", store.Attributes{Title: ptr("Buy milk")}, Membership{}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if n := count(kinds(t, s), store.KindLeaseReleased); n != 0 {
		t.Errorf("%d Leases released against an add with no filing", n)
	}
}

// The id comes back even when the filing is refused. The Task exists by then,
// and a surface that dropped the id would leave the person unable to name the
// Task they just made in order to correct it.
func TestAddSaysWhichTaskItWroteEvenWhenTheFilingFails(t *testing.T) {
	s := openTemp(t)
	id, err := Add(s, "alice", "", store.Attributes{Title: ptr("Buy paint")},
		Membership{IntoLists: []string{"no-such-list"}})
	if err == nil {
		t.Fatal("filing under a List that does not exist was taken")
	}
	if id == "" {
		t.Error("the id of the written Task was not said")
	}
	tasks, _ := s.Tasks(store.Query{})
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want the Task to be written despite the filing", len(tasks))
	}
	if len(tasks[0].Lists) != 0 {
		t.Errorf("lists = %v, want it filed under nothing", tasks[0].Lists)
	}
}

func TestAddNestsASubtaskUnderItsParentsLease(t *testing.T) {
	s := openTemp(t)
	parent, err := Add(s, "alice", "", store.Attributes{Title: ptr("Paint the fence")}, Membership{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	child, err := Add(s, "alice", parent, store.Attributes{Title: ptr("Buy paint")}, Membership{})
	if err != nil {
		t.Fatalf("Add subtask: %v", err)
	}

	tasks, err := s.Tasks(store.Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 || tasks[1].ID != child || tasks[1].Parent != parent {
		t.Fatalf("tasks = %v, want the Subtask under its parent", tasks)
	}
	if tasks[1].Depth != 2 {
		t.Errorf("depth = %d, want 2", tasks[1].Depth)
	}
}

func TestEditChangesAttributesAndMembershipTogether(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("alice", "Home", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	id, err := Add(s, "alice", "", store.Attributes{Title: ptr("Buy paint")}, Membership{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	before := count(kinds(t, s), store.KindLeaseReleased)
	if err := Edit(s, "alice", id, &store.Attributes{Why: ptr("the fence is peeling")},
		Membership{IntoLists: []string{list}}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if n := count(kinds(t, s), store.KindLeaseReleased) - before; n != 1 {
		t.Errorf("%d Leases released, want 1 for one edit", n)
	}

	tasks, _ := s.Tasks(store.Query{})
	if tasks[0].Why != "the fence is peeling" || len(tasks[0].Lists) != 1 {
		t.Errorf("why = %q, lists = %v, want both changed under the one Lease", tasks[0].Why, tasks[0].Lists)
	}
}

// An attribute nobody gave is left alone, and one given empty is cleared. The
// two are told apart by presence rather than by the value, so a surface can
// clear a title without meaning to leave it alone.
func TestGivenTellsAbsentApartFromEmpty(t *testing.T) {
	a, err := Given{Why: ptr("")}.Attributes()
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}
	if a == nil {
		t.Fatal("a field given empty reads as nothing given")
	}
	if a.Why == nil || *a.Why != "" {
		t.Errorf("Why = %v, want a pointer to the empty string", a.Why)
	}
	if a.Title != nil {
		t.Errorf("Title = %v, want nothing at all", a.Title)
	}

	if a, err := (Given{}).Attributes(); err != nil || a != nil {
		t.Errorf("nothing given reads as %v, err = %v", a, err)
	}
}

func TestGivenRefusesAValueItCannotRead(t *testing.T) {
	for name, g := range map[string]Given{
		"priority": {Priority: ptr("urgent")},
		"deadline": {Deadline: ptr("next tuesday")},
		"estimate": {Estimate: ptr("a couple of hours")},
		"snooze":   {Snooze: ptr("a bit")},
	} {
		if _, err := g.Attributes(); err == nil {
			t.Errorf("%s took a value it cannot read", name)
		}
	}
}

func TestGivenReadsADateTheWayBothSurfacesType(t *testing.T) {
	for _, v := range []string{"2026-03-04", "2026-03-04 09:30", "2026-03-04T09:30:00Z"} {
		a, err := Given{Deadline: &v}.Attributes()
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if a.Deadline == nil || a.Deadline.Year() != 2026 || a.Deadline.Month() != time.March {
			t.Errorf("%q read as %v", v, a.Deadline)
		}
	}
}

// What the Broker cannot be read out of is dropped rather than refused: it
// said what the work is, and an estimate it phrased in prose is not a reason
// to refuse the Task.
func TestFromCaptureDropsWhatItCannotRead(t *testing.T) {
	a, err := FromCapture(ai.Capture{
		Title:    "Buy paint",
		Estimate: "a couple of hours",
		Deadline: "whenever",
		Priority: "urgent",
	})
	if err != nil {
		t.Fatalf("FromCapture: %v", err)
	}
	if a.Title == nil || *a.Title != "Buy paint" {
		t.Errorf("title = %v, want the one it read", a.Title)
	}
	if a.Estimate != nil || a.Deadline != nil || a.Priority != nil {
		t.Errorf("a value it could not read was kept: %+v", a)
	}
}

func TestFromCaptureRefusesAnAnswerWithNoTaskInIt(t *testing.T) {
	if _, err := FromCapture(ai.Capture{Title: "   "}); err == nil {
		t.Error("an answer with no title was read as a Task")
	} else if !strings.Contains(err.Error(), "no title") {
		t.Errorf("err = %v, want it to say what was missing", err)
	}
}

func TestNamedInMatchesByNameAndAppendsOnce(t *testing.T) {
	have := ListNames([]store.List{{ID: "l1", Name: "Home"}, {ID: "l2", Name: "Work"}})
	got := NamedIn([]string{"home", " HOME ", "Errands", "Work"}, have)
	if len(got) != 2 || got[0] != "l1" || got[1] != "l2" {
		t.Errorf("got = %v, want [l1 l2]: matched case-insensitively, once each, unknown dropped", got)
	}
}

func TestFromCaptureReadsAValueTheBrokerPaddedWithSpace(t *testing.T) {
	// The Broker writes prose, not a flag value, so the space around a
	// duration is the Broker's rather than something the person typed.
	a, err := FromCapture(ai.Capture{
		Title:    "Paint the fence",
		Estimate: " 90m ",
		Deadline: " 2026-03-04 ",
	})
	if err != nil {
		t.Fatalf("FromCapture: %v", err)
	}
	if a.Estimate == nil || *a.Estimate != 90*time.Minute {
		t.Errorf("estimate = %v, want 90m", a.Estimate)
	}
	if a.Deadline == nil || a.Deadline.Year() != 2026 || a.Deadline.Month() != time.March {
		t.Errorf("deadline = %v, want the fourth of March", a.Deadline)
	}
}

func TestGivenReportsTheFirstBadValueInFlagOrder(t *testing.T) {
	// A flag set visits what was typed in the order the names sort in, so
	// two bad values report the earlier name, and both surfaces say so.
	for _, c := range []struct {
		given Given
		want  string
	}{
		{Given{Deadline: ptr("next tuesday"), Priority: ptr("urgent")}, "next tuesday"},
		{Given{Impact: ptr("enormous"), Priority: ptr("urgent")}, "enormous"},
		{Given{Estimate: ptr("a while"), Snooze: ptr("a bit")}, "a while"},
	} {
		_, err := c.given.Attributes()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("err = %v, want the sentence naming %q", err, c.want)
		}
	}
}

func TestGatherShowsTheBrokerTodayAndWhatItMayFileUnder(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("alice", "House", "blue")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	tag, err := s.AddTag("alice", "outdoors", "green")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	d, err := Gather(s, "paint the fence before march")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if d.Shown.Text != "paint the fence before march" {
		t.Errorf("the dump says %q, want the words that were typed", d.Shown.Text)
	}
	// The weekday is in it because "Friday" in a dump is a date only where
	// the Broker is told which day today is.
	if d.Shown.Today != time.Now().Format(dumpDate) {
		t.Errorf("today reads %q, want %q", d.Shown.Today, time.Now().Format(dumpDate))
	}
	if len(d.Shown.Lists) != 1 || d.Shown.Lists[0] != "House" {
		t.Errorf("it may file under %v, want the lists by name", d.Shown.Lists)
	}
	if len(d.Shown.Tags) != 1 || d.Shown.Tags[0] != "outdoors" {
		t.Errorf("it may carry %v, want the tags by name", d.Shown.Tags)
	}

	// A name it chose comes back as the id the write takes, ignoring case,
	// and a name nobody offered is dropped.
	m := d.Filed(ai.Capture{Lists: []string{"house", "Shed"}, Tags: []string{"OUTDOORS"}})
	if len(m.IntoLists) != 1 || m.IntoLists[0] != list {
		t.Errorf("it would be filed in %v, want %q alone", m.IntoLists, list)
	}
	if len(m.AddTags) != 1 || m.AddTags[0] != tag {
		t.Errorf("it would carry %v, want %q alone", m.AddTags, tag)
	}
}

func TestBriefsSayWhatATaskSaysAndNoIdAtAll(t *testing.T) {
	when := time.Date(2026, 3, 4, 9, 30, 0, 0, time.UTC)
	got := Briefs([]store.Task{{
		ID:       "task_whatever",
		Title:    "Paint the fence",
		Why:      "it is peeling",
		Deadline: when,
		Estimate: 90 * time.Minute,
		Priority: store.LevelHigh,
	}, {
		ID:    "task_other",
		Title: "Buy paint",
	}})

	if len(got) != 2 {
		t.Fatalf("Briefs returned %d, want one per task", len(got))
	}
	if got[0].Deadline != when.Local().Format(time.DateOnly) {
		t.Errorf("the deadline reads %q, want the day it falls on", got[0].Deadline)
	}
	if got[0].Estimate != "1h30m0s" {
		t.Errorf("the estimate reads %q, want a duration the broker can read", got[0].Estimate)
	}
	// A Task with neither is shown neither rather than shown a zero one.
	if got[1].Deadline != "" || got[1].Estimate != "" {
		t.Errorf("a task with no deadline or estimate reads %+v, want both left out", got[1])
	}
}
