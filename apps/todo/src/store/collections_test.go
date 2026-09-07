package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAListOutlivesTheTasksInIt(t *testing.T) {
	s := openTemp(t)

	// It exists before any Task is in it.
	id, err := s.AddList("alice", "Home", "#88cc88")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	lists, err := s.Lists()
	if err != nil {
		t.Fatalf("Lists: %v", err)
	}
	if len(lists) != 1 || lists[0].Name != "Home" || lists[0].Colour != "#88cc88" || lists[0].Count != 0 {
		t.Fatalf("an empty List reads back as %+v", lists)
	}

	task := leased(t, s, "alice", Attributes{Title: Set("Fix the sink")})
	if err := s.AddToList("alice", task, id); err != nil {
		t.Fatalf("AddToList: %v", err)
	}
	if lists, _ := s.Lists(); lists[0].Count != 1 {
		t.Errorf("the List holds %d tasks, want 1", lists[0].Count)
	}

	// And it survives after the last one leaves.
	if err := s.RemoveFromList("alice", task, id); err != nil {
		t.Fatalf("RemoveFromList: %v", err)
	}
	lists, err = s.Lists()
	if err != nil {
		t.Fatalf("Lists: %v", err)
	}
	if len(lists) != 1 || lists[0].Count != 0 {
		t.Errorf("the List did not survive its last Task: %+v", lists)
	}
}

