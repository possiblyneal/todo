// Package cli dispatches an invocation of the todo binary to one of its
// modes. They are modes of one artifact, not several programs: a person's TUI,
// an Agent's verb, an SSH server for the same TUI, and the JSON the browser
// client reads all reach the same store through the same in-process call, and
// neither consumer gets a weaker or a stronger contract than the other.
//
// See docs/adrs/0003-replace-the-tui-with-a-browser-client.md, which supersedes
// ADR 0001: the TUI and serve modes go once the browser client does everything
// they did.
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
	// ModeAPI is `todo api`: listen on HTTP so the browser client reaches the
	// same store. It is what replaces ModeTUI and ModeServe.
	ModeAPI
)

func (m Mode) String() string {
	switch m {
	case ModeTUI:
		return "tui"
	case ModeVerb:
		return "verb"
	case ModeServe:
		return "serve"
	case ModeAPI:
		return "api"
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
	if args[0] == "api" {
		return ModeAPI
	}
	return ModeVerb
}
