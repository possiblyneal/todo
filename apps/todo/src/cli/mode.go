// Package cli dispatches an invocation of the todo binary to one of its
// modes. They are modes of one artifact, not several programs: an Agent's
// verb and the JSON the browser client reads reach the same store through the
// same in-process call, and neither consumer gets a weaker or a stronger
// contract than the other.
//
// See docs/adrs/0003-replace-the-tui-with-a-browser-client.md, which supersedes
// ADR 0001: the TUI and serve modes are gone, and the person's surface is
// apps/web over the API.
package cli

// Mode is how one invocation was asked to behave.
type Mode int

const (
	// ModeUsage is bare `todo`: say what the binary does and exit. It was the
	// TUI until the browser client replaced it, and it is usage rather than a
	// default mode because a binary with no argument should not be guessed at.
	ModeUsage Mode = iota
	// ModeVerb is `todo <verb>`: act and exit.
	ModeVerb
	// ModeAPI is `todo api`: listen on HTTP so the browser client reaches the
	// same store.
	ModeAPI
)

func (m Mode) String() string {
	switch m {
	case ModeUsage:
		return "usage"
	case ModeVerb:
		return "verb"
	case ModeAPI:
		return "api"
	}
	return "unknown"
}

// ModeOf reads the mode out of the arguments following the program name.
func ModeOf(args []string) Mode {
	if len(args) == 0 {
		return ModeUsage
	}
	if args[0] == "api" {
		return ModeAPI
	}
	return ModeVerb
}
