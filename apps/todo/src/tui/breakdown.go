package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// breakdownTTL is how long the Lease over the tree is held. A breakdown is
// think-time by construction: the box asks, a person answers, and the
// proposals are read before any of them is approved. Holding the whole
// top-level tree for that long is the cost ADR 0002 names by name, and it is
// paid deliberately here rather than worked around.
const breakdownTTL = 10 * time.Minute

// breakdown is one interaction with the box. Everything in it dies when it
// does: the questions, the answers, and above all the proposals, which are
// values on this struct and become Tasks only where somebody ticks them.
type breakdown struct {
	task store.Task
	// root is the top-level Task whose tree is leased for the duration.
	root string

	answers   []ai.QA
	asking    []string
	replies   []string
	proposals []ai.Proposal
	approved  []string

	form    *huh.Form
	waiting bool
}

// stepMsg is a turn coming back from the box, off the event loop. The call
// takes as long as it takes and the view stays drawn while it does.
type stepMsg struct {
	step ai.Step
	err  error
}

// inquiry is `/ask`: a question about the list, and the prose that came back.
// It writes nothing and takes no Lease, because a question is a read.
type inquiry struct {
	Question string
	Answer   string
	form     *huh.Form
	waiting  bool
}

type answerMsg struct {
	answer string
	err    error
}

// startBreakdown takes the Lease over the Task's whole tree and asks the box
// for a first turn. A tree somebody else holds refuses here, before any of
// this is on screen.
func (m Model) startBreakdown() (Model, tea.Cmd) {
	t, ok := m.selected()
	if !ok {
		return m, nil
	}
	root, err := m.store.RootOf(t.ID)
	if err != nil {
		m.err = err
		return m, nil
	}
	if _, err := m.store.TakeLease(m.actor, root, breakdownTTL); err != nil {
		m.err = err
		return m, nil
	}
	m.bd = &breakdown{task: t, root: root, waiting: true}
	return m, m.turn(m.bd)
}

// turn asks the box for the next step, carrying everything answered so far.
func (m Model) turn(bd *breakdown) tea.Cmd {
	client, task, answers := m.ai, brief(bd.task), append([]ai.QA(nil), bd.answers...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		step, err := client.Breakdown(ctx, task, answers)
		return stepMsg{step: step, err: err}
	}
}

// updateBreakdown drives the interaction. Escape ends it at any point, and
// ending it releases the Lease and leaves nothing behind.
func (m Model) updateBreakdown(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "esc" || (m.bd.waiting && msg.String() == "ctrl+c") {
			return m.endBreakdown(), nil
		}

	case stepMsg:
		m.bd.waiting = false
		if msg.err != nil {
			m.err = msg.err
			return m.endBreakdown(), nil
		}
		switch {
		case len(msg.step.Proposals) > 0:
			m.bd.proposals = msg.step.Proposals
			m.bd.approved = titlesOf(msg.step.Proposals)
			m.bd.form = m.approvalForm(m.bd)
		case len(msg.step.Questions) > 0:
			m.bd.asking = msg.step.Questions
			m.bd.replies = make([]string, len(msg.step.Questions))
			m.bd.form = m.questionForm(m.bd)
		default:
			m.err = fmt.Errorf("the box had nothing to ask and nothing to propose")
			return m.endBreakdown(), nil
		}
		return m, m.bd.form.Init()
	}

	if m.bd.waiting || m.bd.form == nil {
		return m, nil
	}
	form, cmd := m.bd.form.Update(msg)
	m.bd.form, _ = form.(*huh.Form)
	switch m.bd.form.State {
	case huh.StateCompleted:
		if len(m.bd.proposals) > 0 {
			return m.approve()
		}
		// Answering is the person's half of a turn: what they said goes
		// back with the next one.
		for i, q := range m.bd.asking {
			m.bd.answers = append(m.bd.answers, ai.QA{Question: q, Answer: m.bd.replies[i]})
		}
		m.bd.asking, m.bd.replies, m.bd.form, m.bd.waiting = nil, nil, nil, true
		return m, m.turn(m.bd)
	case huh.StateAborted:
		return m.endBreakdown(), nil
	}
	return m, cmd
}

// approve writes the ticked proposals as Subtasks and nothing else. The Lease
// over the tree is the one taken when the breakdown started, so this is the
// only surface that writes without taking one of its own.
func (m Model) approve() (Model, tea.Cmd) {
	bd := m.bd
	for _, p := range bd.proposals {
		if !contains(bd.approved, p.Title) {
			continue
		}
		if _, err := m.store.AddSubtask(m.actor, bd.task.ID, proposed(p)); err != nil {
			m.err = err
			break
		}
	}
	next := m.endBreakdown()
	if next.err == nil {
		next.err = next.refresh()
	}
	return next, nil
}

// endBreakdown gives the Lease back and forgets everything. A proposal nobody
// ticked leaves no trace at all: it was a value on a struct that is now gone.
func (m Model) endBreakdown() Model {
	if m.bd != nil {
		_ = m.store.ReleaseLease(m.actor, m.bd.root)
		m.bd = nil
	}
	return m
}

// questionForm puts what the box still needs to know to the person.
func (m Model) questionForm(bd *breakdown) *huh.Form {
	fields := make([]huh.Field, 0, len(bd.asking))
	for i, q := range bd.asking {
		fields = append(fields, huh.NewInput().Title(q).Value(&bd.replies[i]))
	}
	return huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(min(m.width-8, 72)).WithHeight(max(m.height-8, 8))
}

