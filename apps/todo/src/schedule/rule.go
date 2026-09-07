// Package schedule is Scheduling: rules and dates, and nothing else.
//
// Nothing in this package runs on a schedule. There is no timer, no sweeper
// and no background pass; the name says otherwise and the name lost that
// argument. An Occurrence is worked out from its Series whenever something
// looks, so the only thing that ever happens here is a function being called
// by a reader that is already awake.
//
// No task content lives here either. A Series is a rule and a date, never a
// title, a Tag, a priority or an Attachment: those stay in Tracking, which is
// what keeps Scheduling from having to understand a Task at all.
package schedule

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Unit is what a Series repeats by.
type Unit string

const (
	Daily   Unit = "day"
	Weekly  Unit = "week"
	Monthly Unit = "month"
)

// Rule is a Series: the whole recurrence, edited as one thing. It is stored as
// the text String returns, so a Series is one value and not a row of columns
// that can disagree with each other.
type Rule struct {
	// Every is the interval, so 2 with Weekly is every second week.
	Every int
	Unit  Unit

	// Weekdays are the days a weekly rule lands on. Empty means the day the
	// rule is anchored to.
	Weekdays []time.Weekday

	// Anchor is the first date the rule can produce, and for a monthly rule
	// it is also the day of the month, clamped to short months.
	Anchor time.Time

	// Until is the last date the rule can produce. Zero means it does not
	// stop.
	Until time.Time
}

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
	"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday,
	"sat": time.Saturday,
}

