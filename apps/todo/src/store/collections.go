package store

import (
	"database/sql"
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
	ID     string
	Name   string
	Colour string
	Count  int
}

// Tag has the same shape as List and a different meaning, which is why it is
// its own type rather than an alias.
type Tag struct {
	ID     string
	Name   string
	Colour string
	Count  int
}

// AddList creates a List. It takes no Lease: a Lease covers a top-level Task
// and its tree, and a List is not in anyone's tree.
func (s *Store) AddList(actor, name, colour string) (string, error) {
	return s.create(actor, KindListCreated, "List", name, colour)
}

// AddTag creates a Tag, which is what gives a label an identity of its own
// instead of leaving it as text typed on a Task.
func (s *Store) AddTag(actor, name, colour string) (string, error) {
	return s.create(actor, KindTagCreated, "Tag", name, colour)
}

func (s *Store) create(actor, kind, noun, name, colour string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("a %s needs a name", noun)
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	if _, err := s.Append(actor, kind, id, map[string]any{"name": name, "colour": colour}); err != nil {
		return "", err
	}
	return id, nil
}

// DescribeList renames or recolours a List once, wherever it is carried. A nil
// argument leaves that attribute alone.
func (s *Store) DescribeList(actor, listID string, name, colour *string) error {
	return s.describe(actor, KindListDescribed, listID, name, colour)
}

// DescribeTag renames or recolours a Tag once, wherever it is carried.
func (s *Store) DescribeTag(actor, tagID string, name, colour *string) error {
	return s.describe(actor, KindTagDescribed, tagID, name, colour)
}

func (s *Store) describe(actor, kind, id string, name, colour *string) error {
	payload := map[string]any{}
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return fmt.Errorf("a rename needs a name")
		}
		payload["name"] = *name
	}
	if colour != nil {
		payload["colour"] = *colour
	}
	if len(payload) == 0 {
		return fmt.Errorf("an edit has to change something")
	}
	_, err := s.Append(actor, kind, id, payload)
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
SELECT l.id, l.name, COALESCE(l.colour, ''),
	(SELECT COUNT(*) FROM task_list m JOIN task t ON t.id = m.task_id
	 WHERE m.list_id = l.id AND t.deleted_at IS NULL)
FROM list l ORDER BY l.name, l.id`)
	if err != nil {
		return nil, fmt.Errorf("read lists: %w", err)
	}
	return scanCollection(rows, func(id, name, colour string, count int) List {
		return List{ID: id, Name: name, Colour: colour, Count: count}
	})
}

// Tags reads every Tag, most carried first, which is the order a sidebar
// ranking by frequency wants.
func (s *Store) Tags() ([]Tag, error) {
	rows, err := s.db.Query(`
SELECT g.id, g.name, COALESCE(g.colour, ''),
	(SELECT COUNT(*) FROM task_tag m JOIN task t ON t.id = m.task_id
	 WHERE m.tag_id = g.id AND t.deleted_at IS NULL) AS carried
FROM tag g ORDER BY carried DESC, g.name, g.id`)
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return scanCollection(rows, func(id, name, colour string, count int) Tag {
		return Tag{ID: id, Name: name, Colour: colour, Count: count}
	})
}

func scanCollection[T any](rows *sql.Rows, build func(id, name, colour string, count int) T) ([]T, error) {
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		var id, name, colour string
		var count int
		if err := rows.Scan(&id, &name, &colour, &count); err != nil {
			return nil, fmt.Errorf("read a collection: %w", err)
		}
		out = append(out, build(id, name, colour, count))
	}
	return out, rows.Err()
}
