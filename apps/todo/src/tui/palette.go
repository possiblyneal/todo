package tui

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// command is one entry in the slash palette.
type command struct {
	name string
	what string
}

func (c command) FilterValue() string { return c.name + " " + c.what }

// commands is the palette's whole vocabulary. Go ships no maintained command
// palette, so this is bubbles/list's fuzzy filter over a fixed set: the same
// widget the main view searches with, pointed at verbs instead of Tasks.
func commands() []list.Item {
	items := []list.Item{
		command{"/add", "a new Task"},
		command{"/edit", "the Task under the cursor"},
		command{"/complete", "the Task under the cursor"},
		command{"/decline", "the Task under the cursor: it will not be done"},
		command{"/reopen", "the Task under the cursor"},
		command{"/delete", "the Task under the cursor"},
		command{"/list", "a new List"},
		command{"/tag", "a new Tag"},
		command{"/snooze", "the Task under the cursor, until a date you pick"},
		command{"/repeat", "the Task under the cursor, on a schedule you write"},
		command{"/breakdown", "the Task under the cursor, into Subtasks, with the broker's help"},
		command{"/ask", "the broker a question about this list"},
		command{"/sort", "by the next order"},
		command{"/quit", "leave"},
	}
	for _, s := range store.SnoozeDefaults {
		items = append(items, command{"/snooze " + s.Label, "hide it for " + s.Label})
	}
	return items
}

// commandDelegate draws a command in one line.
type commandDelegate struct{}

func (commandDelegate) Height() int                         { return 1 }
func (commandDelegate) Spacing() int                        { return 0 }
func (commandDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (commandDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	c, ok := item.(command)
	if !ok {
		return
	}
	line := "  " + c.name + "  " + dimStyle.Render(c.what)
	if index == m.Index() {
		line = selectedStyle.Render("▸ " + c.name + "  " + c.what)
	}
	io.WriteString(w, line)
}

func newPalette(width, height int) list.Model {
	l := list.New(commands(), commandDelegate{}, width, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	return l
}

// run carries out a chosen command. It is the single place a palette entry
// turns into a write, so the palette itself knows nothing about Leases.
func (m Model) run(name string) (Model, tea.Cmd) {
	if after, found := strings.CutPrefix(name, "/snooze "); found {
		for _, s := range store.SnoozeDefaults {
			if s.Label == after {
				return m.onSelected(func(id string) error { return m.snooze(id, s) })
			}
		}
		return m, nil
	}

	switch name {
	case "/add":
		m.draft = &draft{}
		m.editor = m.form(m.draft)
		return m, m.editor.Init()

	case "/edit":
		t, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.draft = draftOf(t)
		m.editor = m.form(m.draft)
		return m, m.editor.Init()

	case "/complete", "/decline", "/reopen", "/delete":
		what := strings.TrimPrefix(name, "/")
		return m.onSelected(func(id string) error { return m.lifecycle(id, what) })

	case "/list", "/tag":
		m.pop = &popupDraft{Noun: strings.ToUpper(name[1:2]) + name[2:]}
		m.popup = m.collectionForm(m.pop.Noun, &m.pop.Name, &m.pop.Colour)
		return m, m.popup.Init()

	case "/snooze":
		t, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.pop = &popupDraft{Task: t.ID}
		m.popup = m.snoozeForm(&m.pop.Until)
		return m, m.popup.Init()

	case "/repeat":
		return m.startRepeat()

	case "/breakdown":
		return m.startBreakdown()

	case "/ask":
		return m.startInquiry()

	case "/sort":
		m.sort = nextSort(m.sort)
		m.err = m.refresh()
		return m, nil

	case "/quit":
		return m, tea.Quit
	}
	return m, nil
}

// onSelected runs a write against the Task under the cursor and re-reads.
func (m Model) onSelected(write func(id string) error) (Model, tea.Cmd) {
	t, ok := m.selected()
	if !ok {
		return m, nil
	}
	if err := write(t.ID); err != nil {
		m.err = err
		return m, nil
	}
	m.err = m.refresh()
	return m, nil
}
