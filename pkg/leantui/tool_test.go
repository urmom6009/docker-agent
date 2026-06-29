package leantui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/docker/docker-agent/pkg/tools"
	builtinshell "github.com/docker/docker-agent/pkg/tools/builtin/shell"
	"github.com/docker/docker-agent/pkg/tui/animation"
	tuitypes "github.com/docker/docker-agent/pkg/tui/types"
)

func TestRenderToolOutputTruncatesOutput(t *testing.T) {
	t.Parallel()
	output := strings.Repeat("line\n", 50)
	lines := renderToolOutput(output, 80)

	assert.LessOrEqual(t, len(lines), maxToolOutputLines+1)
	assert.Contains(t, strings.Join(lines, "\n"), "earlier lines")
}

func TestRenderToolUsesFullTUIRenderer(t *testing.T) {
	t.Parallel()
	tv := shellToolView(tuitypes.ToolStatusCompleted)
	tv.message.Content = "hi\n"

	joined := strings.Join(renderTool(*tv, 80), "\n")
	assert.Contains(t, joined, builtinshell.ToolNameShell)
	assert.Contains(t, joined, "echo hi")
	assert.Contains(t, joined, "hi")
	assert.NotContains(t, joined, "Took")
}

func TestRenderToolDoesNotLeakAnimationSubscription(t *testing.T) {
	t.Parallel()
	assert.False(t, animation.HasActive())
	renderToolWithState(*shellToolView(tuitypes.ToolStatusRunning), 80, 3, nil)
	assert.False(t, animation.HasActive())
}

func shellToolView(status tuitypes.ToolStatus) *toolView {
	return newToolView("root", tools.ToolCall{
		ID: "call-1",
		Function: tools.FunctionCall{
			Name:      builtinshell.ToolNameShell,
			Arguments: `{"cmd":"echo hi"}`,
		},
	}, tools.Tool{Name: builtinshell.ToolNameShell}, status)
}
