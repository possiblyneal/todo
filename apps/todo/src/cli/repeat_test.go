package cli

import (
	"strings"
	"testing"
	"time"
)

// `todo repeat` is the whole of Scheduling from a keyboard: set the rule, see
// the dates, act on one of them.
func TestRepeatSetsShowsAndMarks(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")

	_, out, errs := run(t, "add", "Water the plants")
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatalf("todo add printed no id: %s", errs)
	}

	if code, _, errs := run(t, "repeat", id, "every", "week", "on", "mon"); code != 0 {
		t.Fatalf("todo repeat exited %d: %s", code, errs)
	}

	code, out, errs := run(t, "repeat", "-n", "3", id)
	if code != 0 {
		t.Fatalf("todo repeat exited %d: %s", code, errs)
	}
	if !strings.HasPrefix(out, "every week on mon") {
		t.Errorf("todo repeat printed %q, want the rule first", out)
	}
	dates := strings.Split(strings.TrimSpace(out), "\n")[1:]
	if len(dates) != 3 {
		t.Fatalf("todo repeat -n 3 printed %d dates: %q", len(dates), out)
	}
	first, err := time.Parse("2006-01-02", dates[0])
	if err != nil {
		t.Fatalf("cannot read %q as a date: %v", dates[0], err)
	}
	if first.Weekday() != time.Monday {
		t.Errorf("first date %s is a %s, want a Monday", dates[0], first.Weekday())
	}
	// Ticking one date marks that date and nothing else.
	if code, _, errs := run(t, "repeat", "-tick", dates[0], id); code != 0 {
		t.Fatalf("todo repeat -tick exited %d: %s", code, errs)
	}
	_, out, _ = run(t, "repeat", "-n", "2", id)
	if !strings.Contains(out, dates[0]+"  (ticked)") {
		t.Errorf("todo repeat printed %q, want %s ticked", out, dates[0])
	}
	if strings.Contains(out, dates[1]+"  (") {
		t.Errorf("todo repeat printed %q, want %s left alone", out, dates[1])
	}

	// Detaching prints the ordinary Task the date became and stops producing
	// that date.
	code, out, errs = run(t, "repeat", "-detach", dates[1], id)
	if code != 0 {
		t.Fatalf("todo repeat -detach exited %d: %s", code, errs)
	}
	detached := strings.TrimSpace(out)
	_, out, _ = run(t, "list")
	if !strings.Contains(out, detached) {
		t.Errorf("todo list printed %q, want the detached task %s", out, detached)
	}
	_, out, _ = run(t, "repeat", "-n", "3", id)
	if strings.Contains(out, dates[1]) {
		t.Errorf("todo repeat still produces %s after detaching it: %q", dates[1], out)
	}

	if code, out, errs := run(t, "repeat", "-off", id); code != 0 {
		t.Fatalf("todo repeat -off exited %d: %s %s", code, out, errs)
	}
	_, out, _ = run(t, "repeat", id)
	if strings.TrimSpace(out) != "does not repeat" {
		t.Errorf("after -off, todo repeat printed %q", out)
	}
}

// A rule the parser cannot read is refused where it was typed, exit 1 rather
// than a crash, and nothing is written.
func TestRepeatRefusesNonsense(t *testing.T) {
	storeInTemp(t)
	t.Setenv("TODO_ACTOR", "alice")

	_, out, _ := run(t, "add", "Water the plants")
	id := strings.TrimSpace(out)

	code, _, errs := run(t, "repeat", id, "every", "blue", "moon")
	if code == 0 {
		t.Fatalf("todo repeat took a nonsense rule")
	}
	if !strings.Contains(errs, "todo repeat:") {
		t.Errorf("stderr = %q, want a sentence naming the verb", errs)
	}
	_, out, _ = run(t, "repeat", id)
	if strings.TrimSpace(out) != "does not repeat" {
		t.Errorf("after a refused rule, todo repeat printed %q", out)
	}
}

// A date is local. schedule reads a date's year, month and day in the local
// zone, so handing it a UTC instant hands it the UTC calendar date: west of
// Greenwich after evening that is tomorrow, and today's Occurrence falls off
// the list of what comes next.
//
// The rule is anchored years back, so the first date it produces from here is
// today whatever the zone, and a window opened on the UTC date shows yesterday
// or tomorrow instead. The two zones are what make that hold at any hour: at
// every instant one of them has a calendar date that is not the UTC one.
func TestRepeatShowsTodayInTheLocalZone(t *testing.T) {
	for _, zone := range []*time.Location{
		time.FixedZone("west", -11*60*60),
		time.FixedZone("east", +14*60*60),
	} {
		t.Run(zone.String(), func(t *testing.T) {
			local(t, zone)
			storeInTemp(t)
			t.Setenv("TODO_ACTOR", "alice")

			_, out, errs := run(t, "add", "Water the plants")
			id := strings.TrimSpace(out)
			if id == "" {
				t.Fatalf("todo add printed no id: %s", errs)
			}
			if code, _, errs := run(t, "repeat", id, "daily", "from", "2020-01-01"); code != 0 {
				t.Fatalf("todo repeat exited %d: %s", code, errs)
			}

			code, out, errs := run(t, "repeat", "-n", "1", id)
			if code != 0 {
				t.Fatalf("todo repeat exited %d: %s", code, errs)
			}
			dates := strings.Split(strings.TrimSpace(out), "\n")[1:]
			if len(dates) != 1 {
				t.Fatalf("todo repeat -n 1 printed %d dates: %q", len(dates), out)
			}
			if want := time.Now().In(zone).Format("2006-01-02"); dates[0] != want {
				t.Errorf("the next date is %s, want today in this zone, %s", dates[0], want)
			}
		})
	}
}

// local holds the zone for one test. time.Local is what schedule reads a date
// in, and a test that needs a particular zone cannot get one from TZ: the
// package caches it long before the test runs.
func local(t *testing.T, zone *time.Location) {
	t.Helper()
	was := time.Local
	time.Local = zone
	t.Cleanup(func() { time.Local = was })
}
