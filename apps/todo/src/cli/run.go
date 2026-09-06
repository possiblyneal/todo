package cli

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Run is the whole program behind main, taking its streams as arguments so the
// three modes are testable without a process. It returns the exit status.
//
// The TUI lands at #18 and `serve` at #23; both are stubs until then.
func Run(args []string, stdout, stderr io.Writer) int {
	switch ModeOf(args) {
	case ModeTUI:
		fmt.Fprintln(stdout, "todo: tui")
	case ModeServe:
		fmt.Fprintln(stdout, "todo: serve")
	case ModeVerb:
		return runVerb(args, stdout, stderr)
	}
	return 0
}

// runVerb is the acts-and-exits mode. It is the identical in-process call the
// TUI makes, not a second implementation of the rules: both reach the store
// through the same package, so an Agent and a person get the same contract.
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
		title := strings.TrimSpace(strings.Join(rest, " "))
		if title == "" {
			fmt.Fprintln(stderr, "todo add: a task needs a title")
			return 2
		}
		id, err := s.AddTask(actor(), title)
		if err != nil {
			fmt.Fprintf(stderr, "todo add: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, id)
	case "list":
		tasks, err := s.Tasks()
		if err != nil {
			fmt.Fprintf(stderr, "todo list: %v\n", err)
			return 1
		}
		for _, t := range tasks {
			fmt.Fprintf(stdout, "%s  %s\n", t.ID, t.Title)
		}
	default:
		fmt.Fprintf(stderr, "todo: unknown verb %q\n", verb)
		return 2
	}
	return 0
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
