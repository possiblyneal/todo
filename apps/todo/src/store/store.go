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
	"os"
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
	KindTaskCompleted = "task_completed"
	KindTaskReopened  = "task_reopened"
	KindTaskDeleted   = "task_deleted"
	KindLeaseTaken    = "lease_taken"
	KindLeaseReleased = "lease_released"
	KindLeaseBroken   = "lease_broken"

	KindListCreated   = "list_created"
	KindListDescribed = "list_described"
	KindTaskListed    = "task_listed"
	KindTaskUnlisted  = "task_unlisted"
	KindTagCreated    = "tag_created"
	KindTagDescribed  = "tag_described"
	KindTagAttached   = "tag_attached"
	KindTagDetached   = "tag_detached"

	KindAttachmentAdded   = "attachment_added"
	KindAttachmentRemoved = "attachment_removed"

	// Scheduling's four. A Series Created or Edited is the rule changing; the
	// three Occurrence entries are the only marks a date ever leaves, and each
	// is written because someone acted on that date.
	KindSeriesCreated      = "series_created"
	KindSeriesEdited       = "series_edited"
	KindOccurrenceTicked   = "occurrence_ticked"
	KindOccurrenceSkipped  = "occurrence_skipped"
	KindOccurrenceDetached = "occurrence_detached"
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
	db   *sql.DB
	path string
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
	ID     string
	Parent string
	// Depth is 1 for a top-level Task and at most 5, the level a Subtask
	// nests to. Tasks returns a tree depth first, so a parent is followed
	// by its own children before the next sibling.
	Depth int

	Title        string
	Description  string
	Why          string
	CreatedAt    time.Time
	Deadline     time.Time
	Estimate     time.Duration
	Priority     Level
	Impact       Level
	SnoozedUntil time.Time
	Colour       string
	Fields       map[string]string

	// Lists and Tags are the ids the Task carries, never the names: a List
	// or a Tag is renamed once, in its own aggregate, and every Task
	// carrying it shows the new name without being written to.
	Lists []string
	Tags  []string

	// Attachments are the pointers the Task holds, oldest first. Each is a
	// file path or a web address, and nothing here says whether what it
	// points at still exists.
	Attachments []string

	// Series is the id of the recurrence rule this Task repeats on, empty on
	// a Task that does not repeat. The dates themselves are computed by
	// Occurrences and are never stored ahead of being acted on.
	Series string

	CompletedAt time.Time
	DeletedAt   time.Time

	// Overdue is the expression deadline < now, worked out by the read that
	// produced this Task. It is never stored, and nothing records the moment
	// it became true.
	Overdue bool
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
// busy_timeout is ten seconds because it only ever has to outlast a real
// write. No surface holds a transaction across think-time -- the TUI's form
// gathers first and writes on submit -- so a writer waiting here is waiting on
// milliseconds of work, and ten seconds means a queue, not a deadlock.
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
	id               TEXT PRIMARY KEY,
	parent_id        TEXT REFERENCES task(id),
	depth            INTEGER NOT NULL,
	title            TEXT NOT NULL,
	description      TEXT,
	why              TEXT,
	created_at       TEXT NOT NULL,
	deadline         TEXT,
	estimate_seconds INTEGER,
	priority         TEXT,
	impact           TEXT,
	snoozed_until    TEXT,
	colour           TEXT,
	completed_at     TEXT,
	deleted_at       TEXT,
	-- The rule this Task repeats on, or NULL. Tracking points at Scheduling;
	-- Scheduling never holds a word of what the Task says.
	series_id        TEXT
);

