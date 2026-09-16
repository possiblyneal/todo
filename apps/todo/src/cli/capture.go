package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
)

// today is the date the broker works a "tomorrow" out against. The ai package
// reads no clock, so every dump carries one.
const today = "2006-01-02, Monday"

// captureTask is `todo capture`: a brain dump, read by the broker, written as
// one Task. It is the one verb that writes what the broker said, and it can be
// because the dump is a person's own words about a Task they are already
// describing rather than a suggestion nobody asked for. `-dry` prints what was
// read and writes nothing, which is where it is checked before it is taken at
// its word.
func captureTask(s *store.Store, args []string, stdout, stderr io.Writer) int {
	fs := flags("capture", stderr)
	dry := fs.Bool("dry", false, "print what the broker read out of it and write nothing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		fmt.Fprintln(stderr, "todo capture: say what the task is")
		return 2
	}

	lists, err := s.Lists()
	if err != nil {
		fmt.Fprintf(stderr, "todo capture: %v\n", err)
		return 1
	}
	tags, err := s.Tags()
	if err != nil {
		fmt.Fprintf(stderr, "todo capture: %v\n", err)
		return 1
	}

	dump := ai.Dump{Text: text, Today: time.Now().Format(today)}
	for _, l := range lists {
		dump.Lists = append(dump.Lists, l.Name)
	}
	for _, t := range tags {
		dump.Tags = append(dump.Tags, t.Name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	read, err := ai.New().Read(ctx, dump)
	if err != nil {
		fmt.Fprintf(stderr, "todo capture: %v\n", err)
		return 1
	}
	// What was read is printed before it is judged, because `-dry` is where
	// an answer this program cannot use is looked at rather than guessed at.
	if *dry {
		writeCapture(stdout, read)
	}
	a, err := write.FromCapture(read)
	if err != nil {
		fmt.Fprintf(stderr, "todo capture: %v\n", err)
		return 1
	}
	if *dry {
		return 0
	}

	// Membership is a write to the Task, so it goes under the Task's own
	// Lease like any other, which is write.Add's business rather than this
	// verb's. The id is said whether or not the filing went through: the
	// Task is written by then, so a refusal leaves it filed under nothing,
	// which is one `todo edit` away for whoever is told which Task it is.
	id, err := write.Add(s, actor(), "", a, write.Membership{
		IntoLists: write.NamedIn(read.Lists, write.ListNames(lists)),
		AddTags:   write.NamedIn(read.Tags, write.TagNames(tags)),
	})
	if id != "" {
		fmt.Fprintln(stdout, id)
	}
	return refuse(stderr, "capture", err)
}

// writeCapture is `-dry`: what the broker made of the dump, in the same words
// the attributes are named in everywhere else.
func writeCapture(stdout io.Writer, read ai.Capture) {
	say := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(stdout, "%-11s %s\n", label, value)
		}
	}
	say("title", read.Title)
	say("description", read.Description)
	say("why", read.Why)
	say("deadline", read.Deadline)
	say("estimate", read.Estimate)
	say("priority", read.Priority)
	say("impact", read.Impact)
	say("lists", strings.Join(read.Lists, ", "))
	say("tags", strings.Join(read.Tags, ", "))
}
