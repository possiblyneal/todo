package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// fields collects repeated -field k=v flags.
type fields map[string]string

func (f fields) String() string { return "" }

func (f fields) Set(v string) error {
	key, value, ok := strings.Cut(v, "=")
	if !ok || key == "" {
		return fmt.Errorf("want key=value, got %q", v)
	}
	f[key] = value
	return nil
}

// ids collects a repeatable flag naming things by id.
type ids []string

func (v *ids) String() string { return strings.Join(*v, ",") }

func (v *ids) Set(s string) error {
	*v = append(*v, s)
	return nil
}

// attributeFlags registers every Task attribute on fs and returns a function
// that reads back only the flags actually given. A flag nobody typed leaves
// its attribute alone; a flag given empty clears it.
func attributeFlags(fs *flag.FlagSet) func() (store.Attributes, bool, error) {
	var (
		title       = fs.String("title", "", "the task's title")
		description = fs.String("description", "", "what the task is")
		why         = fs.String("why", "", "why the task is worth doing")
		deadline    = fs.String("deadline", "", "when it is due: 2006-01-02, 2006-01-02 15:04, or RFC 3339")
		estimate    = fs.String("estimate", "", "how long it will take, as a duration such as 90m")
		priority    = fs.String("priority", "", "low, med or high")
		impact      = fs.String("impact", "", "low, med or high")
		snooze      = fs.String("snooze", "", "hide it for a while: a duration, or "+snoozeLabels())
		colour      = fs.String("colour", "", "the task's colour")
		pairs       = fields{}
	)
	fs.Var(pairs, "field", "a key=value pair, repeatable")

	return func() (store.Attributes, bool, error) {
		var a store.Attributes
		var err error
		given := false
		fs.Visit(func(f *flag.Flag) {
			if err != nil {
				return
			}
			// Only the flags registered here count as an attribute: a
			// verb may hang others on the same set, and edit does.
			matched := true
			switch f.Name {
			case "title":
				a.Title = title
			case "description":
				a.Description = description
			case "why":
				a.Why = why
			case "colour":
				a.Colour = colour
			case "priority":
				var l store.Level
				if l, err = store.ParseLevel(*priority); err == nil {
					a.Priority = &l
				}
			case "impact":
				var l store.Level
				if l, err = store.ParseLevel(*impact); err == nil {
					a.Impact = &l
				}
			case "deadline":
				var when time.Time
				if when, err = parseWhen(*deadline); err == nil {
					a.Deadline = &when
				}
			case "snooze":
				var until time.Time
				if until, err = parseSnooze(*snooze); err == nil {
					a.SnoozedUntil = &until
				}
			case "estimate":
				var d time.Duration
				if *estimate != "" {
					if d, err = time.ParseDuration(*estimate); err != nil {
						err = fmt.Errorf("estimate %q: %w", *estimate, err)
					}
				}
				a.Estimate = &d
			default:
				matched = false
			}
			given = given || matched
		})
		if len(pairs) > 0 {
			a.Fields = pairs
		}
		return a, given || len(pairs) > 0, err
	}
}

// parseWhen reads a deadline. An empty string is the zero time, which clears.
func parseWhen(v string) (time.Time, error) {
	if strings.TrimSpace(v) == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.DateOnly, "2006-01-02 15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read %q as a date: want 2006-01-02, 2006-01-02 15:04, or RFC 3339", v)
}

// parseSnooze reads either one of the offered defaults or a plain duration.
func parseSnooze(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	now := time.Now()
	for _, s := range store.SnoozeDefaults {
		if strings.EqualFold(v, s.Label) {
			return s.Until(now).UTC(), nil
		}
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot read %q as a snooze: want a duration, or %s", v, snoozeLabels())
	}
	return now.Add(d).UTC(), nil
}

func snoozeLabels() string {
	labels := make([]string, len(store.SnoozeDefaults))
	for i, s := range store.SnoozeDefaults {
		labels[i] = s.Label
	}
	return strings.Join(labels, ", ")
}

