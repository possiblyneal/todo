package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// today is the date the broker works a "tomorrow" out against. The ai package
// reads no clock, so every dump carries one.
const today = "2006-01-02, Monday"

// handed is the key that hands a dump over. Enter is a new line in that box,
// so it cannot be the key that submits it.
const handed = "ctrl+d"

// capture is a brain dump on its way to the broker: one box somebody says the
// work into however they think of it, and the wait while it is read. What
// comes back fills a draft and opens the add or edit screen on it, which is
// where it is approved.
//
// Nothing is written from here and no Lease is taken. A dump that is read and
// then escaped leaves nothing behind, the same as a proposal nobody ticks.
type capture struct {
	Text string

	// onto is the Task a dump amends, and nil for a new one. The broker is
	// shown that Task and answers with the whole of it as it should end up,
	// so an attribute the dump says nothing about comes back unchanged.
	onto *store.Task

	form    *huh.Form
	waiting bool
}

// readMsg is what the broker made of a dump, off the event loop. It carries
// the capture that asked for the reason stepMsg carries its breakdown:
// escaping does not cancel the call already in flight, and a late answer must
// not fill in a box somebody has since opened on something else.
type readMsg struct {
	cap  *capture
	read ai.Capture
	err  error
}

// startCapture opens the box: nil for a new Task, and the Task under the
// cursor when the dump is a change to one.
func (m Model) startCapture(onto *store.Task) (Model, tea.Cmd) {
	c := &capture{onto: onto}
	c.form = m.captureForm(c)
	m.capturing = c
	return m, c.form.Init()
}

// captureForm is the one box. Enter is a new line in it rather than a submit,
// which is the opposite of every other text field in this package and the
// whole point of the screen: a dump is paragraphs, and ctrl+d is done.
func (m Model) captureForm(c *capture) *huh.Form {
	title := "Say what the Task is"
	if c.onto != nil {
		title = "Say what changes about " + c.onto.Title
	}
	// ctrl+d never reaches the form: huh's Text acts on Next and Submit and
	// then hands the same key to the textarea underneath, which binds ctrl+d
	// to delete-forward, so a submit through huh would eat the character
	// under the cursor on its way out. updateCapture takes the key instead,
	// and these bindings are what the help line says.
	keys := huh.NewDefaultKeyMap()
	keys.Text.NewLine = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "new line"))
	done := key.NewBinding(key.WithKeys(handed), key.WithHelp(handed, "read it"))
	keys.Text.Next, keys.Text.Submit = done, done

	return huh.NewForm(huh.NewGroup(
		huh.NewText().Title(title).
			Description("However you think of it. ctrl+d hands it to the broker, esc goes back, and an empty box opens the form itself.").
			Value(&c.Text).Lines(8),
	)).WithWidth(min(m.width-8, 72)).WithHeight(max(m.height-8, 12)).WithKeyMap(keys)
}

// updateCapture drives the box. Escape ends it at any point, including while
// the broker is being waited on.
func (m Model) updateCapture(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "esc" || (m.capturing.waiting && msg.String() == "ctrl+c") {
			m.capturing = nil
			return m, nil
		}
		if msg.String() == handed && !m.capturing.waiting {
			return m.hand()
		}

	case readMsg:
		if msg.cap != m.capturing {
			return m, nil
		}
		onto := m.capturing.onto
		m.capturing = nil
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		return m.edit(m.drafted(msg.read, onto))
	}

	if m.capturing.waiting {
		return m, nil
	}
	form, cmd := m.capturing.form.Update(msg)
	m.capturing.form, _ = form.(*huh.Form)
	// There is no completed state to handle: the only keys that would
	// complete a one-field form are Next and Submit, and both are the key
	// intercepted above, so a completed form here would be a keymap that
	// has changed underneath this screen.
	if m.capturing.form.State == huh.StateAborted {
		m.capturing = nil
	}
	return m, cmd
}

// hand is ctrl+d: the dump goes to the broker, or, when there is nothing in
// the box, straight to the form the broker would have filled in.
func (m Model) hand() (Model, tea.Cmd) {
	if strings.TrimSpace(m.capturing.Text) == "" {
		onto := m.capturing.onto
		m.capturing = nil
		return m.edit(blank(onto))
	}
	m.capturing.waiting = true
	return m, m.read(m.capturing)
}

