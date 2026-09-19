package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
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
// its attribute alone; a flag given empty clears it. The reading itself is
// write.Given's, so a date typed at a terminal and one sent by the client mean
// the same day.
func attributeFlags(fs *flag.FlagSet) func() (*store.Attributes, error) {
	var (
		title       = fs.String("title", "", "the task's title")
		description = fs.String("description", "", "what the task is")
		why         = fs.String("why", "", "why the task is worth doing")
		deadline    = fs.String("deadline", "", "when it is due: 2006-01-02, 2006-01-02 15:04, or RFC 3339")
		estimate    = fs.String("estimate", "", "how long it will take, as a duration such as 90m")
		priority    = fs.String("priority", "", "one of "+write.LevelLabels())
		impact      = fs.String("impact", "", "one of "+write.LevelLabels())
		snooze      = fs.String("snooze", "", "hide it for a while: a duration, or "+write.SnoozeLabels())
		color       = fs.String("color", "", "one of "+write.ColorLabels())
		pairs       = fields{}
	)
	fs.Var(pairs, "field", "a key=value pair, repeatable")

	return func() (*store.Attributes, error) {
		var given write.Given
		fs.Visit(func(f *flag.Flag) {
			// Only the flags registered here count as an attribute: a
			// verb may hang others on the same set, and edit does.
			switch f.Name {
			case "title":
				given.Title = title
			case "description":
				given.Description = description
			case "why":
				given.Why = why
			case "color":
				given.Color = color
			case "priority":
				given.Priority = priority
			case "impact":
				given.Impact = impact
			case "deadline":
				given.Deadline = deadline
			case "snooze":
				given.Snooze = snooze
			case "estimate":
				given.Estimate = estimate
			}
		})
		if len(pairs) > 0 {
			given.Fields = pairs
		}
		return given.Attributes()
	}
}

func addTask(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("add", stderr)
	parent := fs.String("parent", "", "nest the new task under this task id, to five levels")
	read := attributeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	given, err := read()
	if err != nil {
		fmt.Fprintf(stderr, "todo add: %v\n", err)
		return 2
	}
	var a store.Attributes
	if given != nil {
		a = *given
	}
	if title := strings.TrimSpace(strings.Join(fs.Args(), " ")); title != "" {
		a.Title = &title
	}
	if a.Title == nil {
		fmt.Fprintln(stderr, "todo add: a task needs a title")
		return 2
	}

	// A Subtask is written under its tree's Lease, the same one every other
	// write to that tree needs, and write.Add is what knows that.
	id, err := write.Add(s, actor(), *parent, a, write.Membership{})
	if code := refuse(stderr, "add", err); code != 0 {
		return code
	}
	fmt.Fprintln(stdout, id)
	return 0
}

// tagIDs is -tag given more than once. A Task carrying any of them is in the
// answer, which is what store.Query.Tags means; there is no separator to get
// wrong and no tag id with a comma in it to worry about.
type tagIDs []string

func (t *tagIDs) String() string { return strings.Join(*t, ",") }

func (t *tagIDs) Set(value string) error {
	*t = append(*t, value)
	return nil
}

