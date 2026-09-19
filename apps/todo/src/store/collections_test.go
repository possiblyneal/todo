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
	id, err := s.AddList("alice", "Home", "green")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	lists, err := s.Lists()
	if err != nil {
		t.Fatalf("Lists: %v", err)
	}
	if len(lists) != 1 || lists[0].Name != "Home" || lists[0].Color != "green" || lists[0].Count != 0 {
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
	deep, err := s.AddTag("alice", "deep-work", "blue")
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

	// Recolored once, and every Task carrying it is untouched.
	if err := s.DescribeTag("alice", deep, nil, Set("violet")); err != nil {
		t.Fatalf("DescribeTag: %v", err)
	}
	if tags, _ = s.Tags(); tags[0].Color != "violet" || tags[0].Name != "deep-work" {
		t.Errorf("recoloring changed %+v", tags[0])
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

	if _, err := s.Tasks(Query{Sort: "color"}); err == nil {
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

// A List can be deleted, and the Tasks that were in it are not. Each one is
// unfiled by its own entry, so a Task's history says it left the List.
func TestDeletingAListUnfilesTheTasksAndKeepsThem(t *testing.T) {
	s := openTemp(t)
	home, err := s.AddList("alice", "Home", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	roof := leased(t, s, "alice", Attributes{Title: Set("Fix the roof")})
	sink := leased(t, s, "alice", Attributes{Title: Set("Fix the sink")})
	for _, id := range []string{roof, sink} {
		if err := s.WithLease("alice", id, WriteTTL, func() error {
			return s.AddToList("alice", id, home)
		}); err != nil {
			t.Fatalf("AddToList: %v", err)
		}
	}

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if err := s.DeleteList("alice", home); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	// One unfiling per Task, then the List itself.
	if grew := after - before; grew != 3 {
		t.Errorf("deleting the List appended %d entries, want 3", grew)
	}

	lists, err := s.Lists()
	if err != nil {
		t.Fatalf("Lists: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("the List is still there: %+v", lists)
	}
	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("%d Tasks survived the List, want both", len(tasks))
	}
	for _, got := range tasks {
		if len(got.Lists) != 0 {
			t.Errorf("%q is still filed under %v", got.Title, got.Lists)
		}
	}
}

// A Tag goes the same way, and the Tasks that carried it keep everything else.
func TestDeletingATagTakesItOffTheTasksThatCarriedIt(t *testing.T) {
	s := openTemp(t)
	urgent, err := s.AddTag("alice", "urgent", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	slow, err := s.AddTag("alice", "slow", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	roof := leased(t, s, "alice", Attributes{Title: Set("Fix the roof")})
	if err := s.WithLease("alice", roof, WriteTTL, func() error {
		if err := s.AttachTag("alice", roof, urgent); err != nil {
			return err
		}
		return s.AttachTag("alice", roof, slow)
	}); err != nil {
		t.Fatalf("AttachTag: %v", err)
	}

	if err := s.DeleteTag("alice", urgent); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	tags, err := s.Tags()
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 1 || tags[0].ID != slow {
		t.Errorf("the Tags read %+v, want only the one that was kept", tags)
	}
	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("%d Tasks survived the Tag, want the one", len(tasks))
	}
	if got := tasks[0].Tags; len(got) != 1 || got[0] != slow {
		t.Errorf("the Task carries %v, want only the Tag that was kept", got)
	}
}

func TestDeletingSomethingThatIsNotThereIsRefused(t *testing.T) {
	s := openTemp(t)
	was, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}

	if err := s.DeleteList("alice", "no-such-list"); err == nil {
		t.Error("deleting a list that does not exist was reported as done")
	}
	if err := s.DeleteTag("alice", "no-such-tag"); err == nil {
		t.Error("deleting a tag that does not exist was reported as done")
	}

	// The Change History is append-only, so an entry deleting something that
	// never existed is one nothing can take back.
	now, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if now != was {
		t.Errorf("history went from %d to %d entries, want it left alone", was, now)
	}
}

// A Tag narrows the way a List does, because a Task belongs to one the same
// way it belongs to the other.
func TestTasksNarrowToATag(t *testing.T) {
	s := openTemp(t)
	urgent, err := s.AddTag("alice", "urgent", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	tagged := leased(t, s, "alice", Attributes{Title: Set("Fix the leak")})
	leased(t, s, "alice", Attributes{Title: Set("Read a book")})
	if err := s.WithLease("alice", tagged, WriteTTL, func() error {
		return s.AttachTag("alice", tagged, urgent)
	}); err != nil {
		t.Fatalf("AttachTag: %v", err)
	}

	tasks, err := s.Tasks(Query{Tags: []string{urgent}})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != tagged {
		t.Errorf("narrowed to the tag the answer is %v, want only %s", ids(tasks), tagged)
	}
}

// Search reaches up where the other narrowings do not. A Subtask matching
// under a parent that does not is the whole case: hiding it would mean the
// word somebody would actually type finds nothing.
func TestSearchFindsAMatchUnderAParentThatDoesNot(t *testing.T) {
	s := openTemp(t)
	root := leased(t, s, "alice", Attributes{Title: Set("Redecorate")})
	paint, err := s.AddSubtask("alice", root, Attributes{Title: Set("Buy paint")})
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	leased(t, s, "alice", Attributes{Title: Set("Read a book")})

	tasks, err := s.Tasks(Query{Search: "paint"})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	// The parent comes back unmatched, so the match has somewhere to sit and
	// the answer is still a tree rather than a row with a Parent nobody sent.
	want := []string{root, paint}
	if strings.Join(ids(tasks), ",") != strings.Join(want, ",") {
		t.Errorf("searching for paint found %v, want the match and the Task above it %v", ids(tasks), want)
	}
	if tasks[1].Depth != 2 || tasks[1].Parent != root {
		t.Errorf("the match came back at depth %d under %q, want depth 2 under %s", tasks[1].Depth, tasks[1].Parent, root)
	}
}

// The words searched are the ones somebody typed into the Task, and the match
// is a case-insensitive substring rather than a whole word.
func TestSearchReadsTitleDescriptionAndWhy(t *testing.T) {
	s := openTemp(t)
	byTitle := leased(t, s, "alice", Attributes{Title: Set("Varnish the DOOR")})
	byDescription := leased(t, s, "alice", Attributes{
		Title:       Set("Saturday"),
		Description: Set("the door sticks in the rain"),
	})
	byWhy := leased(t, s, "alice", Attributes{
		Title: Set("Call the joiner"),
		Why:   Set("because of the door"),
	})
	leased(t, s, "alice", Attributes{Title: Set("Read a book")})

	tasks, err := s.Tasks(Query{Search: "doo"})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	want := []string{byTitle, byDescription, byWhy}
	if strings.Join(ids(tasks), ",") != strings.Join(want, ",") {
		t.Errorf("searching found %v, want the three that hold the word %v", ids(tasks), want)
	}
}

// The text is compared as characters. A person searching for a per cent sign
// means the character, which is what LIKE would have read as a wildcard.
func TestSearchTakesAWildcardLiterally(t *testing.T) {
	s := openTemp(t)
	literal := leased(t, s, "alice", Attributes{Title: Set("Pay the 50% deposit")})
	leased(t, s, "alice", Attributes{Title: Set("Read a book")})

	tasks, err := s.Tasks(Query{Search: "50%"})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != literal {
		t.Errorf("searching for 50%% found %v, want only %s", ids(tasks), literal)
	}
}

// Search narrows within the view rather than around it: a Task the other
// narrowings hide stays hidden, and so does a match underneath it.
func TestSearchDoesNotReachPastTheOtherNarrowings(t *testing.T) {
	s := openTemp(t)
	root := leased(t, s, "alice", Attributes{Title: Set("Redecorate")})
	if _, err := s.AddSubtask("alice", root, Attributes{Title: Set("Buy paint")}); err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	if err := s.WithLease("alice", root, WriteTTL, func() error {
		return s.DeleteTask("alice", root)
	}); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	tasks, err := s.Tasks(Query{Search: "paint"})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("searching found %v under a deleted Task, want nothing", ids(tasks))
	}
	if tasks, err = s.Tasks(Query{Search: "paint", IncludeDeleted: true}); err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("asked for the deleted too, searching found %v, want the pair", ids(tasks))
	}
}

func ids(tasks []Task) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.ID)
	}
	return out
}

