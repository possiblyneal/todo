package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// updatePalette gives the slash palette the keyboard. Enter runs the command
// under the cursor and closes; escape closes without running one.
func (m Model) updatePalette(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.paletteOpen = false
			if c, ok := m.palette.SelectedItem().(command); ok {
				return m.run(c.name)
			}
			return m, nil
		case "esc", "ctrl+c":
			m.paletteOpen = false
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(msg)
	return m, cmd
}

// updateEditor gives the add or edit screen the keyboard, and writes when the
// form says it is complete. The store is not touched before that: the form is
// think-time, and think-time holds no transaction.
func (m Model) updateEditor(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.editor, m.draft = nil, nil
			return m, nil
		case "ctrl+a":
			m.files = newBrowser(m.height)
			return m, m.files.picker.Init()
		case "ctrl+l", "ctrl+t":
			noun := "List"
			if key.String() == "ctrl+t" {
				noun = "Tag"
			}
			m.pop = &popupDraft{Noun: noun}
			m.popup = m.collectionForm(noun, &m.pop.Name, &m.pop.Colour)
			return m, m.popup.Init()
		}
	}

	form, cmd := m.editor.Update(msg)
	m.editor, _ = form.(*huh.Form)
	switch m.editor.State {
	case huh.StateCompleted:
		if err := m.save(m.draft); err != nil {
			m.err = err
		} else {
			m.err = m.refresh()
		}
		m.editor, m.draft = nil, nil
	case huh.StateAborted:
		m.editor, m.draft = nil, nil
	}
	return m, cmd
}

// updatePopup gives the new-List or new-Tag popup the keyboard. Creating one
// takes no Lease, because a List is in nobody's tree.
func (m Model) updatePopup(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" {
		m.popup, m.pop = nil, nil
		return m, nil
	}

	form, cmd := m.popup.Update(msg)
	m.popup, _ = form.(*huh.Form)
	switch m.popup.State {
	case huh.StateCompleted:
		m.popup = nil
		if m.pop != nil && m.pop.Task != "" {
			return m.applySnooze()
		}
		return m.createCollection()
	case huh.StateAborted:
		m.popup, m.pop = nil, nil
	}
	return m, cmd
}

// applySnooze hides the Task the picker was opened on until the date it
// settled on. An empty date takes the snooze off, which is what clearing it in
// the calendar means.
func (m Model) applySnooze() (Model, tea.Cmd) {
	pop := m.pop
	m.pop = nil
	until, err := parseDate(pop.Until)
	if err != nil {
		m.err = err
		return m, nil
	}
	if err := m.snoozeUntil(pop.Task, until); err != nil {
		m.err = err
		return m, nil
	}
	m.err = m.refresh()
	return m, nil
}

// createCollection writes the List or Tag the popup asked for, then rebuilds
// the screen that wanted it around the same draft, so nothing typed is lost
// and the new one is already carried.
func (m Model) createCollection() (Model, tea.Cmd) {
	var (
		id  string
		err error
	)
	pop := m.pop
	m.pop = nil
	if pop == nil {
		return m, nil
	}
	if pop.Noun == "List" {
		id, err = m.store.AddList(m.actor, pop.Name, pop.Colour)
	} else {
		id, err = m.store.AddTag(m.actor, pop.Name, pop.Colour)
	}
	if err != nil {
		m.err = err
		return m, nil
	}
	if err := m.refresh(); err != nil {
		m.err = err
		return m, nil
	}

	if m.draft == nil {
		return m, nil
	}
	if pop.Noun == "List" {
		m.draft.Lists = append(m.draft.Lists, id)
	} else {
		m.draft.Tags = append(m.draft.Tags, id)
	}
	m.editor = m.form(m.draft)
	return m, m.editor.Init()
}

// screen returns whichever screen has the keyboard, and whether one does. The
// popup is drawn over the form that opened it rather than replacing it, so a
// person creating a List can still see the Task they were writing.
func (m Model) screen() (string, bool) {
	switch {
	case m.bd != nil:
		return boxStyle.Render(m.breakdownView()), true
	case m.ask != nil:
		return boxStyle.Render(m.inquiryView()), true
	case m.rep != nil:
		if m.rep.form != nil {
			return strings.Join([]string{
				boxStyle.Render(m.rep.form.View()),
				dimStyle.Render(m.repeatView()),
			}, "\n"), true
		}
		return boxStyle.Render(m.repeatView()), true
	case m.files != nil:
		under := ""
		if m.editor != nil {
			under = dimStyle.Render(m.editor.View())
		}
		return strings.Join([]string{boxStyle.Render(m.files.View()), under}, "\n"), true
	case m.popup != nil:
		under := ""
		if m.editor != nil {
			under = dimStyle.Render(m.editor.View())
		}
		return strings.Join([]string{boxStyle.Render(m.popup.View()), under}, "\n"), true
	case m.editor != nil:
		return m.editor.View(), true
	case m.paletteOpen:
		return boxStyle.Render(m.palette.View()), true
	}
	return "", false
}
