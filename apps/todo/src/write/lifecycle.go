package write

import (
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// lifecycle is the rest of a Task's life: the four writes that take no
// attributes and change what state it is in. Which verb makes which store call
// is written here once, so a surface naming the four is naming these and not a
// second list of its own.
var lifecycle = []struct {
	verb string
	act  func(*store.Store, string, string) error
}{
	{"complete", (*store.Store).CompleteTask},
	{"decline", (*store.Store).DeclineTask},
	{"reopen", (*store.Store).ReopenTask},
	{"delete", (*store.Store).DeleteTask},
}

// Lifecycle is the write one of those verbs makes, or false where the verb is
// none of them. The write is the whole sequence and not the bare store call:
// each is guarded, so each takes the Lease covering the Task's tree, writes,
// and gives it back.
//
// A verb nobody recognises is answered false rather than with an error of this
// package's own, because a URL naming one and a command line naming one are
// different mistakes and each surface says so in its own words.
func Lifecycle(verb string) (func(s *store.Store, actor, id string) error, bool) {
	for _, one := range lifecycle {
		if one.verb != verb {
			continue
		}
		act := one.act
		return func(s *store.Store, actor, id string) error {
			return s.WithLease(actor, id, store.WriteTTL, func() error {
				return act(s, actor, id)
			})
		}, true
	}
	return nil, false
}

// LifecycleVerbs names the four, for the sentence a surface turns an unknown
// one away with.
func LifecycleVerbs() []string {
	verbs := make([]string, len(lifecycle))
	for i, one := range lifecycle {
		verbs[i] = one.verb
	}
	return verbs
}