func TestATaskBelongsToMoreThanOneList(t *testing.T) {
	s := openTemp(t)
	home, err := s.AddList("alice", "Home", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	errands, err := s.AddList("alice", "Errands", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	task := leased(t, s, "alice", Attributes{Title: Set("Fix the sink")})

	for _, list := range []string{home, errands} {
		if err := s.AddToList("alice", task, list); err != nil {
			t.Fatalf("AddToList: %v", err)
		}
	}
	if got := only(t, s, Query{}); len(got.Lists) != 2 {
		t.Errorf("the Task belongs to %v, want both lists", got.Lists)
	}
	if got, _ := s.Tasks(Query{List: home}); len(got) != 1 {
		t.Errorf("narrowing to one List returned %d tasks, want 1", len(got))
	}
	if got, _ := s.Tasks(Query{List: "no such list"}); len(got) != 0 {
		t.Errorf("narrowing to a List nothing is in returned %d tasks", len(got))
	}
}

// Renaming is one write, and every Task carrying it shows the new name without
// being written to: the Task holds the id, never the text.
func TestRenamingAListChangesOneThing(t *testing.T) {
	s := openTemp(t)
	list, err := s.AddList("alice", "Home", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	for i := range 3 {
		task := leased(t, s, "alice", Attributes{Title: Set("task " + string(rune('a'+i)))})
		if err := s.AddToList("alice", task, list); err != nil {
			t.Fatalf("AddToList: %v", err)
		}
	}

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if err := s.DescribeList("alice", list, Set("House"), nil); err != nil {
		t.Fatalf("DescribeList: %v", err)
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after-before != 1 {
		t.Errorf("renaming a List of three Tasks appended %d entries, want 1", after-before)
	}

	lists, _ := s.Lists()
	if lists[0].Name != "House" {
		t.Errorf("the List reads back as %q", lists[0].Name)
	}
	tasks, _ := s.Tasks(Query{})
	for _, task := range tasks {
		if len(task.Lists) != 1 || task.Lists[0] != list {
			t.Errorf("%s carries %v, want the List's id unchanged", task.ID, task.Lists)
		}
	}
}

func TestATagIsCountedAcrossTheTracker(t *testing.T) {
	s := openTemp(t)
	deep, err := s.AddTag("alice", "deep-work", "#4444ff")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	errand, err := s.AddTag("alice", "errand", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	for i := range 3 {
		task := leased(t, s, "alice", Attributes{Title: Set("task " + string(rune('a'+i)))})
		if err := s.AttachTag("alice", task, deep); err != nil {
			t.Fatalf("AttachTag: %v", err)
		}
		if i == 0 {
			if err := s.AttachTag("alice", task, errand); err != nil {
				t.Fatalf("AttachTag: %v", err)
			}
		}
	}

	// Most carried first: the order a sidebar ranking by frequency wants.
	tags, err := s.Tags()
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 2 || tags[0].ID != deep || tags[0].Count != 3 || tags[1].Count != 1 {
		t.Fatalf("the tags rank as %+v", tags)
	}

	// Recoloured once, and every Task carrying it is untouched.
	if err := s.DescribeTag("alice", deep, nil, Set("#000000")); err != nil {
		t.Fatalf("DescribeTag: %v", err)
	}
	if tags, _ = s.Tags(); tags[0].Colour != "#000000" || tags[0].Name != "deep-work" {
		t.Errorf("recolouring changed %+v", tags[0])
	}
}

func TestADeletedTaskStopsCountingTowardsATag(t *testing.T) {
	s := openTemp(t)
	tag, err := s.AddTag("alice", "errand", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	task := leased(t, s, "alice", Attributes{Title: Set("Fix the sink")})
	if err := s.AttachTag("alice", task, tag); err != nil {
		t.Fatalf("AttachTag: %v", err)
	}
	if err := s.DeleteTask("alice", task); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if tags, _ := s.Tags(); tags[0].Count != 0 {
		t.Errorf("a deleted Task still counts towards its Tag: %+v", tags[0])
	}
}

// Membership is a write to the Task, so it needs the Task's Lease. Creating
// and renaming the List itself does not: a List is nobody's tree.
func TestCarryingATagHonoursTheTasksLease(t *testing.T) {
	s := openTemp(t)
	tag, err := s.AddTag("alice", "errand", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	list, err := s.AddList("alice", "Home", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	task := leased(t, s, "alice", Attributes{Title: Set("Fix the sink")})

	if err := s.AttachTag("agent-7", task, tag); !errors.Is(err, ErrRefused) {
		t.Errorf("attaching a Tag without the Lease failed with %v, want a refusal", err)
	}
	if err := s.AddToList("agent-7", task, list); !errors.Is(err, ErrRefused) {
		t.Errorf("listing a Task without the Lease failed with %v, want a refusal", err)
	}
	if _, err := s.AddTag("agent-7", "unleased", ""); err != nil {
		t.Errorf("creating a Tag needed a Lease: %v", err)
	}
	if err := s.DescribeList("agent-7", list, Set("House"), nil); err != nil {
		t.Errorf("renaming a List needed a Lease: %v", err)
	}
}

func TestTasksSortByDeadlineOrTitle(t *testing.T) {
	s := openTemp(t)
	day := func(d int) *time.Time {
		when := time.Date(2027, 3, d, 9, 0, 0, 0, time.UTC)
		return &when
	}
	// Created c, a, b; deadlines b, c, a; no deadline on the last.
	c := leased(t, s, "alice", Attributes{Title: Set("Ceilings"), Deadline: day(3)})
	a := leased(t, s, "alice", Attributes{Title: Set("Anvils"), Deadline: day(9)})
	b := leased(t, s, "alice", Attributes{Title: Set("Buckets"), Deadline: day(1)})
	d := leased(t, s, "alice", Attributes{Title: Set("Doors")})

	for _, tc := range []struct {
		sort Sort
		want []string
	}{
		{SortCreated, []string{c, a, b, d}},
		{SortDeadline, []string{b, c, a, d}},
		{SortTitle, []string{a, b, c, d}},
	} {
		tasks, err := s.Tasks(Query{Sort: tc.sort})
		if err != nil {
			t.Fatalf("Tasks by %s: %v", tc.sort, err)
		}
		var got []string
		for _, task := range tasks {
			got = append(got, task.ID)
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("sorted by %s the order is %v, want %v", tc.sort, got, tc.want)
		}
	}

	if _, err := s.Tasks(Query{Sort: "colour"}); err == nil {
		t.Error("an unknown sort was accepted")
	}
}

// A sort orders siblings; it does not flatten the tree.
func TestASortKeepsTheTreeShape(t *testing.T) {
	s := openTemp(t)
	root := leased(t, s, "alice", Attributes{Title: Set("Zebra")})
	second, err := s.AddSubtask("alice", root, Attributes{Title: Set("Beta")})
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	first, err := s.AddSubtask("alice", root, Attributes{Title: Set("Alpha")})
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	other := leased(t, s, "alice", Attributes{Title: Set("Aardvark")})

	tasks, err := s.Tasks(Query{Sort: SortTitle})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	var got []string
	for _, task := range tasks {
		got = append(got, task.ID)
	}
	want := []string{other, root, first, second}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sorted by title the tree came back %v, want the roots sorted and the children under theirs %v", got, want)
	}
}

func TestACollectionNeedsAName(t *testing.T) {
	s := openTemp(t)
	if _, err := s.AddList("alice", "  ", ""); err == nil {
		t.Error("a List with no name was created")
	}
	if _, err := s.AddTag("alice", "", ""); err == nil {
		t.Error("a Tag with no name was created")
	}
}

func TestTasksSortByEstimate(t *testing.T) {
	s := openTemp(t)
	hours := func(n int) *time.Duration {
		d := time.Duration(n) * time.Hour
		return &d
	}
	long := leased(t, s, "alice", Attributes{Title: Set("Long"), Estimate: hours(10)})
	short := leased(t, s, "alice", Attributes{Title: Set("Short"), Estimate: hours(9)})
	none := leased(t, s, "alice", Attributes{Title: Set("Unknown")})

	tasks, err := s.Tasks(Query{Sort: SortEstimate})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	var got []string
	for _, task := range tasks {
		got = append(got, task.ID)
	}
	want := []string{short, long, none}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sorted by estimate the order is %v, want the shortest first and no estimate last %v", got, want)
	}
}

// A List narrows the walk, not what the walk returned. A Subtask in the List
// under a parent that is not in it comes back with a Depth to indent by and a
// Parent that is not in the answer, which is the same orphan a hidden parent
// would leave: a subtree is reachable only through a root that is in view.
func TestNarrowingToAListLeavesNoOrphan(t *testing.T) {
	s := openTemp(t)
	home, err := s.AddList("alice", "Home", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	root := leased(t, s, "alice", Attributes{Title: Set("Fix the sink")})
	child, err := s.AddSubtask("alice", root, Attributes{Title: Set("Buy a washer")})
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	if err := s.AddToList("alice", child, home); err != nil {
		t.Fatalf("AddToList: %v", err)
	}

	if got, err := s.Tasks(Query{List: home}); err != nil {
		t.Fatalf("Tasks: %v", err)
	} else if len(got) != 0 {
		t.Errorf("narrowing to a List returned %d tasks under no visible root: %+v", len(got), got)
	}

	// The root in the List brings the whole subtree with it, in view or not.
	if err := s.AddToList("alice", root, home); err != nil {
		t.Fatalf("AddToList: %v", err)
	}
	got, err := s.Tasks(Query{List: home})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(got) != 2 || got[0].ID != root || got[1].ID != child {
		t.Errorf("the List came back %+v, want the root then its Subtask", got)
	}
}
