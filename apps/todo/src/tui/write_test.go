package tui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

func historyLength(t *testing.T, s *store.Store) int64 {
	t.Helper()
	n, err := s.HistoryLength()
	if err != nil {
		t.Fatalf("HistoryLength: %v", err)
	}
	return n
}

func TestTheFormValidatesWhatItAsksFor(t *testing.T) {
	if err := required("  "); err == nil {
		t.Error("a blank title was accepted")
	}
	// A deadline is picked, not typed, so what the draft carries is only ever
	// what the calendar wrote into it.
	for _, good := range []string{"", "2026-01-02", "2026-01-02 15:04"} {
		if _, err := parseDate(good); err != nil {
			t.Errorf("%q was refused as a date: %v", good, err)
		}
	}
	for _, bad := range []string{"an hour", "90", "-2h"} {
		if err := validDuration(bad); err == nil {
			t.Errorf("%q was accepted as a length of time", bad)
		}
	}
	if _, err := (draft{Title: " "}).attributes(); err == nil {
		t.Error("a draft with no title produced attributes")
	}
}

func TestAddingATaskFromTheTUIWritesItOnce(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	home := idOf(t, "Home", m.lists, m.tags)
	urgent := idOf(t, "urgent", m.lists, m.tags)

	before := historyLength(t, s)
	d := &draft{
		Title:    "Rake the leaves",
		Deadline: "2026-11-01",
		Estimate: "45m",
		Priority: store.LevelLow,
		Lists:    []string{home},
		Tags:     []string{urgent},
	}
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Task Added, Lease Taken, List added, Tag attached, Lease Released.
	if grew := historyLength(t, s) - before; grew != 5 {
		t.Errorf("adding a Task appended %d entries, want 5", grew)
	}

	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	for _, item := range m.tasks.Items() {
		r := item.(row)
		if r.task.Title != "Rake the leaves" {
			continue
		}
		if r.task.Estimate != 45*time.Minute {
			t.Errorf("the estimate came back %v, want 45m", r.task.Estimate)
		}
		if !slices.Contains(r.task.Lists, home) || !slices.Contains(r.task.Tags, urgent) {
			t.Errorf("the new Task carries lists %v and tags %v", r.task.Lists, r.task.Tags)
		}
		return
	}
	t.Fatal("the new Task never came back")
}

func TestAnEditWritesOnlyTheDifferences(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	var roof store.Task
	for _, item := range m.tasks.Items() {
		if r := item.(row); r.task.Title == "Fix the roof" {
			roof = r.task
		}
	}
	d := draftOf(roof)
	d.Title = "Fix the roof properly"

	before := historyLength(t, s)
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Lease Taken, Task Edited, Lease Released, and no membership churn for
	// the Lists and Tags the Task already carried.
	if grew := historyLength(t, s) - before; grew != 3 {
		t.Errorf("an edit that changed a title appended %d entries, want 3", grew)
	}
}

func TestATUIWriteTakesALeaseAndGivesItBack(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	task, _ := m.selected()
	if err := m.lifecycle(task.ID, "delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// A Lease left standing would refuse this one.
	if _, err := s.TakeLease("bob", task.ID, time.Minute); err != nil {
		t.Fatalf("the TUI kept the Lease: %v", err)
	}
}

func TestARefusedWriteSurfacesRatherThanCrashing(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	task, _ := m.selected()
	if _, err := s.TakeLease("bob", task.ID, time.Minute); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}

	err := m.lifecycle(task.ID, "complete")
	if !errors.Is(err, store.ErrHeld) {
		t.Fatalf("writing under someone else's Lease returned %v, want ErrHeld", err)
	}
	if strings.Contains(err.Error(), "bob") {
		t.Errorf("the refusal named the holder: %v", err)
	}
}

// TestTheTUIHoldsNoTransactionWhileTheFormIsOpen is the rule the whole store
// design rests on. The add screen is open and waiting on a person; a verb run
// in another process must not queue behind it.
func TestTheTUIHoldsNoTransactionWhileTheFormIsOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")

	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	m := newModel(t, s)
	m, _ = m.run("/add")
	if m.editor == nil {
		t.Fatal("/add did not open the form")
	}
	m = press(m, "R")
	m = press(m, "a")

	other, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = other.Close() }()

	done := make(chan error, 1)
	go func() {
		_, err := other.AddTask("bob", store.Attributes{Title: store.Set("from a verb")})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a verb writing while the form was open failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a verb writing while the form was open queued behind it")
	}
}

