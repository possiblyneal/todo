package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Level is a priority or an impact. Both use the same three, and each level
// carries an example, because a level nobody can tell apart from the next one
// is not a level.
type Level string

const (
	LevelLow  Level = "low"
	LevelMed  Level = "med"
	LevelHigh Level = "high"
)

// Levels is the three in order, low first.
var Levels = []Level{LevelLow, LevelMed, LevelHigh}

// PriorityExamples and ImpactExamples answer "what does med mean here". They
// are shown wherever a level is chosen. The wording is the tracker's to state
// and the operator's to rewrite; what is settled is that each level has one.
var (
	PriorityExamples = map[Level]string{
		LevelLow:  "Nothing goes wrong if this waits a month.",
		LevelMed:  "This week. Something starts slipping if it does not happen.",
		LevelHigh: "Today. Someone is blocked, or a deadline lands.",
	}
	ImpactExamples = map[Level]string{
		LevelLow:  "Only I notice. A tidy-up, or a small convenience.",
		LevelMed:  "One person or one piece of work moves forward.",
		LevelHigh: "It unblocks other work, or skipping it costs money or trust.",
	}
)

// ParseLevel reads a level off a surface, so a bad one is refused where it is
// typed rather than deep in the write path. An empty string clears the level.
func ParseLevel(v string) (Level, error) {
	l := Level(strings.ToLower(strings.TrimSpace(v)))
	if l != "" && !l.valid() {
		return "", fmt.Errorf("%q is not one of low, med, high", v)
	}
	return l, nil
}

func (l Level) valid() bool {
	for _, known := range Levels {
		if l == known {
			return true
		}
	}
	return false
}

// Snooze is one of the offered ways to hide a Task for a while.
type Snooze struct {
	Label string
	Until func(time.Time) time.Time
}

// SnoozeDefaults are the four the operator asked for. A month is a calendar
// month rather than 30 days, which is why these are functions of a time and
// not durations.
var SnoozeDefaults = []Snooze{
	{"1 hour", func(t time.Time) time.Time { return t.Add(time.Hour) }},
	{"1 day", func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }},
	{"1 week", func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }},
	{"1 month", func(t time.Time) time.Time { return addMonth(t) }},
}

// addMonth moves to the same day of the next month, clamped to that month's
// last day. AddDate on its own normalises 31 January plus a month to 3 March,
// which skips February altogether and is not what "snooze for a month" means.
func addMonth(t time.Time) time.Time {
	year, month, day := t.Date()
	firstOfNext := time.Date(year, month+1, 1, 0, 0, 0, 0, t.Location())
	lastOfNext := firstOfNext.AddDate(0, 1, -1).Day()
	if day > lastOfNext {
		day = lastOfNext
	}
	return time.Date(firstOfNext.Year(), firstOfNext.Month(), day,
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// Attributes is a partial edit to a Task. A nil field is left alone; a field
// pointing at its zero value clears the attribute. That distinction is carried
// all the way into the payload, where an absent key means untouched and a JSON
// null means cleared, so the fold can tell them apart too.
//
// The Change History stores the payload without reading inside it. Reading
// inside it is the fold's job, which is why a new attribute here changes
// Tracking alone.
type Attributes struct {
	Title        *string
	Description  *string
	Why          *string
	Deadline     *time.Time
	Estimate     *time.Duration
	Priority     *Level
	Impact       *Level
	SnoozedUntil *time.Time
	Colour       *string

	// Fields are the any-number-of key/value pairs. A key mapped to the empty
	// string removes it; keys absent from the map are left alone.
	Fields map[string]string
}

// payload turns an edit into what the Change History carries.
func (a Attributes) payload(requireTitle bool) (map[string]any, error) {
	p := map[string]any{}

	if a.Title != nil {
		if *a.Title == "" {
			return nil, fmt.Errorf("a task needs a title")
		}
		p["title"] = *a.Title
	} else if requireTitle {
		return nil, fmt.Errorf("a task needs a title")
	}

	putText(p, "description", a.Description)
	putText(p, "why", a.Why)
	putText(p, "colour", a.Colour)
	putTime(p, "deadline", a.Deadline)
	putTime(p, "snoozed_until", a.SnoozedUntil)

	if a.Estimate != nil {
		if *a.Estimate == 0 {
			p["estimate_seconds"] = nil
		} else {
			p["estimate_seconds"] = int64(a.Estimate.Seconds())
		}
	}
	if err := putLevel(p, "priority", a.Priority); err != nil {
		return nil, err
	}
	if err := putLevel(p, "impact", a.Impact); err != nil {
		return nil, err
	}
	if len(a.Fields) > 0 {
		p["fields"] = a.Fields
	}
	return p, nil
}

func putText(p map[string]any, key string, v *string) {
	if v == nil {
		return
	}
	if *v == "" {
		p[key] = nil
		return
	}
	p[key] = *v
}

func putTime(p map[string]any, key string, v *time.Time) {
	if v == nil {
		return
	}
	if v.IsZero() {
		p[key] = nil
		return
	}
	p[key] = v.UTC().Format(stamp)
}

func putLevel(p map[string]any, key string, v *Level) error {
	if v == nil {
		return nil
	}
	if *v == "" {
		p[key] = nil
		return nil
	}
	if !v.valid() {
		return fmt.Errorf("%s %q is not one of low, med, high", key, *v)
	}
	p[key] = string(*v)
	return nil
}

// text reads an attribute the fold stored as nullable TEXT.
func text(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func parseStamp(v *string) time.Time {
	if v == nil {
		return time.Time{}
	}
	t, _ := time.Parse(stamp, *v)
	return t
}

func decodeFields(raw string) map[string]string {
	fields := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &fields)
	return fields
}
