package chat

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/tui/components/messages"
	"github.com/docker/docker-agent/pkg/tui/components/sidebar"
	"github.com/docker/docker-agent/pkg/tui/service"
)

func newDelegationMarkerTestPage() *chatPage {
	sessionState := &service.SessionState{}
	msgs := messages.NewScrollableView(80, 24, sessionState)
	msgs.SetSize(80, 24)
	return &chatPage{
		sidebar:      sidebar.New(sessionState),
		messages:     msgs,
		sessionState: sessionState,
	}
}

// TestAgentSwitchingInsertsReturnMarker covers the Phase 2 design decision to
// bracket only the EXIT of a forwarded sub-agent. AgentSwitching(false, child,
// parent) — emitted when a transfer_task/run_skill sub-session returns — inserts
// a "↩ child → parent" marker, while the entry event AgentSwitching(true, ...)
// inserts nothing (the transfer_task card is the visible entry).
func TestAgentSwitchingInsertsReturnMarker(t *testing.T) {
	t.Parallel()

	t.Run("return (Switching=false) inserts a marker", func(t *testing.T) {
		t.Parallel()
		p := newDelegationMarkerTestPage()

		handled, cmd := p.handleRuntimeEvent(&runtime.AgentSwitchingEvent{
			Switching: false,
			FromAgent: "librarian",
			ToAgent:   "root",
		})

		require.True(t, handled)
		require.NotNil(t, cmd, "a returning sub-agent should insert a marker")
		assert.Contains(t, ansi.Strip(p.messages.View()), "↩ librarian → root")
	})

	t.Run("entry (Switching=true) inserts nothing", func(t *testing.T) {
		t.Parallel()
		p := newDelegationMarkerTestPage()

		handled, cmd := p.handleRuntimeEvent(&runtime.AgentSwitchingEvent{
			Switching: true,
			FromAgent: "root",
			ToAgent:   "librarian",
		})

		require.True(t, handled)
		assert.Nil(t, cmd, "entering a delegation must not insert a return marker")
		assert.NotContains(t, ansi.Strip(p.messages.View()), "↩")
	})

	t.Run("missing agent names insert nothing", func(t *testing.T) {
		t.Parallel()
		p := newDelegationMarkerTestPage()

		handled, cmd := p.handleRuntimeEvent(&runtime.AgentSwitchingEvent{
			Switching: false,
			FromAgent: "",
			ToAgent:   "root",
		})

		require.True(t, handled)
		assert.Nil(t, cmd)
		assert.NotContains(t, ansi.Strip(p.messages.View()), "↩")
	})
}
