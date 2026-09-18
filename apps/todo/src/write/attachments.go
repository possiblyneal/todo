package write

import (
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Pointing a Task somewhere else, and taking the pointer off again. Both are
// guarded writes to the Task, so they are the same take-write-release shape the
// rest of this package holds, and both surfaces call these rather than each
// wrapping the store call in a Lease of its own.

// Attach points a Task at somewhere else. What counts as somewhere is the
// store's, which resolves a path against the directory the process runs in and
// keeps a web address as it was typed.
func Attach(s *store.Store, actor, id, target string) error {
	return s.WithLease(actor, id, store.WriteTTL, func() error {
		return s.Attach(actor, id, target)
	})
}

// Detach takes one pointer off. The Change History keeps the entry that added
// it, which is the store's rule and not this one's.
func Detach(s *store.Store, actor, id, target string) error {
	return s.WithLease(actor, id, store.WriteTTL, func() error {
		return s.Detach(actor, id, target)
	})
}
