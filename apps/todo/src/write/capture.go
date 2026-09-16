package write

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// dumpDate is the date the Broker works a "tomorrow" out against. The weekday
// is part of it because "Friday" in a dump is a date only where the Broker is
// told which day today is.
const dumpDate = "2006-01-02, Monday"

// Dump is one brain dump ready to be read: what the Broker is shown, and the
// Lists and Tags whose names it chooses from. The two collections are kept
// beside it because a name it chose is matched back to an id afterwards, and
// asking the store for them twice would be asking the same question twice.
type Dump struct {
	Shown ai.Dump
	Lists []store.List
	Tags  []store.Tag
}

// Gather builds one. Today's date comes from here rather than from src/ai,
// which reads no clock, and the Lists and Tags are offered by name because an
// id means nothing to the Broker.
func Gather(s *store.Store, text string) (Dump, error) {
	lists, err := s.Lists()
	if err != nil {
		return Dump{}, err
	}
	tags, err := s.Tags()
	if err != nil {
		return Dump{}, err
	}

	d := Dump{
		Shown: ai.Dump{Text: text, Today: time.Now().Format(dumpDate)},
		Lists: lists,
		Tags:  tags,
	}
	for _, l := range lists {
		d.Shown.Lists = append(d.Shown.Lists, l.Name)
	}
	for _, t := range tags {
		d.Shown.Tags = append(d.Shown.Tags, t.Name)
	}
	return d, nil
}

// Filed is the Lists and Tags the Broker chose, by id: the memberships one
// write files the Task under. A name it invented rather than chose is dropped,
// which is NamedIn's rule and not a second one written here.
func (d Dump) Filed(read ai.Capture) Membership {
	return Membership{
		IntoLists: NamedIn(read.Lists, ListNames(d.Lists)),
		AddTags:   NamedIn(read.Tags, TagNames(d.Tags)),
	}
}

// Briefs is the Tasks in view as the Broker is shown them: what they say, and
// nothing about how this program stores them. No id crosses the wire, because
// the answer comes back as prose about the Tasks rather than about rows.
func Briefs(tasks []store.Task) []ai.Brief {
	out := make([]ai.Brief, 0, len(tasks))
	for _, t := range tasks {
		b := ai.Brief{
			Title:       t.Title,
			Description: t.Description,
			Why:         t.Why,
			Priority:    string(t.Priority),
			Impact:      string(t.Impact),
		}
		if !t.Deadline.IsZero() {
			b.Deadline = t.Deadline.Local().Format(time.DateOnly)
		}
		if t.Estimate > 0 {
			b.Estimate = t.Estimate.String()
		}
		out = append(out, b)
	}
	return out
}

// AsSaid is what the Broker read, unparsed: every attribute it wrote something
// for, as the words it wrote. It is the other half of FromCapture and sits
// beside it so that which of the Broker's answers is which attribute is
// written once.
//
// Nothing is read here and nothing is dropped, which is the difference. A
// surface with a person at it puts these in the fields they came back in and
// lets them correct a value this program cannot read; FromCapture drops that
// value, because `todo capture` has nobody left to ask.
func AsSaid(read ai.Capture) Given {
	g := Given{}
	for _, said := range []struct {
		value string
		to    **string
	}{
		{read.Title, &g.Title},
		{read.Description, &g.Description},
		{read.Why, &g.Why},
		{read.Deadline, &g.Deadline},
		{read.Estimate, &g.Estimate},
		{read.Priority, &g.Priority},
		{read.Impact, &g.Impact},
	} {
		if said.value != "" {
			*said.to = &said.value
		}
	}
	return g
}

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
