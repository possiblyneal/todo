package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// List is a named collection a Task belongs to, and Tag is a named label a
// Task carries. Both are aggregates of their own: each exists before any Task
// carries it and outlives the last one that did, and a Task holds the id
// rather than the text, so a rename is one write however many Tasks show it.
//
// Count is how many Tasks carry it right now, deleted ones aside. It is read,
// never stored: it is the frequency a sidebar ranks Tags by.
type List struct {
	ID    string
	Name  string
	Color string
	Count int
}

// Tag has the same shape as List and a different meaning, which is why it is
// its own type rather than an alias.
type Tag struct {
	ID    string
	Name  string
	Color string
	Count int
}

// AddList creates a List. It takes no Lease: a Lease covers a top-level Task
// and its tree, and a List is not in anyone's tree.
func (s *Store) AddList(actor, name, color string) (string, error) {
	return s.create(actor, KindListCreated, "List", name, color)
}

// AddTag creates a Tag, which is what gives a label an identity of its own
// instead of leaving it as text typed on a Task.
func (s *Store) AddTag(actor, name, color string) (string, error) {
	return s.create(actor, KindTagCreated, "Tag", name, color)
}

func (s *Store) create(actor, kind, noun, name, color string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("a %s needs a name", noun)
	}
	if err := checkColor(color); err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	if _, err := s.Append(actor, kind, id, map[string]any{"name": name, "color": color}); err != nil {
		return "", err
	}
	return id, nil
}

// DescribeList renames or recolors a List once, wherever it is carried. A nil
// argument leaves that attribute alone.
func (s *Store) DescribeList(actor, listID string, name, color *string) error {
	return s.describe(actor, KindListDescribed, listID, name, color)
}

// DescribeTag renames or recolors a Tag once, wherever it is carried.
func (s *Store) DescribeTag(actor, tagID string, name, color *string) error {
	return s.describe(actor, KindTagDescribed, tagID, name, color)
}

func (s *Store) describe(actor, kind, id string, name, color *string) error {
	payload := map[string]any{}
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return fmt.Errorf("a rename needs a name")
		}
		payload["name"] = *name
	}
	if color != nil {
		if err := checkColor(*color); err != nil {
			return err
		}
		payload["color"] = *color
	}
	if len(payload) == 0 {
		return fmt.Errorf("an edit has to change something")
	}
	_, err := s.Append(actor, kind, id, payload)
	return err
}

// DeleteList removes a List. Every Task that was in it goes on existing and
// loses the membership, one appended task_unlisted per Task, so a Task's
// history says it left the List rather than falling silent about it. The
// unfiling and the removal are one transaction: a half-deleted List is a List
// nothing can be filed under and everything is still in.
func (s *Store) DeleteList(actor, listID string) error {
	return s.remove(actor, listID, lists)
}

// DeleteTag removes a Tag and takes it off every Task that carried it, on the
// same terms as DeleteList.
func (s *Store) DeleteTag(actor, tagID string) error {
	return s.remove(actor, tagID, tags)
}

// collection is everything a delete needs to know about which of the two it is
// deleting. The two are the same shape in five places at once, and naming them
// once here is what keeps a pair of them from being passed in the wrong order.
type collection struct {
	// name is the collection's own table, the key its membership entries are
	// written under, and the word an error calls it. All three are one word
	// by design: "list" and "tag" are the vocabulary, not three spellings.
	name string

	// carries is the membership table and column joining a Task to one.
	carries, column string

	// off is appended once per Task that carried it, gone once for the
	// collection itself.
	off, gone string
}

var (
	lists = collection{name: "list", carries: "task_list", column: "list_id", off: KindTaskUnlisted, gone: KindListDeleted}
	tags  = collection{name: "tag", carries: "task_tag", column: "tag_id", off: KindTagDetached, gone: KindTagDeleted}
)

