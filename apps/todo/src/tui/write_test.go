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
	m = addScreen(m)
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

// /decline is the palette's other ending, and it takes the same Lease the
// other lifecycle commands do.
func TestThePaletteDeclinesTheTaskUnderTheCursor(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	// Declining refuses over an open child, so the cursor goes to a leaf.
	for {
		task, ok := m.selected()
		if ok && task.Title == "Buy apples" {
			break
		}
		m = press(m, "j")
	}
	task, _ := m.selected()

	m, _ = m.run("/decline")
	if m.err != nil {
		t.Fatalf("/decline reported %v", m.err)
	}

	declined, err := s.Tasks(store.Query{IncludeDeclined: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	var seen bool
	for _, got := range declined {
		if got.ID != task.ID {
			continue
		}
		seen = true
		if got.DeclinedAt.IsZero() {
			t.Error("/decline did not decline the Task under the cursor")
		}
		if !got.CompletedAt.IsZero() {
			t.Error("/decline completed the Task instead of declining it")
		}
	}
	if !seen {
		t.Error("the declined Task is not readable with IncludeDeclined")
	}
}

func TestCreatingAListFromTheFormKeepsTheDraft(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	m = addScreen(m)
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

	m = addScreen(m)
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

// The screen's window is local, not UTC. schedule reads a date's year, month
// and day in the local zone, so opening the window on a UTC instant whose
// calendar date is not the local one drops today's Occurrence off the screen
// or leaves yesterday's on it.
//
// The rule is anchored years back, so the first date it produces from here is
// today whatever the zone. The two zones make that hold at any hour: at every
// instant one of them has a calendar date that is not the UTC one.
func TestTheScheduleScreenOpensOnTodayInTheLocalZone(t *testing.T) {
	for _, zone := range []*time.Location{
		time.FixedZone("west", -11*60*60),
		time.FixedZone("east", +14*60*60),
	} {
		t.Run(zone.String(), func(t *testing.T) {
			was := time.Local
			time.Local = zone
			t.Cleanup(func() { time.Local = was })

			s := fixture(t)
			m := newModel(t, s)
			apples, ok := taskNamed(m, "Buy apples")
			if !ok {
				t.Fatal("the fixture Task was not on the screen")
			}
			carry(t, s, apples.ID, func() error {
				_, err := s.Repeat("alice", apples.ID, "daily from 2020-01-01")
				return err
			})

			m.tasks.Select(indexOf(t, m, apples.ID))
			m, _ = m.run("/repeat")
			if m.rep == nil || len(m.rep.dates) == 0 {
				t.Fatal("/repeat showed no dates for a daily rule")
			}
			got := m.rep.dates[0].Date.Format("2006-01-02")
			if want := time.Now().In(zone).Format("2006-01-02"); got != want {
				t.Errorf("the screen opens on %s, want today in this zone, %s", got, want)
			}
		})
	}
}

// Snooze is one of the attributes docs/features.md asks for, so the screen
// that shows every attribute shows it: set it there and the Task hides, clear
// it there and the Task comes back, with no trip through the palette.
func TestTheEditScreenSetsAndClearsASnooze(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	task, _ := m.selected()
	d := draftOf(task)
	d.Snooze = time.Now().Local().Add(48 * time.Hour).Format(dateLayouts[0])
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	after, ok := taskIn(t, s, task.ID)
	if !ok || after.SnoozedUntil.IsZero() {
		t.Fatalf("the edit screen set no snooze on %s", task.ID)
	}

	// The screen opens on what is there, and clearing the line takes it off.
	back := draftOf(after)
	if back.Snooze == "" {
		t.Error("the edit screen opened with the snooze line empty")
	}
	back.Snooze = ""
	if err := m.save(back); err != nil {
		t.Fatalf("save: %v", err)
	}
	if after, ok := taskIn(t, s, task.ID); !ok || !after.SnoozedUntil.IsZero() {
		t.Errorf("clearing the snooze line left %v", after.SnoozedUntil)
	}
}

func taskIn(t *testing.T, s *store.Store, id string) (store.Task, bool) {
	t.Helper()
	tasks, err := s.Tasks(store.Query{IncludeSnoozed: true})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return store.Task{}, false
}

// A snoozed Task is out of the way, not gone. "z" is how it comes back into
// view, which is the only way the edit screen can be opened on one to take
// the snooze off again.
func TestZBringsSnoozedTasksBackIntoView(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	task, _ := m.selected()
	if err := m.snoozeUntil(task.ID, time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, drawn := m.taskByID(task.ID); drawn {
		t.Fatal("a snoozed Task is still in the main view")
	}

	m = press(m, "z")
	back, drawn := m.taskByID(task.ID)
	if !drawn {
		t.Fatal("z did not bring the snoozed Task back")
	}
	if !slices.Contains(back.Marks(), "snoozed") {
		t.Errorf("the Task came back marked %v, want it to say snoozed", back.Marks())
	}

	// And off again, because the point of the key is that the view goes
	// back to what is in front of you.
	m = press(m, "z")
	if _, drawn := m.taskByID(task.ID); drawn {
		t.Error("z a second time left the snoozed Task in view")
	}

	// And back on, because the snooze is taken off from the screen it is
	// visible on. #30 was that this path was unreachable, so the edit goes
	// through a Task the view actually holds.
	m = press(m, "z")
	off := draftOf(back)
	off.Snooze = ""
	if err := m.save(off); err != nil {
		t.Fatalf("clearing the snooze: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	cleared, drawn := m.taskByID(task.ID)
	if !drawn {
		t.Fatal("the Task went out of view when its snooze came off")
	}
	if slices.Contains(cleared.Marks(), "snoozed") {
		t.Errorf("the Task is still marked %v after its snooze was cleared", cleared.Marks())
	}
}

// #20 says editing one date Detaches it, so the edit is how a person says
// "this week's is different" without having to learn that detaching is the way
// to say it. Escaping the form says nothing, so the date stays an Occurrence.
func TestEditingOneDateDetachesIt(t *testing.T) {
	s := fixture(t)
	m := newModel(t, s)

	apples, _ := taskNamed(m, "Buy apples")
	carry(t, s, apples.ID, func() error {
		_, err := s.Repeat("alice", apples.ID, "every week")
		return err
	})
	m.tasks.Select(indexOf(t, m, apples.ID))
	m, _ = m.run("/repeat")
	on := m.rep.dates[0].Date

	// Escaping writes nothing: the Series still produces the date, and the
	// screen it was opened from is still the one in front of you.
	m = press(m, "e")
	if m.draft == nil || m.editor == nil {
		t.Fatal("e opened no form on the date under the cursor")
	}
	m = press(m, "esc")
	if m.rep == nil {
		t.Error("escaping the form left the Scheduling screen behind")
	}
	dates, err := s.Occurrences(apples.ID, on, on)
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(dates) != 1 || dates[0].State != store.Pending {
		t.Fatalf("escaping the form left %+v, want the date still pending", dates)
	}

	m = press(m, "e")
	d := m.draft
	if want := on.Format(dateLayouts[0]); d.Deadline != want {
		t.Errorf("the form opened on deadline %q, want the date %s", d.Deadline, want)
	}
	if len(d.Attachments) != 0 || d.Snooze != "" {
		t.Errorf("the form opened carrying %v and snooze %q, want neither", d.Attachments, d.Snooze)
	}
	d.Title = "Buy pears instead"
	// A pointer typed on the way through is written under the copy's own
	// Lease, not the recurring tree's, and the copy is a top-level Task of
	// its own: guarding it with the wrong Lease refuses every time.
	d.Attach = "https://example.invalid/pears"
	if err := m.save(d); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := m.refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// The date left the Series, and what it became carries the edit and the
	// Lists the recurring Task was on.
	if dates, err := s.Occurrences(apples.ID, on, on); err != nil || len(dates) != 0 {
		t.Errorf("the Series still produces %+v (%v), want the date detached", dates, err)
	}
	edited, ok := taskNamed(m, "Buy pears instead")
	if !ok {
		t.Fatal("the edited date is not a Task of its own")
	}
	if edited.ID == apples.ID {
		t.Fatal("the edit landed on the recurring Task rather than on the date")
	}
	if got := edited.Deadline.Local().Format(dateLayouts[1]); got != on.Format(dateLayouts[1]) {
		t.Errorf("the detached Task is due %s, want %s", got, on.Format(dateLayouts[1]))
	}
	if !slices.Equal(edited.Lists, apples.Lists) {
		t.Errorf("the detached Task is on %v, want the recurring Task's %v", edited.Lists, apples.Lists)
	}
	if !slices.Contains(edited.Attachments, "https://example.invalid/pears") {
		t.Errorf("the detached Task holds %v, want the pointer typed into the form", edited.Attachments)
	}
	if again, ok := taskNamed(m, "Buy apples"); !ok || again.ID != apples.ID {
		t.Error("the recurring Task did not survive the edit under its own title")
	}
}
