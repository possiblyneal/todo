// Package tui is the screen: the same store the verbs write through, read and
// drawn, and written to by the same calls. The main view itself writes
// nothing -- moving the cursor, sorting, expanding a row and opening the
// palette all leave the Change History the length they found it -- and every
// write goes through a screen a person opened on purpose.
package tui

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

const (
	sidebarWidth = 22
	everyList    = ""

	// labelWidth is the column every attribute's name is padded into on
	// the detail pane. It is the longest of them, "snoozed until".
	labelWidth = 13
)

// Model is the read-only main view. It holds the store rather than a copy of
// its contents: every refresh is a read, and a read writes nothing.
//
// The zone manager is per-Model rather than the package-global one, because
// `todo serve` runs many sessions in one process and a global manager would
// hand one session's hit boxes to another.
type Model struct {
	store *store.Store
	actor string
	zones *zone.Manager
	rand  *rand.Rand

	// ai is the breakdown box. It is a field rather than a package call so
	// a test points it at a stand-in rather than at the LAN.
	ai *ai.Client

	lists  []store.List
	tags   []store.Tag // ranked by frequency, with variation
	names  map[string]string
	chosen map[string]bool // tag ids narrowing the view

	list string // the chosen List, or everyList
	sort store.Sort

	// snoozed is whether the view is showing Tasks that are put away. It
	// is off by default, because the main view means what is in front of
	// you; "z" is how a snooze is looked at, and taken off.
	snoozed  bool
	dropdown bool
	expanded bool

	tasks         list.Model
	width, height int
	err           error

	// The palette is the slash commands, the editor is the add and edit
	// screen, and the popup is the List or Tag being created from inside
	// it. At most one of them has the keyboard, in that order outwards.
	palette     list.Model
	paletteOpen bool

	editor *huh.Form
	draft  *draft

	popup *huh.Form
	pop   *popupDraft

	// capturing is a brain dump on its way to the broker: the box that
	// opens the add and edit screens, before the form they fill in.
	capturing *capture

	// bd is a breakdown in progress and ask is a question about the list.
	// Both are the box in src/ai; neither leaves anything behind unless a
	// proposal is approved.
	bd  *breakdown
	ask *inquiry

	// rep is the Scheduling screen: one Task's rule and its dates. It is
	// the only screen that reads a Series, because a Series is the one
	// thing on a Task that is not one of its attributes.
	rep *series

	// files is the file selector, open over the add or edit screen while a
	// person is choosing something to point at.
	files *browser

	// wal is the last write-ahead log token seen, which is how a write made
	// in another process reaches this one.
	wal string
}

// New reads the store once and builds the view. It returns the read's error
// rather than swallowing it, so an unopenable store fails before Bubble Tea
// takes the terminal.
func New(s *store.Store, actor string, r *rand.Rand) (Model, error) {
	m := Model{
		store:  s,
		actor:  actor,
		ai:     ai.New(),
		zones:  zone.New(),
		rand:   r,
		chosen: map[string]bool{},
		names:  map[string]string{},
		list:   everyList,
		sort:   store.SortCreated,
		width:  80,
		height: 24,
	}
	m.tasks = list.New(nil, rowDelegate{zones: m.zones, width: m.width - sidebarWidth}, m.width-sidebarWidth, m.height-4)
	m.tasks.SetShowTitle(false)
	m.tasks.SetShowStatusBar(false)
	m.tasks.SetShowHelp(false)
	m.tasks.SetFilteringEnabled(true)
	m.tasks.SetShowFilter(true)
	// The searchbox gives up "/" to the slash palette and takes "f"; both
	// are the same fuzzy filter, pointed at Tasks and at verbs.
	m.tasks.KeyMap.Filter = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "search"))
	m.palette = newPalette(m.zones, m.width/2, m.height-4)
	m.wal = s.WALToken()
	if err := m.refresh(); err != nil {
		return Model{}, err
	}
	return m, nil
}

