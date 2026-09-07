package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

const (
	paperclip = "📎"
	rowHeight = 4
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	overdueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// row is one Task in the main view. It carries the names of the Lists it
// belongs to rather than their ids, because a row is what a person reads.
type row struct {
	task  store.Task
	lists []string
}

// FilterValue is what the searchbox matches against: the title and the
// description, which is what a person types half of when looking for a Task.
func (r row) FilterValue() string { return r.task.Title + " " + r.task.Description }

// rowDelegate draws a Task in the four lines docs/features.md asks for: title,
// two lines of description, and the dates and Lists underneath.
type rowDelegate struct {
	zones *zone.Manager
	width int
}

func (d rowDelegate) Height() int                         { return rowHeight }
func (d rowDelegate) Spacing() int                        { return 0 }
func (d rowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d rowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	r, ok := item.(row)
	if !ok {
		return
	}
	fmt.Fprint(w, d.zones.Mark("task:"+r.task.ID, d.render(r, index == m.Index())))
}

func (d rowDelegate) render(r row, selected bool) string {
	cursor := "  "
	style := titleStyle
	if selected {
		cursor, style = "▸ ", selectedStyle
	}
	indent := strings.Repeat("  ", r.task.Depth-1)

	head := cursor + indent + style.Render(r.task.Title)
	if len(r.task.Attachments) > 0 {
		head += " " + paperclip
	}
	head += marks(r.task)

	// Two lines of description, no more and no fewer, so every row is the
	// same height and the list can page.
	body := describe(r.task.Description, d.width-len(indent)-4)
	foot := dimStyle.Render(fmt.Sprintf("    %s  created %s%s%s",
		indent, day(r.task.CreatedAt), due(r.task.Deadline), inLists(r.lists)))

	return strings.Join([]string{
		head,
		dimStyle.Render("    " + indent + body[0]),
		dimStyle.Render("    " + indent + body[1]),
		foot,
	}, "\n")
}

// describe cuts a description into exactly two lines, breaking on words.
//
// Widths are measured in cells rather than bytes, so an emoji takes the two
// columns it draws in and a nerd font glyph takes one. Bubble Tea v2 negotiates
// Unicode mode 2027 with the terminal at startup, which is what makes the
// terminal agree with that measurement.
func describe(text string, width int) [2]string {
	var lines [2]string
	if width < 8 {
		width = 8
	}
	words := strings.Fields(text)
	for i := range lines {
		var line string
		for len(words) > 0 {
			candidate := words[0]
			if line != "" {
				candidate = line + " " + words[0]
			}
			if lipgloss.Width(candidate) > width {
				break
			}
			line, words = candidate, words[1:]
		}
		if line == "" && len(words) > 0 {
			line, words = ansi.Truncate(words[0], width, ""), words[1:]
		}
		lines[i] = line
	}
	if len(words) > 0 {
		lines[1] = ansi.Truncate(lines[1], width-1, "") + "…"
	}
	return lines
}

// marks draws what a read worked out about a Task. The words are
// store.Task.Marks; only the colour on "overdue" is the screen's.
func marks(t store.Task) string {
	m := t.Marks()
	if len(m) == 0 {
		return ""
	}
	drawn := make([]string, len(m))
	for i, mark := range m {
		drawn[i] = mark
		if mark == "overdue" {
			drawn[i] = overdueStyle.Render(mark)
		}
	}
	return "  " + strings.Join(drawn, " ")
}

func day(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(time.DateOnly)
}

func due(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return "  due " + day(t)
}

func inLists(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return "  " + strings.Join(names, ", ")
}
