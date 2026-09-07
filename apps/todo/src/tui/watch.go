package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// pollEvery is how often the TUI looks for someone else's write. A second is
// short enough that a verb run in another terminal shows up while the person
// is still looking at the screen, and long enough that the stat costs nothing.
const pollEvery = time.Second

// pollMsg is the tick that asks the store whether anything changed.
type pollMsg struct{}

// watch is the only way this program learns of a write made outside it.
//
// The watch lives inside the TUI process deliberately. There is no watcher
// service, because the only process that needs to know about a change is the
// one already rendering the list, and when the TUI is closed nothing watches
// and nothing needs to. SQLite cannot push a change to another process, so a
// poll of the write-ahead log is the whole mechanism, and it is the accepted
// price of shipping zero deployables beyond the one binary.
func watch() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

// poll re-reads when the write-ahead log has moved since the last look.
func (m Model) poll() (Model, tea.Cmd) {
	if token := m.store.WALToken(); token != m.wal {
		m.wal = token
		m.err = m.refresh()
	}
	return m, watch()
}
