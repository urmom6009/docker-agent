package leantui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditorInsertAndText(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.insert([]rune("hello"))
	assert.Equal(t, "hello", e.text())
	assert.Equal(t, 5, e.cursor)

	e.moveLeft()
	e.insert([]rune("X"))
	assert.Equal(t, "hellXo", e.text())
}

func TestEditorInsertStripsCarriageReturns(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.insert([]rune("a\r\nb"))
	assert.Equal(t, "a\nb", e.text())
}

func TestEditorBackspaceAndDelete(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.setText("abc")
	e.backspace()
	assert.Equal(t, "ab", e.text())

	e.moveLineStart()
	e.deleteForward()
	assert.Equal(t, "b", e.text())
}

func TestEditorWordOps(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.setText("foo bar baz")
	e.moveWordLeft()
	assert.Equal(t, 8, e.cursor)
	e.moveWordLeft()
	assert.Equal(t, 4, e.cursor)

	e.moveLineStart()
	e.moveWordRight()
	assert.Equal(t, 3, e.cursor)

	e.moveLineEnd()
	e.deleteWordBack()
	assert.Equal(t, "foo bar ", e.text())
}

func TestEditorLayoutSingleLine(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.setText("hello")
	lines, row, col := e.layout(20)
	require.Len(t, lines, 1)
	assert.Equal(t, 0, row)
	assert.Equal(t, promptWidth+5, col)
	assert.LessOrEqual(t, displayWidth(lines[0]), 20)
}

func TestEditorLayoutWrapping(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.setText(strings.Repeat("a", 25))
	lines, row, col := e.layout(12) // content width 10
	require.Len(t, lines, 3)
	assert.Equal(t, 2, row)
	assert.Equal(t, promptWidth+5, col)
	for _, l := range lines {
		assert.LessOrEqual(t, displayWidth(l), 12)
	}
}

func TestEditorLayoutPlaceholder(t *testing.T) {
	t.Parallel()
	e := newEditor("type here")
	lines, row, col := e.layout(40)
	require.Len(t, lines, 1)
	assert.Equal(t, 0, row)
	assert.Equal(t, promptWidth, col)
	assert.Contains(t, lines[0], "type here")
}

func TestEditorVerticalMovement(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.setText("line1\nline2\nline3")
	// cursor at end (line3)
	require.True(t, e.up(40))
	_, _, col := e.layout(40)
	assert.Equal(t, promptWidth+5, col) // preserved column on "line2"

	require.True(t, e.up(40))
	// now on the first row; up should fail and let history take over
	assert.False(t, e.up(40))
}

func TestEditorHistory(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.rememberHistory("first")
	e.rememberHistory("second")

	e.setText("draft")
	e.historyPrev()
	assert.Equal(t, "second", e.text())
	e.historyPrev()
	assert.Equal(t, "first", e.text())
	e.historyPrev()
	assert.Equal(t, "first", e.text()) // clamped

	e.historyNext()
	assert.Equal(t, "second", e.text())
	e.historyNext()
	assert.Equal(t, "draft", e.text()) // restored draft
}

func TestEditorHistoryDeduplicates(t *testing.T) {
	t.Parallel()
	e := newEditor("")
	e.rememberHistory("same")
	e.rememberHistory("same")
	assert.Len(t, e.history, 1)
}