// Any of them, not all of them. Clicking a second Tag widens what somebody is
// willing to look at; an intersection would empty the list on the second click
// far more often than it would narrow it usefully.
func TestNarrowingToSeveralTagsTakesATaskCarryingAnyOfThem(t *testing.T) {
	s := openTemp(t)
	urgent, err := s.AddTag("alice", "urgent", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	errand, err := s.AddTag("alice", "errand", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	first := leased(t, s, "alice", Attributes{Title: Set("Fix the leak")})
	second := leased(t, s, "alice", Attributes{Title: Set("Buy stamps")})
	leased(t, s, "alice", Attributes{Title: Set("Read a book")})
	for id, tag := range map[string]string{first: urgent, second: errand} {
		if err := s.WithLease("alice", id, WriteTTL, func() error {
			return s.AttachTag("alice", id, tag)
		}); err != nil {
			t.Fatalf("AttachTag: %v", err)
		}
	}

	tasks, err := s.Tasks(Query{Tags: []string{urgent, errand}})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("narrowed to two tags the answer is %v, want the two carrying one each", ids(tasks))
	}
}

// A Task carrying both is one Task in the answer. The membership test is an
// EXISTS, so it says whether rather than how many, and a Task wearing every
// named Tag cannot arrive once per Tag.
func TestATaskCarryingEveryNamedTagIsListedOnce(t *testing.T) {
	s := openTemp(t)
	urgent, err := s.AddTag("alice", "urgent", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	errand, err := s.AddTag("alice", "errand", "")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}

	both := leased(t, s, "alice", Attributes{Title: Set("Post the forms")})
	if err := s.WithLease("alice", both, WriteTTL, func() error {
		if err := s.AttachTag("alice", both, urgent); err != nil {
			return err
		}
		return s.AttachTag("alice", both, errand)
	}); err != nil {
		t.Fatalf("AttachTag: %v", err)
	}

	tasks, err := s.Tasks(Query{Tags: []string{urgent, errand}})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("the answer is %v, want the one Task once", ids(tasks))
	}
}

// No Tags named is no narrowing, which is what the everyday view asks for. An
// empty slice and a nil one are the same question.
func TestNamingNoTagsNarrowsNothing(t *testing.T) {
	s := openTemp(t)
	leased(t, s, "alice", Attributes{Title: Set("Read a book")})

	// An id with nothing in it is named no Tag, the way an empty List is. A
	// caller that built the query out of a variable nobody set asks for the
	// list rather than for silence, and an Agent reaching the store through
	// the API gets the same answer a person typing the verb does.
	for _, tags := range [][]string{nil, {}, {""}, {"  "}, {"", "\t"}} {
		tasks, err := s.Tasks(Query{Tags: tags})
		if err != nil {
			t.Fatalf("Tasks(%v): %v", tags, err)
		}
		if len(tasks) != 1 {
			t.Errorf("Tags %v gives %v, want the whole list", tags, ids(tasks))
		}
	}

	// And an empty id beside a real one is the real one asked for on its own,
	// rather than a set nothing can match.
	tag, err := s.AddTag("alice", "errand", "green")
	if err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	tasks, err := s.Tasks(Query{Tags: []string{"", tag}})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("an empty id beside a real one gives %v, want only what carries the tag", ids(tasks))
	}
}
