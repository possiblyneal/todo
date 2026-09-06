// Package store is the tracker's whole write path and the only thing that
// touches SQLite. It is linked as a library: there is no resident process and
// no server, so nothing here is awake between invocations.
//
// Two rules from the domain are enforced here rather than asked for politely,
// because a promise a store cannot enforce is advisory and an agent that
// ignores it simply wins:
//
//   - The Change History is one global append-only ordered sequence. UPDATE
//     and DELETE against it abort.
//   - Current state is folded by a trigger running inside the appending
//     writer's own transaction. Nothing sweeps, and no reader materializes.
//
// Reads are pure. Overdue is a comparison against the clock and an Occurrence
// is computed from its Series; neither is recorded when it becomes true, which
// is what removes the need for a process awake to notice.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

// The kinds of entry the Change History carries. Each is something that
// happened, named in the past tense, and every one of them is appended.
//
// There is no Lease Expired. Expiry is a condition that becomes true on its
// own and is read as the clause expires_at > now; breaking is something a
// writer did, so it is recorded.
const (
	KindTaskAdded     = "task_added"
	KindTaskDescribed = "task_described"
	KindLeaseTaken    = "lease_taken"
	KindLeaseReleased = "lease_released"
	KindLeaseBroken   = "lease_broken"
)

// ErrRefused is a write with no unexpired Lease held by the writing Actor on
// the target's top-level root. The store refused it; nothing was applied.
var ErrRefused = errors.New("refused: the write needs an unexpired Lease on the Task's top-level root")

// ErrHeld is a Lease that another Actor holds and that has not expired. It
// does not say who holds it: a Lease is symmetric, and a writer that finds one
// cannot tell whether a person or an Agent took it, nor does it need to.
var ErrHeld = errors.New("refused: an unexpired Lease is held on this Task")

// stamp is how every instant is stored. It is fixed-width on purpose, unlike
// time.RFC3339Nano, which trims trailing zeros from the fraction and so orders
// "12:00:00.5Z" before "12:00:00Z" under the TEXT comparison SQLite does. Both
// the Lease's expires_at > now and, later, a deadline < now depend on
// lexicographic order being chronological order.
const stamp = "2006-01-02T15:04:05.000000000Z07:00"

// Store is an open handle on the tracker's SQLite file.
type Store struct {
	db *sql.DB
}

// Entry is one appended fact. Seq is its position in the single global
// sequence, which is gapless: the reader that scopes the log to its own work
// needs to know it has seen everything between two positions.
type Entry struct {
	Seq     int64
	At      time.Time
	Actor   string
	Kind    string
	Subject string
	Payload string
}

// Task is folded current state, not a stored aggregate. It exists so a read
// does not have to replay the whole history to render a list.
//
// Parent is empty on a top-level Task. A Subtask never moves, so the walk from
// any Task to its top-level root is stable.
type Task struct {
	ID        string
	Parent    string
	Title     string
	CreatedAt time.Time
}

// Lease is an exclusive claim on one top-level Task and everything nested
// under it. It carries no notion of what kind of writer holds it.
type Lease struct {
	TaskID    string
	Actor     string
	ExpiresAt time.Time
}

// dsn builds the connection string.
//
// _txlock=immediate is not tuning. database/sql issues a plain deferred BEGIN,
// which takes the write lock only at the first write, so a transaction that
// read first finds the database changed under it and is asked to retry. The
// store research measured the deferred path creating 228 of 250 rows under
// four-process contention and reporting no error for the 22 it lost, against
// 250 of 250 with the flag. Append reads the next sequence number inside its
// transaction, so here the deferred path fails loudly instead -- every writer
// but one gets SQLITE_BUSY on its first append, which the concurrency test
// reproduces if this line is changed.
func dsn(path string) string {
	pragmas := []string{
		"journal_mode(WAL)",
		"busy_timeout(10000)",
		"foreign_keys(1)",
		"synchronous(NORMAL)",
	}
	q := url.Values{}
	q.Set("_txlock", "immediate")
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	return "file:" + path + "?" + q.Encode()
}

