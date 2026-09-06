package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func repeating(t *testing.T, s *Store, rule string) string {
	t.Helper()
	id := leased(t, s, "alice", Attributes{Title: Set("Water the plants")})
	if _, err := s.Repeat("alice", id, rule); err != nil {
		t.Fatalf("Repeat: %v", err)
	}
	return id
}

func on(t *testing.T, date string) time.Time {
	t.Helper()
	when, err := time.ParseInLocation(dateOnly, date, time.UTC)
	if err != nil {
		t.Fatalf("parse %q: %v", date, err)
	}
	return when
}

func dates(occurrences []Occurrence) []string {
	out := make([]string, len(occurrences))
	for i, o := range occurrences {
		out[i] = o.Date.Format(dateOnly)
	}
	return out
}

// A Series is one rule edited as one thing. Changing it moves every date it
// produces, in one append.
func TestASeriesIsOneRuleEditedAsOneThing(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every week on mon from 2026-01-05")

	weekly, err := s.Occurrences(id, on(t, "2026-01-01"), on(t, "2026-01-31"))
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if got := strings.Join(dates(weekly), " "); got != "2026-01-05 2026-01-12 2026-01-19 2026-01-26" {
		t.Fatalf("weekly = %s", got)
	}

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if _, err := s.Repeat("alice", id, "every 2 weeks on mon from 2026-01-05"); err != nil {
		t.Fatalf("Repeat again: %v", err)
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after-before != 1 {
		t.Errorf("editing the rule appended %d entries, want 1", after-before)
	}

	fortnightly, err := s.Occurrences(id, on(t, "2026-01-01"), on(t, "2026-01-31"))
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if got := strings.Join(dates(fortnightly), " "); got != "2026-01-05 2026-01-19" {
		t.Errorf("after the edit = %s, want the fortnightly dates", got)
	}
}

// An Occurrence is computed on every look and never persisted ahead of time. A
// reader that shows next year's dates writes nothing at all.
func TestReadingOccurrencesWritesNothing(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every day from 2026-01-01")

	before, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	for range 3 {
		occurrences, err := s.Occurrences(id, on(t, "2027-01-01"), on(t, "2027-12-31"))
		if err != nil {
			t.Fatalf("Occurrences: %v", err)
		}
		if len(occurrences) != 365 {
			t.Fatalf("a year of daily occurrences = %d, want 365", len(occurrences))
		}
	}
	after, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	if after != before {
		t.Errorf("reading a year of dates appended %d entries, want 0", after-before)
	}

	var rows int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM occurrence`).Scan(&rows); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if rows != 0 {
		t.Errorf("%d occurrence rows stored, want 0 until someone acts on a date", rows)
	}
}

// Stored state exists only for a date somebody acted on, keyed by the pair
// (series, date).
func TestOnlyAnActedOnDateIsStored(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every day from 2026-01-01")

	if err := s.TickOccurrence("alice", id, on(t, "2026-01-02")); err != nil {
		t.Fatalf("TickOccurrence: %v", err)
	}
	if err := s.SkipOccurrence("alice", id, on(t, "2026-01-03")); err != nil {
		t.Fatalf("SkipOccurrence: %v", err)
	}

	var rows int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM occurrence`).Scan(&rows); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if rows != 2 {
		t.Errorf("%d occurrence rows, want the 2 acted on", rows)
	}

	week, err := s.Occurrences(id, on(t, "2026-01-01"), on(t, "2026-01-05"))
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	want := []OccurrenceState{Pending, Ticked, Skipped, Pending, Pending}
	for i, o := range week {
		if o.State != want[i] {
			t.Errorf("%s = %q, want %q", o.Date.Format(dateOnly), o.State, want[i])
		}
	}
}

// Each mark is an ordinary attributed write by an Actor holding the Lease on
// the Task's tree, so one without it is refused like any other write.
func TestAMarkNeedsTheLeaseOnTheTask(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every day from 2026-01-01")

	if err := s.TickOccurrence("bob", id, on(t, "2026-01-02")); !errors.Is(err, ErrRefused) {
		t.Errorf("a tick without the Lease = %v, want ErrRefused", err)
	}
	if err := s.ReleaseLease("alice", id); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := s.TickOccurrence("alice", id, on(t, "2026-01-02")); !errors.Is(err, ErrRefused) {
		t.Errorf("a tick after giving the Lease back = %v, want ErrRefused", err)
	}

	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, e := range entries {
		if e.Kind == KindOccurrenceTicked {
			t.Fatalf("a refused tick was appended: %+v", e)
		}
	}
}

// A date the rule does not produce is not an Occurrence, and marking one is
// refused where it was asked for.
func TestADateTheRuleDoesNotProduceIsNotAnOccurrence(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every week on mon from 2026-01-05")

	err := s.TickOccurrence("alice", id, on(t, "2026-01-06"))
	if !errors.Is(err, ErrNotAnOccurrence) {
		t.Errorf("ticking a Tuesday = %v, want ErrNotAnOccurrence", err)
	}
}

