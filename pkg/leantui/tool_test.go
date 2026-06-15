package leantui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribeToolCallCommand(t *testing.T) {
	cmd, summary := describeToolCall(`{"command":"ls -l"}`)
	assert.Equal(t, "ls -l", cmd)
	assert.Empty(t, summary)
}

func TestDescribeToolCallPath(t *testing.T) {
	cmd, summary := describeToolCall(`{"path":"/tmp/x"}`)
	assert.Empty(t, cmd)
	assert.Equal(t, "path: /tmp/x", summary)
}

func TestDescribeToolCallInvalidJSON(t *testing.T) {
	cmd, summary := describeToolCall(`{"command": "ls`)
	assert.Empty(t, cmd)
	assert.NotEmpty(t, summary)
}

func TestDescribeToolCallEmpty(t *testing.T) {
	cmd, summary := describeToolCall("")
	assert.Empty(t, cmd)
	assert.Empty(t, summary)
}

func TestRenderToolTruncatesOutput(t *testing.T) {
	output := strings.Repeat("line\n", 50)
	tv := toolView{name: "shell", command: "seq 50", output: output, done: true}
	lines := renderTool(tv, 80)

	// Header + command + earlier-lines note + capped output + footer.
	assert.LessOrEqual(t, len(lines), maxToolOutputLines+5)
	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, "earlier lines")
	assert.Contains(t, joined, "Took")
}
