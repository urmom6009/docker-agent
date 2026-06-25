package sidebar

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agenttool "github.com/docker/docker-agent/pkg/tools/builtin/agent"
	"github.com/docker/docker-agent/pkg/tui/service"
)

func runningTask(id, agent string, startedAt time.Time) agenttool.TaskInfo {
	return agenttool.TaskInfo{
		ID:        id,
		Agent:     agent,
		Task:      agent + " task",
		Status:    agenttool.StatusRunning,
		StartedAt: startedAt,
	}
}

func TestBackgroundAgentsSection_HiddenWhenEmpty(t *testing.T) {
	t.Parallel()

	m := New(&service.SessionState{}).(*model)
	assert.Empty(t, m.backgroundAgentsSection(40))
}

func TestBackgroundAgentsSection_RendersRunningRoster(t *testing.T) {
	t.Parallel()

	m := New(&service.SessionState{}).(*model)
	now := time.Now()
	m.SetBackgroundAgents([]agenttool.TaskInfo{
		runningTask("t1", "researcher", now.Add(-90*time.Second)),
		runningTask("t2", "writer", now.Add(-5*time.Second)),
	})

	out := ansi.Strip(m.backgroundAgentsSection(40))

	assert.Contains(t, out, "Background agents (2)")
	assert.Contains(t, out, "●")
	assert.Contains(t, out, "researcher")
	assert.Contains(t, out, "writer")
	// Elapsed run time is shown per row (reusing the shared duration formatter).
	assert.Contains(t, out, "1m30s")
}

// TestSetBackgroundAgents_FiltersToRunningAndSorts verifies the read-model seam:
// finished tasks linger in the runtime snapshot but are dropped from the panel,
// and the surviving running tasks are ordered by start time so rows stay put
// across polls.
func TestSetBackgroundAgents_FiltersToRunningAndSorts(t *testing.T) {
	t.Parallel()

	m := New(&service.SessionState{}).(*model)
	now := time.Now()
	m.SetBackgroundAgents([]agenttool.TaskInfo{
		{ID: "done", Agent: "old", Status: agenttool.StatusCompleted, StartedAt: now.Add(-time.Hour)},
		runningTask("late", "writer", now.Add(-5*time.Second)),
		runningTask("early", "researcher", now.Add(-90*time.Second)),
		{ID: "stopped", Agent: "cancelled", Status: agenttool.StatusStopped, StartedAt: now},
	})

	require.Len(t, m.backgroundAgents, 2)
	assert.Equal(t, "researcher", m.backgroundAgents[0].Agent)
	assert.Equal(t, "writer", m.backgroundAgents[1].Agent)
}

// TestSetBackgroundAgents_DrainsToEmpty verifies the panel hides again once the
// snapshot no longer contains running tasks.
func TestSetBackgroundAgents_DrainsToEmpty(t *testing.T) {
	t.Parallel()

	m := New(&service.SessionState{}).(*model)
	m.SetBackgroundAgents([]agenttool.TaskInfo{runningTask("t1", "researcher", time.Now())})
	require.Len(t, m.backgroundAgents, 1)

	m.SetBackgroundAgents([]agenttool.TaskInfo{
		{ID: "t1", Agent: "researcher", Status: agenttool.StatusCompleted, StartedAt: time.Now()},
	})
	assert.Empty(t, m.backgroundAgents)
	assert.Empty(t, m.backgroundAgentsSection(40))
}

func TestBackgroundAgentsSection_InRenderSections(t *testing.T) {
	t.Parallel()

	m := New(&service.SessionState{}).(*model)
	m.SetSize(40, 100)

	without := strings.Join(m.renderSections(35), "\n")
	assert.NotContains(t, without, "Background agents")

	m.SetBackgroundAgents([]agenttool.TaskInfo{runningTask("t1", "researcher", time.Now())})
	with := ansi.Strip(strings.Join(m.renderSections(35), "\n"))
	assert.Contains(t, with, "Background agents (1)")
	assert.Contains(t, with, "researcher")
}

// TestDelegationBreadcrumb_AppendsBackgroundCount covers wiring the live
// background-agent count into the Phase 2 delegation breadcrumb.
func TestDelegationBreadcrumb_AppendsBackgroundCount(t *testing.T) {
	t.Parallel()

	t.Run("appends +N background while delegating", func(t *testing.T) {
		t.Parallel()
		m := New(&service.SessionState{}).(*model)
		m.agentChain = []string{"root", "librarian"}
		m.SetBackgroundAgents([]agenttool.TaskInfo{
			runningTask("t1", "researcher", time.Now()),
			runningTask("t2", "writer", time.Now()),
		})

		out := ansi.Strip(m.delegationBreadcrumb(80))
		assert.Contains(t, out, "root ⏵ librarian")
		assert.Contains(t, out, "+2 background")
	})

	t.Run("no addendum without background agents", func(t *testing.T) {
		t.Parallel()
		m := New(&service.SessionState{}).(*model)
		m.agentChain = []string{"root", "librarian"}
		assert.NotContains(t, ansi.Strip(m.delegationBreadcrumb(80)), "background")
	})

	t.Run("no breadcrumb at depth <= 1 even with background agents", func(t *testing.T) {
		t.Parallel()
		m := New(&service.SessionState{}).(*model)
		m.agentChain = []string{"root"}
		m.SetBackgroundAgents([]agenttool.TaskInfo{runningTask("t1", "researcher", time.Now())})
		assert.Empty(t, m.delegationBreadcrumb(80))
	})
}
