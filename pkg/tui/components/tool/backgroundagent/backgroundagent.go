// Package backgroundagent renders the background-agent tool calls
// (run/list/view/stop) so a dispatched fleet reads as delegations rather than
// raw text blobs. It is pure presentation: no runtime behavior changes here.
package backgroundagent

import (
	"strings"

	"charm.land/lipgloss/v2"

	agenttool "github.com/docker/docker-agent/pkg/tools/builtin/agent"
	"github.com/docker/docker-agent/pkg/tui/components/spinner"
	"github.com/docker/docker-agent/pkg/tui/components/toolcommon"
	"github.com/docker/docker-agent/pkg/tui/core/layout"
	"github.com/docker/docker-agent/pkg/tui/service"
	"github.com/docker/docker-agent/pkg/tui/styles"
	"github.com/docker/docker-agent/pkg/tui/types"
)

// NewRun renders run_background_agent as a delegation-style card:
// "<sender> dispatches <agent>" plus the task text, led by a status-aware icon.
func NewRun(msg *types.Message, sessionState service.SessionStateReader) layout.Model {
	return toolcommon.NewBase(msg, sessionState, renderRun)
}

func renderRun(msg *types.Message, s spinner.Spinner, _ service.SessionStateReader, width, _ int) string {
	params, err := toolcommon.ParseArgs[agenttool.RunBackgroundAgentArgs](msg.ToolCall.Function.Arguments)
	if err != nil {
		return ""
	}

	header := styles.AgentBadgeStyleFor(msg.Sender).MarginLeft(2).Render(msg.Sender) +
		" dispatches " +
		styles.AgentBadgeStyleFor(params.Agent).Render(params.Agent)

	// Status-aware icon: spinner while the task runs, ✓ on success, ✗ on error.
	// Single glyph (no elapsed suffix) keeps the wrap math below stable.
	icon := statusIcon(msg, s)
	iconWithSpace := icon + " "
	iconWidth := lipgloss.Width(iconWithSpace)

	availableWidth := max(width-iconWidth, 10)
	lines := toolcommon.WrapLines(params.Task, availableWidth)

	var taskContent strings.Builder
	for i, line := range lines {
		if i == 0 {
			taskContent.WriteString(iconWithSpace)
			taskContent.WriteString(styles.ToolMessageStyle.Render(line))
		} else {
			// Subsequent lines indent to align with the first line's text.
			taskContent.WriteString("\n")
			taskContent.WriteString(strings.Repeat(" ", iconWidth))
			taskContent.WriteString(styles.ToolMessageStyle.Render(line))
		}
	}

	return header + "\n\n" + taskContent.String()
}

// NewList renders list_background_agents. There are no arguments worth
// surfacing; the tool result already lists the tasks, so the renderer shows the
// tool header plus that result. (NoArgsRenderer is avoided here because it drops
// the result, which is the only useful payload for list until the C.2 live
// "Background agents (N)" surface lands.)
func NewList(msg *types.Message, sessionState service.SessionStateReader) layout.Model {
	return toolcommon.NewBase(msg, sessionState, renderList)
}

func renderList(msg *types.Message, s spinner.Spinner, sessionState service.SessionStateReader, width, _ int) string {
	result := ""
	if msg.ToolStatus == types.ToolStatusCompleted || msg.ToolStatus == types.ToolStatusError {
		result = msg.Content
	}
	return toolcommon.RenderTool(msg, s, "", result, width, sessionState.HideToolResults())
}

// NewView renders view_background_agent: the task id as the argument plus the
// already human-formatted result text (Handler.formatView) once it lands.
func NewView(msg *types.Message, sessionState service.SessionStateReader) layout.Model {
	return toolcommon.NewBase(msg, sessionState, toolcommon.SimpleRendererWithResult(
		toolcommon.ExtractField(func(a agenttool.ViewBackgroundAgentArgs) string { return a.TaskID }),
		func(m *types.Message) string { return m.Content },
	))
}

// NewStop renders stop_background_agent: the task id as the argument plus the
// confirmation result text once it lands.
func NewStop(msg *types.Message, sessionState service.SessionStateReader) layout.Model {
	return toolcommon.NewBase(msg, sessionState, toolcommon.SimpleRendererWithResult(
		toolcommon.ExtractField(func(a agenttool.StopBackgroundAgentArgs) string { return a.TaskID }),
		func(m *types.Message) string { return m.Content },
	))
}

// statusIcon picks the leading glyph for the dispatch card from the tool status:
// an animated spinner while the background agent runs, ✓ on success, ✗ on error.
// The Base spinner is ModeSpinnerOnly, so s.View() is a single 1-cell glyph whose
// width matches ✓/✗, keeping the task-text wrap math stable (no elapsed-time
// suffix, unlike toolcommon.Icon). Mirrors the transfertask renderer for visual
// consistency across delegation cards.
func statusIcon(msg *types.Message, s spinner.Spinner) string {
	switch msg.ToolStatus {
	case types.ToolStatusRunning, types.ToolStatusPending:
		return styles.NoStyle.MarginLeft(2).Render(s.View())
	case types.ToolStatusError:
		return styles.ToolErrorIcon.Render("✗")
	default: // Completed and any terminal/unknown state
		return styles.ToolCompletedIcon.Render("✓")
	}
}
