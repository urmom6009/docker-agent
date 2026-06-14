package sidebar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/tui/messages"
	"github.com/docker/docker-agent/pkg/tui/service"
)

func newBreadcrumbSidebar(t *testing.T) *model {
	t.Helper()
	sess := session.New()
	sessionState := service.NewSessionState(sess)
	sessionState.SetCurrentAgentName("root")

	m := New(sessionState).(*model)
	// Driving the real Update handlers starts the model's spinner, which registers
	// with the process-global animation coordinator. Release it on cleanup so a
	// leaked registration can't make HasActive() true for other sidebar tests
	// (e.g. TestSidebar_TitleRegenerating asserts the first animation is active).
	t.Cleanup(func() { m.spinner.Stop() })
	m.sessionHasContent = true
	m.titleGenerated = true
	m.sessionTitle = "Test"
	m.currentAgent = "root"
	m.availableAgents = []runtime.AgentDetails{
		{Name: "root", Provider: "openai", Model: "gpt-4o", Description: "Orchestrator"},
		{Name: "librarian", Provider: "openai", Model: "gpt-4o", Description: "Finds documents"},
	}
	m.width = 60
	m.height = 60
	return m
}

func streamStarted(agent, sessionID string) *runtime.StreamStartedEvent {
	return &runtime.StreamStartedEvent{
		AgentContext: runtime.AgentContext{AgentName: agent},
		SessionID:    sessionID,
	}
}

// TestAgentChainTracksSessionStack enforces the invariant
// len(agentChain) == len(sessionStack): the chain is pushed on StreamStarted and
// popped on StreamStopped, in lockstep with the session stack.
//
// Not parallel: it drives the real Update handlers, which start the spinner and
// touch the process-global animation coordinator shared across tests.
func TestAgentChainTracksSessionStack(t *testing.T) {
	m := newBreadcrumbSidebar(t)
	require.Len(t, m.agentChain, len(m.sessionStack))

	m.Update(streamStarted("root", "s-root"))
	assert.Equal(t, []string{"root"}, m.agentChain)
	assert.Len(t, m.agentChain, len(m.sessionStack))

	m.Update(streamStarted("librarian", "s-lib"))
	assert.Equal(t, []string{"root", "librarian"}, m.agentChain)
	assert.Len(t, m.agentChain, len(m.sessionStack))

	m.Update(&runtime.StreamStoppedEvent{SessionID: "s-lib"})
	assert.Equal(t, []string{"root"}, m.agentChain)
	assert.Len(t, m.agentChain, len(m.sessionStack))

	m.Update(&runtime.StreamStoppedEvent{SessionID: "s-root"})
	assert.Empty(t, m.agentChain)
	assert.Len(t, m.agentChain, len(m.sessionStack))
}

// Not parallel: drives the real Update handlers, which touch the process-global
// animation coordinator shared across tests.
func TestAgentChainResets(t *testing.T) {
	t.Run("StreamCancelledMsg clears the chain", func(t *testing.T) {
		m := newBreadcrumbSidebar(t)
		m.Update(streamStarted("root", "s-root"))
		m.Update(streamStarted("librarian", "s-lib"))

		m.Update(messages.StreamCancelledMsg{})

		assert.Empty(t, m.agentChain)
		assert.Empty(t, m.sessionStack)
	})

	t.Run("ResetStreamTracking clears the chain", func(t *testing.T) {
		m := newBreadcrumbSidebar(t)
		m.Update(streamStarted("root", "s-root"))
		m.Update(streamStarted("librarian", "s-lib"))

		m.ResetStreamTracking()

		assert.Empty(t, m.agentChain)
		assert.Empty(t, m.sessionStack)
	})
}

// TestDelegationBreadcrumbRendersOnlyWhenNested verifies the breadcrumb shows the
// active chain (e.g. "root ⏵ librarian") only once delegation depth exceeds 1.
func TestDelegationBreadcrumbRendersOnlyWhenNested(t *testing.T) {
	t.Parallel()

	t.Run("hidden at depth <= 1", func(t *testing.T) {
		t.Parallel()
		m := newBreadcrumbSidebar(t)
		m.agentChain = []string{"root"}
		assert.NotContains(t, ansi.Strip(m.View()), "⏵")
	})

	t.Run("shown at depth > 1", func(t *testing.T) {
		t.Parallel()
		m := newBreadcrumbSidebar(t)
		m.agentChain = []string{"root", "librarian"}
		assert.Contains(t, ansi.Strip(m.View()), "root ⏵ librarian")
	})
}

// TestDelegationBreadcrumbPreservesAgentClickZones guards the buildAgentClickZones
// skip logic: the breadcrumb block agentInfo prepends must not become a click
// zone or shift the per-agent rows. Without the skip, the breadcrumb would claim
// the first agent slot and the current-agent row would mis-map.
func TestDelegationBreadcrumbPreservesAgentClickZones(t *testing.T) {
	t.Parallel()

	m := newBreadcrumbSidebar(t)
	m.agentChain = []string{"root", "librarian"}

	_ = m.View() // populate agentClickZones + cachedLines

	for i, line := range m.cachedLines {
		stripped := ansi.Strip(line)
		// The breadcrumb (joined by ⏵) must never be an agent click target.
		if strings.Contains(stripped, "⏵") {
			_, isZone := m.agentClickZones[i]
			assert.False(t, isZone, "breadcrumb line %d must not be a click zone", i)
		}
		// The current-agent roster row (prefixed with ▶) must map to root.
		if strings.Contains(stripped, "▶") {
			assert.Equal(t, "root", m.agentClickZones[i], "current-agent row %d should map to root", i)
		}
	}

	var clickable []string
	for _, name := range m.agentClickZones {
		clickable = append(clickable, name)
	}
	assert.Contains(t, clickable, "root")
	assert.Contains(t, clickable, "librarian")
}
