package leantui

import (
	"charm.land/lipgloss/v2"

	"github.com/docker/docker-agent/pkg/tui/styles"
)

// The style helpers are evaluated lazily (on each call) so they always reflect
// the theme that styles.ApplyTheme installed before the TUI started.

func stAccent() lipgloss.Style      { return lipgloss.NewStyle().Foreground(styles.Accent) }
func stMuted() lipgloss.Style       { return lipgloss.NewStyle().Foreground(styles.TextMutedGray) }
func stSecondary() lipgloss.Style   { return lipgloss.NewStyle().Foreground(styles.TextSecondary) }
func stPrimary() lipgloss.Style     { return lipgloss.NewStyle().Foreground(styles.TextPrimary) }
func stBold() lipgloss.Style        { return lipgloss.NewStyle().Foreground(styles.TextPrimary).Bold(true) }
func stError() lipgloss.Style       { return lipgloss.NewStyle().Foreground(styles.Error) }
func stWarning() lipgloss.Style     { return lipgloss.NewStyle().Foreground(styles.Warning) }
func stSuccess() lipgloss.Style     { return lipgloss.NewStyle().Foreground(styles.Success) }
func stPlaceholder() lipgloss.Style { return lipgloss.NewStyle().Foreground(styles.TextMuted) }
func stReasoning() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMutedGray).Italic(true)
}

const (
	promptText   = "❯ "
	promptWidth  = 2
	continuation = "  "
)