const schema = `
CREATE TABLE IF NOT EXISTS change_history (
	seq     INTEGER PRIMARY KEY,
	at      TEXT NOT NULL,
	actor   TEXT NOT NULL,
	kind    TEXT NOT NULL,
	subject TEXT NOT NULL,
	payload TEXT NOT NULL
);

-- Append-only, enforced. A Task Deleted is an entry rather than an erasure,
-- which only holds if erasure is refused.
CREATE TRIGGER IF NOT EXISTS change_history_is_append_only_update
BEFORE UPDATE ON change_history
BEGIN
	SELECT RAISE(ABORT, 'the Change History is append-only');
END;

CREATE TRIGGER IF NOT EXISTS change_history_is_append_only_delete
BEFORE DELETE ON change_history
BEGIN
	SELECT RAISE(ABORT, 'the Change History is append-only');
END;

CREATE TABLE IF NOT EXISTS task (
	id         TEXT PRIMARY KEY,
	parent_id  TEXT REFERENCES task(id),
	title      TEXT NOT NULL,
	created_at TEXT NOT NULL
);

-- A Lease is a row keyed on the Task it covers, and it covers that Task's
-- whole nested tree. Keying it on the root is what makes "a parent cannot
-- complete with open children" enforceable rather than conventional; see
-- docs/adrs/0002-subtask-tree-is-one-aggregate.md.
CREATE TABLE IF NOT EXISTS lease (
	task_id    TEXT PRIMARY KEY REFERENCES task(id),
	actor      TEXT NOT NULL,
	expires_at TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS lease_covers_a_top_level_task
BEFORE INSERT ON lease
WHEN (SELECT parent_id FROM task WHERE id = NEW.task_id) IS NOT NULL
BEGIN
	SELECT RAISE(ABORT, 'a Lease covers a top-level Task, not a subtask');
END;

-- The folds. Each fires inside the appending writer's transaction, so current
-- state and the entry that produced it commit together or not at all.
CREATE TRIGGER IF NOT EXISTS fold_task_added
AFTER INSERT ON change_history WHEN NEW.kind = 'task_added'
BEGIN
	INSERT INTO task (id, parent_id, title, created_at)
	VALUES (
		NEW.subject,
		json_extract(NEW.payload, '$.parent'),
		json_extract(NEW.payload, '$.title'),
		NEW.at
	);
END;

CREATE TRIGGER IF NOT EXISTS fold_task_described
AFTER INSERT ON change_history WHEN NEW.kind = 'task_described'
BEGIN
	UPDATE task SET title = json_extract(NEW.payload, '$.title') WHERE id = NEW.subject;
END;

CREATE TRIGGER IF NOT EXISTS fold_lease_taken
AFTER INSERT ON change_history WHEN NEW.kind = 'lease_taken'
BEGIN
	INSERT INTO lease (task_id, actor, expires_at)
	VALUES (NEW.subject, NEW.actor, json_extract(NEW.payload, '$.expires_at'))
	ON CONFLICT(task_id) DO UPDATE SET actor = excluded.actor, expires_at = excluded.expires_at;
END;

CREATE TRIGGER IF NOT EXISTS fold_lease_released
AFTER INSERT ON change_history WHEN NEW.kind = 'lease_released'
BEGIN
	DELETE FROM lease WHERE task_id = NEW.subject;
END;

-- Lease Broken folds nothing. It is the trail a crashed run leaves, and the
-- Lease Taken appended straight after it is what replaces the stale row.
`

// Open opens the store at path, creating it if it is not there.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema to %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close releases the handle.
func (s *Store) Close() error { return s.db.Close() }

// Append writes one entry outside any guard and lets the fold triggers run
// with it. It is for facts no Lease governs: creating a top-level Task, and
// taking or breaking a Lease.
func (s *Store) Append(actor, kind, subject string, payload any) (Entry, error) {
	return s.inTx(func(tx *sql.Tx) (Entry, error) {
		return appendTx(tx, actor, kind, subject, payload)
	})
}