// approvalForm is the gate. Every proposal starts ticked and unticking one is
// how it is declined; submitting writes the ticked ones and no others.
func (m Model) approvalForm(bd *breakdown) *huh.Form {
	options := make([]huh.Option[string], 0, len(bd.proposals))
	for _, p := range bd.proposals {
		options = append(options, huh.NewOption(describeProposal(p), p.Title).Selected(true))
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Subtasks of " + bd.task.Title).
			Description("Untick anything you do not want. Nothing is written until you submit.").
			Value(&bd.approved).Height(rows(len(options)) + 2).Options(options...),
	)).WithWidth(min(m.width-8, 72)).WithHeight(max(m.height-8, 10))
}

// startInquiry opens the question box for `/ask`.
func (m Model) startInquiry() (Model, tea.Cmd) {
	m.ask = &inquiry{}
	m.ask.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Ask about the list").Value(&m.ask.Question).Validate(required),
	)).WithWidth(min(m.width-8, 64)).WithHeight(7)
	return m, m.ask.form.Init()
}

// updateInquiry sends the question with the list attached and shows what comes
// back. Nothing here writes, which is why no Lease is involved.
func (m Model) updateInquiry(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" {
		m.ask = nil
		return m, nil
	}
	if answer, ok := msg.(answerMsg); ok {
		m.ask.waiting = false
		if answer.err != nil {
			m.err = answer.err
			m.ask = nil
			return m, nil
		}
		m.ask.Answer = answer.answer
		return m, nil
	}
	if m.ask.waiting {
		return m, nil
	}
	if m.ask.Answer != "" {
		// The answer is read and dismissed; any key closes it.
		if _, ok := msg.(tea.KeyPressMsg); ok {
			m.ask = nil
		}
		return m, nil
	}

	form, cmd := m.ask.form.Update(msg)
	m.ask.form, _ = form.(*huh.Form)
	switch m.ask.form.State {
	case huh.StateCompleted:
		m.ask.waiting = true
		return m, m.enquire(m.ask.Question)
	case huh.StateAborted:
		m.ask = nil
	}
	return m, cmd
}

// enquire sends the question and the Tasks in view. The box holds nothing
// between calls, so the list goes with every question.
func (m Model) enquire(question string) tea.Cmd {
	client := m.ai
	list := make([]ai.Brief, 0, len(m.tasks.Items()))
	for _, item := range m.tasks.Items() {
		list = append(list, brief(item.(row).task))
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		answer, err := client.Ask(ctx, question, list)
		return answerMsg{answer: answer, err: err}
	}
}

// brief is a Task as the box is shown it. No id crosses the wire.
func brief(t store.Task) ai.Brief {
	b := ai.Brief{
		Title:       t.Title,
		Description: t.Description,
		Why:         t.Why,
		Priority:    string(t.Priority),
		Impact:      string(t.Impact),
	}
	if !t.Deadline.IsZero() {
		b.Deadline = t.Deadline.Local().Format(dateLayouts[1])
	}
	if t.Estimate > 0 {
		b.Estimate = t.Estimate.String()
	}
	return b
}

// proposed turns an approved proposal into the attributes a Subtask is written
// with. An attribute the box wrote in a way this program cannot read is
// dropped rather than refused: the person approved a Task, not a guess at how
// long it takes.
func proposed(p ai.Proposal) store.Attributes {
	a := store.Attributes{
		Title:       store.Set(strings.TrimSpace(p.Title)),
		Description: store.Set(p.Description),
		Why:         store.Set(p.Why),
	}
	if d, err := parseDuration(p.Estimate); err == nil && d > 0 {
		a.Estimate = store.Set(d)
	}
	if l, err := store.ParseLevel(p.Priority); err == nil && l != "" {
		a.Priority = store.Set(l)
	}
	if l, err := store.ParseLevel(p.Impact); err == nil && l != "" {
		a.Impact = store.Set(l)
	}
	return a
}

func titlesOf(proposals []ai.Proposal) []string {
	titles := make([]string, 0, len(proposals))
	for _, p := range proposals {
		titles = append(titles, p.Title)
	}
	return titles
}

// describeProposal is the one line a proposal is approved or declined on.
func describeProposal(p ai.Proposal) string {
	line := p.Title
	var extra []string
	if p.Estimate != "" {
		extra = append(extra, p.Estimate)
	}
	if p.Priority != "" {
		extra = append(extra, "priority "+p.Priority)
	}
	if p.Impact != "" {
		extra = append(extra, "impact "+p.Impact)
	}
	if len(extra) > 0 {
		line += "  (" + strings.Join(extra, ", ") + ")"
	}
	return line
}

// breakdownView draws whichever half of the interaction is current: the box
// being waited on, the questions, or the proposals waiting to be approved.
func (m Model) breakdownView() string {
	head := headerStyle.Render("Breakdown: " + m.bd.task.Title)
	switch {
	case m.bd.waiting:
		return head + "\n\n" + dimStyle.Render("asking the box… esc to give up")
	case m.bd.form != nil:
		return head + "\n\n" + m.bd.form.View()
	}
	return head
}

// inquiryView draws the question, or the prose that came back.
func (m Model) inquiryView() string {
	switch {
	case m.ask.waiting:
		return dimStyle.Render("asking the box… esc to give up")
	case m.ask.Answer != "":
		return headerStyle.Render(m.ask.Question) + "\n\n" +
			lipgloss.NewStyle().Width(min(m.width-8, 72)).Render(m.ask.Answer) +
			"\n\n" + dimStyle.Render("any key to close")
	}
	return m.ask.form.View()
}
