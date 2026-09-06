package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/possiblyneal/todo/apps/todo/src/datepicker"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// dateField is the calendar as a huh field, so a deadline is picked in the
// form rather than typed into it. It replaces the validated text input the add
// screen shipped with: a date that cannot be typed cannot be mistyped, and
// there is nothing left for a validator to catch.
//
// huh has no date field and Go had no date picker, so this is both halves:
// src/datepicker is the component, and this is the ten-line-per-method shim
// that satisfies huh.Field.
type dateField struct {
	picker   datepicker.Model
	value    *string
	title    string
	key      string
	focused  bool
	width    int
	height   int
	theme    huh.Theme
	darkBG   bool
	keymap   *huh.KeyMap
	position huh.FieldPosition
}

// newDateField binds a picker to the text a draft carries. The draft stays
// text because that is what the store's parse and the rest of the form speak;
// the picker is what fills it in.
func newDateField(title string, value *string, shortcuts []datepicker.Shortcut) *dateField {
	picker := datepicker.New()
	picker.Shortcuts = shortcuts
	if when, err := parseDate(*value); err == nil && !when.IsZero() {
		picker = picker.SetValue(when)
	}
	return &dateField{picker: picker, value: value, title: title, key: title}
}

// snoozeShortcuts are the four offered snoozes as one-key jumps, so the same
// picker covers "next Tuesday" and "in an hour".
func snoozeShortcuts() []datepicker.Shortcut {
	shortcuts := make([]datepicker.Shortcut, 0, len(store.SnoozeDefaults))
	for i, s := range store.SnoozeDefaults {
		shortcuts = append(shortcuts, datepicker.Shortcut{
			Key:   fmt.Sprint(i + 1),
			Label: s.Label,
			When:  s.Until,
		})
	}
	return shortcuts
}

func (f *dateField) Init() tea.Cmd { return nil }

// Update gives the picker the arrow keys and leaves tab, enter and escape to
// the form. Whatever the picker settles on is written straight back to the
// draft, so the form has no separate confirm to forget.
func (f *dateField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if bg, ok := msg.(tea.BackgroundColorMsg); ok {
		f.darkBG = bg.IsDark()
	}
	picker, handled := f.picker.Update(msg)
	if handled {
		f.picker = picker
		f.write()
		return f, nil
	}
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(press, f.keys().Prev):
			return f, huh.PrevField
		case key.Matches(press, f.keys().Next, f.keys().Submit):
			return f, huh.NextField
		}
	}
	return f, nil
}

// write puts the picked date back where the draft can read it.
func (f *dateField) write() {
	when := f.picker.Value()
	if when.IsZero() {
		*f.value = ""
		return
	}
	// A date with no time of day is written as a date. The layouts read both,
	// and "2026-11-01" is what a person and the CLI both write down.
	layout := dateLayouts[1]
	if when.Hour() != 0 || when.Minute() != 0 {
		layout = dateLayouts[0]
	}
	*f.value = when.Format(layout)
}

func (f *dateField) View() string {
	styles := f.styles()
	f.picker.Styles = datepicker.Styles{
		Header:   styles.Title,
		Weekday:  styles.Description,
		Day:      styles.Base,
		Cursor:   styles.SelectSelector.Reverse(true),
		Today:    styles.Base.Underline(true),
		Dim:      styles.Description,
		Selected: styles.Title,
	}
	body := []string{styles.Title.Render(f.title), f.picker.View()}
	if f.focused {
		body = append(body, styles.Description.Render(f.picker.Keys()))
	}
	return styles.Base.Render(strings.Join(body, "\n"))
}

func (f *dateField) styles() huh.FieldStyles {
	s := huh.ThemeCharm(f.darkBG)
	if f.theme != nil {
		s = f.theme.Theme(f.darkBG)
	}
	if f.focused {
		return s.Focused
	}
	return s.Blurred
}

func (f *dateField) keys() huh.InputKeyMap {
	if f.keymap == nil {
		return huh.NewDefaultKeyMap().Input
	}
	return f.keymap.Input
}

func (f *dateField) Blur() tea.Cmd  { f.focused = false; return nil }
func (f *dateField) Focus() tea.Cmd { f.focused = true; return nil }

// Error is always nil. There is nothing to validate: every date the picker can
// produce is a date.
func (f *dateField) Error() error { return nil }

func (*dateField) Skip() bool { return false }
func (*dateField) Zoom() bool { return false }

func (f *dateField) KeyBinds() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("left", "right", "up", "down"), key.WithHelp("←→↑↓", "move")),
		key.NewBinding(key.WithKeys("[", "]"), key.WithHelp("[ ]", "month")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "today")),
		key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "clear")),
		f.keys().Next,
		f.keys().Prev,
	}
}

func (f *dateField) WithTheme(t huh.Theme) huh.Field    { f.theme = t; return f }
func (f *dateField) WithKeyMap(k *huh.KeyMap) huh.Field { f.keymap = k; return f }
func (f *dateField) WithWidth(w int) huh.Field          { f.width = w; return f }
func (f *dateField) WithHeight(h int) huh.Field         { f.height = h; return f }

func (f *dateField) WithPosition(p huh.FieldPosition) huh.Field { f.position = p; return f }

func (f *dateField) GetKey() string { return f.key }
func (f *dateField) GetValue() any  { return *f.value }

// Run and RunAccessible are huh's standalone paths. The picker is drawn inside
// this program's own screen, and a calendar read out line by line is a worse
// way to type a date than typing one, so both fall back to the text a screen
// reader can handle.
func (f *dateField) Run() error { return nil }

func (f *dateField) RunAccessible(w io.Writer, r io.Reader) error {
	fmt.Fprintf(w, "%s (%s): ", f.title, dateLayouts[0])
	var typed string
	if _, err := fmt.Fscanln(r, &typed); err != nil && err != io.EOF {
		return err
	}
	when, err := parseDate(typed)
	if err != nil {
		return err
	}
	f.picker = f.picker.SetValue(when)
	f.write()
	return nil
}

// picked reads a date field back, which is what a test asserts on.
func (f *dateField) picked() time.Time { return f.picker.Value() }
