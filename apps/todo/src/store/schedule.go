package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/schedule"
)

// This file is the store's half of Scheduling. The rule arithmetic lives in
// src/schedule, which knows nothing about a database; here a rule is text in
// one column and the dates it produces are worked out on every look.
//
// Nothing in it runs on a schedule. There is no sweep that materializes next
// month's dates, because a date nobody has acted on has nothing worth storing:
// it is a function of the rule and the calendar, and both are already here.

// OccurrenceState is what somebody did to one date of a Series. The zero value
// is a date nobody has touched, which is every date until it is not.
type OccurrenceState string

const (
	Pending OccurrenceState = ""
	Ticked  OccurrenceState = "ticked"
	Skipped OccurrenceState = "skipped"

	// detached is not an OccurrenceState a reader ever sees. A Detached date
	// stops being an Occurrence: it became an ordinary Task, and the rule no
	// longer produces it.
	detached = "detached"
)

// Occurrence is one date of a Series, computed. It is not a stored row and not
// a Task: it carries no title, no description and no estimate, because no task
// content enters Scheduling.
type Occurrence struct {
	// Task is the recurring Task the Series belongs to; Series is the rule.
	Task   string
	Series string
	Date   time.Time
	State  OccurrenceState
}

// dateOnly is how a date is written in an Occurrence's key. A date has no time
// of day: a Series produces the 5th, not 09:00 on the 5th.
const dateOnly = time.DateOnly

// ErrNotRecurring is a Scheduling call against a Task that carries no Series.
var ErrNotRecurring = errors.New("this task does not repeat")

// ErrNotAnOccurrence is a mark against a date the rule does not produce.
var ErrNotAnOccurrence = errors.New("the series does not produce that date")