// refresh re-reads everything the view shows. Tags are re-ranked on each read,
// so the sidebar varies as the operator asked; the chosen ones are kept by id
// across the re-rank.
func (m *Model) refresh() error {
	lists, err := m.store.Lists()
	if err != nil {
		return err
	}
	tags, err := m.store.Tags()
	if err != nil {
		return err
	}
	tasks, err := m.store.Tasks(store.Query{List: m.list, Sort: m.sort, IncludeSnoozed: m.snoozed})
	if err != nil {
		return err
	}

	m.lists = lists
	m.tags = rankTags(tags, m.rand)
	m.names = map[string]string{}
	for _, l := range lists {
		m.names[l.ID] = l.Name
	}
	for _, t := range tags {
		m.names[t.ID] = t.Name
	}

	items := make([]list.Item, 0, len(tasks))
	// A Task the chosen Tags pass over takes its subtree with it. The store
	// returns the tree depth first, so everything under a dropped Task is the
	// run of rows deeper than it, and cut is where that run began.
	cut := 0
	for _, t := range tasks {
		if cut > 0 && t.Depth > cut {
			continue
		}
		cut = 0
		if !m.carriesChosen(t) {
			cut = t.Depth
			continue
		}
		items = append(items, row{task: t, lists: m.nameEach(t.Lists)})
	}
	m.tasks.SetItems(items)
	return nil
}

// carriesChosen says whether a Task carries every chosen Tag. Narrowing is
// done here rather than in the query because a Tag is not a Query field, and
// the caller drops a Task's subtree along with it: a Subtask left behind by a
// parent the Tags passed over would draw at a Depth under nothing, which is
// the rule the store already keeps for a List and for a Task out of sight.
func (m Model) carriesChosen(t store.Task) bool {
	for id := range m.chosen {
		if !slices.Contains(t.Tags, id) {
			return false
		}
	}
	return true
}

func (m Model) nameEach(ids []string) []string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := m.names[id]; ok {
			names = append(names, name)
		}
	}
	return names
}

// Init starts the watch on the write-ahead log, which is how this process
// learns of a write made by a verb in another terminal.
func (m Model) Init() tea.Cmd { return watch() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// These two belong to the process rather than to whichever screen is
	// open, so they are read before the screens get a look. The tick
	// especially: tea.Tick fires once and the chain only continues because
	// poll returns the next watch, so a tick swallowed by an open form is
	// the last one this process ever sees.
	switch msg := msg.(type) {
	case pollMsg:
		return m.poll()
	case tea.WindowSizeMsg:
		m = m.resize(msg)
	}

	// A screen that is open owns the keyboard, outermost first, so a key
	// typed into the new-List popup never reaches the task list behind it.
	switch {
	case m.files != nil:
		next, cmd := m.updateFiles(msg)
		return next, cmd
	case m.capturing != nil:
		next, cmd := m.updateCapture(msg)
		return next, cmd
	case m.bd != nil:
		next, cmd := m.updateBreakdown(msg)
		return next, cmd
	case m.ask != nil:
		next, cmd := m.updateInquiry(msg)
		return next, cmd
	case m.popup != nil:
		next, cmd := m.updatePopup(msg)
		return next, cmd
	case m.editor != nil:
		next, cmd := m.updateEditor(msg)
		return next, cmd
	case m.rep != nil:
		next, cmd := m.updateRepeat(msg)
		return next, cmd
	case m.paletteOpen:
		next, cmd := m.updatePalette(msg)
		return next, cmd
	}

	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		next, cmd := m.click(msg)
		return next, cmd

	case tea.MouseWheelMsg:
		next, cmd := m.wheel(msg)
		return next, cmd

	case tea.KeyPressMsg:
		// While the searchbox has the keyboard every key belongs to it,
		// including the ones bound below.
		if m.tasks.FilterState() == list.Filtering {
			break
		}
		if handled, next, cmd := m.press(msg); handled {
			return next, cmd
		}
	}

	var cmd tea.Cmd
	m.tasks, cmd = m.tasks.Update(msg)
	return m, cmd
}

