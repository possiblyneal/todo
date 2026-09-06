package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// leaseTTL is how long a verb holds the Lease it takes. A verb acts and exits,
// so it wants just enough to cover its own write and nothing that would strand
// the tree if the process dies.
const leaseTTL = 30 * time.Second

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

// attributeFlags registers every Task attribute on fs and returns a function
// that reads back only the flags actually given. A flag nobody typed leaves
// its attribute alone; a flag given empty clears it.
func attributeFlags(fs *flag.FlagSet) func() (store.Attributes, error) {
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

	return func() (store.Attributes, error) {
		var a store.Attributes
		var err error
		fs.Visit(func(f *flag.Flag) {
			if err != nil {
				return
			}
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
			}
		})
		if len(pairs) > 0 {
			a.Fields = pairs
		}
		return a, err
	}
}

// parseWhen reads a deadline. An empty string is the zero time, which clears.
func parseWhen(v string) (time.Time, error) {
	if strings.TrimSpace(v) == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04", time.RFC3339} {
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
	read := attributeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	a, err := read()
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
	all := fs.Bool("all", false, "include completed, snoozed and deleted tasks")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	q := store.Query{IncludeCompleted: *all, IncludeSnoozed: *all, IncludeDeleted: *all}

	tasks, err := s.Tasks(q)
	if err != nil {
		fmt.Fprintf(stderr, "todo list: %v\n", err)
		return 1
	}
	for _, t := range tasks {
		fmt.Fprintf(stdout, "%s  %s%s\n", t.ID, t.Title, marks(t))
	}
	return 0
}

// marks says what a read worked out about a Task, rather than what is stored
// on it: overdue is deadline < now, and it was computed by the read above.
func marks(t store.Task) string {
	var m []string
	if t.Overdue {
		m = append(m, "overdue")
	}
	if !t.CompletedAt.IsZero() {
		m = append(m, "done")
	}
	if !t.DeletedAt.IsZero() {
		m = append(m, "deleted")
	}
	if !t.SnoozedUntil.IsZero() && t.SnoozedUntil.After(time.Now().UTC()) {
		m = append(m, "snoozed")
	}
	if len(m) == 0 {
		return ""
	}
	return "  (" + strings.Join(m, ", ") + ")"
}

func editTask(s *store.Store, args []string, stderr io.Writer) int {
	fs := flags("edit", stderr)
	read := attributeFlags(fs)
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
	id := fs.Arg(0)
	return refuse(stderr, "edit", s.WithLease(actor(), id, leaseTTL, func() error {
		return s.EditTask(actor(), id, a)
	}))
}

// lifecycle is complete, reopen and delete: one id, no flags, the same guard.
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
		"reopen":   s.ReopenTask,
		"delete":   s.DeleteTask,
	}[verb]

	return refuse(stderr, verb, s.WithLease(actor(), id, leaseTTL, func() error {
		return act(actor(), id)
	}))
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