-- Any number of key/value pairs, one row each, so a new pair is data rather
-- than a column.
CREATE TABLE IF NOT EXISTS task_field (
	task_id TEXT NOT NULL REFERENCES task(id),
	key     TEXT NOT NULL,
	value   TEXT NOT NULL,
	PRIMARY KEY (task_id, key)
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

-- A Task nests Subtasks to five levels and no further. The depth is folded
-- from the parent's rather than walked, so the limit is one comparison.
CREATE TRIGGER IF NOT EXISTS subtasks_nest_to_five_levels
BEFORE INSERT ON task
WHEN NEW.depth > 5
BEGIN
	SELECT RAISE(ABORT, 'a Subtask nests to five levels, no deeper');
END;

-- A Subtask is created where it lives and stays there. Re-parenting is a
-- cross-aggregate operation this boundary does not have; see
-- docs/adrs/0002-subtask-tree-is-one-aggregate.md.
CREATE TRIGGER IF NOT EXISTS a_subtask_never_moves
BEFORE UPDATE OF parent_id ON task
WHEN NEW.parent_id IS NOT OLD.parent_id
BEGIN
	SELECT RAISE(ABORT, 'a Subtask never moves');
END;

-- The invariant the coarse aggregate boundary was bought for. Only direct
-- children are consulted: an open grandchild keeps its own parent open, so the
-- rule reaches the whole tree one level at a time.
CREATE TRIGGER IF NOT EXISTS a_parent_completes_after_its_children
BEFORE UPDATE OF completed_at ON task
WHEN NEW.completed_at IS NOT NULL
 AND EXISTS (
	SELECT 1 FROM task child
	WHERE child.parent_id = NEW.id
	  AND child.completed_at IS NULL
	  AND child.deleted_at IS NULL
 )
BEGIN
	SELECT RAISE(ABORT, 'a parent cannot complete while a child is open');
END;

-- The folds. Each fires inside the appending writer's transaction, so current
-- state and the entry that produced it commit together or not at all.
CREATE TRIGGER IF NOT EXISTS fold_task_added
AFTER INSERT ON change_history WHEN NEW.kind = 'task_added'
BEGIN
	INSERT INTO task (
		id, parent_id, depth, title, description, why, created_at,
		deadline, estimate_seconds, priority, impact, snoozed_until, colour,
		series_id
	)
	VALUES (
		NEW.subject,
		json_extract(NEW.payload, '$.parent'),
		COALESCE((SELECT depth FROM task WHERE id = json_extract(NEW.payload, '$.parent')), 0) + 1,
		json_extract(NEW.payload, '$.title'),
		json_extract(NEW.payload, '$.description'),
		json_extract(NEW.payload, '$.why'),
		NEW.at,
		json_extract(NEW.payload, '$.deadline'),
		json_extract(NEW.payload, '$.estimate_seconds'),
		json_extract(NEW.payload, '$.priority'),
		json_extract(NEW.payload, '$.impact'),
		json_extract(NEW.payload, '$.snoozed_until'),
		json_extract(NEW.payload, '$.colour'),
		json_extract(NEW.payload, '$.series')
	);
END;

-- An edit is partial. A key absent from the payload leaves the column alone; a
-- key holding JSON null clears it. json_type tells the two apart, which
-- json_extract on its own cannot.
CREATE TRIGGER IF NOT EXISTS fold_task_described
AFTER INSERT ON change_history WHEN NEW.kind = 'task_described'
BEGIN
	UPDATE task SET
		title            = CASE WHEN json_type(NEW.payload, '$.title')            IS NULL THEN title            ELSE json_extract(NEW.payload, '$.title')            END,
		description      = CASE WHEN json_type(NEW.payload, '$.description')      IS NULL THEN description      ELSE json_extract(NEW.payload, '$.description')      END,
		why              = CASE WHEN json_type(NEW.payload, '$.why')              IS NULL THEN why              ELSE json_extract(NEW.payload, '$.why')              END,
		deadline         = CASE WHEN json_type(NEW.payload, '$.deadline')         IS NULL THEN deadline         ELSE json_extract(NEW.payload, '$.deadline')         END,
		estimate_seconds = CASE WHEN json_type(NEW.payload, '$.estimate_seconds') IS NULL THEN estimate_seconds ELSE json_extract(NEW.payload, '$.estimate_seconds') END,
		priority         = CASE WHEN json_type(NEW.payload, '$.priority')         IS NULL THEN priority         ELSE json_extract(NEW.payload, '$.priority')         END,
		impact           = CASE WHEN json_type(NEW.payload, '$.impact')           IS NULL THEN impact           ELSE json_extract(NEW.payload, '$.impact')           END,
		snoozed_until    = CASE WHEN json_type(NEW.payload, '$.snoozed_until')    IS NULL THEN snoozed_until    ELSE json_extract(NEW.payload, '$.snoozed_until')    END,
		colour           = CASE WHEN json_type(NEW.payload, '$.colour')           IS NULL THEN colour           ELSE json_extract(NEW.payload, '$.colour')           END,
		series_id        = CASE WHEN json_type(NEW.payload, '$.series')           IS NULL THEN series_id        ELSE json_extract(NEW.payload, '$.series')           END
	WHERE id = NEW.subject;
END;

-- Key/value pairs fold on both creation and edit. A pair whose value is empty
-- is removed, which is how a pair is taken off a Task.
CREATE TRIGGER IF NOT EXISTS fold_task_fields
AFTER INSERT ON change_history
WHEN NEW.kind IN ('task_added', 'task_described')
 AND json_type(NEW.payload, '$.fields') = 'object'
BEGIN
	DELETE FROM task_field
	WHERE task_id = NEW.subject
	  AND key IN (SELECT key FROM json_each(NEW.payload, '$.fields') WHERE value = '');

	INSERT INTO task_field (task_id, key, value)
	SELECT NEW.subject, key, value FROM json_each(NEW.payload, '$.fields') WHERE value <> ''
	ON CONFLICT (task_id, key) DO UPDATE SET value = excluded.value;
END;

-- Completing and reopening are separate entries, because undoing a terminal
-- state is something the business cares happened.
CREATE TRIGGER IF NOT EXISTS fold_task_completed
AFTER INSERT ON change_history WHEN NEW.kind = 'task_completed'
BEGIN
	UPDATE task SET completed_at = NEW.at WHERE id = NEW.subject;
END;

CREATE TRIGGER IF NOT EXISTS fold_task_reopened
AFTER INSERT ON change_history WHEN NEW.kind = 'task_reopened'
BEGIN
	UPDATE task SET completed_at = NULL WHERE id = NEW.subject;
END;

-- A deletion is an appended entry, so the fold marks the Task gone and the
-- Change History keeps every word of what it was.
CREATE TRIGGER IF NOT EXISTS fold_task_deleted
AFTER INSERT ON change_history WHEN NEW.kind = 'task_deleted'
BEGIN
	UPDATE task SET deleted_at = NEW.at WHERE id = NEW.subject;
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

-- A List and a Tag are aggregates of their own: each exists before any Task
-- carries it and outlives the last one that did. A Task carries the id, never
-- the text, which is what makes a rename one write rather than one per Task.
CREATE TABLE IF NOT EXISTS list (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	colour     TEXT,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tag (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	colour     TEXT,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_list (
	task_id TEXT NOT NULL REFERENCES task(id),
	list_id TEXT NOT NULL REFERENCES list(id),
	PRIMARY KEY (task_id, list_id)
);

CREATE TABLE IF NOT EXISTS task_tag (
	task_id TEXT NOT NULL REFERENCES task(id),
	tag_id  TEXT NOT NULL REFERENCES tag(id),
	PRIMARY KEY (task_id, tag_id)
);

CREATE TRIGGER IF NOT EXISTS fold_list_created
AFTER INSERT ON change_history WHEN NEW.kind = 'list_created'
BEGIN
	INSERT INTO list (id, name, colour, created_at)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.name'), json_extract(NEW.payload, '$.colour'), NEW.at);
END;

CREATE TRIGGER IF NOT EXISTS fold_list_described
AFTER INSERT ON change_history WHEN NEW.kind = 'list_described'
BEGIN
	UPDATE list SET
		name   = CASE WHEN json_type(NEW.payload, '$.name')   IS NULL THEN name   ELSE json_extract(NEW.payload, '$.name')   END,
		colour = CASE WHEN json_type(NEW.payload, '$.colour') IS NULL THEN colour ELSE json_extract(NEW.payload, '$.colour') END
	WHERE id = NEW.subject;
END;

CREATE TRIGGER IF NOT EXISTS fold_tag_created
AFTER INSERT ON change_history WHEN NEW.kind = 'tag_created'
BEGIN
	INSERT INTO tag (id, name, colour, created_at)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.name'), json_extract(NEW.payload, '$.colour'), NEW.at);
END;

CREATE TRIGGER IF NOT EXISTS fold_tag_described
AFTER INSERT ON change_history WHEN NEW.kind = 'tag_described'
BEGIN
	UPDATE tag SET
		name   = CASE WHEN json_type(NEW.payload, '$.name')   IS NULL THEN name   ELSE json_extract(NEW.payload, '$.name')   END,
		colour = CASE WHEN json_type(NEW.payload, '$.colour') IS NULL THEN colour ELSE json_extract(NEW.payload, '$.colour') END
	WHERE id = NEW.subject;
END;

-- Membership is folded on the Task, whose Lease governed the append.
CREATE TRIGGER IF NOT EXISTS fold_task_listed
AFTER INSERT ON change_history WHEN NEW.kind = 'task_listed'
BEGIN
	INSERT INTO task_list (task_id, list_id)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.list'))
	ON CONFLICT DO NOTHING;
END;

CREATE TRIGGER IF NOT EXISTS fold_task_unlisted
AFTER INSERT ON change_history WHEN NEW.kind = 'task_unlisted'
BEGIN
	DELETE FROM task_list
	WHERE task_id = NEW.subject AND list_id = json_extract(NEW.payload, '$.list');
END;

CREATE TRIGGER IF NOT EXISTS fold_tag_attached
AFTER INSERT ON change_history WHEN NEW.kind = 'tag_attached'
BEGIN
	INSERT INTO task_tag (task_id, tag_id)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.tag'))
	ON CONFLICT DO NOTHING;
END;

CREATE TRIGGER IF NOT EXISTS fold_tag_detached
AFTER INSERT ON change_history WHEN NEW.kind = 'tag_detached'
BEGIN
	DELETE FROM task_tag
	WHERE task_id = NEW.subject AND tag_id = json_extract(NEW.payload, '$.tag');
END;

-- An Attachment is a pointer and nothing more: the text naming somewhere else,
-- and when it was pointed at. Nothing is copied in, so there is nothing here to
-- collect when a Task is deleted, and nothing looks to see whether the target
-- is still there.
CREATE TABLE IF NOT EXISTS attachment (
	task_id  TEXT NOT NULL REFERENCES task(id),
	target   TEXT NOT NULL,
	added_at TEXT NOT NULL,
	PRIMARY KEY (task_id, target)
);

CREATE TRIGGER IF NOT EXISTS fold_attachment_added
AFTER INSERT ON change_history WHEN NEW.kind = 'attachment_added'
BEGIN
	INSERT INTO attachment (task_id, target, added_at)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.target'), NEW.at)
	ON CONFLICT DO NOTHING;
END;

CREATE TRIGGER IF NOT EXISTS fold_attachment_removed
AFTER INSERT ON change_history WHEN NEW.kind = 'attachment_removed'
BEGIN
	DELETE FROM attachment
	WHERE task_id = NEW.subject AND target = json_extract(NEW.payload, '$.target');
END;

-- Scheduling. A Series is a rule and nothing else: the text a person edits as
-- one thing, kept as one value so editing the rule is one append however many
-- dates it produces. No title, no description, no estimate -- no task content
-- crosses into this context, which is why the whole of it is two narrow tables.
CREATE TABLE IF NOT EXISTS series (
	id         TEXT PRIMARY KEY,
	rule       TEXT NOT NULL,
	created_at TEXT NOT NULL
);

-- An Occurrence is computed from the rule on every look and is never written
-- here in advance. A row exists only for a date somebody acted on -- ticked,
-- skipped, or Detached -- keyed by the pair (series, date), so reading next
-- year's dates leaves this table exactly as it was.
--
-- task_id is filled for a Detached date alone: it names the ordinary Task that
-- date became, and the rule stops producing it.
CREATE TABLE IF NOT EXISTS occurrence (
	series_id TEXT NOT NULL REFERENCES series(id),
	date      TEXT NOT NULL,
	state     TEXT NOT NULL,
	-- No foreign key on task_id. It is a pointer across a context boundary,
	-- and the mark is appended before the Task it names exists: the refusal
	-- has to abort the whole detach, so the guarded entry goes first.
	task_id   TEXT,
	PRIMARY KEY (series_id, date)
);

CREATE TRIGGER IF NOT EXISTS fold_series_created
AFTER INSERT ON change_history WHEN NEW.kind = 'series_created'
BEGIN
	INSERT INTO series (id, rule, created_at)
	VALUES (NEW.subject, json_extract(NEW.payload, '$.rule'), NEW.at);
END;

CREATE TRIGGER IF NOT EXISTS fold_series_edited
AFTER INSERT ON change_history WHEN NEW.kind = 'series_edited'
BEGIN
	UPDATE series SET rule = json_extract(NEW.payload, '$.rule') WHERE id = NEW.subject;
END;

-- The three marks fold the same way, into the same row. Acting on a date twice
-- replaces the mark and appends a second entry: the log keeps both, the fold
-- keeps the latest.
CREATE TRIGGER IF NOT EXISTS fold_occurrence_marked
AFTER INSERT ON change_history
WHEN NEW.kind IN ('occurrence_ticked', 'occurrence_skipped', 'occurrence_detached')
BEGIN
	INSERT INTO occurrence (series_id, date, state, task_id)
	VALUES (
		json_extract(NEW.payload, '$.series'),
		json_extract(NEW.payload, '$.date'),
		CASE NEW.kind
			WHEN 'occurrence_ticked'  THEN 'ticked'
			WHEN 'occurrence_skipped' THEN 'skipped'
			ELSE 'detached'
		END,
		json_extract(NEW.payload, '$.task')
	)
	ON CONFLICT (series_id, date) DO UPDATE
		SET state = excluded.state, task_id = excluded.task_id;
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
	return &Store{db: db, path: path}, nil
}

// WALToken is a cheap stand-in for "has anyone written since I last looked".
// It is the size and modification time of the write-ahead log, which every
// commit touches, so a surface can poll it instead of re-reading the store.
//
// SQLite cannot push a change to another process, so the token is the whole of
// the mechanism: it is compared, not subscribed to. An empty token means the
// log is not there to stat, which is what a store nobody has written to since
// its last checkpoint looks like.
func (s *Store) WALToken() string {
	info, err := os.Stat(s.path + "-wal")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d@%d", info.Size(), info.ModTime().UnixNano())
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

// Set is a pointer to v, for filling in an Attributes field. A nil field is
// left alone and a field pointing at its zero value clears the attribute, so
// every edit has to say which of the two it means.
func Set[T any](v T) *T { return &v }

// AddTask is the Add Task command for a top-level Task. It appends Task Added
// and returns the id the fold now knows the Task by. No Lease governs it:
// there is no tree yet to hold one.
func (s *Store) AddTask(actor string, a Attributes) (string, error) {
	payload, err := a.payload(true)
	if err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	if _, err := s.Append(actor, KindTaskAdded, id, payload); err != nil {
		return "", err
	}
	return id, nil
}

// AddSubtask nests a Task under an existing one. It is a write to the parent's
// tree, so it needs the Lease that covers that tree's root.
func (s *Store) AddSubtask(actor, parentID string, a Attributes) (string, error) {
	payload, err := a.payload(true)
	if err != nil {
		return "", err
	}
	payload["parent"] = parentID
	id, err := newID()
	if err != nil {
		return "", err
	}
	_, err = s.inTx(func(tx *sql.Tx) (Entry, error) {
		return guardedAppend(tx, actor, KindTaskAdded, id, parentID, payload)
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// EditTask is the Edit Task command. Every attribute reaches the Task through
// it, and it needs the Lease on the tree's root.
func (s *Store) EditTask(actor, taskID string, a Attributes) error {
	payload, err := a.payload(false)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return fmt.Errorf("an edit has to change something")
	}
	return s.guarded(actor, taskID, KindTaskDescribed, payload)
}

// CompleteTask, ReopenTask and DeleteTask are the rest of a Task's life. Each
// honours the same guard as an edit.
//
// Reopening is its own entry rather than a second Task Described, because it
// undoes a terminal state and the business cares that it happened. Deleting is
// an entry too: the fold marks the Task gone and the Change History keeps it.
func (s *Store) CompleteTask(actor, taskID string) error {
	return s.guarded(actor, taskID, KindTaskCompleted, map[string]any{})
}

func (s *Store) ReopenTask(actor, taskID string) error {
	return s.guarded(actor, taskID, KindTaskReopened, map[string]any{})
}

func (s *Store) DeleteTask(actor, taskID string) error {
	return s.guarded(actor, taskID, KindTaskDeleted, map[string]any{})
}

func (s *Store) guarded(actor, taskID, kind string, payload map[string]any) error {
	_, err := s.inTx(func(tx *sql.Tx) (Entry, error) {
		return guardedAppend(tx, actor, kind, taskID, taskID, payload)
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

// RootOf walks from a Task to the top of its tree. That root is where a Lease
// is keyed, and a Subtask never moves, so the answer is stable.
func (s *Store) RootOf(taskID string) (string, error) {
	var root string
	err := s.db.QueryRow(`
WITH RECURSIVE ancestry(id, parent_id) AS (
	SELECT id, parent_id FROM task WHERE id = ?
	UNION ALL
	SELECT t.id, t.parent_id FROM task t JOIN ancestry a ON t.id = a.parent_id
)
SELECT id FROM ancestry WHERE parent_id IS NULL`, taskID).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no such task: %s", taskID)
	}
	if err != nil {
		return "", fmt.Errorf("find the root of %s: %w", taskID, err)
	}
	return root, nil
}

// WithLease takes the Lease covering taskID's tree, runs fn, and gives the
// Lease back. It is the shape a verb wants: an Agent is invoked, writes, and
// exits, so it should not leave a Lease behind it.
func (s *Store) WithLease(actor, taskID string, ttl time.Duration, fn func() error) error {
	root, err := s.RootOf(taskID)
	if err != nil {
		return err
	}
	if _, err := s.TakeLease(actor, root, ttl); err != nil {
		return err
	}
	defer func() { _ = s.ReleaseLease(actor, root) }()
	return fn()
}

// Sort is the order siblings come back in. The tree shape is kept whichever
// one is asked for: a sort orders siblings, it does not flatten the tree.
type Sort string

const (
	SortCreated  Sort = "created"
	SortDeadline Sort = "deadline"
	SortTitle    Sort = "title"
	SortEstimate Sort = "estimate"
)

// sortKeys holds the expression each Sort orders on. A Task with no deadline
// sorts after every Task that has one, since '~' follows the digits a stamp
// starts with.
var sortKeys = map[Sort]string{
	SortCreated:  "created_at",
	SortDeadline: "COALESCE(deadline, '~')",
	SortTitle:    "lower(title)",
	// Estimates are an integer column and the path is text, so they are
	// padded: '9' would otherwise sort after '10'. A Task with no estimate
	// sorts last, as one with no deadline does.
	SortEstimate: "printf('%020d', COALESCE(estimate_seconds, 9223372036854775807))",
}

// Query narrows what Tasks returns. The zero Query is the everyday view: open
// Tasks that are neither snoozed nor deleted, in creation order.
type Query struct {
	IncludeCompleted bool
	IncludeSnoozed   bool
	IncludeDeleted   bool

	// List narrows to the Tasks in one List, named by its id.
	List string
	// Sort orders siblings. The zero value is SortCreated.
	Sort Sort
}

// Tasks reads current state. It writes nothing, and it evaluates Overdue and
// the snooze against the clock as it goes: neither is stored, and nothing
// records the moment either becomes true.
//
// The result is depth first: a Task is followed by everything nested under it
// before the next sibling, so a surface indents by Depth rather than
// rebuilding the tree.
func (s *Store) Tasks(q Query) ([]Task, error) {
	at := now()
	if q.Sort == "" {
		q.Sort = SortCreated
	}
	key, ok := sortKeys[q.Sort]
	if !ok {
		return nil, fmt.Errorf("%q is not a sort: want created, deadline or title", q.Sort)
	}
	rows, err := s.db.Query(fmt.Sprintf(`
WITH RECURSIVE depth_first(id, path) AS (
	SELECT id, %[1]s || '/' || id FROM task WHERE parent_id IS NULL
	UNION ALL
	SELECT t.id, d.path || '/' || %[1]s || '/' || t.id
	FROM task t JOIN depth_first d ON t.parent_id = d.id
)
SELECT
	t.id, COALESCE(t.parent_id, ''), t.depth, t.title, t.description, t.why, t.created_at,
	t.deadline, t.estimate_seconds, t.priority, t.impact, t.snoozed_until, t.colour,
	t.completed_at, t.deleted_at, COALESCE(t.series_id, ''),
	(t.deadline IS NOT NULL AND t.deadline < ?) AS overdue,
	COALESCE((SELECT json_group_object(key, value) FROM task_field f WHERE f.task_id = t.id), '{}'),
	COALESCE((SELECT json_group_array(list_id) FROM task_list m WHERE m.task_id = t.id), '[]'),
	COALESCE((SELECT json_group_array(tag_id) FROM task_tag m WHERE m.task_id = t.id), '[]'),
	COALESCE((SELECT json_group_array(target) FROM (
		SELECT target FROM attachment a WHERE a.task_id = t.id ORDER BY a.added_at, a.target
	)), '[]')
FROM task t JOIN depth_first d ON d.id = t.id
WHERE (? OR t.deleted_at IS NULL)
  AND (? OR t.completed_at IS NULL)
  AND (? OR t.snoozed_until IS NULL OR t.snoozed_until <= ?)
  AND (? = '' OR EXISTS (SELECT 1 FROM task_list m WHERE m.task_id = t.id AND m.list_id = ?))
ORDER BY d.path`, key),
		at, q.IncludeDeleted, q.IncludeCompleted, q.IncludeSnoozed, at, q.List, q.List)
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		var (
			t                                            Task
			description, why, deadline, priority, impact *string
			snoozedUntil, colour, completedAt, deletedAt *string
			createdAt, fields, lists, tags, attachments  string
			estimate                                     *int64
		)
		err := rows.Scan(
			&t.ID, &t.Parent, &t.Depth, &t.Title, &description, &why, &createdAt,
			&deadline, &estimate, &priority, &impact, &snoozedUntil, &colour,
			&completedAt, &deletedAt, &t.Series, &t.Overdue, &fields, &lists, &tags, &attachments,
		)
		if err != nil {
			return nil, fmt.Errorf("read task: %w", err)
		}
		t.Description = text(description)
		t.Why = text(why)
		t.Colour = text(colour)
		t.CreatedAt, _ = time.Parse(stamp, createdAt)
		t.Deadline = parseStamp(deadline)
		t.SnoozedUntil = parseStamp(snoozedUntil)
		t.CompletedAt = parseStamp(completedAt)
		t.DeletedAt = parseStamp(deletedAt)
		t.Priority = Level(text(priority))
		t.Impact = Level(text(impact))
		if estimate != nil {
			t.Estimate = time.Duration(*estimate) * time.Second
		}
		t.Fields = decodeFields(fields)
		t.Lists = decodeIDs(lists)
		t.Tags = decodeIDs(tags)
		t.Attachments = decodeIDs(attachments)
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