var weekdayText = [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

const dateLayout = "2006-01-02"

// Parse reads a rule as a person writes it: "every week on mon,thu from
// 2026-01-05", "daily", "every 3 months on 15 until 2026-12-01".
func Parse(text string) (Rule, error) {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(words) == 0 {
		return Rule{}, fmt.Errorf("a series needs a rule, like %q", "every week on mon")
	}

	r := Rule{Every: 1}
	rest, err := parseInterval(&r, words)
	if err != nil {
		return Rule{}, err
	}

	for len(rest) > 0 {
		switch rest[0] {
		case "on":
			if len(rest) < 2 {
				return Rule{}, fmt.Errorf("%q needs a day after it", "on")
			}
			if err := parseOn(&r, rest[1]); err != nil {
				return Rule{}, err
			}
			rest = rest[2:]
		case "from", "until":
			if len(rest) < 2 {
				return Rule{}, fmt.Errorf("%q needs a date after it", rest[0])
			}
			when, err := time.ParseInLocation(dateLayout, rest[1], time.Local)
			if err != nil {
				return Rule{}, fmt.Errorf("%q is not a date; try 2006-01-02", rest[1])
			}
			if rest[0] == "from" {
				r.Anchor = when
			} else {
				r.Until = when
			}
			rest = rest[2:]
		default:
			return Rule{}, fmt.Errorf("%q is not part of a rule", rest[0])
		}
	}

	if r.Anchor.IsZero() {
		r.Anchor = today()
	}
	if !r.Until.IsZero() && r.Until.Before(r.Anchor) {
		return Rule{}, fmt.Errorf("a rule that ends before it starts produces nothing")
	}
	return r, nil
}

// parseInterval reads the "every N units" or "daily" head of a rule and
// returns what is left.
func parseInterval(r *Rule, words []string) ([]string, error) {
	switch words[0] {
	case "daily":
		r.Unit = Daily
		return words[1:], nil
	case "weekly":
		r.Unit = Weekly
		return words[1:], nil
	case "monthly":
		r.Unit = Monthly
		return words[1:], nil
	case "every":
	default:
		return nil, fmt.Errorf("a rule starts with %q, %q, %q or %q", "every", "daily", "weekly", "monthly")
	}

	rest := words[1:]
	if len(rest) == 0 {
		return nil, fmt.Errorf("%q needs something to repeat by", "every")
	}
	if n, err := strconv.Atoi(rest[0]); err == nil {
		if n < 1 {
			return nil, fmt.Errorf("a rule repeats at least every 1")
		}
		r.Every, rest = n, rest[1:]
	}
	if len(rest) == 0 {
		return nil, fmt.Errorf("%q needs a day, week or month", "every")
	}

	switch strings.TrimSuffix(rest[0], "s") {
	case "day":
		r.Unit = Daily
	case "week":
		r.Unit = Weekly
	case "month":
		r.Unit = Monthly
	default:
		return nil, fmt.Errorf("%q is not a day, a week or a month", rest[0])
	}
	return rest[1:], nil
}

// parseOn reads the days a weekly rule lands on, or the day of the month a
// monthly one does.
func parseOn(r *Rule, text string) error {
	if r.Unit == Monthly {
		day, err := strconv.Atoi(text)
		if err != nil || day < 1 || day > 31 {
			return fmt.Errorf("%q is not a day of the month", text)
		}
		r.Anchor = time.Date(anchorYear(r), anchorMonth(r), day, 0, 0, 0, 0, time.Local)
		return nil
	}
	for _, name := range strings.Split(text, ",") {
		day, ok := weekdayNames[strings.TrimSpace(name)]
		if !ok {
			return fmt.Errorf("%q is not a day of the week", name)
		}
		r.Weekdays = append(r.Weekdays, day)
	}
	if r.Unit == Daily {
		r.Unit = Weekly
	}
	return nil
}

func anchorYear(r *Rule) int {
	if r.Anchor.IsZero() {
		return today().Year()
	}
	return r.Anchor.Year()
}

func anchorMonth(r *Rule) time.Month {
	if r.Anchor.IsZero() {
		return today().Month()
	}
	return r.Anchor.Month()
}

// String writes the rule back out in the form Parse reads, which is how a
// Series survives a round trip through the Change History.
func (r Rule) String() string {
	// "every week", not "every 1 week": the interval is dropped when it is
	// one, which is the form a person types and the form Parse reads back.
	text := "every " + string(r.Unit)
	if r.Every != 1 {
		text = fmt.Sprintf("every %d %ss", r.Every, r.Unit)
	}

	switch {
	case r.Unit == Weekly && len(r.Weekdays) > 0:
		names := make([]string, len(r.Weekdays))
		for i, day := range r.Weekdays {
			names[i] = weekdayText[day]
		}
		text += " on " + strings.Join(names, ",")
	case r.Unit == Monthly:
		text += fmt.Sprintf(" on %d", r.Anchor.Day())
	}

	text += " from " + r.Anchor.Format(dateLayout)
	if !r.Until.IsZero() {
		text += " until " + r.Until.Format(dateLayout)
	}
	return text
}

// Between is every date the rule produces in [from, to], both ends included.
// It computes; it stores nothing, and a reader calling it writes nothing.
func (r Rule) Between(from, to time.Time) []time.Time {
	from, to = day(from), day(to)
	if !r.Until.IsZero() && r.Until.Before(to) {
		to = day(r.Until)
	}
	if to.Before(from) || to.Before(r.Anchor) {
		return nil
	}

	var dates []time.Time
	for _, date := range r.walk(to) {
		if !date.Before(from) {
			dates = append(dates, date)
		}
	}
	return dates
}

// walk produces every date from the anchor up to and including to. The rule is
// counted from the anchor rather than from the window, so asking about next
// month gives the same dates as asking about the whole year.
func (r Rule) walk(to time.Time) []time.Time {
	anchor := day(r.Anchor)
	var dates []time.Time

	switch r.Unit {
	case Daily:
		for date := anchor; !date.After(to); date = date.AddDate(0, 0, r.Every) {
			dates = append(dates, date)
		}

	case Weekly:
		weekdays := r.Weekdays
		if len(weekdays) == 0 {
			weekdays = []time.Weekday{anchor.Weekday()}
		}
		// Weeks are counted from the week the anchor falls in, so a rule
		// landing on more than one weekday keeps them in the same week.
		start := anchor.AddDate(0, 0, -int(anchor.Weekday()))
		for week := start; !week.After(to); week = week.AddDate(0, 0, 7*r.Every) {
			for _, weekday := range weekdays {
				date := week.AddDate(0, 0, int(weekday))
				if !date.Before(anchor) && !date.After(to) {
					dates = append(dates, date)
				}
			}
		}

	case Monthly:
		for i := 0; ; i++ {
			date := monthly(anchor, i*r.Every)
			if date.After(to) {
				break
			}
			dates = append(dates, date)
		}
	}

	sortDates(dates)
	return dates
}

// monthly moves the anchor on by months, clamping to the target month's last
// day. The 31st of a month is the last of February, not the 3rd of March.
func monthly(anchor time.Time, months int) time.Time {
	first := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location()).
		AddDate(0, months, 0)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(anchor.Day(), last), 0, 0, 0, 0, anchor.Location())
}

// Next is the first date the rule produces on or after from, or the zero time
// if the rule has run out.
//
// The window is two of the rule's own steps rather than a fixed year, because
// a fixed year cannot see past a rule whose step is longer than one: "every 60
// weeks" produces nothing inside 366 days and would read as run out.
func (r Rule) Next(from time.Time) time.Time {
	every := max(r.Every, 1)
	var to time.Time
	switch r.Unit {
	case Monthly:
		to = day(from).AddDate(0, 2*every+1, 0)
	case Weekly:
		to = day(from).AddDate(0, 0, 14*every+7)
	default:
		to = day(from).AddDate(0, 0, 2*every+1)
	}
	dates := r.Between(from, to)
	if len(dates) == 0 {
		return time.Time{}
	}
	return dates[0]
}

// Produces says whether the rule lands on a date, which is what tells a marker
// for a date the rule no longer produces from one it still does.
func (r Rule) Produces(date time.Time) bool {
	date = day(date)
	return len(r.Between(date, date)) == 1
}

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func today() time.Time { return day(time.Now()) }

func sortDates(dates []time.Time) {
	slices.SortFunc(dates, func(a, b time.Time) int { return a.Compare(b) })
}
