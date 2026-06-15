package leantui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/docker/docker-agent/pkg/tui/components/markdown"
)

// renderUserLines renders a submitted user message as committed scrollback,
// echoing it with the same prompt marker used by the input box.
func renderUserLines(text string, width int) []string {
	text = strings.TrimRight(text, "\n")
	wrapped := wrapANSI(text, width-promptWidth)
	out := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		prefix := stAccent().Render(promptText)
		if i > 0 {
			prefix = continuation
		}
		out = append(out, prefix+stPrimary().Render(line))
	}
	return out
}

// renderReasoningLines renders agent reasoning as dimmed italic text.
func renderReasoningLines(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	style := stReasoning()
	var out []string
	for _, line := range wrapANSI(text, width-2) {
		out = append(out, "  "+style.Render(line))
	}
	return out
}

// renderAssistantLines renders an assistant message as markdown. Each returned
// line is guaranteed to fit within width so the differential renderer's row
// accounting stays correct.
func renderAssistantLines(text string, width int) []string {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}

	rendered, err := markdown.NewRenderer(width).Render(text)
	if err != nil {
		return wrapANSI(text, width)
	}

	var out []string
	for line := range strings.SplitSeq(strings.Trim(rendered, "\n"), "\n") {
		if displayWidth(line) > width {
			out = append(out, wrapANSI(line, width)...)
			continue
		}
		out = append(out, line)
	}
	return out
}

func renderNoticeLines(prefix, text string, width int, style lipgloss.Style) []string {
	wrapped := wrapANSI(text, width-displayWidth(prefix))
	out := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		p := prefix
		if i > 0 {
			p = strings.Repeat(" ", displayWidth(prefix))
		}
		out = append(out, style.Render(p+line))
	}
	return out
}