func addTask(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("add", stderr)
	parent := fs.String("parent", "", "nest the new task under this task id, to five levels")
	read := attributeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	a, _, err := read()
	if err != nil {
		fmt.Fprintf(stderr, "todo add: %v\n", err)
		return 2
	}
	if title := strings.TrimSpace(strings.Join(fs.Args(), " ")); title != "" {
		a.Title = &title
	}
	if a.Title == nil {
		fmt.Fprintln(stderr, "todo add: a task needs a title")
		return 2
	}

	// A Subtask is written under its tree's Lease, the same one every other
	// write to that tree needs.
	if *parent != "" {
		var id string
		err := s.WithLease(actor(), *parent, store.WriteTTL, func() error {
			var err error
			id, err = s.AddSubtask(actor(), *parent, a)
			return err
		})
		if code := refuse(stderr, "add", err); code != 0 {
			return code
		}
		fmt.Fprintln(stdout, id)
		return 0
	}

	id, err := s.AddTask(actor(), a)
	if err != nil {
		fmt.Fprintf(stderr, "todo add: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, id)
	return 0
}

func listTasks(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("list", stderr)
	all := fs.Bool("all", false, "include completed, declined, snoozed and deleted tasks")
	in := fs.String("list", "", "only the tasks in this list, by id")
	sort := fs.String("sort", string(store.SortCreated),
		"order siblings by "+strings.Join(store.SortNames(), ", "))
	if err := fs.Parse(args); err != nil {
		return 2
	}
	q := store.Query{
		IncludeCompleted: *all,
		IncludeDeclined:  *all,
		IncludeSnoozed:   *all,
		IncludeDeleted:   *all,
		List:             *in,
		Sort:             store.Sort(*sort),
	}

	tasks, err := s.Tasks(q)
	if err != nil {
		fmt.Fprintf(stderr, "todo list: %v\n", err)
		return 2
	}
	// Tasks comes back depth first, so indenting by depth draws the tree
	// without the list having to rebuild it.
	for _, t := range tasks {
		indent := strings.Repeat("  ", t.Depth-1)
		fmt.Fprintf(stdout, "%s  %s%s%s\n", t.ID, indent, t.Title, marks(t))
	}
	return 0
}

// marks writes what a read worked out about a Task in the shape a terminal
// line takes. The words are store.Task.Marks; only the brackets are the CLI's.
func marks(t store.Task) string {
	m := t.Marks()
	if len(m) == 0 {
		return ""
	}
	return "  (" + strings.Join(m, ", ") + ")"
}

func editTask(s *store.Store, args []string, stderr io.Writer) int {
	fs := flags("edit", stderr)
	read := attributeFlags(fs)
	var into, outOf, carry, drop ids
	fs.Var(&into, "list", "put the task in this list, by id; repeatable")
	fs.Var(&outOf, "unlist", "take the task out of this list, by id; repeatable")
	fs.Var(&carry, "tag", "put this tag on the task, by id; repeatable")
	fs.Var(&drop, "untag", "take this tag off the task, by id; repeatable")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "todo edit: name one task by id")
		return 2
	}
	a, attributes, err := read()
	if err != nil {
		fmt.Fprintf(stderr, "todo edit: %v\n", err)
		return 2
	}
	id, who := fs.Arg(0), actor()

	// One Lease covers the whole edit: the attributes and every List and Tag
	// it joins or leaves are one visit to the tree.
	return refuse(stderr, "edit", s.WithLease(who, id, store.WriteTTL, func() error {
		if attributes {
			if err := s.EditTask(who, id, a); err != nil {
				return err
			}
		}
		for _, member := range []struct {
			ids ids
			do  func(actor, taskID, otherID string) error
		}{
			{into, s.AddToList}, {outOf, s.RemoveFromList},
			{carry, s.AttachTag}, {drop, s.DetachTag},
		} {
			for _, other := range member.ids {
				if err := member.do(who, id, other); err != nil {
					return err
				}
			}
		}
		return nil
	}))
}

// lifecycle is complete, decline, reopen and delete: one id, no flags, the
// same guard.
func lifecycle(s *store.Store, verb string, args []string, stderr io.Writer) int {
	fs := flags(verb, stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "todo %s: name one task by id\n", verb)
		return 2
	}
	id := fs.Arg(0)

	act := map[string]func(string, string) error{
		"complete": s.CompleteTask,
		"decline":  s.DeclineTask,
		"reopen":   s.ReopenTask,
		"delete":   s.DeleteTask,
	}[verb]

	return refuse(stderr, verb, s.WithLease(actor(), id, store.WriteTTL, func() error {
		return act(actor(), id)
	}))
}

