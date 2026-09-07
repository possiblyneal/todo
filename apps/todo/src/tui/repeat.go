package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/possiblyneal/todo/apps/todo/src/schedule"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// horizon is how far ahead the screen asks for dates. A Series is unbounded
// unless its rule says otherwise, so something has to say where the list
// stops; two years is far enough that a yearly rule shows more than one.
const horizon = 2

// shown is how many of them are drawn. The rest are still there and the rule
// still produces them; this is a screen, not the Series.
const shown = 12

// series is the Scheduling screen: one Task's rule and the dates it produces,
// with what a person can do to one of them. It holds no Occurrence rows,
// because there are none to hold -- the dates are computed on every read, and
// re-read after every mark.
//
// It is a pointer on the Model for the reason every screen here is: Model is a
// value, and a form handed the address of a field on a copy writes into that
// copy.
type series struct {
	task    store.Task
	rule    schedule.Rule
	repeats bool
	dates   []store.Occurrence
	cursor  int

	// form is the rule being written, over the top of the dates. Scheduling
	// has one value to edit and it is a sentence, so it is one input rather
	// than a screen of widgets.
	form *huh.Form
	text string
}

// startRepeat opens Scheduling on the Task under the cursor. A Task that does
// not repeat opens on the rule, because that is the only thing there is to do
// to it.
func (m Model) startRepeat() (Model, tea.Cmd) {
	t, ok := m.selected()
	if !ok {
		return m, nil
	}
	sr := &series{task: t}
	if err := m.load(sr); err != nil {
		m.err = err
		return m, nil
	}
	m.rep = sr
	if !sr.repeats {
		return m.editRule()
	}
	return m, nil
}

// load re-reads the rule and the dates it produces. Every mark ends here, so
// what is drawn is what the store computes rather than what the screen
// guessed.
func (m Model) load(sr *series) error {
	rule, repeats, err := m.store.Rule(sr.task.ID)
	if err != nil {
		return err
	}
	sr.rule, sr.repeats, sr.dates = rule, repeats, nil
	if !repeats {
		return nil
	}
	// Local, not UTC: schedule reads a date's year, month and day in the
	// local zone, so a UTC instant west of Greenwich after evening reads as
	// tomorrow and drops today's Occurrence off the screen.
	from := time.Now()
	dates, err := m.store.Occurrences(sr.task.ID, from, from.AddDate(horizon, 0, 0))
	if err != nil {
		return err
	}
	sr.dates = dates
	sr.cursor = min(sr.cursor, max(len(dates)-1, 0))
	return nil
}

// editRule opens the one input the rule is written in.
func (m Model) editRule() (Model, tea.Cmd) {
	m.rep.text = ""
	if m.rep.repeats {
		m.rep.text = m.rep.rule.String()
	}
	m.rep.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Repeats").Value(&m.rep.text).
			Placeholder("every 2 weeks on mon,thu").Validate(validRule),
	)).WithWidth(min(m.width-8, 56)).WithHeight(7)
	return m, m.rep.form.Init()
}

func validRule(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("say how often, or esc to leave it alone")
	}
	_, err := schedule.Parse(v)
	return err
}

// updateRepeat gives the Scheduling screen the keyboard. The rule form has it
// while it is open; otherwise the keys are the marks, one per date.
func (m Model) updateRepeat(msg tea.Msg) (Model, tea.Cmd) {
	if m.rep.form != nil {
		return m.updateRule(msg)
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		m.rep = nil
		return m, nil
	case "up", "k":
		m.rep.cursor = max(m.rep.cursor-1, 0)
		return m, nil
	case "down", "j":
		// A rule that has run out draws no dates, and the last row of
		// none of them is not row -1.
		m.rep.cursor = max(min(m.rep.cursor+1, m.drawn()-1), 0)
		return m, nil
	case "r":
		return m.editRule()
	case "x":
		return m.repeatWrite(func(id string) error { return m.store.Unrepeat(m.actor, id) })
	case "t", "s", "d":
		return m.mark(key.String())
	}
	return m, nil
}

// drawn is how many dates are on the screen, which is what the cursor moves
// over.
func (m Model) drawn() int { return min(len(m.rep.dates), shown) }

// mark puts one of the three marks on the date under the cursor. Detaching
// closes the screen: the date is an ordinary Task now and the Series no longer
// produces it, so there is nothing left here that is about it.
func (m Model) mark(what string) (Model, tea.Cmd) {
	if m.rep.cursor < 0 || m.rep.cursor >= m.drawn() {
		return m, nil
	}
	on := m.rep.dates[m.rep.cursor].Date
	switch what {
	case "t":
		return m.repeatWrite(func(id string) error { return m.store.TickOccurrence(m.actor, id, on) })
	case "s":
		return m.repeatWrite(func(id string) error { return m.store.SkipOccurrence(m.actor, id, on) })
	}
	next, cmd := m.repeatWrite(func(id string) error {
		_, err := m.store.DetachOccurrence(m.actor, id, on)
		return err
	})
	if next.err == nil {
		next.rep = nil
	}
	return next, cmd
}

// repeatWrite runs one Scheduling write under the Lease covering the Task's tree,
// then re-reads both the dates and the main view: a detached date is a new
// Task, and a ticked one may have moved the Task's own deadline.
func (m Model) repeatWrite(do func(id string) error) (Model, tea.Cmd) {
	id := m.rep.task.ID
	if err := m.store.WithLease(m.actor, id, store.WriteTTL, func() error { return do(id) }); err != nil {
		m.err = err
		return m, nil
	}
	if err := m.load(m.rep); err != nil {
		m.err = err
		return m, nil
	}
	m.err = m.refresh()
	return m, nil
}

// updateRule gives the rule input the keyboard, and writes the Series when it
// says it is complete.
func (m Model) updateRule(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" {
		m.rep.form = nil
		if !m.rep.repeats {
			m.rep = nil
		}
		return m, nil
	}

	form, cmd := m.rep.form.Update(msg)
	m.rep.form, _ = form.(*huh.Form)
	switch m.rep.form.State {
	case huh.StateCompleted:
		rule := m.rep.text
		m.rep.form = nil
		return m.repeatWrite(func(id string) error {
			_, err := m.store.Repeat(m.actor, id, rule)
			return err
		})
	case huh.StateAborted:
		m.rep.form = nil
		if !m.rep.repeats {
			m.rep = nil
		}
	}
	return m, cmd
}

// repeatView draws the rule and the dates it produces, each with whatever mark
// it carries.
func (m Model) repeatView() string {
	sr := m.rep
	rule := "does not repeat"
	if sr.repeats {
		rule = sr.rule.String()
	}
	lines := []string{
		titleStyle.Render(sr.task.Title),
		"",
		dimStyle.Render("repeats: ") + rule,
		"",
	}
	if sr.repeats && len(sr.dates) == 0 {
		lines = append(lines, dimStyle.Render("no dates in the next two years"))
	}
	for i, o := range sr.dates[:m.drawn()] {
		line := "  " + o.Date.Format(time.DateOnly)
		if o.State != store.Pending {
			line += "  (" + string(o.State) + ")"
		}
		if i == sr.cursor {
			line = selectedStyle.Render("▸ " + strings.TrimSpace(line))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "",
		dimStyle.Render("t tick   s skip   d detach   r change the rule   x stop repeating   esc back"))
	return strings.Join(lines, "\n")
}
