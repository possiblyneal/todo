package cli

import (
	"fmt"
	"io"
)

// Run is the whole program behind main, taking its streams as arguments so the
// three modes are testable without a process. It returns the exit status.
//
// Every mode is a stub until the ticket that fills it in: the store and the
// Change History land at #13, the TUI at #18, and `serve` at #23.
func Run(args []string, stdout, stderr io.Writer) int {
	switch ModeOf(args) {
	case ModeTUI:
		fmt.Fprintln(stdout, "todo: tui")
	case ModeVerb:
		fmt.Fprintf(stdout, "todo: verb %q\n", args[0])
	case ModeServe:
		fmt.Fprintln(stdout, "todo: serve")
	}
	return 0
}
