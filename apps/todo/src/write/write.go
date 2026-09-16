// Package write holds the shape a write to a Task takes: take the tree's
// Lease, do every part of the write inside it, release it. Both surfaces call
// this rather than each repeating it, because a second copy of the sequence is
// a second path through the store's rules, which is what
// docs/adrs/0003-replace-the-tui-with-a-browser-client.md warns about and what
// docs/plans/browser-client.md asks be moved here before src/api/ exists.
//
// It enforces nothing the store does not. Everything here is either the
// sequence a write is made of or the reading of text into store.Attributes,
// and both surfaces get the same answer from both because there is one of
// each.
package write

import (
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Membership is the Lists and Tags one write joins and leaves, by id. Every
// field is optional and an empty one is nothing to do rather than a clearing.
type Membership struct {
	IntoLists  []string
	OutOfLists []string
	AddTags    []string
	DropTags   []string
}

// Add writes one Task and files it, under one Lease where there is a tree to
// hold one. A parent makes it a Subtask, which is a write to that parent's
// tree and so needs the Lease around the adding as well as the filing; a
// top-level Task has no tree until it exists, so it is added first and filed
// under its own Lease afterwards.
//
// The id comes back whether or not the filing went through. The Task is
// written by then, so a refusal leaves it filed under nothing rather than not
// written, and a surface that dropped the id would leave the person unable to
// name the Task they just made.
func Add(s *store.Store, actor, parent string, a store.Attributes, m Membership) (string, error) {
	if parent != "" {
		var id string
		err := s.WithLease(actor, parent, store.WriteTTL, func() error {
			var err error
			if id, err = s.AddSubtask(actor, parent, a); err != nil {
				return err
			}
			return file(s, actor, id, m)
		})
		return id, err
	}

	id, err := s.AddTask(actor, a)
	if err != nil {
		return "", err
	}
	if m.empty() {
		return id, nil
	}
	return id, s.WithLease(actor, id, store.WriteTTL, func() error {
		return file(s, actor, id, m)
	})
}

// Edit changes a Task's attributes and its memberships together. One Lease
// covers the whole edit, because the attributes and every List and Tag it
// joins or leaves are one visit to the tree.
//
// attributes says whether a is a change at all: nothing given is a membership
// edit, and calling EditTask with an empty Attributes would append an entry
// saying a person changed nothing.
func Edit(s *store.Store, actor, id string, a store.Attributes, attributes bool, m Membership) error {
	return s.WithLease(actor, id, store.WriteTTL, func() error {
		if attributes {
			if err := s.EditTask(actor, id, a); err != nil {
				return err
			}
		}
		return file(s, actor, id, m)
	})
}

// file is the membership half of a write, already inside the Lease. The four
// go through one table so the order they happen in is written once and is the
// same order both surfaces get.
func file(s *store.Store, actor, id string, m Membership) error {
	for _, part := range []struct {
		ids []string
		do  func(actor, taskID, otherID string) error
	}{
		{m.IntoLists, s.AddToList},
		{m.OutOfLists, s.RemoveFromList},
		{m.AddTags, s.AttachTag},
		{m.DropTags, s.DetachTag},
	} {
		for _, other := range part.ids {
			if err := part.do(actor, id, other); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m Membership) empty() bool {
	return len(m.IntoLists) == 0 && len(m.OutOfLists) == 0 &&
		len(m.AddTags) == 0 && len(m.DropTags) == 0
}
