package leantui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCommands() []command {
	return []command{
		{name: "new", desc: "Start a new session", kind: cmdBuiltin},
		{name: "help", desc: "Show help", kind: cmdBuiltin},
		{name: "compact", desc: "Compact", kind: cmdBuiltin},
		{name: "plan", desc: "Switch to planner", kind: cmdAgent},
	}
}

func TestAutocompleteActivation(t *testing.T) {
	t.Parallel()
	a := newAutocomplete()
	a.setCommands(testCommands())

	assert.True(t, a.sync("/ne"))
	cur, ok := a.current()
	require.True(t, ok)
	assert.Equal(t, "new", cur.name)

	assert.False(t, a.sync("hello"))  // no leading slash
	assert.False(t, a.sync("/new x")) // contains a space
	assert.False(t, a.sync("/zzzzz")) // no matches
}

func TestAutocompleteNavigation(t *testing.T) {
	t.Parallel()
	a := newAutocomplete()
	a.setCommands(testCommands())
	require.True(t, a.sync("/")) // all commands match

	first, _ := a.current()
	a.moveDown()
	second, _ := a.current()
	assert.NotEqual(t, first.name, second.name)

	a.moveUp()
	back, _ := a.current()
	assert.Equal(t, first.name, back.name)
}

func TestAutocompleteRenderWidth(t *testing.T) {
	t.Parallel()
	a := newAutocomplete()
	a.setCommands(testCommands())
	require.True(t, a.sync("/"))
	rows := a.render(60)
	assert.NotEmpty(t, rows)
	for _, r := range rows {
		assert.LessOrEqual(t, displayWidth(r), 60)
	}
}

func TestAutocompleteBuiltinsBeforeAgent(t *testing.T) {
	t.Parallel()
	matches := filterCommands(testCommands(), "")
	// The agent command "plan" must sort after every built-in.
	last := matches[len(matches)-1]
	assert.Equal(t, "plan", last.name)
	assert.Equal(t, cmdAgent, last.kind)
}
