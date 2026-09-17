package write

import (
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// This file is the Scheduling half of a write. Setting a rule, taking one off,
// and the three marks against one date are all guarded writes to the Task's
// tree, so each is the same take-write-release shape the rest of this package
// holds, and both surfaces call these rather than each wrapping the store call
// in a Lease of its own.

// Repeat gives a Task a rule, or replaces the one it has. The store answers
// the Series' id and nothing wants it: a Series is reached through the Task it
// is on, so the Task's id is what every surface already holds.
func Repeat(s *store.Store, actor, id, rule string) error {
	return s.WithLease(actor, id, store.WriteTTL, func() error {
		_, err := s.Repeat(actor, id, rule)
		return err
	})
}

// Unrepeat takes the rule off. The Series and the marks left on its dates stay
// in the record, which is the store's rule and not this one's.
func Unrepeat(s *store.Store, actor, id string) error {
	return s.WithLease(actor, id, store.WriteTTL, func() error {
		return s.Unrepeat(actor, id)
	})
}

// marks is what can be done to one date of a Series. Which mark makes which
// store call is written here once, so a surface naming the three is naming
// these and not a second list of its own.
//
// All three answer an id so one table can hold them: detach lifts the date out
// into an ordinary Task and that Task's id is the answer, and the two that
// leave the Series alone have nothing to name and answer empty.
var marks = []struct {
	mark string
	act  func(*store.Store, string, string, time.Time) (string, error)
}{
	{"tick", func(s *store.Store, actor, id string, on time.Time) (string, error) {
		return "", s.TickOccurrence(actor, id, on)
	}},
	{"skip", func(s *store.Store, actor, id string, on time.Time) (string, error) {
		return "", s.SkipOccurrence(actor, id, on)
	}},
	{"detach", func(s *store.Store, actor, id string, on time.Time) (string, error) {
		return s.DetachOccurrence(actor, id, on)
	}},
}

// Mark is the write one of those makes against one date, or false where the
// mark is none of the three. Like Lifecycle, an unrecognised one is answered
// false rather than with an error of this package's own: a URL naming one and
// a flag naming one are different mistakes, and each surface says so in its
// own words.
func Mark(mark string) (func(s *store.Store, actor, id string, on time.Time) (string, error), bool) {
	for _, one := range marks {
		if one.mark != mark {
			continue
		}
		act := one.act
		return func(s *store.Store, actor, id string, on time.Time) (string, error) {
			var written string
			err := s.WithLease(actor, id, store.WriteTTL, func() error {
				var err error
				written, err = act(s, actor, id, on)
				return err
			})
			return written, err
		}, true
	}
	return nil, false
}

// DetachEdited lifts one date out of its Series as the Task it was corrected
// into, which is one store call and so one entry rather than a detach followed
// by an edit of what it became. It is the fourth thing a surface does to a
// date, and it is not in marks because it carries a whole Task with it where
// the three carry only the date.
//
// Nothing is written until this is called, which is what lets a surface open
// the corrected copy for reading and leave the date an Occurrence if nobody
// saves it.
func DetachEdited(s *store.Store, actor, id string, on time.Time, a store.Attributes, m Membership) (string, error) {
	var written string
	err := s.WithLease(actor, id, store.WriteTTL, func() error {
		var err error
		written, err = s.DetachEdited(actor, id, on, a, m.IntoLists, m.AddTags)
		return err
	})
	return written, err
}

// MarkNames names the three, for the sentence a surface turns an unknown one
// away with.
func MarkNames() []string {
	names := make([]string, len(marks))
	for i, one := range marks {
		names[i] = one.mark
	}
	return names
}
