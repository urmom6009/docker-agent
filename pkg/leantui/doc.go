// Package leantui implements a minimal, scrollback-friendly terminal UI used
// when docker-agent runs with --lean. Unlike the full bubbletea TUI it never
// switches to the alternate screen: finished conversation content is committed
// into the terminal's normal scrollback while a small live region (the input
// box and status footer) stays pinned to the bottom and is redrawn in place.
//
// The package is intentionally self-contained. It builds on a handful of
// low-level Charmbracelet dependencies (lipgloss for styling, x/ansi for cell
// math and escape sequences) and implements its own input parser, differential
// renderer, and input editor rather than relying on a full TUI framework.
package leantui
