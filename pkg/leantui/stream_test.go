package leantui

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bareModel builds a model with just the pieces buildLines needs, so the
// rendering pipeline can be exercised without a real App or terminal.
func bareModel(width, height int) *model {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	return &model{
		width:     width,
		height:    height,
		r:         newRenderer(w, width, height),
		editor:    newEditor("type here"),
		ac:        newAutocomplete(),
		tools:     map[string]*toolView{},
		toolStart: map[string]time.Time{},
		status:    statusData{workingDir: "/tmp/project"},
	}
}

func TestStreamingGrowthScrollsAndRendersMarkdown(t *testing.T) {
	m := bareModel(80, 10)
	m.busy = true
	m.render() // initial frame

	m.pending = &pendingBlock{kind: blockAssistant}
	for i := range 40 {
		m.pending.text.WriteString("Paragraph " + strconv.Itoa(i) + " with some streamed text.\n\n")
		lines, cl, cc := m.buildLines()
		require.NotPanics(t, func() { m.r.frame(lines, cl, cc) })
	}

	// Content far exceeds the 10-row viewport, so it must have scrolled.
	assert.Positive(t, m.r.viewportTop)

	// Finalizing the stream turns it into a cached block; the visible output is
	// unchanged because it was already rendered as markdown live.
	m.flushPending()
	assert.Len(t, m.blocks, 1)
	require.NotPanics(t, func() {
		lines, cl, cc := m.buildLines()
		m.r.frame(lines, cl, cc)
	})
}

func TestBuildLinesPlacesCursorOnInput(t *testing.T) {
	m := bareModel(80, 24)
	m.editor.setText("hello")

	lines, cursorLine, cursorCol := m.buildLines()
	require.NotEmpty(t, lines)
	// The cursor line must point at the input row and the column past the prompt.
	assert.Contains(t, lines[cursorLine], "hello")
	assert.Equal(t, promptWidth+5, cursorCol)
}

func TestConversationLinesShowsSpinnerWhenBusy(t *testing.T) {
	m := bareModel(80, 24)
	m.busy = true
	lines := m.conversationLines(80)
	assert.Contains(t, strings.Join(lines, ""), "Working")
}