// TestTheWatcherNoticesAnotherProcess covers the poll on the write-ahead log,
// which is the only way this program hears about a write it did not make.
func TestTheWatcherNoticesAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")

	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	m := newModel(t, s)
	if len(m.tasks.Items()) != 0 {
		t.Fatalf("the view started with %d Tasks, want none", len(m.tasks.Items()))
	}

	other, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = other.Close() }()
	if _, err := other.AddTask("bob", store.Attributes{Title: store.Set("from a verb")}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	m, _ = m.poll()
	if got := titles(m); len(got) != 1 || got[0] != "from a verb" {
		t.Errorf("after a poll the view held %v, want the other process's Task", got)
	}
}

func TestThePaletteRunsTheChosenCommand(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	// A Task with an open Subtask cannot complete, so the cursor goes to a
	// leaf first.
	for {
		task, ok := m.selected()
		if ok && task.Title == "Buy apples" {
			break
		}
		m = press(m, "j")
	}
	task, _ := m.selected()

	m = press(m, "/")
	if !m.paletteOpen {
		t.Fatal("/ did not open the palette")
	}
	for _, key := range []string{"c", "o", "m", "p"} {
		m = press(m, key)
	}
	if c, ok := m.palette.SelectedItem().(command); !ok || c.name != "/complete" {
		t.Fatalf("typing comp selected %v, want /complete", m.palette.SelectedItem())
	}
	m = press(m, "enter")
	if m.paletteOpen {
		t.Error("running a command left the palette open")
	}
	if m.err != nil {
		t.Fatalf("/complete reported %v", m.err)
	}

	done, err := s.Tasks(store.Query{IncludeCompleted: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	for _, got := range done {
		if got.ID == task.ID && got.CompletedAt.IsZero() {
			t.Error("/complete did not complete the Task under the cursor")
		}
	}
}

func TestCreatingAListFromTheFormKeepsTheDraft(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	m, _ = m.run("/add")
	m.draft.Title = "Half typed"

	m.pop = &popupDraft{Noun: "List", Name: "Errands", Colour: "amber"}
	m, _ = m.createCollection()

	if m.err != nil {
		t.Fatalf("creating a List from the form: %v", m.err)
	}
	if m.draft.Title != "Half typed" {
		t.Errorf("the draft came back as %q, want what was typed", m.draft.Title)
	}
	if m.editor == nil {
		t.Error("the form did not come back after the popup")
	}

	errands := idOf(t, "Errands", m.lists, m.tags)
	if !slices.Contains(m.draft.Lists, errands) {
		t.Errorf("the draft carries %v, want the List it just made", m.draft.Lists)
	}
}

// What is typed into a popup reaches the write. The form holds the address of
// a field on the Model, and the Model is a value: a popup bound to a field on
// the copy that opened it would swallow every keystroke silently.
func TestWhatIsTypedIntoThePopupIsWhatIsCreated(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	m, cmd := m.run("/list")
	m = send(m, cmd())
	for _, key := range []string{"E", "r", "r", "a", "n", "d", "s", "enter", "enter"} {
		m = press(m, key)
	}
	if m.err != nil {
		t.Fatalf("creating a List: %v", m.err)
	}
	for _, l := range m.lists {
		if l.Name == "Errands" {
			return
		}
	}
	t.Errorf("no List named Errands: %v", m.lists)
}

func TestSnoozeHidesTheTask(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)
	task, _ := m.selected()

	m, _ = m.run("/snooze 1 day")
	if m.err != nil {
		t.Fatalf("snoozing: %v", m.err)
	}
	for _, item := range m.tasks.Items() {
		if item.(row).task.ID == task.ID {
			t.Error("a snoozed Task is still in the view")
		}
	}
}

// TestEmojiAndNerdGlyphsKeepTheirWidth measures what the row drawing measures.
// Bubble Tea v2 negotiates Unicode mode 2027 with the terminal, so the cells a
// glyph draws in and the cells this counts are the same ones.
func TestEmojiAndNerdGlyphsKeepTheirWidth(t *testing.T) {
	if got := lipgloss.Width(paperclip); got != 2 {
		t.Errorf("the paperclip measured %d cells, want 2", got)
	}
	if got := lipgloss.Width(""); got != 1 {
		t.Errorf("a nerd font glyph measured %d cells, want 1", got)
	}

	const width = 24
	lines := describe(strings.Repeat("📎 ", 40), width)
	for _, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("a wrapped line measured %d cells, want at most %d", got, width)
		}
	}
}

