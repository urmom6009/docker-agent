package leantui

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func newTestRenderer(height int) (*renderer, *bytes.Buffer) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	return newRenderer(w, 80, height), &buf
}

func TestRendererFirstFrame(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(24)
	r.frame([]string{"alpha", "beta", "input"}, 2, 0)

	out := buf.String()
	assert.Contains(t, out, seqSyncStart)
	assert.Contains(t, out, seqSyncEnd)
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
	assert.Equal(t, []string{"alpha", "beta", "input"}, r.prev)
}

func TestRendererInPlaceUpdate(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(24)
	r.frame([]string{"alpha", "beta", "in"}, 2, 2)
	buf.Reset()

	// Only the last line changes; the differ should rewrite just that row.
	r.frame([]string{"alpha", "beta", "input"}, 2, 5)
	out := buf.String()
	assert.Contains(t, out, "input")
	assert.NotContains(t, out, "alpha") // unchanged rows are not rewritten
}

func TestRendererAppendScrolls(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(3) // tiny viewport forces scrolling
	r.frame([]string{"l1", "l2", "input"}, 2, 0)
	buf.Reset()

	r.frame([]string{"l1", "l2", "l3", "l4", "input"}, 4, 0)
	out := buf.String()
	// Appending past the bottom scrolls via CRLF and the viewport tracks the tail.
	assert.Contains(t, out, "\r\n")
	assert.Equal(t, max(0, 5-3), r.viewportTop)
}

func TestRendererShrinkClearsTrailing(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(24)
	r.frame([]string{"a", "b", "c", "d"}, 3, 0)
	buf.Reset()

	r.frame([]string{"a", "b"}, 1, 0)
	out := buf.String()
	assert.Contains(t, out, seqEraseLine)
	assert.Equal(t, []string{"a", "b"}, r.prev)
}

func TestRendererNoChangeOnlyMovesCursor(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(24)
	r.frame([]string{"a", "b", "c"}, 0, 0)
	buf.Reset()

	r.frame([]string{"a", "b", "c"}, 2, 0)
	out := buf.String()
	assert.NotContains(t, out, seqEraseLine) // nothing redrawn
	assert.Contains(t, out, seqSyncStart)
}

func TestRendererMoveCursorClampsCurrentRow(t *testing.T) {
	t.Parallel()
	r, _ := newTestRenderer(3)
	r.viewportTop = 5

	var b strings.Builder
	row := r.moveCursor(&b, 100, 5, 0)

	assert.Equal(t, 5, row)
	assert.Contains(t, b.String(), ansi.CursorUp(2))
	assert.NotContains(t, b.String(), ansi.CursorUp(95))
}

func TestRendererResizeForcesFullRedraw(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(24)
	r.frame([]string{"a", "b", "c"}, 0, 0)
	buf.Reset()

	r.setSize(100, 30)
	r.frame([]string{"a", "b", "c"}, 0, 0)
	assert.Contains(t, buf.String(), seqClearScreen)
}

func TestDiffLines(t *testing.T) {
	t.Parallel()
	first, last := diffLines([]string{"a", "b", "c"}, []string{"a", "x", "c"})
	assert.Equal(t, 1, first)
	assert.Equal(t, 1, last)

	first, last = diffLines([]string{"a"}, []string{"a"})
	assert.Equal(t, -1, first)
	assert.Equal(t, -1, last)

	first, last = diffLines([]string{"a"}, []string{"a", "b", "c"})
	assert.Equal(t, 1, first)
	assert.Equal(t, 2, last)
}

func TestRendererEraseBelow(t *testing.T) {
	t.Parallel()
	r, buf := newTestRenderer(24)
	r.frame([]string{"msg1", "msg2", "input", "footer"}, 2, 0)
	buf.Reset()
	r.eraseBelow(2) // drop the input/footer chrome
	out := buf.String()
	assert.Contains(t, out, seqShowCursor)
	assert.Equal(t, 2, r.cursorRow)
}