// edit opens the add or edit screen on a draft, which is the one gate every
// dump passes through: what the broker read is shown, corrected and submitted
// by a person before a word of it is written.
func (m Model) edit(d *draft) (Model, tea.Cmd) {
	m.draft = d
	m.editor = m.form(d)
	return m, m.editor.Init()
}

// blank is the draft a dump would have filled in, unfilled.
func blank(onto *store.Task) *draft {
	if onto == nil {
		return &draft{}
	}
	return draftOf(*onto)
}

// read sends the dump with today's date and the names it may file under. The
// broker holds nothing between calls, so all of that goes with every one.
func (m Model) read(c *capture) tea.Cmd {
	dump := ai.Dump{Text: c.Text, Today: time.Now().Format(today)}
	for _, l := range m.lists {
		dump.Lists = append(dump.Lists, l.Name)
	}
	for _, t := range m.tags {
		dump.Tags = append(dump.Tags, t.Name)
	}
	if c.onto != nil {
		was := m.captured(*c.onto)
		dump.Was = &was
	}

	client := m.ai
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		read, err := client.Read(ctx, dump)
		return readMsg{cap: c, read: read, err: err}
	}
}

// captured is a Task as the broker is shown it when a dump amends one: the
// same shape it answers in, so what comes back is a whole Task rather than a
// difference nothing here could apply. No id crosses the wire, and a List and
// a Tag go by name.
func (m Model) captured(t store.Task) ai.Capture {
	c := ai.Capture{
		Title:       t.Title,
		Description: t.Description,
		Why:         t.Why,
		Priority:    string(t.Priority),
		Impact:      string(t.Impact),
		Lists:       m.nameEach(t.Lists),
		Tags:        m.nameEach(t.Tags),
	}
	if !t.Deadline.IsZero() {
		c.Deadline = t.Deadline.Local().Format(dateLayouts[0])
	}
	if t.Estimate > 0 {
		c.Estimate = t.Estimate.String()
	}
	return c
}

// drafted fills a draft from what the broker read. Two rules govern it, and
// both keep a person's Task safe from a guess: an answer left empty leaves
// what was already there, because the broker is told to leave a field out
// rather than invent one; and a value written in a way this program cannot
// read is dropped rather than carried onto a form that would then refuse to
// submit, the same rule an approved proposal is written under.
func (m Model) drafted(read ai.Capture, onto *store.Task) *draft {
	d := blank(onto)
	set := func(into *string, value string) {
		if strings.TrimSpace(value) != "" {
			*into = value
		}
	}
	set(&d.Title, read.Title)
	set(&d.Description, read.Description)
	set(&d.Why, read.Why)

	if when, err := parseDate(read.Deadline); err == nil && !when.IsZero() {
		d.Deadline = when.Format(dateLayouts[0])
	}
	if estimate, err := parseDuration(read.Estimate); err == nil && estimate > 0 {
		d.Estimate = estimate.String()
	}
	if l, err := store.ParseLevel(read.Priority); err == nil && l != "" {
		d.Priority = l
	}
	if l, err := store.ParseLevel(read.Impact); err == nil && l != "" {
		d.Impact = l
	}
	if ids := matched(read.Lists, m.lists, func(l store.List) (string, string) { return l.ID, l.Name }); len(ids) > 0 {
		d.Lists = ids
	}
	if ids := matched(read.Tags, m.tags, func(t store.Tag) (string, string) { return t.ID, t.Name }); len(ids) > 0 {
		d.Tags = ids
	}
	return d
}

// matched turns the names the broker chose into ids. A name that is not one of
// the offered ones is dropped: filing under a List means one that exists, and
// creating one is a write of its own that nobody asked for here. The same name
// said twice is one id: names are matched without case, so "work" and "Work"
// are the same List, and carrying it twice would append the membership twice.
func matched[T any](names []string, have []T, of func(T) (id, name string)) []string {
	var out []string
	for _, name := range names {
		for _, one := range have {
			id, known := of(one)
			if strings.EqualFold(known, strings.TrimSpace(name)) {
				if !slices.Contains(out, id) {
					out = append(out, id)
				}
				break
			}
		}
	}
	return out
}

// captureView draws the box, or the wait for the broker.
func (m Model) captureView() string {
	if m.capturing.waiting {
		return dimStyle.Render("reading it… esc to give up")
	}
	return m.capturing.form.View()
}
