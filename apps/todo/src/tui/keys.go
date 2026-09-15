package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// verb is one key the main view answers to: what it does, what the footer
// calls it, and whether it needs a Task under the cursor. The two tables below
// are the single statement of the keyboard, the footer and the hit boxes, so a
// key added to one of the three is added to all three and none of them can
// drift from the others.
type verb struct {
	key   string
	what  string
	needs bool // a Task under the cursor
	run   func(Model) (Model, tea.Cmd)
}

// taskVerbs are the footer's first row: what is done to a Task. All but "add"
// act on the one under the cursor, and are drawn faint and do nothing when
// there is none.
var taskVerbs = []verb{
	{"a", "add", false, func(m Model) (Model, tea.Cmd) { return m.startCapture(nil) }},
	{"e", "edit", true, func(m Model) (Model, tea.Cmd) {
		t, _ := m.selected()
		return m.startCapture(&t)
	}},
	{"n", "subtask", true, Model.startSubtask},
	{"c", "complete", true, writes("complete")},
	{"x", "decline", true, writes("decline")},
	{"o", "reopen", true, writes("reopen")},
	{"d", "delete", true, writes("delete")},
	{"z", "snooze", true, func(m Model) (Model, tea.Cmd) {
		t, _ := m.selected()
		m.pop = &popupDraft{Task: t.ID}
		m.popup = m.snoozeForm(&m.pop.Until)
		return m, m.popup.Init()
	}},
	{"r", "repeat", true, Model.startRepeat},
	{"b", "breakdown", true, Model.startBreakdown},
}

// viewVerbs are the footer's second row: what is done to the view rather than
// to a Task. None of them needs a cursor on anything.
var viewVerbs = []verb{
	{"/", "search", false, Model.searching},
	{"s", "sort", false, func(m Model) (Model, tea.Cmd) { return m.sorted(), nil }},
	{"h", "hidden", false, func(m Model) (Model, tea.Cmd) { return m.showing(snoozedState), nil }},
	{"l", "lists", false, func(m Model) (Model, tea.Cmd) { return m.listing(), nil }},
	{"ctrl+l", "list", false, collection("List")},
	{"ctrl+t", "tag", false, collection("Tag")},
	{"?", "ask", false, Model.startInquiry},
	{"q", "quit", false, func(m Model) (Model, tea.Cmd) { return m, tea.Quit }},
}

// allVerbs is both rows flattened, for the readers that answer "what does this
// key do" rather than "what goes in which row". The footer is the only reader
// that wants them apart.
var allVerbs = slices.Concat(taskVerbs, viewVerbs)

// writes is the shape the four lifecycle verbs share: one Task, the Lease
// covering its tree, and a re-read.
func writes(what string) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) {
		return m.onSelected(func(id string) error { return m.lifecycle(id, what) })
	}
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

// collection opens the popup on the List or the Tag in front of you, and on a
// new one when there is none. The List in front of you is the one the view is
// narrowed to, and the Tag is the one chosen when exactly one is: with none or
// several there is nothing the key could mean but "make one".
//
// It is the same popup the add and edit screens open on ctrl+l and ctrl+t, on
// the same keys, so the fingers that make one while writing a Task make one
// here too. Those screens always make a new one: a Task being filed wants a
// List that does not exist yet, not a rename of one it is going into.
func collection(noun string) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) {
		if pop, ok := m.inFrontOf(noun); ok {
			m.pop = pop
			m.popup = m.describeForm(m.pop)
			return m, m.popup.Init()
		}
		m.pop = &popupDraft{Noun: noun}
		m.popup = m.collectionForm(noun, &m.pop.Name, &m.pop.Color)
		return m, m.popup.Init()
	}
}

// inFrontOf is the List or Tag a describe would be about.
func (m Model) inFrontOf(noun string) (*popupDraft, bool) {
	if noun == "List" {
		for _, l := range m.lists {
			if l.ID == m.list {
				return &popupDraft{Noun: noun, ID: l.ID, Name: l.Name, Color: l.Color}, true
			}
		}
		return nil, false
	}
	var only store.Tag
	for _, t := range m.tags {
		if !m.chosen[t.ID] {
			continue
		}
		if only.ID != "" {
			return nil, false
		}
		only = t
	}
	if only.ID == "" {
		return nil, false
	}
	return &popupDraft{Noun: noun, ID: only.ID, Name: only.Name, Color: only.Color}, true
}

// label is how a key is drawn in the footer: ctrl+l is four columns of hint
// and one of key, so it is drawn as ^l.
func (v verb) label() string { return strings.ReplaceAll(v.key, "ctrl+", "^") }

// lookup finds the verb a key names.
func lookup(key string) (verb, bool) {
	for _, v := range allVerbs {
		if v.key == key {
			return v, true
		}
	}
	return verb{}, false
}

// do runs the verb a key names. It is the single place a key or a click on
// the footer turns into a write, and the tables are the whole vocabulary, so
// there is no screen in between a person and a verb. An unknown key and a
// Task verb with nothing under the cursor both do nothing.
func (m Model) do(key string) (Model, tea.Cmd) {
	v, ok := lookup(key)
	if !ok {
		return m, nil
	}
	if _, chosen := m.selected(); v.needs && !chosen {
		return m, nil
	}
	return v.run(m)
}

// keys is what the Task list is left holding once the view has taken the keys
// above. Paging is pgup and pgdn and the ends are home and end, because that
// is what those keys are for and because bubbles/list's letter aliases for
// them -- b, u, d, f, h, l, g and G -- are letters the verbs want. The
// searchbox keeps "/", which is what a person reaches for to find something.
func (m *Model) keys() {
	m.tasks.KeyMap.Filter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	m.tasks.KeyMap.PrevPage = key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up"))
	m.tasks.KeyMap.NextPage = key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down"))
	m.tasks.KeyMap.GoToStart = key.NewBinding(key.WithKeys("home"), key.WithHelp("home", "first"))
	m.tasks.KeyMap.GoToEnd = key.NewBinding(key.WithKeys("end"), key.WithHelp("end", "last"))
}
