// Package cli dispatches an invocation of the todo binary to one of its three
// modes. The three are modes of one artifact, not three programs: a person's
// TUI, an Agent's verb, and an SSH server for the same TUI all reach the same
// store through the same in-process call, and neither consumer gets a weaker
// or a stronger contract than the other.
//
// See docs/adrs/0001-ship-todo-as-one-go-binary.md.
package cli

// Mode is how one invocation was asked to behave.
type Mode int

const (
	// ModeTUI is bare `todo`: open the TUI on this terminal.
	ModeTUI Mode = iota
	// ModeVerb is `todo <verb>`: act and exit.
	ModeVerb
	// ModeServe is `todo serve`: listen on SSH so a phone reaches the same TUI.
	ModeServe
)

func (m Mode) String() string {
	switch m {
	case ModeTUI:
		return "tui"
	case ModeVerb:
		return "verb"
	case ModeServe:
		return "serve"
	}
	return "unknown"
}

// ModeOf reads the mode out of the arguments following the program name.
func ModeOf(args []string) Mode {
	if len(args) == 0 {
		return ModeTUI
	}
	if args[0] == "serve" {
		return ModeServe
	}
	return ModeVerb
}
