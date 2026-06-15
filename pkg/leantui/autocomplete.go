package leantui

import "strings"

const autocompleteMaxRows = 8

// autocomplete drives the slash-command popup. It is active whenever the input
// is a partial command: a single token starting with "/" and no spaces yet.
type autocomplete struct {
	all      []command
	matches  []command
	selected int
	active   bool
}

func newAutocomplete() *autocomplete {
	return &autocomplete{}
}

func (a *autocomplete) setCommands(cmds []command) {
	a.all = cmds
}

// sync recomputes the popup state from the current editor text. It returns true
// while the popup is showing.
func (a *autocomplete) sync(input string) bool {
	if !strings.HasPrefix(input, "/") || strings.ContainsAny(input, " \n") {
		a.active = false
		a.matches = nil
		a.selected = 0
		return false
	}
	a.matches = filterCommands(a.all, input[1:])
	a.active = len(a.matches) > 0
	if a.selected >= len(a.matches) {
		a.selected = len(a.matches) - 1
	}
	if a.selected < 0 {
		a.selected = 0
	}
	return a.active
}

func (a *autocomplete) moveUp() {
	if !a.active {
		return
	}
	if a.selected > 0 {
		a.selected--
	}
}

func (a *autocomplete) moveDown() {
	if !a.active {
		return
	}
	if a.selected < len(a.matches)-1 {
		a.selected++
	}
}

func (a *autocomplete) current() (command, bool) {
	if !a.active || a.selected >= len(a.matches) {
		return command{}, false
	}
	return a.matches[a.selected], true
}

func (a *autocomplete) dismiss() {
	a.active = false
	a.matches = nil
	a.selected = 0
}

// render returns the popup rows (top to bottom) for the given width.
func (a *autocomplete) render(width int) []string {
	if !a.active || len(a.matches) == 0 {
		return nil
	}

	start := 0
	if a.selected >= autocompleteMaxRows {
		start = a.selected - autocompleteMaxRows + 1
	}
	end := min(start+autocompleteMaxRows, len(a.matches))

	nameWidth := 0
	for _, c := range a.matches {
		if w := len(c.name) + 1; w > nameWidth {
			nameWidth = w
		}
	}
	nameWidth = min(nameWidth, 24)

	var rows []string
	for i := start; i < end; i++ {
		c := a.matches[i]
		name := padRight("/"+c.name, nameWidth)
		line := " " + name + "  " + c.desc
		line = truncate(line, width)
		if i == a.selected {
			rows = append(rows, lipglossSelected(line, width))
		} else {
			rows = append(rows, stMuted().Render(line))
		}
	}
	return rows
}

// lipglossSelected highlights the selected popup row across the full width.
func lipglossSelected(line string, width int) string {
	padded := padRight(line, width)
	return stAccent().Bold(true).Render(padded)
}
