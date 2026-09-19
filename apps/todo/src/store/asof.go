package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrAbsent is a Task asked for at a position in the Change History where it
// was not there: before it was added, or before there was any log at all. It
// is not a refusal -- nothing was turned away -- so it is not in the list
// Refused reads, and a surface answers it the way it answers a question about
// something that does not exist.
var ErrAbsent = errors.New("not found: no Task there at that position")

// TaskAsOf is the Task as it stood when one entry was appended: the Change
// History up to and including that position, folded the way it was folded the
// first time.
//
// The folding is not repeated here. Every column on a Task is written by a
// trigger on change_history, and two of those triggers are rules rather than
// transcriptions -- reopening clears every ended Task above the subject, and
// leaves no entry per ancestor it cleared, so the ancestors' state comes back
// only by running that rule again. A replay written in Go would be a second
// statement of it, and a kind added to the store would fold correctly on a
// write and wrongly on a read until somebody noticed.
//
// So the entries are replayed into a database carrying this package's own
// schema, the same triggers fire in the same order, and the Task is read out of
// it by the ordinary read path. A kind added to the store replays the day it is
// appended and nothing here is touched.
//
// The Marks are worked out as of now rather than as of the entry, because a
// mark is a read's conclusion and not a stored fact: a Task with a deadline
// last month reads as overdue whichever position it is read at.
func (s *Store) TaskAsOf(taskID string, seq int64) (Task, error) {
	entries, err := s.entries(
		`SELECT seq, at, actor, kind, subject, payload FROM change_history WHERE seq <= ? ORDER BY seq`,
		seq,
	)
	if err != nil {
		return Task{}, err
	}
	if len(entries) == 0 {
		return Task{}, fmt.Errorf("%w: the Change History holds nothing at or before %d", ErrAbsent, seq)
	}
	// A position is an entry's own seq, not a number the log happens to be
	// shorter than. Past the end, the read above returns the whole log and the
	// Task would be answered as it stands now under a route that says it is
	// answering as it stood then, which is the one wrong answer this cannot
	// give.
	if last := entries[len(entries)-1].Seq; last != seq {
		return Task{}, fmt.Errorf("%w: the Change History has no entry %d", ErrAbsent, seq)
	}

	replica, err := replay(entries)
	if err != nil {
		return Task{}, err
	}
	defer replica.Close()

	// Every narrowing is off: the point of this read is the Task that a
	// narrowing would hide, and a deleted one is the usual reason to ask.
	tasks, err := replica.Tasks(Query{
		IncludeDeleted:   true,
		IncludeCompleted: true,
		IncludeDeclined:  true,
		IncludeSnoozed:   true,
	})
	if err != nil {
		return Task{}, err
	}
	for _, t := range tasks {
		if t.ID == taskID {
			return t, nil
		}
	}
	return Task{}, fmt.Errorf("%w: no Task %s at position %d", ErrAbsent, taskID, seq)
}

// replay builds a store holding nothing but these entries. It is in memory and
// it is thrown away by the caller: it exists for the length of one read.
//
// One connection, not a pool. Each connection to an anonymous in-memory
// database gets a database of its own, so a pool would insert the entries down
// one and read an empty schema down the next. It is also why the replica is
// read inside the call that filled it and thrown away at the end of it: a
// connection database/sql retires and redials is a fresh empty database, so
// there is nothing here worth keeping past the read it was built for.
//
// The DSN is the store's own rather than a second set of settings written out
// beside it. Three of its four pragmas apply here; journal_mode(WAL) does not,
// since SQLite answers `memory` to it on an in-memory database and carries on.
// That is a no-op and not a mistake: there is no write-ahead log to want on a
// database thrown away at the end of one read.
func replay(entries []Entry) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(":memory:"))
	if err != nil {
		return nil, fmt.Errorf("open the replay: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := applySchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema to the replay: %w", err)
	}
	if err := replayInto(db, entries); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, path: ":memory:"}, nil
}

// replayInto puts the entries back in their own order. The seq is carried over
// rather than reassigned: an entry's position is what the caller asked about,
// and a payload naming another entry would name the wrong one under fresh
// numbering.
func replayInto(db *sql.DB, entries []Entry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, e := range entries {
		_, err := tx.Exec(
			`INSERT INTO change_history (seq, at, actor, kind, subject, payload) VALUES (?, ?, ?, ?, ?, ?)`,
			e.Seq, e.At.UTC().Format(stamp), e.Actor, e.Kind, e.Subject, e.Payload,
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("replay entry %d (%s): %w", e.Seq, e.Kind, err)
		}
	}
	return tx.Commit()
}