// inTx runs fn in one immediate transaction and commits it.
func (s *Store) inTx(fn func(*sql.Tx) (Entry, error)) (Entry, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Entry{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	e, err := fn(tx)
	if err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, fmt.Errorf("commit %s: %w", e.Kind, err)
	}
	return e, nil
}

// appendTx inserts one entry. The sequence number is computed here rather than
// by AUTOINCREMENT, which leaves gaps when a transaction rolls back; under
// BEGIN IMMEDIATE this transaction already holds the write lock, so no other
// writer can be between the read and the insert.
func appendTx(tx *sql.Tx, actor, kind, subject string, payload any) (Entry, error) {
	body, at, err := entryFields(kind, payload)
	if err != nil {
		return Entry{}, err
	}
	var seq int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM change_history`).Scan(&seq); err != nil {
		return Entry{}, fmt.Errorf("next sequence: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO change_history (seq, at, actor, kind, subject, payload) VALUES (?, ?, ?, ?, ?, ?)`,
		seq, at, actor, kind, subject, body,
	)
	if err != nil {
		return Entry{}, fmt.Errorf("append %s: %w", kind, err)
	}
	return entry(seq, at, actor, kind, subject, body), nil
}

// guardedAppend is the guarded write. Its predicate walks from guardOn up to
// that Task's top-level root and requires an unexpired Lease held by actor; if
// the walk finds none, the INSERT selects no row, and zero rows affected is a
// refusal rather than a silent no-op.
//
// The guard sits on the append rather than on the folded row because the fold
// is itself a trigger on the append. Guarding here guards every write exactly
// once, and makes an unleased entry unrepresentable rather than merely
// unwritten by the callers that remember.
//
// guardOn is the Task whose tree is being written to, which is not always the
// entry's subject: adding a Subtask writes a new id under an existing parent,
// and it is the parent's tree the Lease covers.
func guardedAppend(tx *sql.Tx, actor, kind, subject, guardOn string, payload any) (Entry, error) {
	body, at, err := entryFields(kind, payload)
	if err != nil {
		return Entry{}, err
	}
	var seq int64
	err = tx.QueryRow(`
WITH RECURSIVE ancestry(id, parent_id) AS (
	SELECT id, parent_id FROM task WHERE id = ?
	UNION ALL
	SELECT t.id, t.parent_id FROM task t JOIN ancestry a ON t.id = a.parent_id
)
INSERT INTO change_history (seq, at, actor, kind, subject, payload)
SELECT (SELECT COALESCE(MAX(seq), 0) + 1 FROM change_history), ?, ?, ?, ?, ?
WHERE EXISTS (
	SELECT 1 FROM lease
	WHERE task_id = (SELECT id FROM ancestry WHERE parent_id IS NULL)
	  AND actor = ? AND expires_at > ?
)
RETURNING seq`,
		guardOn, at, actor, kind, subject, body, actor, at,
	).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		// The predicate matched nothing, so the INSERT wrote no row. A refusal
		// is a refusal, never a silent no-op.
		return Entry{}, ErrRefused
	}
	if err != nil {
		return Entry{}, fmt.Errorf("append %s: %w", kind, err)
	}
	return entry(seq, at, actor, kind, subject, body), nil
}

func entryFields(kind string, payload any) (body, at string, err error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("encode %s payload: %w", kind, err)
	}
	return string(encoded), now(), nil
}

func entry(seq int64, at, actor, kind, subject, body string) Entry {
	when, _ := time.Parse(stamp, at)
	return Entry{Seq: seq, At: when, Actor: actor, Kind: kind, Subject: subject, Payload: body}
}

func now() string { return time.Now().UTC().Format(stamp) }

// AddTask is the Add Task command for a top-level Task. It appends Task Added
// and returns the id the fold now knows the Task by. No Lease governs it:
// there is no tree yet to hold one.
func (s *Store) AddTask(actor, title string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	_, err = s.Append(actor, KindTaskAdded, id, map[string]any{"title": title})
	if err != nil {
		return "", err
	}
	return id, nil
}

