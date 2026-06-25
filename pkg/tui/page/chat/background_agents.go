package chat

import (
	"time"

	tea "charm.land/bubbletea/v2"

	agenttool "github.com/docker/docker-agent/pkg/tools/builtin/agent"
)

// backgroundAgentPollInterval is the cadence at which the chat page re-reads the
// runtime's background-agent snapshot to refresh the sidebar's live status
// panel. One second keeps the elapsed times current without measurable cost.
const backgroundAgentPollInterval = time.Second

// backgroundAgentPollMsg ticks the background-agent poll loop.
type backgroundAgentPollMsg struct{}

// startBackgroundAgentPoll starts the background-agent poll loop unless it is
// already running. The loop self-reschedules from handleBackgroundAgentPoll and
// stops once no work remains, so it adds nothing to ordinary single-agent turns.
func (p *chatPage) startBackgroundAgentPoll() tea.Cmd {
	if p.backgroundPollActive {
		return nil
	}
	p.backgroundPollActive = true
	return scheduleBackgroundAgentPoll()
}

func scheduleBackgroundAgentPoll() tea.Cmd {
	return tea.Tick(backgroundAgentPollInterval, func(time.Time) tea.Msg {
		return backgroundAgentPollMsg{}
	})
}

// handleBackgroundAgentPoll pushes the latest runtime snapshot to the sidebar
// and keeps the loop alive while a stream is working or background agents are
// still running. When both are idle it pushes the final (empty) snapshot so the
// panel clears, then stops polling until the next stream starts.
func (p *chatPage) handleBackgroundAgentPoll() tea.Cmd {
	tasks := p.app.BackgroundAgents()
	p.sidebar.SetBackgroundAgents(tasks)

	if p.working || hasRunningBackgroundAgent(tasks) {
		return scheduleBackgroundAgentPoll()
	}
	p.backgroundPollActive = false
	return nil
}

// hasRunningBackgroundAgent reports whether any task in the snapshot is still
// running, gating whether the poll loop continues after the active stream ends
// (background agents can outlive the turn that spawned them).
func hasRunningBackgroundAgent(tasks []agenttool.TaskInfo) bool {
	for _, t := range tasks {
		if t.Status == agenttool.StatusRunning {
			return true
		}
	}
	return false
}