func listTasks(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("list", stderr)
	all := fs.Bool("all", false, "include completed, declined, snoozed and deleted tasks")
	in := fs.String("list", "", "only the tasks in this list, by id")
	var tags tagIDs
	fs.Var(&tags, "tag", "only the tasks carrying this tag, by id; repeat for any of several")
	search := fs.String("search", "", "only the tasks whose title, description or why hold this text, and the tasks they sit under")
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
		Tags:             tags,
		Search:           *search,
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
	a, err := read()
	if err != nil {
		fmt.Fprintf(stderr, "todo edit: %v\n", err)
		return 2
	}
	id, who := fs.Arg(0), actor()

	// One Lease covers the whole edit: the attributes and every List and Tag
	// it joins or leaves are one visit to the tree, and write.Edit is what
	// holds that shape for both surfaces.
	return refuse(stderr, "edit", write.Edit(s, who, id, a, write.Membership{
		IntoLists:  into,
		OutOfLists: outOf,
		AddTags:    carry,
		DropTags:   drop,
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

	// Which verb makes which write, and the Lease around it, is write's: this
	// mode and the API name the same four rather than each keeping a list.
	act, ok := write.Lifecycle(verb)
	if !ok {
		fmt.Fprintf(stderr, "todo %s: not something a task does\n", verb)
		return 2
	}
	return refuse(stderr, verb, act(s, actor(), id))
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
	if *off {
		return refuse(stderr, "attach", write.Detach(s, actor(), id, target))
	}
	return refuse(stderr, "attach", write.Attach(s, actor(), id, target))
}

// markHelp is what each mark does, in this surface's words. It describes the
// marks write.MarkNames answers rather than deciding which there are: a mark
// missing from here still gets a flag, and only its help text is duller.
var markHelp = map[string]string{
	"tick":   "mark one date done",
	"skip":   "mark one date skipped",
	"detach": "lift one date out into an ordinary task",
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
	// One flag per mark, and which marks there are is write.MarkNames' answer
	// rather than three names retyped here. A mark added there arrives as a
	// flag the same day, described by markHelp or by the fallback below.
	dates := map[string]*string{}
	for _, mark := range write.MarkNames() {
		help, ok := markHelp[mark]
		if !ok {
			help = "mark one date " + mark + "ed"
		}
		dates[mark] = fs.String(mark, "", help+": 2006-01-02")
	}
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

	// One date, one mark. Each is a guarded write to the task's tree, which is
	// write.Mark's shape rather than a second copy of it here. The order is
	// write.MarkNames' too, so the flag that wins when two are given is the
	// same one the API would pick out of the same list.
	for _, mark := range write.MarkNames() {
		asked := *dates[mark]
		if asked == "" {
			continue
		}
		on, err := time.ParseInLocation(time.DateOnly, asked, time.UTC)
		if err != nil {
			fmt.Fprintf(stderr, "todo repeat: cannot read %q as a date: want 2006-01-02\n", asked)
			return 2
		}
		act, ok := write.Mark(mark)
		if !ok {
			// Unreachable while the flags are registered from the same list,
			// and said rather than assumed: the alternative is calling a nil
			// function if the two ever stop being the same list.
			fmt.Fprintf(stderr, "todo repeat: a date is not %sed\n", mark)
			return 2
		}
		written, err := act(s, who, id, on)
		// Detaching answers the id of the Task the date became, and that id is
		// the only thing naming it afterwards.
		if err == nil && written != "" {
			fmt.Fprintln(stdout, written)
		}
		return refuse(stderr, "repeat", err)
	}

	switch {
	case *off:
		return refuse(stderr, "repeat", write.Unrepeat(s, who, id))
	case rule != "":
		return refuse(stderr, "repeat", write.Repeat(s, who, id, rule))
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
	// store.Refused is the one statement of which errors are a refusal, so this
	// exit code and the API's 409 cannot come to disagree about what one is.
	case store.Refused(err):
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
// act on. Bare, it shows them ranked; `new`, `rename`, `recolor` and `delete`
// are the four writes, and none of them needs a Lease: a List and a Tag are
// aggregates of their own, not part of anybody's tree.
func collections(s *store.Store, noun string, args []string, stdout, stderr io.Writer) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := flags(noun, stderr)
	color := fs.String("color", "", "one of "+write.ColorLabels())
	if err := fs.Parse(args); err != nil {
		return 2
	}

	add, describe, show, drop := s.AddList, s.DescribeList, listsOf(s), s.DeleteList
	if noun == "tags" {
		add, describe, show, drop = s.AddTag, s.DescribeTag, tagsOf(s), s.DeleteTag
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
		id, err := add(actor(), strings.Join(fs.Args(), " "), *color)
		if err != nil {
			fmt.Fprintf(stderr, "todo %s new: %v\n", noun, err)
			return 2
		}
		fmt.Fprintln(stdout, id)
		return 0

	case "rename", "recolor":
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

	case "delete":
		if fs.NArg() != 1 {
			fmt.Fprintf(stderr, "todo %s delete: name one by id\n", noun)
			return 2
		}
		if err := drop(actor(), fs.Arg(0)); err != nil {
			fmt.Fprintf(stderr, "todo %s delete: %v\n", noun, err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(stderr, "todo %s: unknown form %q: want new, rename, recolor or delete\n", noun, sub)
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
