package runtime

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/agent"
	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/hooks"
	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/team"
)

// TestAfterLLMCallHook_PopulatesModelID is a regression test for the
// doc/impl mismatch where [hooks.Input.ModelID] is documented as
// populated for after_llm_call but executeAfterLLMCallHooks never
// actually set it — handlers reading model_id always saw an empty
// string. A single successful turn must dispatch after_llm_call with
// ModelID equal to the provider's canonical "<provider>/<model>" id.
func TestAfterLLMCallHook_PopulatesModelID(t *testing.T) {
	t.Parallel()

	const (
		hookName = "test-after-llm-model-id"
		modelID  = "test/mock-model"
	)

	var captured atomic.Pointer[hooks.Input]

	stream := newStreamBuilder().
		AddContent("ok").
		AddStopWithUsage(1, 1).
		Build()
	prov := &mockProvider{id: modelID, stream: stream}

	root := agent.New("root", "test agent",
		agent.WithModel(prov),
		agent.WithHooks(&latest.HooksConfig{
			AfterLLMCall: []latest.HookDefinition{
				{Type: "builtin", Command: hookName},
			},
		}),
	)
	tm := team.New(team.WithAgents(root))

	rt, err := NewLocalRuntime(tm,
		WithSessionCompaction(false),
		WithModelStore(mockModelStore{}),
	)
	require.NoError(t, err)

	require.NoError(t, rt.hooksRegistry.RegisterBuiltin(
		hookName,
		func(_ context.Context, in *hooks.Input, _ []string) (*hooks.Output, error) {
			snap := *in
			captured.Store(&snap)
			return nil, nil
		},
	))

	sess := session.New(session.WithUserMessage("hi"))
	sess.Title = "Unit Test"

	for range rt.RunStream(t.Context(), sess) {
	}

	got := captured.Load()
	require.NotNil(t, got, "after_llm_call hook must fire on a successful turn")
	assert.Equal(t, modelID, got.ModelID,
		"after_llm_call payload must include the canonical model id; "+
			"see pkg/hooks/types.go:177-186 for the documented contract")
}
