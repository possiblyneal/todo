package cli

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/api"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Run is the whole program behind main, taking its streams as arguments so the
// modes are testable without a process. It returns the exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	switch ModeOf(args) {
	case ModeAPI:
		return runAPI(args[1:], stderr)
	case ModeVerb:
		return runVerb(args, stdout, stderr)
	default:
		// ModeUsage, and anything a later mode adds to ModeOf without adding
		// itself here. Saying what the binary is for is the right answer to
		// both, and it is the wrong answer loudly rather than exit 0 quietly.
		return usage(stderr)
	}
}

// verbs is every verb there is, in the order usage names them. Which verb makes
// which call is written here once, the way `write.Lifecycle` holds the four
// lifecycle verbs, so the sentence a bare `todo` prints and the dispatch below
// cannot name different sets and let the binary lie about what it accepts.
var verbs = []struct {
	verb string
	run  func(s *store.Store, args []string, stdout, stderr io.Writer) int
}{
	{"add", addTask},
	{"capture", captureTask},
	{"list", listTasks},
	{"edit", func(s *store.Store, args []string, _, stderr io.Writer) int {
		return editTask(s, args, stderr)
	}},
	{"lists", func(s *store.Store, args []string, stdout, stderr io.Writer) int {
		return collections(s, "lists", args, stdout, stderr)
	}},
	{"tags", func(s *store.Store, args []string, stdout, stderr io.Writer) int {
		return collections(s, "tags", args, stdout, stderr)
	}},
	{"attach", attachTask},
	{"repeat", repeatTask},
	{"complete", lifecycleVerb("complete")},
	{"decline", lifecycleVerb("decline")},
	{"reopen", lifecycleVerb("reopen")},
	{"delete", lifecycleVerb("delete")},
}

// lifecycleVerb is one of the four, which take one id and no flags and differ
// only in the name they pass on. `write.Lifecycle` is what knows the four; this
// is only how the CLI reaches one of them.
func lifecycleVerb(verb string) func(*store.Store, []string, io.Writer, io.Writer) int {
	return func(s *store.Store, args []string, _, stderr io.Writer) int {
		return lifecycle(s, verb, args, stderr)
	}
}

// usage is bare `todo`: what the binary does and how to reach it. Opening the
// TUI was what this used to do; the person's surface is the browser client now,
// which `todo api` serves, so there is nothing left for a bare invocation to
// open. It is an error rather than a help screen because nothing was asked for.
func usage(stderr io.Writer) int {
	named := make([]string, len(verbs))
	for i, one := range verbs {
		named[i] = one.verb
	}
	fmt.Fprintf(stderr, `todo: a task tracker for a person and for agents.

  todo <verb> [flags]   act and exit
  todo api [flags]      serve the json and the browser client on the lan

Verbs: %s. Each takes -h for its own flags.
`, strings.Join(named, ", "))
	return 2
}

// runAPI is `todo api`: the same store over HTTP, for the browser client. It
// opens the store the other modes open, in this one process, and serves the
// compiled client's files beside the JSON when it is given a directory of
// them, so there is no second process and no CORS.
//
// There is no authentication, deliberately: the listener is for the LAN. That
// is the posture `todo serve` had minus the public key it wanted, and ADR
// 0003's first re-check trigger is what covers changing it.
func runAPI(args []string, stderr io.Writer) int {
	fs := flags("api", stderr)
	// Every write through the listener is attributed to whoever started it,
	// because there is no authentication and so nobody to name per request.
	// It is the same Actor the verbs resolve, so a person at the terminal and
	// the same person in the browser are one Actor in the Change History.
	o := api.Options{Actor: actor()}
	fs.StringVar(&o.Addr, "addr", ":8080", "address to listen on")
	fs.StringVar(&o.Web, "web", "", "directory of compiled client files to serve beside the json")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	s, err := open()
	if err != nil {
		fmt.Fprintf(stderr, "todo api: %v\n", err)
		return 1
	}
	defer func() { _ = s.Close() }()

	if err := api.ListenAndServe(s, o, stderr); err != nil {
		fmt.Fprintf(stderr, "todo api: %v\n", err)
		return 1
	}
	return 0
}

// runVerb is the acts-and-exits mode. It is the identical in-process call the
// API makes, not a second implementation of the rules: both reach the store
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

	for _, one := range verbs {
		if one.verb == verb {
			return one.run(s, rest, stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "todo: unknown verb %q\n", verb)
	return 2
}

// isRefusal says whether the store turned a write away rather than failing at
// it. Which errors those are is store.Refused's to say, so this surface and
// the API's status codes cannot come to disagree about what a refusal is.
func isRefusal(err error) bool {
	return store.Refused(err)
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
