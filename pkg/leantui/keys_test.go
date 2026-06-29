package leantui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func singleKey(t *testing.T, b string) key {
	t.Helper()
	p := &inputParser{}
	keys := p.feed([]byte(b))
	require.Len(t, keys, 1)
	return keys[0]
}

func TestParseSimpleKeys(t *testing.T) {
	t.Parallel()
	assert.Equal(t, keyEnter, singleKey(t, "\r").typ)
	assert.Equal(t, keyEnter, singleKey(t, "\n").typ)
	assert.Equal(t, keyTab, singleKey(t, "\t").typ)
	assert.Equal(t, keyBackspace, singleKey(t, "\x7f").typ)
	assert.Equal(t, keyBackspace, singleKey(t, "\x08").typ)
	assert.Equal(t, keyCtrlC, singleKey(t, "\x03").typ)
	assert.Equal(t, keyCtrlD, singleKey(t, "\x04").typ)
	assert.Equal(t, keyHome, singleKey(t, "\x01").typ)
	assert.Equal(t, keyEnd, singleKey(t, "\x05").typ)
	assert.Equal(t, keyCtrlW, singleKey(t, "\x17").typ)
}

func TestParseRunes(t *testing.T) {
	t.Parallel()
	k := singleKey(t, "a")
	assert.Equal(t, keyRune, k.typ)
	assert.Equal(t, []rune{'a'}, k.runes)

	k = singleKey(t, "é")
	assert.Equal(t, keyRune, k.typ)
	assert.Equal(t, []rune{'é'}, k.runes)
}

func TestParseEscapeSequences(t *testing.T) {
	t.Parallel()
	assert.Equal(t, keyUp, singleKey(t, "\x1b[A").typ)
	assert.Equal(t, keyDown, singleKey(t, "\x1b[B").typ)
	assert.Equal(t, keyRight, singleKey(t, "\x1b[C").typ)
	assert.Equal(t, keyLeft, singleKey(t, "\x1b[D").typ)
	assert.Equal(t, keyUp, singleKey(t, "\x1bOA").typ)
	assert.Equal(t, keyWordRight, singleKey(t, "\x1b[1;5C").typ)
	assert.Equal(t, keyWordLeft, singleKey(t, "\x1b[1;5D").typ)
	assert.Equal(t, keyDelete, singleKey(t, "\x1b[3~").typ)
	assert.Equal(t, keyHome, singleKey(t, "\x1b[H").typ)
	assert.Equal(t, keyEnd, singleKey(t, "\x1b[F").typ)
	assert.Equal(t, keyShiftTab, singleKey(t, "\x1b[Z").typ)
	assert.Equal(t, keyWordLeft, singleKey(t, "\x1bb").typ)
	assert.Equal(t, keyWordRight, singleKey(t, "\x1bf").typ)
	assert.Equal(t, keyAltEnter, singleKey(t, "\x1b\r").typ)
}

func TestParseLoneEscape(t *testing.T) {
	t.Parallel()
	assert.Equal(t, keyEsc, singleKey(t, "\x1b").typ)
}

func TestParseBracketedPaste(t *testing.T) {
	t.Parallel()
	k := singleKey(t, "\x1b[200~hello world\x1b[201~")
	assert.Equal(t, keyPaste, k.typ)
	assert.Equal(t, "hello world", string(k.runes))
}

func TestParseBracketedPasteAcrossReads(t *testing.T) {
	t.Parallel()
	p := &inputParser{}
	assert.Empty(t, p.feed([]byte("\x1b[200~hel")))
	assert.Empty(t, p.feed([]byte("lo")))
	keys := p.feed([]byte(" there\x1b[201~"))
	require.Len(t, keys, 1)
	assert.Equal(t, keyPaste, keys[0].typ)
	assert.Equal(t, "hello there", string(keys[0].runes))
}

func TestParseMixedRun(t *testing.T) {
	t.Parallel()
	p := &inputParser{}
	keys := p.feed([]byte("hi\r"))
	require.Len(t, keys, 3)
	assert.Equal(t, keyRune, keys[0].typ)
	assert.Equal(t, keyRune, keys[1].typ)
	assert.Equal(t, keyEnter, keys[2].typ)
}
