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
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true)
	chosenStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().Width(sidebarWidth).PaddingRight(1)
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

	list     string // the chosen List, or everyList
	sort     store.Sort
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
	m.palette = newPalette(m.width/2, m.height-4)
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
	tasks, err := m.store.Tasks(store.Query{List: m.list, Sort: m.sort})
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
	case m.bd != nil:
		next, cmd := m.updateBreakdown(msg)
		return next, cmd
	case m.ask != nil:
		next, cmd := m.updateInquiry(msg)
		return next, cmd
	case m.rep != nil:
		next, cmd := m.updateRepeat(msg)
		return next, cmd
	case m.popup != nil:
		next, cmd := m.updatePopup(msg)
		return next, cmd
	case m.editor != nil:
		next, cmd := m.updateEditor(msg)
		return next, cmd
	case m.paletteOpen:
		next, cmd := m.updatePalette(msg)
		return next, cmd
	}

	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		next, cmd := m.click(msg)
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
		// The palette opens already filtering, so the slash the person
		// typed keeps going into the command they meant.
		m.palette.ResetFilter()
		m.paletteOpen = true
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		return true, m, cmd

	case "s":
		m.sort = nextSort(m.sort)
		m.err = m.refresh()
		return true, m, nil

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

// click routes a mouse press to whatever was drawn under it. Every hit box is
// a zone marked during the last render, so the layout stays the one authority
// on where things are.
func (m Model) click(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if in(m.zones, "listbox", msg) {
		m.dropdown = !m.dropdown
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
	for i, item := range m.tasks.Items() {
		r, ok := item.(row)
		if ok && in(m.zones, "task:"+r.task.ID, msg) {
			m.tasks.Select(i)
			m.expanded = true
			return m, nil
		}
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

	body := m.tasks.View()
	if m.expanded {
		if r, ok := m.tasks.SelectedItem().(row); ok {
			body = m.detail(r)
		}
	}
	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebarStyle.Render(m.sidebar()), body)

	screen := strings.Join([]string{m.header(), main, m.footer()}, "\n")

	// v2 carries the screen and mouse modes on the View rather than on the
	// program, so the model says what it needs and nothing is toggled behind
	// its back.
	view := tea.NewView(m.zones.Scan(screen))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// header is the List dropdown and the sort, with the dropdown drawn open
// underneath when it is.
func (m Model) header() string {
	line := m.zones.Mark("listbox", headerStyle.Render("▾ "+m.listName())) +
		dimStyle.Render("   sort "+string(m.sort)+"   / commands   f search   s sort   L lists   q quit")
	if !m.dropdown {
		return line
	}
	options := []string{m.option("every", "every List", m.list == everyList)}
	for _, l := range m.lists {
		options = append(options, m.option(l.ID, fmt.Sprintf("%s (%d)", l.Name, l.Count), m.list == l.ID))
	}
	return line + "\n" + boxStyle.Render(strings.Join(options, "\n"))
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
	lines := []string{headerStyle.Render("Tags")}
	for _, t := range m.tags {
		label := fmt.Sprintf("%s %d", t.Name, t.Count)
		if m.chosen[t.ID] {
			label = chosenStyle.Render("✓ " + label)
		} else {
			label = "  " + label
		}
		lines = append(lines, m.zones.Mark("tag:"+t.ID, label))
	}
	return strings.Join(lines, "\n")
}

func (m Model) footer() string {
	if m.err != nil {
		return overdueStyle.Render(m.err.Error())
	}
	return dimStyle.Render(fmt.Sprintf("%d shown", len(m.tasks.Items())))
}

// detail is the whole of a Task: every attribute it carries, its Lists and
// Tags by name, and every pointer it holds. A pointer is shown as it was
// written down; nothing here says whether what it names is still there.
func (m Model) detail(r row) string {
	t := r.task
	lines := []string{
		titleStyle.Render(t.Title) + marks(t),
		"",
		t.Description,
		"",
	}
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, dimStyle.Render(label+": ")+value)
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
	add("colour", t.Colour)
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
	lines = append(lines, "", dimStyle.Render("enter or esc to go back"))
	return strings.Join(lines, "\n")
}

func sortedKeys(fields map[string]string) []string {
	return slices.Sorted(maps.Keys(fields))
}