// resize fits the main view to the terminal. A screen open over the top keeps
// the width it was built with until it is closed, but the view behind it and
// the next screen built are both right.
func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	m.tasks.SetSize(m.width-sidebarWidth, m.height-4)
	m.tasks.SetDelegate(rowDelegate{zones: m.zones, width: m.width - sidebarWidth})
	m.palette.SetSize(m.width/2, m.height-4)
	return m
}

// press handles the view's own keys. It reports whether it took the key, so
// everything else falls through to the task list.
func (m Model) press(msg tea.KeyPressMsg) (bool, Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return true, m, tea.Quit

	case "/":
		next, cmd := m.openPalette()
		return true, next, cmd

	case "s":
		return true, m.sorted(), nil

	case "z":
		return true, m.showingSnoozed(), nil

	case "L":
		m.dropdown = !m.dropdown
		return true, m, nil

	case "enter":
		if m.dropdown {
			return true, m, nil
		}
		if m.tasks.SelectedItem() != nil {
			m.expanded = !m.expanded
		}
		return true, m, nil

	case "esc":
		switch {
		case m.dropdown:
			m.dropdown = false
		case m.expanded:
			m.expanded = false
		default:
			return false, m, nil
		}
		return true, m, nil
	}
	return false, m, nil
}

// openPalette shows the slash commands, already filtering, so the slash the
// person typed keeps going into the command they meant.
func (m Model) openPalette() (Model, tea.Cmd) {
	m.palette.ResetFilter()
	m.paletteOpen = true
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	return m, cmd
}

// sorted moves to the next order, and showingSnoozed puts the Tasks that are
// away in front of you or takes them back out. Both are here rather than
// inline in press because the footer's keys are clickable and a click has to
// mean exactly what the key means.
func (m Model) sorted() Model {
	m.sort = nextSort(m.sort)
	m.err = m.refresh()
	return m
}

func (m Model) showingSnoozed() Model {
	m.snoozed = !m.snoozed
	m.err = m.refresh()
	return m
}

// searching hands the keyboard to the searchbox, which is what "f" does.
func (m Model) searching() (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.tasks, cmd = m.tasks.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	return m, cmd
}

// click routes a mouse press to whatever was drawn under it. Every hit box is
// a zone marked during the last render, so the layout stays the one authority
// on where things are.
func (m Model) click(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if in(m.zones, "listbox", msg) || in(m.zones, "hint:lists", msg) {
		m.dropdown = !m.dropdown
		return m, nil
	}
	if in(m.zones, "sortbox", msg) || in(m.zones, "hint:sort", msg) {
		return m.sorted(), nil
	}
	if in(m.zones, "hint:snoozed", msg) {
		return m.showingSnoozed(), nil
	}
	if in(m.zones, "hint:commands", msg) {
		return m.openPalette()
	}
	if in(m.zones, "hint:search", msg) {
		return m.searching()
	}
	if in(m.zones, "hint:quit", msg) {
		return m, tea.Quit
	}
	// A Task blown up covers the rows, so the only thing left to click is
	// the way back to them.
	if m.expanded {
		m.expanded = false
		return m, nil
	}
	if m.dropdown {
		if in(m.zones, "list:every", msg) {
			m.list, m.dropdown = everyList, false
			m.err = m.refresh()
			return m, nil
		}
		for _, l := range m.lists {
			if in(m.zones, "list:"+l.ID, msg) {
				m.list, m.dropdown = l.ID, false
				m.err = m.refresh()
				return m, nil
			}
		}
		return m, nil
	}
	for _, t := range m.tags {
		if in(m.zones, "tag:"+t.ID, msg) {
			if m.chosen[t.ID] {
				delete(m.chosen, t.ID)
			} else {
				m.chosen[t.ID] = true
			}
			m.err = m.refresh()
			return m, nil
		}
	}
	// A click puts the cursor on a Task; a second one on the Task already
	// under it blows it up. That is what makes a Task reachable by mouse
	// without a slash command having to be typed at it blind, and it is
	// also how a Task is chosen for one.
	for i, item := range m.tasks.Items() {
		r, ok := item.(row)
		if ok && in(m.zones, "task:"+r.task.ID, msg) {
			m.expanded = i == m.tasks.Index()
			m.tasks.Select(i)
			return m, nil
		}
	}
	return m, nil
}