var _ tea.Model = Model{}

// TestTheWatchSurvivesAnOpenScreen is the whole watch, not the main view's.
// tea.Tick fires once and the chain only continues because poll returns the
// next watch: a tick swallowed by an open form was the last one the process
// ever saw, and it then never heard about another terminal's write again.
func TestTheWatchSurvivesAnOpenScreen(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	m, _ = m.run("/add")
	if m.editor == nil {
		t.Fatal("/add did not open the form")
	}

	next, cmd := m.Update(pollMsg{})
	m = next.(Model)
	if m.editor == nil {
		t.Error("the poll closed the form it arrived over")
	}
	if cmd == nil {
		t.Fatal("a poll that arrived over an open screen scheduled no next watch")
	}
	if _, ok := cmd().(pollMsg); !ok {
		t.Error("what the poll scheduled was not the next watch")
	}
}

// A Task's Fields are the one attribute with no widget of its own: they are
// typed as lines and read back as pairs. The round trip matters more than the
// parse, because a key left out of an edit is left alone by the store and
// would otherwise be impossible to take off from the screen.
func TestFieldsTypedIntoTheFormReachTheStoreAndCanBeTakenOff(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	d := &draft{Title: "File the tax return", Fields: "repo: todo\npr: 42"}
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	task, ok := taskNamed(m, "File the tax return")
	if !ok {
		t.Fatal("the Task was not on the screen after it was added")
	}
	if task.Fields["repo"] != "todo" || task.Fields["pr"] != "42" {
		t.Fatalf("the fields came back %v, want both pairs", task.Fields)
	}

	// Reopening the Task shows them back as lines, and deleting one line is
	// how that key comes off.
	edit := draftOf(task)
	if edit.Fields != "pr: 42\nrepo: todo" {
		t.Errorf("the edit screen showed %q, want a line each in key order", edit.Fields)
	}
	edit.Fields = "repo: todo"
	if err := m.save(edit); err != nil {
		t.Fatalf("save the edit: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	task, _ = taskNamed(m, "File the tax return")
	if len(task.Fields) != 1 || task.Fields["repo"] != "todo" {
		t.Errorf("the fields came back %v, want only the one that was left", task.Fields)
	}
}

// taskNamed finds a Task on the screen by title.
func taskNamed(m Model, title string) (store.Task, bool) {
	for _, item := range m.tasks.Items() {
		if r, ok := item.(row); ok && r.task.Title == title {
			return r.task, true
		}
	}
	return store.Task{}, false
}

func TestABadFieldLineIsRefusedBeforeItIsWritten(t *testing.T) {
	for _, bad := range []string{"repo", ": todo", "repo: a\nrepo: b"} {
		if err := validFields(bad); err == nil {
			t.Errorf("%q was accepted as fields", bad)
		}
	}
	if fields, err := parseFields("  \nrepo:  todo  \n"); err != nil ||
		len(fields) != 1 || fields["repo"] != "todo" {
		t.Errorf("parseFields = %v, %v; want the one trimmed pair", fields, err)
	}
}

// Scheduling had no way in from the screen: the store computed Occurrences and
// only a verb could mark one. This is the screen reaching them.
func TestTheScheduleScreenMarksADate(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	apples, ok := taskNamed(m, "Buy apples")
	if !ok {
		t.Fatal("the fixture Task was not on the screen")
	}
	carry(t, s, apples.ID, func() error {
		_, err := s.Repeat("alice", apples.ID, "every week")
		return err
	})

	m.tasks.Select(indexOf(t, m, apples.ID))
	m, _ = m.run("/repeat")
	if m.rep == nil {
		t.Fatal("/repeat opened nothing")
	}
	if m.rep.form != nil {
		t.Fatal("a Task that already repeats opened on the rule form")
	}
	if len(m.rep.dates) == 0 {
		t.Fatal("the screen showed no dates for a weekly rule")
	}
	first := m.rep.dates[0].Date

	before := historyLength(t, s)
	m = press(m, "t")
	// Lease Taken, the tick, Lease Released.
	if grew := historyLength(t, s) - before; grew != 3 {
		t.Errorf("ticking a date appended %d entries, want 3", grew)
	}
	if m.err != nil {
		t.Fatalf("ticking: %v", m.err)
	}
	if m.rep.dates[0].State != store.Ticked || !m.rep.dates[0].Date.Equal(first) {
		t.Errorf("the first date came back %+v, want it ticked", m.rep.dates[0])
	}

	// Skipping the one below it leaves the ticked one alone.
	m = press(m, "j")
	m = press(m, "s")
	if m.err != nil {
		t.Fatalf("skipping: %v", m.err)
	}
	if m.rep.dates[1].State != store.Skipped || m.rep.dates[0].State != store.Ticked {
		t.Errorf("the dates came back %+v, want the first ticked and the second skipped",
			m.rep.dates[:2])
	}
}

// Detaching lifts a date out into an ordinary Task, so the Series no longer
// produces it and there is nothing left on this screen that is about it.
func TestDetachingADateFromTheScreenClosesIt(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	apples, _ := taskNamed(m, "Buy apples")
	carry(t, s, apples.ID, func() error {
		_, err := s.Repeat("alice", apples.ID, "every week")
		return err
	})
	m.tasks.Select(indexOf(t, m, apples.ID))
	m, _ = m.run("/repeat")

	m = press(m, "d")
	if m.err != nil {
		t.Fatalf("detaching: %v", m.err)
	}
	if m.rep != nil {
		t.Error("the screen stayed open on a date it no longer holds")
	}
	// The detached date is an ordinary Task now, carrying the copy of the
	// attributes it was lifted with, so there are two of that title.
	var copies int
	for _, item := range m.tasks.Items() {
		if item.(row).task.Title == "Buy apples" {
			copies++
		}
	}
	if copies != 2 {
		t.Errorf("%d Tasks named Buy apples, want the recurring one and the detached date", copies)
	}
}

func TestARuleIsCheckedBeforeItIsWritten(t *testing.T) {
	for _, bad := range []string{"", "  ", "every purple"} {
		if err := validRule(bad); err == nil {
			t.Errorf("%q was accepted as a rule", bad)
		}
	}
	if err := validRule("every 2 weeks on mon,thu"); err != nil {
		t.Errorf("a good rule was refused: %v", err)
	}
}

func indexOf(t *testing.T, m Model, id string) int {
	t.Helper()
	for i, item := range m.tasks.Items() {
		if r, ok := item.(row); ok && r.task.ID == id {
			return i
		}
	}
	t.Fatalf("%s is not on the screen", id)
	return 0
}

// A Series whose rule has run out draws no dates, and there is nothing on that
// screen to move the cursor onto or to mark.
func TestAScheduleWithNoDatesLeftTakesNoMark(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	apples, _ := taskNamed(m, "Buy apples")
	carry(t, s, apples.ID, func() error {
		_, err := s.Repeat("alice", apples.ID, "every week from 2020-01-01 until 2020-06-01")
		return err
	})
	m.tasks.Select(indexOf(t, m, apples.ID))
	m, _ = m.run("/repeat")
	if m.rep == nil || len(m.rep.dates) != 0 {
		t.Fatalf("the screen opened on %d dates, want a rule that has run out", len(m.rep.dates))
	}

	// The order matters: moving down and then marking is what reaches a
	// date, and a cursor allowed below zero reaches one that is not there.
	before := historyLength(t, s)
	for _, key := range []string{"j", "t", "j", "s", "j", "d", "k", "t"} {
		m = press(m, key)
		if m.err != nil {
			t.Fatalf("%q on an empty schedule: %v", key, m.err)
		}
		if m.rep != nil && m.rep.cursor < 0 {
			t.Fatalf("%q put the cursor at %d", key, m.rep.cursor)
		}
	}
	if m.rep == nil {
		t.Error("the screen closed on a date it never had")
	}
	if grew := historyLength(t, s) - before; grew != 0 {
		t.Errorf("marking nothing appended %d entries", grew)
	}
}
