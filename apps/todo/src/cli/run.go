package cli

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/possiblyneal/todo/apps/todo/src/serve"
	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/tui"
)

// Run is the whole program behind main, taking its streams as arguments so the
// three modes are testable without a process. It returns the exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	switch ModeOf(args) {
	case ModeTUI:
		return runTUI(stdout, stderr)
	case ModeServe:
		return runServe(args[1:], stderr)
	case ModeVerb:
		return runVerb(args, stdout, stderr)
	}
	return 0
}

// runTUI is the bare-invocation mode: the main view, reading the same store
// the verbs write through.
//
// The TUI needs a terminal to take, so an invocation without one is a usage
// error rather than a crash inside the renderer. That is what makes `todo`
// safe for an Agent to run by accident.
func runTUI(stdout, stderr io.Writer) int {
	if !terminal(stdout) {
		fmt.Fprintln(stderr, "todo: the tui needs a terminal; run a verb instead")
		return 2
	}

	s, err := open()
	if err != nil {
		fmt.Fprintf(stderr, "todo: %v\n", err)
		return 1
	}
	defer func() { _ = s.Close() }()

	// The Tag ranking varies from one run to the next, so it is seeded from
	// the clock rather than fixed. A test seeds it itself.
	seed := uint64(time.Now().UnixNano())
	m, err := tui.New(s, actor(), rand.New(rand.NewPCG(seed, seed>>32)))
	if err != nil {
		fmt.Fprintf(stderr, "todo: %v\n", err)
		return 1
	}
	if _, err := tea.NewProgram(m, tea.WithOutput(stdout)).Run(); err != nil {
		fmt.Fprintf(stderr, "todo: %v\n", err)
		return 1
	}
	return 0
}

// terminal says whether a stream is a terminal the TUI can take.
func terminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// runServe is `todo serve`: the same TUI over SSH, for a phone on the LAN.
// It opens the same store the other two modes open, in this one process, and
// every session takes Leases through it like any other Actor.
func runServe(args []string, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	config, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintf(stderr, "todo serve: %v\n", err)
		return 1
	}

	fs := flags("serve", stderr)
	o := serve.Options{}
	fs.StringVar(&o.Addr, "addr", ":23234", "address to listen on")
	fs.StringVar(&o.HostKey, "host-key", filepath.Join(config, "todo", "ssh_host_ed25519"),
		"the server's own key, created if it is not there")
	fs.StringVar(&o.AuthorizedKeys, "authorized-keys", filepath.Join(home, ".ssh", "authorized_keys"),
		"the keys allowed in; a public key is the only way in")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	s, err := open()
	if err != nil {
		fmt.Fprintf(stderr, "todo serve: %v\n", err)
		return 1
	}
	defer func() { _ = s.Close() }()

	if err := serve.ListenAndServe(s, o, stderr); err != nil {
		fmt.Fprintf(stderr, "todo serve: %v\n", err)
		return 1
	}
	return 0
}

// runVerb is the acts-and-exits mode. It is the identical in-process call the
// TUI makes, not a second implementation of the rules: both reach the store
// through the same package, so an Agent and a person get the same contract.
//
// Every verb that writes takes the Lease covering its target's tree, writes,
// and gives it back. An Agent is invoked and exits, so it should not leave a
// Lease standing behind it.
func runVerb(args []string, stdout, stderr io.Writer) int {
	verb, rest := args[0], args[1:]

	s, err := open()
	if err != nil {
		fmt.Fprintf(stderr, "todo: %v\n", err)
		return 1
	}
	defer func() { _ = s.Close() }()

	switch verb {
	case "add":
		return addTask(s, rest, stdout, stderr)
	case "capture":
		return captureTask(s, rest, stdout, stderr)
	case "list":
		return listTasks(s, rest, stdout, stderr)
	case "edit":
		return editTask(s, rest, stderr)
	case "lists", "tags":
		return collections(s, verb, rest, stdout, stderr)
	case "attach":
		return attachTask(s, rest, stdout, stderr)
	case "repeat":
		return repeatTask(s, rest, stdout, stderr)
	case "complete", "decline", "reopen", "delete":
		return lifecycle(s, verb, rest, stderr)
	default:
		fmt.Fprintf(stderr, "todo: unknown verb %q\n", verb)
		return 2
	}
}

// isRefusal says whether the store turned a write away rather than failing at
// it: no Lease, or one another Actor holds.
func isRefusal(err error) bool {
	return errors.Is(err, store.ErrRefused) || errors.Is(err, store.ErrHeld)
}

func open() (*store.Store, error) {
	path, err := storePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("make the store directory: %w", err)
	}
	return store.Open(path)
}

// storePath is where the SQLite file lives. TODO_DB overrides it, which is how
// a test and an Agent run against a store of their own.
func storePath() (string, error) {
	if path := os.Getenv("TODO_DB"); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the store: %w", err)
	}
	return filepath.Join(dir, "todo", "todo.db"), nil
}

// actor names who is writing. Writes are attributed and reads are not, and an
// Agent says who it is through TODO_ACTOR; a person at a keyboard is their
// login.
func actor() string {
	if a := strings.TrimSpace(os.Getenv("TODO_ACTOR")); a != "" {
		return a
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}
