package write

import (
	"fmt"
	"slices"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// FromCapture turns what the Broker read out of a dump into the attributes a
// Task is written with. A value written in a way this program cannot read is
// dropped rather than refused, the same rule an approved proposal is written
// under: somebody said what the work is, not how long it takes.
//
// A missing title is the one refusal, because a Task with no title is not one.
func FromCapture(read ai.Capture) (store.Attributes, error) {
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
	if when, err := Deadline(read.Deadline); err == nil && !when.IsZero() {
		a.Deadline = &when
	}
	if d, err := Estimate(read.Estimate); err == nil && d > 0 {
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

// NamedIn is the ids of the Lists or Tags the Broker chose by name, ignoring
// case. A name that is not one of the offered ones is dropped: filing under a
// List means one that exists, and creating one is a write of its own. The same
// name said twice is one id, so a membership is appended once.
func NamedIn(chosen []string, have map[string]string) []string {
	var out []string
	for _, name := range chosen {
		id, ok := have[strings.ToLower(strings.TrimSpace(name))]
		if ok && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// ListNames and TagNames index a collection by its lowercased name, which is
// how a name the Broker chose is matched back to the thing it named.
func ListNames(lists []store.List) map[string]string {
	names := make(map[string]string, len(lists))
	for _, l := range lists {
		names[strings.ToLower(l.Name)] = l.ID
	}
	return names
}

func TagNames(tags []store.Tag) map[string]string {
	names := make(map[string]string, len(tags))
	for _, t := range tags {
		names[strings.ToLower(t.Name)] = t.ID
	}
	return names
}
