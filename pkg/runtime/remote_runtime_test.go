package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/api"
	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/session"
)

// runStreamRecordingClient is a stubRemoteClient variant that records the
// model ref forwarded to RunAgent / RunAgentWithAgentName and lets the test
// inject a synthetic dispatch error so the early-error path is exercised.
type runStreamRecordingClient struct {
	stubRemoteClient

	runErr        error
	gotModel      string
	gotInvocation int
}

func (c *runStreamRecordingClient) RunAgent(_ context.Context, _, _ string, _ []api.Message, model string) (<-chan Event, error) {
	c.gotInvocation++
	c.gotModel = model
	if c.runErr != nil {
		return nil, c.runErr
	}
	ch := make(chan Event)
	close(ch)
	return ch, nil
}

func (c *runStreamRecordingClient) RunAgentWithAgentName(ctx context.Context, sessionID, agent, _ string, msgs []api.Message, model string) (<-chan Event, error) {
	return c.RunAgent(ctx, sessionID, agent, msgs, model)
}

// TestRemoteRuntime_SetAgentModel_RetainsOverrideOnDispatchError pins the
// fix for the silent-drop bug: when the next RunStream's HTTP dispatch
// fails (network, auth, server unavailable), the queued override MUST
// remain queued so the next attempt still applies the user's chosen
// model. Clearing it eagerly would silently swallow the request.
func TestRemoteRuntime_SetAgentModel_RetainsOverrideOnDispatchError(t *testing.T) {
	t.Parallel()

	client := &runStreamRecordingClient{
		stubRemoteClient: stubRemoteClient{
			cfg: &latest.Config{Agents: latest.Agents{{Name: "test"}}},
		},
		runErr: errors.New("dial tcp: connection refused"),
	}
	rt, err := NewRemoteRuntime(client)
	require.NoError(t, err)

	require.NoError(t, rt.SetAgentModel(t.Context(), "test", "openai/gpt-4o"))

	// First RunStream fails to dispatch — drain the error event.
	sess := &session.Session{ID: "s1"}
	for range rt.RunStream(t.Context(), sess) {
	}
	require.Equal(t, 1, client.gotInvocation)
	require.Equal(t, "openai/gpt-4o", client.gotModel)

	// Second RunStream must re-forward the override since the first
	// attempt never reached the server.
	client.runErr = nil
	for range rt.RunStream(t.Context(), sess) {
	}
	require.Equal(t, 2, client.gotInvocation)
	assert.Equal(t, "openai/gpt-4o", client.gotModel, "override must persist after dispatch error")

	// Once successfully forwarded, a subsequent call without a new
	// SetAgentModel must NOT re-send the same override (it is now
	// owned by the server-side session state).
	client.gotModel = "sentinel"
	for range rt.RunStream(t.Context(), sess) {
	}
	require.Equal(t, 3, client.gotInvocation)
	assert.Empty(t, client.gotModel, "override must clear after successful dispatch")
}

// TestRemoteRuntime_SetAgentModel_LatestQueuedWins guards the
// concurrent-update path: if SetAgentModel is called between the
// snapshot and the post-dispatch clear, the newer ref must NOT be
// silently overwritten.
func TestRemoteRuntime_SetAgentModel_LatestQueuedWins(t *testing.T) {
	t.Parallel()

	rt, err := NewRemoteRuntime(&stubRemoteClient{
		cfg: &latest.Config{Agents: latest.Agents{{Name: "test"}}},
	})
	require.NoError(t, err)

	require.NoError(t, rt.SetAgentModel(t.Context(), "test", "first"))
	require.NoError(t, rt.SetAgentModel(t.Context(), "test", "second"))

	rt.pendingMu.Lock()
	got := rt.pendingModelOverride
	rt.pendingMu.Unlock()
	assert.Equal(t, "second", got)
}
