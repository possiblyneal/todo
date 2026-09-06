package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// dateLayouts are what a deadline may be typed as. A picker replaces this
// input at #21; until then the text is validated where it is typed, so a
// half-typed date never reaches the store.
var dateLayouts = []string{"2006-01-02 15:04", "2006-01-02"}

// draft is what the add and edit screens fill in. It is text, because that is
// what a person types; turning it into Attributes is attributes(), and that is
// where a bad date stops.
type draft struct {
	// taskID is empty for a new Task and set when an existing one is being
	// edited, which is the only difference between the two screens.
	taskID string

	Title       string
	Description string
	Why         string
	Deadline    string
	Estimate    string
	Priority    store.Level
	Impact      store.Level
	Colour      string
	Lists       []string
	Tags        []string
}

// form builds the add-task screen: every attribute a Task has, with the two
// levels showing what they mean rather than asking a person to guess.
//
// The form holds no transaction. It is built, filled in and submitted, and the
// store is only touched when it completes: a write opened while someone is
// thinking is a write every other process waits behind.
func (m Model) form(d *draft) *huh.Form {
	fields := []huh.Field{
		huh.NewInput().Title("Title").Value(&d.Title).Validate(required),
		huh.NewText().Title("Description").Value(&d.Description).Lines(3),
		huh.NewInput().Title("Why").Value(&d.Why).
			Placeholder("what happens if this never gets done"),
		huh.NewInput().Title("Deadline").Value(&d.Deadline).
			Placeholder("2006-01-02, or 2006-01-02 15:04").Validate(validDate),
		huh.NewInput().Title("Estimate").Value(&d.Estimate).
			Placeholder("90m, 3h, 2h30m").Validate(validDuration),
		huh.NewSelect[store.Level]().Title("Priority").Value(&d.Priority).
			Options(levelOptions(store.PriorityExamples)...),
		huh.NewSelect[store.Level]().Title("Impact").Value(&d.Impact).
			Options(levelOptions(store.ImpactExamples)...),
		huh.NewInput().Title("Colour").Value(&d.Colour),
		huh.NewMultiSelect[string]().Title("Lists").Value(&d.Lists).
			Options(listOptions(m.lists)...),
		huh.NewMultiSelect[string]().Title("Tags").Value(&d.Tags).
			Options(tagOptions(m.tags)...),
	}
	return huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(min(m.width-4, 72)).
		WithHeight(max(m.height-4, 10))
}

// collectionForm is the popup that creates a List or a Tag without leaving the
// screen that wanted one.
func (m Model) collectionForm(noun string, name, colour *string) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("New "+noun).Value(name).Validate(required),
		huh.NewInput().Title("Colour").Value(colour),
	)).WithWidth(min(m.width-8, 48)).WithHeight(7)
}

func levelOptions(examples map[store.Level]string) []huh.Option[store.Level] {
	options := []huh.Option[store.Level]{huh.NewOption("none", store.Level(""))}
	for _, level := range store.Levels {
		options = append(options, huh.NewOption(
			fmt.Sprintf("%-5s %s", level, examples[level]), level))
	}
	return options
}

func listOptions(lists []store.List) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(lists))
	for _, l := range lists {
		options = append(options, huh.NewOption(l.Name, l.ID))
	}
	return options
}

func tagOptions(tags []store.Tag) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(tags))
	for _, t := range tags {
		options = append(options, huh.NewOption(t.Name, t.ID))
	}
	return options
}

func required(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("this one is needed")
	}
	return nil
}

func validDate(v string) error {
	_, err := parseDate(v)
	return err
}

func validDuration(v string) error {
	_, err := parseDuration(v)
	return err
}

func parseDate(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date; try 2006-01-02 or 2006-01-02 15:04", v)
}

func parseDuration(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%q is not a length of time; try 90m or 2h30m", v)
	}
	if d < 0 {
		return 0, fmt.Errorf("an estimate cannot be negative")
	}
	return d, nil
}

// attributes turns a filled-in draft into the edit the store takes. Every
// attribute the screen showed is named, so one left empty is cleared rather
// than left alone: the person saw the whole Task and this is what they left.
func (d draft) attributes() (store.Attributes, error) {
	deadline, err := parseDate(d.Deadline)
	if err != nil {
		return store.Attributes{}, err
	}
	estimate, err := parseDuration(d.Estimate)
	if err != nil {
		return store.Attributes{}, err
	}
	if strings.TrimSpace(d.Title) == "" {
		return store.Attributes{}, fmt.Errorf("a task needs a title")
	}
	return store.Attributes{
		Title:       store.Set(strings.TrimSpace(d.Title)),
		Description: store.Set(d.Description),
		Why:         store.Set(d.Why),
		Deadline:    store.Set(deadline),
		Estimate:    store.Set(estimate),
		Priority:    store.Set(d.Priority),
		Impact:      store.Set(d.Impact),
		Colour:      store.Set(d.Colour),
	}, nil
}

// draftOf fills a draft from a Task, which is what opens the edit screen on
// what is already there.
func draftOf(t store.Task) *draft {
	d := &draft{
		taskID:      t.ID,
		Title:       t.Title,
		Description: t.Description,
		Why:         t.Why,
		Priority:    t.Priority,
		Impact:      t.Impact,
		Colour:      t.Colour,
		Lists:       append([]string(nil), t.Lists...),
		Tags:        append([]string(nil), t.Tags...),
	}
	if !t.Deadline.IsZero() {
		d.Deadline = t.Deadline.Local().Format(dateLayouts[0])
	}
	if t.Estimate > 0 {
		d.Estimate = t.Estimate.String()
	}
	return d
}
