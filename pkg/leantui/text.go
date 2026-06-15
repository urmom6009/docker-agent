package leantui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// displayWidth returns the rendered cell width of s, ignoring ANSI escape
// sequences.
func displayWidth(s string) int {
	return ansi.StringWidth(s)
}

func runeWidth(r rune) int {
	if r == '\t' {
		return 1
	}
	w := runewidth.RuneWidth(r)
	if w < 0 {
		return 0
	}
	return w
}

// truncate shortens s to at most w cells, appending an ellipsis when it had to
// cut anything.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if displayWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// padRight pads s with spaces up to w cells. It never truncates.
func padRight(s string, w int) string {
	gap := w - displayWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// wrapANSI hard-wraps s to width w, keeping ANSI styling intact and returning
// one string per physical row. Existing newlines in s start new rows.
func wrapANSI(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	s = strings.ReplaceAll(s, "\t", "    ")
	wrapped := ansi.Hardwrap(s, w, false)
	return strings.Split(wrapped, "\n")
}
