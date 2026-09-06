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
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

// The kinds of entry the Change History carries. Each is something that
// happened, named in the past tense, and every one of them is appended.
const (
	KindTaskAdded = "task_added"
)

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
type Task struct {
	ID        string
	Title     string
	CreatedAt time.Time
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
	title      TEXT NOT NULL,
	created_at TEXT NOT NULL
);

-- The fold. This fires inside the appending writer's transaction, so current
-- state and the entry that produced it commit together or not at all.
CREATE TRIGGER IF NOT EXISTS fold_task_added
AFTER INSERT ON change_history WHEN NEW.kind = 'task_added'
BEGIN
	INSERT INTO task (id, title, created_at)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.title'), NEW.at);
END;
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

// Append writes one entry and lets the fold triggers run with it. The sequence
// number is computed here rather than by AUTOINCREMENT, which leaves gaps when
// a transaction rolls back; under BEGIN IMMEDIATE this transaction already
// holds the write lock, so no other writer can be between the read and the
// insert.
func (s *Store) Append(actor, kind, subject string, payload any) (Entry, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Entry{}, fmt.Errorf("encode %s payload: %w", kind, err)
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return Entry{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var seq int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM change_history`).Scan(&seq); err != nil {
		return Entry{}, fmt.Errorf("next sequence: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO change_history (seq, at, actor, kind, subject, payload) VALUES (?, ?, ?, ?, ?, ?)`,
		seq, at, actor, kind, subject, string(body),
	)
	if err != nil {
		return Entry{}, fmt.Errorf("append %s: %w", kind, err)
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, fmt.Errorf("commit %s: %w", kind, err)
	}

	when, _ := time.Parse(time.RFC3339Nano, at)
	return Entry{Seq: seq, At: when, Actor: actor, Kind: kind, Subject: subject, Payload: string(body)}, nil
}

// AddTask is the Add Task command. It appends Task Added and returns the id
// the fold now knows the Task by.
func (s *Store) AddTask(actor, title string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	if _, err := s.Append(actor, KindTaskAdded, id, map[string]string{"title": title}); err != nil {
		return "", err
	}
	return id, nil
}

// Tasks reads current state. It writes nothing.
func (s *Store) Tasks() ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, title, created_at FROM task ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		var t Task
		var created string
		if err := rows.Scan(&t.ID, &t.Title, &created); err != nil {
			return nil, fmt.Errorf("read task: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
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
		e.At, _ = time.Parse(time.RFC3339Nano, at)
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
