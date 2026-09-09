package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// save writes a draft: a new Task, or an edit to the one it was opened on.
//
// Nothing here runs while a person is typing. The form gathers, save writes,
// and the Lease is taken and released inside this call, so a CLI write racing
// the TUI waits on a transaction measured in milliseconds rather than on how
// long someone stared at a text box.
func (m *Model) save(d *draft) error {
	a, err := d.attributes()
	if err != nil {
		return err
	}

	// A draft opened on one date of a Series detaches that date and the edit
	// lands on the Task it becomes: "this week's is at 3pm instead" is that
	// and nothing else. The detach and the edit are one store call, so they
	// cannot come apart; the Lease is the recurring tree's, which is what the
	// mark is guarded by.
	if !d.occurrence.IsZero() {
		var id string
		if err := m.store.WithLease(m.actor, d.taskID, store.WriteTTL, func() error {
			var err error
			id, err = m.store.DetachEdited(m.actor, d.taskID, d.occurrence, a, d.Lists, d.Tags)
			return err
		}); err != nil {
			return err
		}
		// A pointer typed into the form is the one thing the copy does
		// not come out with, so it is written after, and under the
		// copy's own Lease: the copy is a top-level Task of its own,
		// and the Lease the detach took on it was given back inside
		// the transaction that made it. The pointer is the one part
		// of this that is not atomic; a failure here leaves the
		// detached Task edited and the pointer one more line to type.
		if strings.TrimSpace(d.Attach) == "" {
			return nil
		}
		return m.store.WithLease(m.actor, id, store.WriteTTL, func() error {
			return m.store.Attach(m.actor, id, d.Attach)
		})
	}

	if d.taskID == "" {
		id, err := m.store.AddTask(m.actor, a)
		if err != nil {
			return err
		}
		return m.store.WithLease(m.actor, id, store.WriteTTL, func() error {
			return m.memberships(id, store.Task{}, d)
		})
	}

	was, ok := m.taskByID(d.taskID)
	if !ok {
		return fmt.Errorf("that task is no longer in view")
	}
	return m.store.WithLease(m.actor, d.taskID, store.WriteTTL, func() error {
		if err := m.store.EditTask(m.actor, d.taskID, a); err != nil {
			return err
		}
		return m.memberships(d.taskID, was, d)
	})
}

// memberships appends only the differences, so opening a Task and saving it
// unchanged adds nothing to the Change History.
func (m *Model) memberships(taskID string, was store.Task, d *draft) error {
	for _, id := range added(was.Lists, d.Lists) {
		if err := m.store.AddToList(m.actor, taskID, id); err != nil {
			return err
		}
	}
	for _, id := range added(d.Lists, was.Lists) {
		if err := m.store.RemoveFromList(m.actor, taskID, id); err != nil {
			return err
		}
	}
	for _, id := range added(was.Tags, d.Tags) {
		if err := m.store.AttachTag(m.actor, taskID, id); err != nil {
			return err
		}
	}
	for _, id := range added(d.Tags, was.Tags) {
		if err := m.store.DetachTag(m.actor, taskID, id); err != nil {
			return err
		}
	}

	// A pointer typed into the form is one more pointer. Nothing is fetched
	// and nothing is checked: the store writes down where it points and that
	// is the whole of it.
	wanted := d.Attachments
	if strings.TrimSpace(d.Attach) != "" {
		wanted = append(append([]string(nil), wanted...), d.Attach)
	}
	for _, target := range added(was.Attachments, wanted) {
		if err := m.store.Attach(m.actor, taskID, target); err != nil {
			return err
		}
	}
	for _, target := range added(wanted, was.Attachments) {
		if err := m.store.Detach(m.actor, taskID, target); err != nil {
			return err
		}
	}
	return nil
}

// added is what is in now and not in was.
func added(was, now []string) []string {
	var out []string
	for _, id := range now {
		if !slices.Contains(was, id) {
			out = append(out, id)
		}
	}
	return out
}

// lifecycle completes, declines, reopens or deletes the Task under the cursor,
// each one taking the Lease covering its tree and giving it back.
func (m *Model) lifecycle(taskID, what string) error {
	return m.store.WithLease(m.actor, taskID, store.WriteTTL, func() error {
		switch what {
		case "complete":
			return m.store.CompleteTask(m.actor, taskID)
		case "decline":
			return m.store.DeclineTask(m.actor, taskID)
		case "reopen":
			return m.store.ReopenTask(m.actor, taskID)
		case "delete":
			return m.store.DeleteTask(m.actor, taskID)
		}
		return fmt.Errorf("no such command %q", what)
	})
}

// snooze hides a Task for one of the offered lengths, counted from now.
func (m *Model) snooze(taskID string, s store.Snooze) error {
	return m.snoozeUntil(taskID, s.Until(time.Now()))
}

// snoozeUntil hides a Task until a moment the calendar picked. The zero time
// takes the snooze off.
func (m *Model) snoozeUntil(taskID string, until time.Time) error {
	return m.store.WithLease(m.actor, taskID, store.WriteTTL, func() error {
		return m.store.EditTask(m.actor, taskID, store.Attributes{
			SnoozedUntil: store.Set(until),
		})
	})
}

func (m Model) taskByID(id string) (store.Task, bool) {
	for _, item := range m.tasks.Items() {
		if r, ok := item.(row); ok && r.task.ID == id {
			return r.task, true
		}
	}
	return store.Task{}, false
}

func (m Model) selected() (store.Task, bool) {
	r, ok := m.tasks.SelectedItem().(row)
	return r.task, ok
}
