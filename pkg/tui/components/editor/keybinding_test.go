package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/history"
	"github.com/docker/docker-agent/pkg/tui/core"
)

// TestConfigureNewlineKeybinding verifies the editor wires its newline keys
// from the configurable EditorNewline binding and still layers shift+enter on
// terminals that report keyboard enhancements (issue #1626). Expectations are
// derived from the resolved config so the test holds whether or not a user has
// remapped editor_newline.
func TestConfigureNewlineKeybinding(t *testing.T) {
	t.Parallel()
	core.ResetKeys()
	t.Cleanup(core.ResetKeys)
	want := core.GetKeys().EditorNewline.Keys()

	h, err := history.New(t.TempDir())
	require.NoError(t, err)
	e := New(h).(*editor)

	e.keyboardEnhancementsSupported = false
	e.configureNewlineKeybinding()
	assert.Equal(t, want, e.textarea.KeyMap.InsertNewline.Keys(),
		"without keyboard enhancements the newline keys should match the configured binding")

	e.keyboardEnhancementsSupported = true
	e.configureNewlineKeybinding()
	got := e.textarea.KeyMap.InsertNewline.Keys()
	require.NotEmpty(t, got)
	assert.Equal(t, "shift+enter", got[0], "shift+enter should be offered first on capable terminals")
	assert.Subset(t, got, want, "configured newline keys must remain available")
}
