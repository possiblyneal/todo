package schedule

import (
	"testing"
	"time"
)

func date(t *testing.T, text string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation(dateLayout, text, time.Local)
	if err != nil {
		t.Fatalf("date %q: %v", text, err)
	}
	return d
}

func dates(t *testing.T, got []time.Time) []string {
	t.Helper()
	out := make([]string, len(got))
	for i, d := range got {
		out[i] = d.Format(dateLayout)
	}
	return out
}

func same(t *testing.T, got []time.Time, want ...string) {
	t.Helper()
	as := dates(t, got)
	if len(as) != len(want) {
		t.Fatalf("got %v, want %v", as, want)
	}
	for i := range want {
		if as[i] != want[i] {
			t.Fatalf("got %v, want %v", as, want)
		}
	}
}

func rule(t *testing.T, text string) Rule {
	t.Helper()
	r, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return r
}

func TestARuleReadsTheWayItIsWritten(t *testing.T) {
	r := rule(t, "every 2 weeks on mon,thu from 2026-01-05 until 2026-03-01")
	if r.Every != 2 || r.Unit != Weekly {
		t.Errorf("got every %d %s, want every 2 weeks", r.Every, r.Unit)
	}
	if len(r.Weekdays) != 2 || r.Weekdays[0] != time.Monday || r.Weekdays[1] != time.Thursday {
		t.Errorf("got weekdays %v, want mon and thu", r.Weekdays)
	}

	// A Series is one value, so it has to survive the round trip through the
	// text the Change History carries.
	again := rule(t, r.String())
	if again.String() != r.String() {
		t.Errorf("round trip gave %q, want %q", again.String(), r.String())
	}
}

func TestARuleThatMakesNoSenseIsRefused(t *testing.T) {
	for _, bad := range []string{
		"", "sometimes", "every", "every 0 days", "every 3 fortnights",
		"weekly on caturday", "monthly on 41", "daily from yesterday",
		"daily from 2026-03-01 until 2026-01-01",
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q was accepted as a rule", bad)
		}
	}
}

func TestDatesAreCountedFromTheAnchorNotTheWindow(t *testing.T) {
	r := rule(t, "every 3 days from 2026-01-01")

	// Asking about a later window gives the same dates as asking about the
	// whole span, which is what makes a read of next month cheap and
	// consistent.
	same(t, r.Between(date(t, "2026-01-08"), date(t, "2026-01-14")),
		"2026-01-10", "2026-01-13")
}

func TestAWeeklyRuleLandsOnEveryDayItNames(t *testing.T) {
	r := rule(t, "every 2 weeks on mon,thu from 2026-01-05")
	same(t, r.Between(date(t, "2026-01-01"), date(t, "2026-02-01")),
		"2026-01-05", "2026-01-08", "2026-01-19", "2026-01-22")
}

func TestAMonthlyRuleClampsToShortMonths(t *testing.T) {
	r := rule(t, "monthly on 31 from 2026-01-31")
	same(t, r.Between(date(t, "2026-01-01"), date(t, "2026-04-30")),
		"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30")
}

func TestARuleStopsWhenItSaysItDoes(t *testing.T) {
	r := rule(t, "daily from 2026-01-01 until 2026-01-03")
	same(t, r.Between(date(t, "2026-01-01"), date(t, "2026-12-31")),
		"2026-01-01", "2026-01-02", "2026-01-03")

	if next := r.Next(date(t, "2026-01-04")); !next.IsZero() {
		t.Errorf("a finished rule offered %v, want nothing", next)
	}
}

func TestNextIsTheFirstDateFromHere(t *testing.T) {
	r := rule(t, "every week on fri from 2026-01-01")
	if got := r.Next(date(t, "2026-01-05")).Format(dateLayout); got != "2026-01-09" {
		t.Errorf("next after 2026-01-05 was %s, want 2026-01-09", got)
	}
	if !r.Produces(date(t, "2026-01-09")) {
		t.Error("the rule does not admit to a date it produces")
	}
	if r.Produces(date(t, "2026-01-10")) {
		t.Error("the rule claims a date it does not produce")
	}
}

// TestNothingHereRunsOnASchedule is the guard the context name needs. The
// package computes when asked and holds no state of its own, so two calls a
// year apart on the same rule give the same answer.
func TestNothingHereRunsOnASchedule(t *testing.T) {
	r := rule(t, "every week on wed from 2026-01-01")
	first := dates(t, r.Between(date(t, "2026-02-01"), date(t, "2026-03-01")))
	second := dates(t, r.Between(date(t, "2026-02-01"), date(t, "2026-03-01")))
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("two identical reads differed: %v then %v", first, second)
		}
	}
}

// TestNextSeesPastALongStep guards the window Next searches. A fixed year
// could not see a rule whose step is longer than one, so "every 60 weeks"
// read as a rule that had run out.
func TestNextSeesPastALongStep(t *testing.T) {
	from := time.Date(2026, 1, 5, 0, 0, 0, 0, time.Local)
	for _, r := range []Rule{
		{Every: 60, Unit: Weekly, Anchor: from, Weekdays: []time.Weekday{time.Monday}},
		{Every: 500, Unit: Daily, Anchor: from},
		{Every: 30, Unit: Monthly, Anchor: from},
	} {
		next := r.Next(from)
		if next.IsZero() {
			t.Errorf("%d %s read as a rule that had run out", r.Every, r.Unit)
			continue
		}
		if next.Before(from) {
			t.Errorf("%d %s came back with %v, which is before %v", r.Every, r.Unit, next, from)
		}
	}
}
