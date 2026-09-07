// Package serve is `todo serve`: the same TUI, over SSH, so a phone with an
// off-the-shelf client reaches it.
//
// It is a seam that carries keystrokes one way and rendered cells the other.
// No Task crosses it and no second implementation of the rules lives behind
// it: a session builds the same tui.Model against the same store, takes Leases
// like any other Actor, and gets no weaker and no stronger contract.
//
// It is LAN only. Reaching it from anywhere else is the homelab effort's, not
// this binary's, and it would mean putting the SQLite file on a network
// filesystem, where the write-ahead log does not work. That is one of ADR
// 0001's re-check triggers, not a thing to work around here. There is no
// offline mode either: an SSH session needs the server reachable, so the phone
// works at home and nowhere else.
package serve

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"charm.land/wish/v2/recover"

	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/tui"
)

// Options are where to listen and which keys to trust.
type Options struct {
	// Addr is the address to listen on, host:port.
	Addr string
	// HostKey is the server's own key. It is created if it is not there.
	HostKey string
	// AuthorizedKeys is the allowlist, in the format OpenSSH writes. It has
	// to exist: a public key is the only way in.
	AuthorizedKeys string
}

// Server builds the SSH server. Every connection gets its own tea.Program with
// the session's pty wired to it, its own Model, and its own zone manager;
// nothing is shared between sessions but the store, which is what makes two
// people on two phones two Actors rather than one.
func Server(s *store.Store, o Options) (*ssh.Server, error) {
	if _, err := os.Stat(o.AuthorizedKeys); err != nil {
		return nil, fmt.Errorf("no authorized_keys at %s: a public key is the only way in", o.AuthorizedKeys)
	}

	return wish.NewServer(
		wish.WithAddress(o.Addr),
		wish.WithHostKeyPath(o.HostKey),
		// Public key only. No password handler is set, so password auth is
		// never offered, which is the whole of the constraint from #6.
		wish.WithAuthorizedKeys(o.AuthorizedKeys),
		wish.WithMiddleware(
			// A panic takes the session it happened on and nothing else. Go
			// has no process-wide handler, so without this one bad render
			// would end every other session on the box too.
			recover.Middleware(
				bubbletea.MiddlewareWithProgramHandler(program(s)),
				activeterm.Middleware(),
			),
			logging.Middleware(),
		),
	)
}

// program is the handler run once per connection. The window-change channel is
// forwarded as a resize by the middleware, so a phone rotating is a
// WindowSizeMsg like any other.
func program(s *store.Store) bubbletea.ProgramHandler {
	return func(sess ssh.Session) *tea.Program {
		// The Actor is who logged in. A write from a phone is attributed to
		// the person at the phone, not to the server it landed on.
		seed := uint64(time.Now().UnixNano())
		m, err := tui.New(s, sess.User(), rand.New(rand.NewPCG(seed, seed>>32)))
		if err != nil {
			wish.Fatalln(sess, "todo:", err)
			return nil
		}
		return tea.NewProgram(m, bubbletea.MakeOptions(sess)...)
	}
}

// ListenAndServe runs until the listener is closed, which is what `todo serve`
// does with the terminal it was started from.
func ListenAndServe(s *store.Store, o Options, stderr io.Writer) error {
	srv, err := Server(s, o)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "todo: serving the tui on %s\n", o.Addr)
	if err := srv.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