// attachTask is `todo attach`: the pointers a Task holds. Bare with an id it
// lists them, with a target it adds one, and -off takes one off.
//
// It never looks at what it is pointed at. A path is written down as an
// absolute path and a web address as typed, and whether either still names
// anything is not the tracker's to know.
func attachTask(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("attach", stderr)
	off := fs.Bool("off", false, "take this pointer off the task")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintf(stderr, "todo attach: name one task by id\n")
		return 2
	}
	id := fs.Arg(0)

	if fs.NArg() == 1 {
		if *off {
			fmt.Fprintf(stderr, "todo attach: say which pointer to take off\n")
			return 2
		}
		tasks, err := s.Tasks(store.Query{IncludeCompleted: true, IncludeDeclined: true, IncludeSnoozed: true, IncludeDeleted: true})
		if err != nil {
			fmt.Fprintf(stderr, "todo attach: %v\n", err)
			return 1
		}
		for _, t := range tasks {
			if t.ID != id {
				continue
			}
			for _, target := range t.Attachments {
				fmt.Fprintln(stdout, target)
			}
			return 0
		}
		fmt.Fprintf(stderr, "todo attach: no task %s\n", id)
		return 2
	}

	target := strings.Join(fs.Args()[1:], " ")
	return refuse(stderr, "attach", s.WithLease(actor(), id, store.WriteTTL, func() error {
		if *off {
			return s.Detach(actor(), id, target)
		}
		return s.Attach(actor(), id, target)
	}))
}

// repeatTask is `todo repeat`: the one verb that reaches Scheduling. Bare, it
// shows the rule and the dates it produces next; with words after the id it
// sets the rule; and -tick, -skip and -detach act on one date.
//
// The dates it prints are computed as it prints them. Nothing is stored by
// showing them, which is why asking for a hundred of them is free.
func repeatTask(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("repeat", stderr)
	off := fs.Bool("off", false, "stop the task repeating; the record of it stays")
	tick := fs.String("tick", "", "mark one date done: 2006-01-02")
	skip := fs.String("skip", "", "mark one date skipped: 2006-01-02")
	detach := fs.String("detach", "", "lift one date out into an ordinary task: 2006-01-02")
	count := fs.Int("n", 5, "how many upcoming dates to show")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "todo repeat: name one task by id")
		return 2
	}
	id, who := fs.Arg(0), actor()
	rule := strings.Join(fs.Args()[1:], " ")

	// One date, one mark. Each is a write to the task's tree and takes the
	// Lease covering it.
	for _, act := range []struct {
		flag string
		date string
		do   func(on time.Time) error
	}{
		{"tick", *tick, func(on time.Time) error { return s.TickOccurrence(who, id, on) }},
		{"skip", *skip, func(on time.Time) error { return s.SkipOccurrence(who, id, on) }},
		{"detach", *detach, func(on time.Time) error {
			detached, err := s.DetachOccurrence(who, id, on)
			if err == nil {
				fmt.Fprintln(stdout, detached)
			}
			return err
		}},
	} {
		if act.date == "" {
			continue
		}
		on, err := time.ParseInLocation(time.DateOnly, act.date, time.UTC)
		if err != nil {
			fmt.Fprintf(stderr, "todo repeat: cannot read %q as a date: want 2006-01-02\n", act.date)
			return 2
		}
		return refuse(stderr, "repeat", s.WithLease(who, id, store.WriteTTL, func() error {
			return act.do(on)
		}))
	}

	switch {
	case *off:
		return refuse(stderr, "repeat", s.WithLease(who, id, store.WriteTTL, func() error {
			return s.Unrepeat(who, id)
		}))
	case rule != "":
		return refuse(stderr, "repeat", s.WithLease(who, id, store.WriteTTL, func() error {
			_, err := s.Repeat(who, id, rule)
			return err
		}))
	}
	return showRepeat(s, id, *count, stdout, stderr)
}