// Repeat gives taskID a recurrence rule, or replaces the rule it already has.
// A Series is one value edited as one thing, so changing "every week" to
// "every 2 weeks on mon,thu" is a single append however many dates it moves.
//
// Creating the Series and pointing the Task at it are one transaction. The
// pointer is a write to the Task's tree and takes the Lease covering it, and a
// refusal rolls the Series back with it rather than leaving one behind that
// nothing repeats on.
func (s *Store) Repeat(actor, taskID, rule string) (string, error) {
	parsed, err := schedule.Parse(rule)
	if err != nil {
		return "", err
	}
	seriesID, existing, err := s.seriesOf(taskID)
	if err != nil {
		return "", err
	}
	payload := map[string]any{"rule": parsed.String()}

	if existing {
		// The Task already repeats, so this edits the rule in place. The
		// entry's subject is the Series; the Lease it needs is the one on the
		// Task's tree, which is what guardOn says.
		_, err := s.inTx(func(tx *sql.Tx) (Entry, error) {
			return guardedAppend(tx, actor, KindSeriesEdited, seriesID, taskID, payload)
		})
		return seriesID, err
	}

	id, err := newID()
	if err != nil {
		return "", err
	}
	_, err = s.inTx(func(tx *sql.Tx) (Entry, error) {
		if _, err := appendTx(tx, actor, KindSeriesCreated, id, payload); err != nil {
			return Entry{}, err
		}
		return guardedAppend(tx, actor, KindTaskDescribed, taskID, taskID,
			map[string]any{"series": id})
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Unrepeat takes the rule off a Task. The Series and every mark left on its
// dates stay in the record: the Task stopped repeating, which is not the same
// as the repeating never having happened.
func (s *Store) Unrepeat(actor, taskID string) error {
	return s.guarded(actor, taskID, KindTaskDescribed, map[string]any{"series": nil})
}

// Rule is the rule a Task repeats on. The second result is false when it does
// not repeat, which is not an error: most Tasks do not.
func (s *Store) Rule(taskID string) (schedule.Rule, bool, error) {
	_, ok, err := s.seriesOf(taskID)
	if err != nil || !ok {
		return schedule.Rule{}, false, err
	}
	rule, err := s.ruleOf(taskID)
	return rule, err == nil, err
}

// Occurrences is the dates a Task's Series produces between from and to, with
// what anybody has done to each. It is a read: it computes every date from the
// rule and writes nothing, so a surface may ask for next year's dates as often
// as it likes.
//
// A Detached date is not in the result. It became an ordinary Task, and the
// rule stopped producing it.
func (s *Store) Occurrences(taskID string, from, to time.Time) ([]Occurrence, error) {
	seriesID, ok, err := s.seriesOf(taskID)
	if err != nil || !ok {
		return nil, err
	}
	rule, err := s.ruleOf(taskID)
	if err != nil {
		return nil, err
	}
	marks, err := s.marks(seriesID)
	if err != nil {
		return nil, err
	}

	var out []Occurrence
	for _, date := range rule.Between(from, to) {
		state := marks[date.Format(dateOnly)]
		if state == detached {
			continue
		}
		out = append(out, Occurrence{
			Task:   taskID,
			Series: seriesID,
			Date:   date,
			State:  OccurrenceState(state),
		})
	}
	return out, nil
}

// TickOccurrence and SkipOccurrence are the two marks that leave the Series
// alone. Each is an ordinary attributed write by an Actor holding the Lease on
// the Task's tree, keyed by the pair (series, date) and holding nothing else.
func (s *Store) TickOccurrence(actor, taskID string, on time.Time) error {
	return s.mark(actor, taskID, on, KindOccurrenceTicked, nil)
}

func (s *Store) SkipOccurrence(actor, taskID string, on time.Time) error {
	return s.mark(actor, taskID, on, KindOccurrenceSkipped, nil)
}

// DetachOccurrence lifts one date out of its Series into an ordinary Task and
// returns that Task's id. The rule no longer produces the date, and later
// edits to the rule do not reach the Task the date became: that is the whole
// point of detaching, and it is why editing one date is not an edit to the
// Series.
//
// The new Task is a copy of the recurring one's content, deadlined on that
// date and repeating on nothing. The content is copied within Tracking; the
// mark left behind in Scheduling names the date and the new Task's id, and no
// word of what the Task says crosses over.
func (s *Store) DetachOccurrence(actor, taskID string, on time.Time) (string, error) {
	source, err := s.taskByID(taskID)
	if err != nil {
		return "", err
	}
	copied := Detached(source, on)
	return s.detach(actor, taskID, on, detachedFrom(copied), copied.Lists, copied.Tags)
}

// DetachEdited lifts the date out and writes an edit on the Task it becomes,
// in the transaction the detach already opens. Editing one date is how a
// person says "this week's is different", and the two halves of that sentence
// cannot come apart: without this the edit is a second write, and one that
// fails leaves a detached Task saying what the recurring one said.
//
// The attributes are the whole of the copy rather than a change to it, because
// the surface that asks for this has shown the person every attribute the Task
// would have.
func (s *Store) DetachEdited(actor, taskID string, on time.Time, a Attributes, lists, tags []string) (string, error) {
	return s.detach(actor, taskID, on, a, lists, tags)
}

// detach is the write both of them make: the mark in Scheduling and the
// ordinary Task in Tracking, together or not at all.
func (s *Store) detach(actor, taskID string, on time.Time, a Attributes, lists, tags []string) (string, error) {
	attributes, err := a.payload(true)
	if err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(WriteTTL).Format(stamp)

	err = s.mark(actor, taskID, on, KindOccurrenceDetached, func(tx *sql.Tx) error {
		if _, err := appendTx(tx, actor, KindTaskAdded, id, attributes); err != nil {
			return err
		}
		// The copy's Lists and Tags are writes to the copy, so they need the
		// Lease covering it. It is taken and given back inside this one
		// transaction: nothing here waits on a person.
		if _, err := appendTx(tx, actor, KindLeaseTaken, id,
			map[string]any{"expires_at": expires}); err != nil {
			return err
		}
		for _, m := range []struct {
			kind string
			key  string
			ids  []string
		}{
			{KindTaskListed, "list", lists},
			{KindTagAttached, "tag", tags},
		} {
			for _, other := range m.ids {
				if _, err := guardedAppend(tx, actor, m.kind, id, id,
					map[string]any{m.key: other}); err != nil {
					return err
				}
			}
		}
		_, err := guardedAppend(tx, actor, KindLeaseReleased, id, id, map[string]any{})
		return err
	}, id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// Detached is the Task a date becomes when it is lifted out of its Series: the
// recurring Task's content, deadlined on that date, carrying its Lists and its
// Tags, and repeating on nothing. It carries no Attachments and no snooze,
// because a pointer and a snooze belong to the Task that holds them rather
// than to the content a date is copied from.
//
// It writes nothing. A surface opens an edit on what the date would become by
// asking here first, so what it shows and what a detach writes are the one
// statement of the shape.
func Detached(t Task, on time.Time) Task {
	t.ID, t.Parent, t.Depth, t.Series = "", "", 0, ""
	t.Deadline = day(on)
	t.CreatedAt, t.CompletedAt, t.DeletedAt = time.Time{}, time.Time{}, time.Time{}
	t.SnoozedUntil = time.Time{}
	t.Attachments = nil
	return t
}

// detachedFrom is the copy as the append writes it down.
func detachedFrom(t Task) Attributes {
	a := Attributes{
		Title:       Set(t.Title),
		Description: Set(t.Description),
		Why:         Set(t.Why),
		Deadline:    Set(t.Deadline),
		Estimate:    Set(t.Estimate),
		Priority:    Set(t.Priority),
		Impact:      Set(t.Impact),
		Colour:      Set(t.Colour),
	}
	if len(t.Fields) > 0 {
		a.Fields = t.Fields
	}
	return a
}

// mark appends one of the three Occurrence entries. The subject is the Task,
// so the guard finds the tree the Lease covers; the payload names the Series
// and the date, and for a Detached date the Task it became.
//
// also runs inside the same transaction, which is how detaching writes the
// ordinary Task and the mark together or not at all.
func (s *Store) mark(actor, taskID string, on time.Time, kind string, also func(*sql.Tx) error, task ...string) error {
	seriesID, ok, err := s.seriesOf(taskID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotRecurring
	}
	rule, err := s.ruleOf(taskID)
	if err != nil {
		return err
	}
	date := day(on)
	if !rule.Produces(date) {
		return fmt.Errorf("%w: %s", ErrNotAnOccurrence, date.Format(dateOnly))
	}
	marks, err := s.marks(seriesID)
	if err != nil {
		return err
	}
	if marks[date.Format(dateOnly)] == detached {
		return fmt.Errorf("%s is already an ordinary task", date.Format(dateOnly))
	}

	payload := map[string]any{"series": seriesID, "date": date.Format(dateOnly)}
	if len(task) > 0 {
		payload["task"] = task[0]
	}
	_, err = s.inTx(func(tx *sql.Tx) (Entry, error) {
		e, err := guardedAppend(tx, actor, kind, taskID, taskID, payload)
		if err != nil {
			return Entry{}, err
		}
		if also != nil {
			if err := also(tx); err != nil {
				return Entry{}, err
			}
		}
		return e, nil
	})
	return err
}

// seriesOf reads the Series a Task points at.
func (s *Store) seriesOf(taskID string) (string, bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT COALESCE(series_id, '') FROM task WHERE id = ?`, taskID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("no such task: %s", taskID)
	}
	if err != nil {
		return "", false, fmt.Errorf("read the series of %s: %w", taskID, err)
	}
	return id, id != "", nil
}

// ruleOf parses the rule a Task's Series holds. It is stored as the text a
// person typed, canonicalised, so the round trip through the column is the
// same rule and not an interpretation of one.
func (s *Store) ruleOf(taskID string) (schedule.Rule, error) {
	var text string
	err := s.db.QueryRow(`
SELECT sr.rule FROM task t JOIN series sr ON sr.id = t.series_id WHERE t.id = ?`, taskID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Rule{}, ErrNotRecurring
	}
	if err != nil {
		return schedule.Rule{}, fmt.Errorf("read the rule of %s: %w", taskID, err)
	}
	return schedule.Parse(text)
}

// marks reads every date of a Series somebody has acted on. There is one row
// per acted-on date and no rows at all for the rest, however far the rule runs.
func (s *Store) marks(seriesID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT date, state FROM occurrence WHERE series_id = ?`, seriesID)
	if err != nil {
		return nil, fmt.Errorf("read the marks on %s: %w", seriesID, err)
	}
	defer func() { _ = rows.Close() }()

	marks := map[string]string{}
	for rows.Next() {
		var date, state string
		if err := rows.Scan(&date, &state); err != nil {
			return nil, fmt.Errorf("read a mark: %w", err)
		}
		marks[date] = state
	}
	return marks, rows.Err()
}

// taskByID reads one Task through the same read every surface uses, so a copy
// made here sees exactly what a person sees.
func (s *Store) taskByID(id string) (Task, error) {
	tasks, err := s.Tasks(Query{IncludeCompleted: true, IncludeSnoozed: true, IncludeDeleted: true})
	if err != nil {
		return Task{}, err
	}
	for _, t := range tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return Task{}, fmt.Errorf("no such task: %s", id)
}

// day drops the time of day. A Series produces dates, not appointments.
//
// The date is built in the local zone, which is the zone schedule computes an
// Occurrence in and the zone every surface writes a deadline in. Building it
// in UTC gave a detached Task a deadline an hour or five off every other one,
// for the same day.
func day(t time.Time) time.Time {
	year, month, date := t.Date()
	return time.Date(year, month, date, 0, 0, 0, 0, time.Local)
}
