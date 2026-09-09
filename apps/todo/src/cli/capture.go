package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
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
	a, err := attributesRead(read)
	if err != nil {
		fmt.Fprintf(stderr, "todo capture: %v\n", err)
		return 1
	}

	if *dry {
		writeCapture(stdout, read)
		return 0
	}

	id, err := s.AddTask(actor(), a)
	if err != nil {
		fmt.Fprintf(stderr, "todo capture: %v\n", err)
		return 1
	}
	// Membership is a write to the Task, so it goes under the Task's own
	// Lease like any other. The Task is written either way: a refusal here
	// leaves it filed under nothing, which is one `todo edit` away.
	err = s.WithLease(actor(), id, store.WriteTTL, func() error {
		for _, listID := range namedIn(read.Lists, listNames(lists)) {
			if err := s.AddToList(actor(), id, listID); err != nil {
				return err
			}
		}
		for _, tagID := range namedIn(read.Tags, tagNames(tags)) {
			if err := s.AttachTag(actor(), id, tagID); err != nil {
				return err
			}
		}
		return nil
	})
	if code := refuse(stderr, "capture", err); code != 0 {
		return code
	}
	fmt.Fprintln(stdout, id)
	return 0
}

// attributesRead turns what the broker read into the attributes a Task is
// written with. A value written in a way this program cannot read is dropped
// rather than refused, the same rule an approved proposal is written under: a
// person said what the work is, not how long it takes.
func attributesRead(read ai.Capture) (store.Attributes, error) {
	title := strings.TrimSpace(read.Title)
	if title == "" {
		return store.Attributes{}, fmt.Errorf("the broker read no title out of that")
	}
	a := store.Attributes{Title: &title}
	if read.Description != "" {
		a.Description = &read.Description
	}
	if read.Why != "" {
		a.Why = &read.Why
	}
	if when, err := parseWhen(read.Deadline); err == nil && !when.IsZero() {
		a.Deadline = &when
	}
	if d, err := time.ParseDuration(strings.TrimSpace(read.Estimate)); err == nil && d > 0 {
		a.Estimate = &d
	}
	if l, err := store.ParseLevel(read.Priority); err == nil && l != "" {
		a.Priority = &l
	}
	if l, err := store.ParseLevel(read.Impact); err == nil && l != "" {
		a.Impact = &l
	}
	return a, nil
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

// namedIn is the ids of the Lists or Tags the broker chose by name, ignoring
// case. A name that is not one of the offered ones is dropped: filing under a
// List means one that exists, and creating one is a write of its own.
func namedIn(chosen []string, have map[string]string) []string {
	var out []string
	for _, name := range chosen {
		if id, ok := have[strings.ToLower(strings.TrimSpace(name))]; ok {
			out = append(out, id)
		}
	}
	return out
}

func listNames(lists []store.List) map[string]string {
	names := make(map[string]string, len(lists))
	for _, l := range lists {
		names[strings.ToLower(l.Name)] = l.ID
	}
	return names
}

func tagNames(tags []store.Tag) map[string]string {
	names := make(map[string]string, len(tags))
	for _, t := range tags {
		names[strings.ToLower(t.Name)] = t.ID
	}
	return names
}