// wheel scrolls the Tasks. bubbles/list binds keys and not the wheel, so this
// is where a scroll becomes a move of the cursor.
func (m Model) wheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		m.tasks.CursorUp()
	case tea.MouseWheelDown:
		m.tasks.CursorDown()
	}
	return m, nil
}

func in(zones *zone.Manager, id string, msg tea.MouseMsg) bool {
	z := zones.Get(id)
	return z != nil && z.InBounds(msg)
}

func nextSort(current store.Sort) store.Sort {
	for i, s := range store.Sorts {
		if s == current {
			return store.Sorts[(i+1)%len(store.Sorts)]
		}
	}
	return store.Sorts[0]
}

func (m Model) View() tea.View {
	if screen, open := m.screen(); open {
		view := tea.NewView(m.zones.Scan(screen))
		view.AltScreen = true
		view.MouseMode = tea.MouseModeCellMotion
		return view
	}

	// The header and the footer are two lines each, and the dropdown pushes
	// the rest down while it is open. What is left is the Tasks' pane, and
	// the sidebar is drawn to the same height so its rule runs the whole way
	// down rather than stopping under the last Tag.
	header, footer := m.header(), m.footer()
	pane := max(m.height-lines(header)-lines(footer), 1)
	m.tasks.SetSize(m.width-sidebarWidth, pane)

	body := m.tasks.View()
	switch {
	case m.expanded:
		if r, ok := m.tasks.SelectedItem().(row); ok {
			body = m.detail(r)
		}
	case len(m.tasks.Items()) == 0:
		body = m.nothing()
	}
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		sidebarStyle.Height(pane).Render(m.sidebar()), body)

	screen := strings.Join([]string{header, main, footer}, "\n")

	// v2 carries the screen and mouse modes on the View rather than on the
	// program, so the model says what it needs and nothing is toggled behind
	// its back.
	view := tea.NewView(m.zones.Scan(screen))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// header is the List dropdown on the left and the sort on the right, ruled off
// from the Tasks below it. Both are clickable, and the dropdown is drawn open
// under the rule when it is.
func (m Model) header() string {
	name := " ▾ " + m.listName() + " "
	sort := " sort " + string(m.sort) + " "
	gap := max(m.width-lipgloss.Width(name)-lipgloss.Width(sort), 1)

	line := m.zones.Mark("listbox", headerStyle.Render(name)) +
		strings.Repeat(" ", gap) +
		m.zones.Mark("sortbox", dimStyle.Render(sort))
	out := line + "\n" + rule(m.width)
	if !m.dropdown {
		return out
	}
	options := []string{m.option("every", "every List", m.list == everyList)}
	for _, l := range m.lists {
		options = append(options, m.option(l.ID, fmt.Sprintf("%s (%d)", l.Name, l.Count), m.list == l.ID))
	}
	return out + "\n" + boxStyle.Render(strings.Join(options, "\n"))
}

// lines is how many rows something drawn takes up.
func lines(drawn string) int { return strings.Count(drawn, "\n") + 1 }

// rule is the horizontal line between a region and the next one.
func rule(width int) string {
	return ruleStyle.Render(strings.Repeat("─", max(width, 1)))
}

func (m Model) option(id, label string, chosen bool) string {
	if chosen {
		label = chosenStyle.Render("• " + label)
	} else {
		label = "  " + label
	}
	return m.zones.Mark("list:"+id, label)
}

func (m Model) listName() string {
	if m.list == everyList {
		return "every List"
	}
	if name, ok := m.names[m.list]; ok {
		return name
	}
	return m.list
}