// AddSubtask nests a Task under an existing one. It is a write to the parent's
// tree, so it needs the Lease that covers that tree's root.
func (s *Store) AddSubtask(actor, parentID, title string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	_, err = s.inTx(func(tx *sql.Tx) (Entry, error) {
		return guardedAppend(tx, actor, KindTaskAdded, id, parentID,
			map[string]any{"title": title, "parent": parentID})
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// DescribeTask is the Edit Task command. It is guarded: an edit to any Task in
// a tree needs the Lease on that tree's root.
func (s *Store) DescribeTask(actor, taskID, title string) error {
	_, err := s.inTx(func(tx *sql.Tx) (Entry, error) {
		return guardedAppend(tx, actor, KindTaskDescribed, taskID, taskID,
			map[string]any{"title": title})
	})
	return err
}

// TakeLease claims taskID and everything nested under it until the ttl runs
// out. A Lease found stale is broken and taken over, which appends Lease
// Broken so a crashed run leaves a trail; an unexpired Lease held by anyone
// else is ErrHeld, and the error does not say who. Re-taking one's own
// unexpired Lease extends it.
//
// No reader deletes a stale Lease. Expiry is the clause expires_at > now, read
// where it matters and never swept.
func (s *Store) TakeLease(actor, taskID string, ttl time.Duration) (Lease, error) {
	expires := time.Now().UTC().Add(ttl)
	_, err := s.inTx(func(tx *sql.Tx) (Entry, error) {
		var holder, expiresAt string
		err := tx.QueryRow(`SELECT actor, expires_at FROM lease WHERE task_id = ?`, taskID).
			Scan(&holder, &expiresAt)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// Free.
		case err != nil:
			return Entry{}, fmt.Errorf("read the Lease on %s: %w", taskID, err)
		case expiresAt > now():
			if holder != actor {
				return Entry{}, ErrHeld
			}
		default:
			// Stale. Breaking it is something this writer did, so it is
			// recorded; the payload names nobody, because the Lease Taken
			// already in the log does.
			if _, err := appendTx(tx, actor, KindLeaseBroken, taskID, map[string]any{}); err != nil {
				return Entry{}, err
			}
		}
		return appendTx(tx, actor, KindLeaseTaken, taskID,
			map[string]any{"expires_at": expires.Format(stamp)})
	})
	if err != nil {
		return Lease{}, err
	}
	return Lease{TaskID: taskID, Actor: actor, ExpiresAt: expires}, nil
}

// ReleaseLease gives up a Lease this Actor holds. Releasing one that has
// already expired is refused: it is no longer this Actor's to give up, and the
// next writer breaks it.
func (s *Store) ReleaseLease(actor, taskID string) error {
	_, err := s.inTx(func(tx *sql.Tx) (Entry, error) {
		return guardedAppend(tx, actor, KindLeaseReleased, taskID, taskID, map[string]any{})
	})
	return err
}

// Tasks reads current state. It writes nothing.
func (s *Store) Tasks() ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(parent_id, ''), title, created_at FROM task ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		var t Task
		var created string
		if err := rows.Scan(&t.ID, &t.Parent, &t.Title, &created); err != nil {
			return nil, fmt.Errorf("read task: %w", err)
		}
		t.CreatedAt, _ = time.Parse(stamp, created)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// History reads the Change History as a log, in order. Agents scope the log to
// their own work rather than keeping a stored cursor, so the sequence has to
// stay readable as a sequence and not only as folded state.
func (s *Store) History() ([]Entry, error) {
	rows, err := s.db.Query(`SELECT seq, at, actor, kind, subject, payload FROM change_history ORDER BY seq`)
	if err != nil {
		return nil, fmt.Errorf("read the Change History: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var at string
		if err := rows.Scan(&e.Seq, &at, &e.Actor, &e.Kind, &e.Subject, &e.Payload); err != nil {
			return nil, fmt.Errorf("read entry: %w", err)
		}
		e.At, _ = time.Parse(stamp, at)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// HistoryLength counts appended entries.
func (s *Store) HistoryLength() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM change_history`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count the Change History: %w", err)
	}
	return n, nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate an id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