// Editing one date Detaches it into an ordinary Task the rule no longer
// produces, and later edits to the Series do not reach it.
func TestDetachingLiftsOneDateOutOfTheSeries(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{
		Title:       Set("Water the plants"),
		Description: Set("The ones on the sill"),
		Estimate:    Set(10 * time.Minute),
		Priority:    Set(LevelMed),
	})
	list, err := s.AddList("alice", "Home", "")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := s.AddToList("alice", id, list); err != nil {
		t.Fatalf("AddToList: %v", err)
	}
	if _, err := s.Repeat("alice", id, "every week on mon from 2026-01-05"); err != nil {
		t.Fatalf("Repeat: %v", err)
	}

	detachedID, err := s.DetachOccurrence("alice", id, on(t, "2026-01-12"))
	if err != nil {
		t.Fatalf("DetachOccurrence: %v", err)
	}

	remaining, err := s.Occurrences(id, on(t, "2026-01-01"), on(t, "2026-01-31"))
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if got := strings.Join(dates(remaining), " "); got != "2026-01-05 2026-01-19 2026-01-26" {
		t.Errorf("after detaching = %s, want the 12th gone", got)
	}

	copied, err := s.taskByID(detachedID)
	if err != nil {
		t.Fatalf("read the detached task: %v", err)
	}
	switch {
	case copied.Title != "Water the plants":
		t.Errorf("title = %q", copied.Title)
	case copied.Description != "The ones on the sill":
		t.Errorf("description = %q", copied.Description)
	case copied.Estimate != 10*time.Minute:
		t.Errorf("estimate = %v", copied.Estimate)
	case copied.Priority != LevelMed:
		t.Errorf("priority = %q", copied.Priority)
	case copied.Deadline.Format(dateOnly) != "2026-01-12":
		t.Errorf("deadline = %v, want the date it was detached from", copied.Deadline)
	case copied.Series != "":
		t.Errorf("the detached task carries series %q, want an ordinary task", copied.Series)
	case len(copied.Lists) != 1 || copied.Lists[0] != list:
		t.Errorf("lists = %v, want the one the recurring task carried", copied.Lists)
	}

	// The rule moves; the date that left does not move with it.
	if _, err := s.Repeat("alice", id, "every week on fri from 2026-01-02"); err != nil {
		t.Fatalf("Repeat again: %v", err)
	}
	moved, err := s.taskByID(detachedID)
	if err != nil {
		t.Fatalf("read the detached task: %v", err)
	}
	if moved.Deadline.Format(dateOnly) != "2026-01-12" {
		t.Errorf("the detached task followed the rule to %v", moved.Deadline)
	}
}

// No task content enters Scheduling. Both its tables hold ids, a rule, a date
// and a state, and nothing a person wrote about the work.
func TestNoTaskContentEntersScheduling(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{
		Title:       Set("Water the plants"),
		Description: Set("The ones on the sill"),
		Why:         Set("They die otherwise"),
	})
	if _, err := s.Repeat("alice", id, "every week on mon from 2026-01-05"); err != nil {
		t.Fatalf("Repeat: %v", err)
	}
	if err := s.TickOccurrence("alice", id, on(t, "2026-01-05")); err != nil {
		t.Fatalf("TickOccurrence: %v", err)
	}
	if _, err := s.DetachOccurrence("alice", id, on(t, "2026-01-12")); err != nil {
		t.Fatalf("DetachOccurrence: %v", err)
	}

	for _, table := range []string{"series", "occurrence"} {
		rows, err := s.db.Query(`SELECT * FROM ` + table)
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatalf("columns of %s: %v", table, err)
		}
		for rows.Next() {
			cells := make([]any, len(columns))
			for i := range cells {
				cells[i] = new(any)
			}
			if err := rows.Scan(cells...); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			for i, cell := range cells {
				value, _ := (*cell.(*any)).(string)
				for _, content := range []string{"Water the plants", "The ones on the sill", "They die otherwise"} {
					if strings.Contains(value, content) {
						t.Errorf("%s.%s holds task content: %q", table, columns[i], value)
					}
				}
			}
		}
		_ = rows.Close()
	}
}

// A Task that does not repeat has no Series, which is not an error, and
// nothing in Scheduling to act on, which is.
func TestATaskThatDoesNotRepeat(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Buy milk")})

	if _, ok, err := s.Rule(id); err != nil || ok {
		t.Errorf("Rule = %v, %v, want no rule and no error", ok, err)
	}
	occurrences, err := s.Occurrences(id, on(t, "2026-01-01"), on(t, "2026-12-31"))
	if err != nil || occurrences != nil {
		t.Errorf("Occurrences = %v, %v, want none and no error", occurrences, err)
	}
	if err := s.TickOccurrence("alice", id, on(t, "2026-01-05")); !errors.Is(err, ErrNotRecurring) {
		t.Errorf("ticking a task that does not repeat = %v, want ErrNotRecurring", err)
	}
}

// Unrepeat takes the rule off the Task and leaves the record of it standing.
func TestUnrepeatLeavesTheRecord(t *testing.T) {
	s := openTemp(t)
	id := repeating(t, s, "every day from 2026-01-01")
	if err := s.TickOccurrence("alice", id, on(t, "2026-01-02")); err != nil {
		t.Fatalf("TickOccurrence: %v", err)
	}
	if err := s.Unrepeat("alice", id); err != nil {
		t.Fatalf("Unrepeat: %v", err)
	}

	if _, ok, err := s.Rule(id); err != nil || ok {
		t.Errorf("Rule after Unrepeat = %v, %v, want no rule", ok, err)
	}
	var marks int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM occurrence`).Scan(&marks); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if marks != 1 {
		t.Errorf("%d marks left after Unrepeat, want the 1 that was made", marks)
	}
}