// remove is the shape the two deletes share.
//
// No Lease is taken over the Tasks it unfiles, which is the one place a write
// touching a Task goes without one. A List is not in anybody's tree, so there
// is no single Lease that covers this; taking one per tree would mean a List
// could not be deleted while somebody held any Task that carried it, and
// abandoning half the unfiling on the first refusal is worse than the
// exception. What is appended takes nothing off a Task but a membership.
func (s *Store) remove(actor, id string, c collection) error {
	_, err := s.inTx(func(tx *sql.Tx) (Entry, error) {
		// An id naming nothing is refused rather than appended. The Change
		// History is append-only, so an entry deleting a List that never
		// existed is one no later write can take back, and a surface that
		// reported a typo as done would leave the person believing it.
		var exists int
		//nolint:gosec // the table is a constant above, never input.
		if err := tx.QueryRow(`SELECT COUNT(*) FROM `+c.name+` WHERE id = ?`, id).Scan(&exists); err != nil {
			return Entry{}, fmt.Errorf("read the %s: %w", c.name, err)
		}
		if exists == 0 {
			return Entry{}, fmt.Errorf("no %s has the id %q", c.name, id)
		}

		//nolint:gosec // table and column are constants above, never input.
		rows, err := tx.Query(`SELECT task_id FROM `+c.carries+` WHERE `+c.column+` = ?`, id)
		if err != nil {
			return Entry{}, fmt.Errorf("read what carries it: %w", err)
		}
		var carriers []string
		for rows.Next() {
			var taskID string
			if err := rows.Scan(&taskID); err != nil {
				_ = rows.Close()
				return Entry{}, fmt.Errorf("read what carries it: %w", err)
			}
			carriers = append(carriers, taskID)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return Entry{}, fmt.Errorf("read what carries it: %w", err)
		}
		for _, taskID := range carriers {
			if _, err := appendTx(tx, actor, c.off, taskID, map[string]any{c.name: id}); err != nil {
				return Entry{}, err
			}
		}
		return appendTx(tx, actor, c.gone, id, map[string]any{})
	})
	return err
}

// AddToList puts a Task in a List, and a Task may be in more than one. It is a
// write to the Task, so it needs the Lease covering the Task's tree.
func (s *Store) AddToList(actor, taskID, listID string) error {
	return s.guarded(actor, taskID, KindTaskListed, map[string]any{"list": listID})
}

// RemoveFromList takes a Task out of a List. The List stays.
func (s *Store) RemoveFromList(actor, taskID, listID string) error {
	return s.guarded(actor, taskID, KindTaskUnlisted, map[string]any{"list": listID})
}

// AttachTag puts a Tag on a Task, under the Task's Lease.
func (s *Store) AttachTag(actor, taskID, tagID string) error {
	return s.guarded(actor, taskID, KindTagAttached, map[string]any{"tag": tagID})
}

// DetachTag takes a Tag off a Task. The Tag stays.
func (s *Store) DetachTag(actor, taskID, tagID string) error {
	return s.guarded(actor, taskID, KindTagDetached, map[string]any{"tag": tagID})
}

// Lists reads every List by name, with the number of Tasks in each.
func (s *Store) Lists() ([]List, error) {
	rows, err := s.db.Query(`
SELECT l.id, l.name, COALESCE(l.color, ''),
	(SELECT COUNT(*) FROM task_list m JOIN task t ON t.id = m.task_id
	 WHERE m.list_id = l.id AND t.deleted_at IS NULL)
FROM list l ORDER BY l.name, l.id`)
	if err != nil {
		return nil, fmt.Errorf("read lists: %w", err)
	}
	return scanCollection(rows, func(id, name, color string, count int) List {
		return List{ID: id, Name: name, Color: color, Count: count}
	})
}

// Tags reads every Tag, most carried first, which is the order a sidebar
// ranking by frequency wants.
func (s *Store) Tags() ([]Tag, error) {
	rows, err := s.db.Query(`
SELECT g.id, g.name, COALESCE(g.color, ''),
	(SELECT COUNT(*) FROM task_tag m JOIN task t ON t.id = m.task_id
	 WHERE m.tag_id = g.id AND t.deleted_at IS NULL) AS carried
FROM tag g ORDER BY carried DESC, g.name, g.id`)
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return scanCollection(rows, func(id, name, color string, count int) Tag {
		return Tag{ID: id, Name: name, Color: color, Count: count}
	})
}

func scanCollection[T any](rows *sql.Rows, build func(id, name, color string, count int) T) ([]T, error) {
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		var id, name, color string
		var count int
		if err := rows.Scan(&id, &name, &color, &count); err != nil {
			return nil, fmt.Errorf("read a collection: %w", err)
		}
		out = append(out, build(id, name, color, count))
	}
	return out, rows.Err()
}
