package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/app"
	"github.com/docker/docker-agent/pkg/session"
	agenttool "github.com/docker/docker-agent/pkg/tools/builtin/agent"
	"github.com/docker/docker-agent/pkg/tui/service"
)

func TestHasRunningBackgroundAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tasks []agenttool.TaskInfo
		want  bool
	}{
		{"nil", nil, false},
		{"only finished", []agenttool.TaskInfo{
			{Status: agenttool.StatusCompleted},
			{Status: agenttool.StatusStopped},
		}, false},
		{"one running", []agenttool.TaskInfo{
			{Status: agenttool.StatusCompleted},
			{Status: agenttool.StatusRunning},
		}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, hasRunningBackgroundAgent(tc.tasks))
		})
	}
}

// TestStartBackgroundAgentPoll_RunsSingleLoop guards the idempotency of the poll
// starter: nested streams each call it, but only one tea.Tick loop must run.
func TestStartBackgroundAgentPoll_RunsSingleLoop(t *testing.T) {
	t.Parallel()

	p := &chatPage{}

	require.NotNil(t, p.startBackgroundAgentPoll(), "first start schedules a tick")
	assert.True(t, p.backgroundPollActive)
	assert.Nil(t, p.startBackgroundAgentPoll(), "second start is a no-op while the loop runs")
}

func TestHandleBackgroundAgentPoll_StopsWhenIdle(t *testing.T) {
	t.Parallel()

	sess := session.New()
	p := New(app.New(t.Context(), queueTestRuntime{}, sess), service.NewSessionState(sess)).(*chatPage)
	p.backgroundPollActive = true
	p.working = false

	assert.Nil(t, p.handleBackgroundAgentPoll(), "no stream and no background tasks stops the loop")
	assert.False(t, p.backgroundPollActive)
}

func TestHandleBackgroundAgentPoll_ContinuesWhileWorking(t *testing.T) {
	t.Parallel()

	sess := session.New()
	p := New(app.New(t.Context(), queueTestRuntime{}, sess), service.NewSessionState(sess)).(*chatPage)
	p.backgroundPollActive = true
	p.working = true

	assert.NotNil(t, p.handleBackgroundAgentPoll(), "an active stream keeps the poll loop alive")
	assert.True(t, p.backgroundPollActive)
}
