package leantui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatTokens(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "500", formatTokens(500))
	assert.Equal(t, "999", formatTokens(999))
	assert.Equal(t, "1.0k", formatTokens(1000))
	assert.Equal(t, "1.2k", formatTokens(1234))
	assert.Equal(t, "1.0M", formatTokens(1_000_000))
	assert.Equal(t, "2.5M", formatTokens(2_500_000))
}

func TestComposeLineRightAligns(t *testing.T) {
	t.Parallel()
	out := composeLine("left", "right", 20)
	assert.Equal(t, 20, displayWidth(out))
	assert.GreaterOrEqual(t, len(out), len("left")+len("right"))
	assert.Contains(t, out, "left")
	assert.Contains(t, out, "right")
}

func TestComposeLineTruncatesLeft(t *testing.T) {
	t.Parallel()
	out := composeLine("a very long left side that does not fit", "right", 15)
	assert.LessOrEqual(t, displayWidth(out), 15)
	assert.Contains(t, out, "right")
}

func TestRenderBarWidth(t *testing.T) {
	t.Parallel()
	assert.Equal(t, contextBarWidth, displayWidth(renderBar(0.5)))
	assert.Equal(t, contextBarWidth, displayWidth(renderBar(0)))
	assert.Equal(t, contextBarWidth, displayWidth(renderBar(1)))
	assert.Equal(t, contextBarWidth, displayWidth(renderBar(1.5))) // clamped
}

func TestRenderStatusFitsWidth(t *testing.T) {
	t.Parallel()
	d := statusData{
		workingDir:    "/home/user/project",
		branch:        "main",
		agent:         "coder",
		model:         "openai/gpt-5",
		thinking:      "high",
		contextLength: 24_000,
		contextLimit:  200_000,
		tokens:        24_000,
	}
	lines := renderStatus(d, 80)
	assert.Len(t, lines, 2)
	for _, l := range lines {
		assert.LessOrEqual(t, displayWidth(l), 80)
	}
}
