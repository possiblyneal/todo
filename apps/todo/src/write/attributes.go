package write

import (
	"fmt"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Given is a Task's attributes as text, each one present or absent. A flag set
// says which were typed and a JSON body says which were sent, and both arrive
// here as the same thing so that neither surface can read a date, a duration
// or a level differently from the other.
//
// Absent leaves an attribute alone. Present and empty clears it, which is what
// a typed `-title ""` and a sent `"title": ""` both mean.
type Given struct {
	Title       *string
	Description *string
	Why         *string
	Color       *string
	Deadline    *string
	Estimate    *string
	Priority    *string
	Impact      *string
	Snooze      *string
	Fields      map[string]string
}

// Attributes reads Given into what the store takes, or nil when nothing was
// given at all: nothing given is not an edit, and calling EditTask with an
// empty Attributes would append an entry saying somebody changed nothing.
//
// The ones that can be read wrongly are read in the order the flags carrying
// them sort in, because that is the order a flag set visits them in and so the
// order the first of two bad values has always been the one reported.
func (g Given) Attributes() (*store.Attributes, error) {
	var a store.Attributes
	given := false

	// A Title is trimmed because the space around it is not part of it, the
	// same reason the space around a date is not part of the date. It is
	// trimmed here rather than at either surface so that a typed
	// `-title "  "` and a sent `"title": "  "` are the one empty Title the
	// store refuses, rather than one refusal and one Task titled with spaces.
	if g.Title != nil {
		title := strings.TrimSpace(*g.Title)
		g.Title = &title
	}

	for _, t := range []struct {
		from *string
		to   **string
	}{
		{g.Title, &a.Title},
		{g.Description, &a.Description},
		{g.Why, &a.Why},
		{g.Color, &a.Color},
	} {
		if t.from != nil {
			*t.to = t.from
			given = true
		}
	}

	if g.Deadline != nil {
		when, err := Deadline(*g.Deadline)
		if err != nil {
			return nil, err
		}
		a.Deadline = &when
		given = true
	}
	if g.Estimate != nil {
		d, err := Estimate(*g.Estimate)
		if err != nil {
			return nil, err
		}
		a.Estimate = &d
		given = true
	}
	for _, l := range []struct {
		from *string
		to   **store.Level
	}{
		{g.Impact, &a.Impact},
		{g.Priority, &a.Priority},
	} {
		if l.from == nil {
			continue
		}
		level, err := store.ParseLevel(*l.from)
		if err != nil {
			return nil, err
		}
		*l.to = &level
		given = true
	}
	if g.Snooze != nil {
		until, err := Snooze(*g.Snooze)
		if err != nil {
			return nil, err
		}
		a.SnoozedUntil = &until
		given = true
	}

	if len(g.Fields) > 0 {
		a.Fields = g.Fields
		given = true
	}
	if !given {
		return nil, nil
	}
	return &a, nil
}

// Deadline reads a due date. An empty string is the zero time, which clears.
// The space around a date is not part of it, for the same reason it is not
// part of an estimate: the Broker writes prose, and " 2026-03-04 " is a date
// it said rather than one this cannot read.
func Deadline(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.DateOnly, "2006-01-02 15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read %q as a date: want 2006-01-02, 2006-01-02 15:04, or RFC 3339", v)
}

// Snooze reads either one of the offered defaults or a plain duration.
func Snooze(v string) (time.Time, error) {
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
		return time.Time{}, fmt.Errorf("cannot read %q as a snooze: want a duration, or %s", v, SnoozeLabels())
	}
	return now.Add(d).UTC(), nil
}

// Estimate reads how long a Task will take. An empty string is no duration,
// which clears. Surrounding space is not part of a duration, and the Broker
// writes its answer as prose rather than as a flag value, so " 90m " is 90m
// here rather than a duration this cannot read and FromCapture drops.
func Estimate(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("estimate %q: %w", v, err)
	}
	return d, nil
}

// ColorLabels names the offered colors, which are the only ones the store
// takes. It is the sentence a surface offers them in.
func ColorLabels() string {
	labels := make([]string, len(store.Colors))
	for i, c := range store.Colors {
		labels[i] = c.Name
	}
	return strings.Join(labels, ", ")
}

// SnoozeLabels names the offered snoozes, which are not the only ones taken:
// Snooze reads a plain duration too.
func SnoozeLabels() string {
	return strings.Join(store.SnoozeNames(), ", ")
}

// LevelLabels names the three, for the reason ColorLabels names the nine: a
// flag spelling them out itself offers a fourth the day one is added to the
// store and not to the flag. The examples beside them are not in here — three
// sentences is a wall in a flag's help — so a surface that has room for them
// reads them off GET /api/state, which is where they are served.
func LevelLabels() string {
	return strings.Join(store.LevelNames(), ", ")
}
