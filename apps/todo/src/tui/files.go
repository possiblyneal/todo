package tui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
)

// browser is the system file selector docs/features.md asks the add screen
// for: bubbles/filepicker, first-party, walking the filesystem where it
// already is.
//
// What comes back is a path, and a path is all that is kept. Nothing is
// copied, so the selector is the whole of adding an Attachment.
type browser struct {
	picker filepicker.Model
}

func newBrowser(height int) *browser {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	p := filepicker.New()
	p.CurrentDirectory = home
	p.ShowHidden = false
	p.AutoHeight = false
	p.SetHeight(max(height-8, 6))
	return &browser{picker: p}
}

// updateFiles gives the selector the keyboard. Escape closes it, and choosing
// a file puts its path on the draft and rebuilds the form around it, the same
// way creating a List does, so nothing typed is lost.
func (m Model) updateFiles(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" {
		m.files = nil
		return m, nil
	}

	picker, cmd := m.files.picker.Update(msg)
	m.files.picker = picker
	if chosen, path := picker.DidSelectFile(msg); chosen {
		m.files = nil
		if m.draft != nil {
			m.draft.Attachments = append(m.draft.Attachments, path)
			m.editor = m.form(m.draft)
			return m, m.editor.Init()
		}
	}
	return m, cmd
}

func (b *browser) View() string {
	return strings.Join([]string{
		titleStyle.Render("Attach a file"),
		b.picker.View(),
		dimStyle.Render("enter to pick · esc to go back"),
	}, "\n")
}