// showRepeat prints the rule and the dates it produces next. The window is two
// years, which is long enough to hold the next n dates of any rule the parser
// accepts and short enough that a daily rule stays cheap to walk.
func showRepeat(s *store.Store, id string, count int, stdout, stderr io.Writer) int {
	rule, repeats, err := s.Rule(id)
	if err != nil {
		fmt.Fprintf(stderr, "todo repeat: %v\n", err)
		return 1
	}
	if !repeats {
		fmt.Fprintln(stdout, "does not repeat")
		return 0
	}
	fmt.Fprintln(stdout, rule.String())

	// Local, not UTC: schedule reads a date in the local zone, so a UTC
	// instant whose calendar date is not the local one shifts the window a
	// day and shows the wrong date first.
	from := time.Now()
	occurrences, err := s.Occurrences(id, from, from.AddDate(2, 0, 0))
	if err != nil {
		fmt.Fprintf(stderr, "todo repeat: %v\n", err)
		return 1
	}
	for i, o := range occurrences {
		if i == count {
			break
		}
		state := ""
		if o.State != store.Pending {
			state = "  (" + string(o.State) + ")"
		}
		fmt.Fprintf(stdout, "%s%s\n", o.Date.Format(time.DateOnly), state)
	}
	return 0
}

// refuse turns a refusal into its own exit status, so an Agent can tell "the
// tree is held by someone else, come back" from "that did not work".
func refuse(stderr io.Writer, verb string, err error) int {
	switch {
	case err == nil:
		return 0
	case isRefusal(err):
		fmt.Fprintf(stderr, "todo %s: %v\n", verb, err)
		return 3
	default:
		fmt.Fprintf(stderr, "todo %s: %v\n", verb, err)
		return 1
	}
}

func flags(verb string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("todo "+verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// collections is `todo lists` and `todo tags`, which differ only in what they
// act on. Bare, it shows them ranked; `new`, `rename` and `recolour` are the
// three writes, and none of them needs a Lease: a List and a Tag are
// aggregates of their own, not part of anybody's tree.
func collections(s *store.Store, noun string, args []string, stdout, stderr io.Writer) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := flags(noun, stderr)
	colour := fs.String("colour", "", "the colour it carries")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	add, describe, show := s.AddList, s.DescribeList, listsOf(s)
	if noun == "tags" {
		add, describe, show = s.AddTag, s.DescribeTag, tagsOf(s)
	}

	switch sub {
	case "":
		rows, err := show()
		if err != nil {
			fmt.Fprintf(stderr, "todo %s: %v\n", noun, err)
			return 1
		}
		for _, row := range rows {
			fmt.Fprintln(stdout, row)
		}
		return 0

	case "new":
		id, err := add(actor(), strings.Join(fs.Args(), " "), *colour)
		if err != nil {
			fmt.Fprintf(stderr, "todo %s new: %v\n", noun, err)
			return 2
		}
		fmt.Fprintln(stdout, id)
		return 0

	case "rename", "recolour":
		if fs.NArg() < 2 {
			fmt.Fprintf(stderr, "todo %s %s: name one by id, then what to call it\n", noun, sub)
			return 2
		}
		value := strings.Join(fs.Args()[1:], " ")
		var name, paint *string
		if sub == "rename" {
			name = &value
		} else {
			paint = &value
		}
		if err := describe(actor(), fs.Arg(0), name, paint); err != nil {
			fmt.Fprintf(stderr, "todo %s %s: %v\n", noun, sub, err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(stderr, "todo %s: unknown form %q: want new, rename or recolour\n", noun, sub)
		return 2
	}
}

// listsOf and tagsOf render a collection the same way, most carried first for
// tags and by name for lists, which is the order each reader returns.
func listsOf(s *store.Store) func() ([]string, error) {
	return func() ([]string, error) {
		lists, err := s.Lists()
		if err != nil {
			return nil, err
		}
		rows := make([]string, len(lists))
		for i, l := range lists {
			rows[i] = fmt.Sprintf("%s  %s  (%d)", l.ID, l.Name, l.Count)
		}
		return rows, nil
	}
}

func tagsOf(s *store.Store) func() ([]string, error) {
	return func() ([]string, error) {
		tags, err := s.Tags()
		if err != nil {
			return nil, err
		}
		rows := make([]string, len(tags))
		for i, g := range tags {
			rows[i] = fmt.Sprintf("%s  %s  (%d)", g.ID, g.Name, g.Count)
		}
		return rows, nil
	}
}