// sidebar is every Tag, ranked by how often it is carried with the variation
// rankTags draws in, each one clickable to narrow the view.
func (m Model) sidebar() string {
	lines := []string{headerStyle.Render("Tags"), ""}
	for _, t := range m.tags {
		// The count is what the ranking is by, so it is shown; it is not
		// what the Tag is, so it is not drawn as loudly as the name.
		label := t.Name + dimStyle.Render(fmt.Sprintf(" %d", t.Count))
		if m.chosen[t.ID] {
			label = chosenStyle.Render("✓ "+t.Name) + dimStyle.Render(fmt.Sprintf(" %d", t.Count))
		} else {
			label = "  " + label
		}
		lines = append(lines, m.zones.Mark("tag:"+t.ID, label))
	}
	return strings.Join(lines, "\n")
}

// hints are the footer's keys, in the order a person meets them: what opens
// everything first, then the ways of narrowing, then the way out. Each is
// clickable and does exactly what its key does.
var hints = []struct{ key, what string }{
	{"/", "commands"},
	{"f", "search"},
	{"s", "sort"},
	{"z", "snoozed"},
	{"L", "lists"},
	{"q", "quit"},
}

// footer is the status line: how many Tasks are in front of you, or the last
// error, and then the keys. An error takes the whole line, because a count
// nobody asked about is not worth crowding it with.
func (m Model) footer() string {
	if m.err != nil {
		return rule(m.width) + "\n" + overdueStyle.Render(" "+m.err.Error())
	}
	drawn := make([]string, 0, len(hints)+1)
	drawn = append(drawn, dimStyle.Render(fmt.Sprintf(" %d shown", len(m.tasks.Items()))))
	for _, h := range hints {
		drawn = append(drawn, m.zones.Mark("hint:"+h.what,
			hintKeyStyle.Render(h.key)+hintStyle.Render(" "+h.what)))
	}
	return rule(m.width) + "\n" + strings.Join(drawn, "   ")
}

// nothing is what the pane says when the narrowing has left it empty. A blank
// pane looks broken; this one says which way back out.
func (m Model) nothing() string {
	if len(m.chosen) > 0 || m.list != everyList {
		return dimStyle.Render("\n  Nothing here. Click a chosen Tag to let it go, or L for another List.")
	}
	return dimStyle.Render("\n  Nothing to do. / add is where the first one comes from.")
}

// detail is the whole of a Task: every attribute it carries, its Lists and
// Tags by name, and every pointer it holds. A pointer is shown as it was
// written down; nothing here says whether what it names is still there.
func (m Model) detail(r row) string {
	t := r.task
	lines := []string{
		headerStyle.Render(t.Title) + marks(t),
		rule(m.width - sidebarWidth),
		t.Description,
		"",
	}
	// Labels are padded to one column so the values line up: a Task is read
	// down its values, not across its labels.
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("%-*s  ", labelWidth, label))+value)
		}
	}
	add("why", t.Why)
	add("created", day(t.CreatedAt))
	add("deadline", day(t.Deadline))
	if t.Estimate > 0 {
		add("estimate", t.Estimate.String())
	}
	add("priority", string(t.Priority))
	add("impact", string(t.Impact))
	if !t.SnoozedUntil.IsZero() {
		add("snoozed until", day(t.SnoozedUntil))
	}
	add("color", t.Color)
	add("lists", strings.Join(r.lists, ", "))
	add("tags", strings.Join(m.nameEach(t.Tags), ", "))
	for _, key := range sortedKeys(t.Fields) {
		add(key, t.Fields[key])
	}
	for i, target := range t.Attachments {
		label := "attachments"
		if i > 0 {
			label = strings.Repeat(" ", len(label))
		}
		add(label, target)
	}
	lines = append(lines, "", dimStyle.Render("enter, esc or a click to go back"))
	return strings.Join(lines, "\n")
}

func sortedKeys(fields map[string]string) []string {
	return slices.Sorted(maps.Keys(fields))
}
