package tuitest

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

// MoveMouse sends a mouse-motion event at screen coordinates x,y.
func (d *Driver) MoveMouse(x, y int) *Driver {
	d.sendSync(tea.MouseMotionMsg{X: x, Y: y})
	return d
}

// Click sends a left-click followed by a release at screen coordinates x,y.
func (d *Driver) Click(x, y int) *Driver {
	d.sendSync(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	d.sendSync(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return d
}

// FindText returns the coordinates of the first occurrence of text in the
// latest captured frame. The x coordinate is display width, not byte offset,
// so it works for wide Unicode before the matched text.
func (d *Driver) FindText(text string) (x, y int, ok bool) {
	if text == "" {
		return 0, 0, false
	}
	for y, line := range strings.Split(d.Frame(), "\n") {
		before, _, ok := strings.Cut(line, text)
		if !ok {
			continue
		}
		return runewidth.StringWidth(before), y, true
	}
	return 0, 0, false
}

// MustFindText is like FindText but fails the test when text is not visible.
func (d *Driver) MustFindText(text string) (x, y int) {
	d.tb.Helper()
	x, y, ok := d.FindText(text)
	if !ok {
		d.tb.Fatalf("tuitest: could not find %q in frame\n--- frame ---\n%s", text, d.Frame())
	}
	return x, y
}

// ClickText clicks the first visible occurrence of text.
func (d *Driver) ClickText(text string) *Driver {
	d.tb.Helper()
	x, y := d.MustFindText(text)
	return d.Click(x, y)
}

// MoveMouseToText moves the mouse to the first visible occurrence of text.
func (d *Driver) MoveMouseToText(text string) *Driver {
	d.tb.Helper()
	x, y := d.MustFindText(text)
	return d.MoveMouse(x, y)
}
