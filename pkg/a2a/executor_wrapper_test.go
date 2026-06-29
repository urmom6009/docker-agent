package a2a

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQueue struct {
	events []a2a.Event
}

func (m *mockQueue) Write(_ context.Context, event a2a.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockQueue) Read(context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, a2a.TaskVersionMissing, nil
}

func (m *mockQueue) WriteVersioned(_ context.Context, event a2a.Event, _ a2a.TaskVersion) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockQueue) Close() error {
	return nil
}

func TestFixingQueue_NilParts(t *testing.T) {
	t.Parallel()
	mock := &mockQueue{}
	fq := &fixingQueue{queue: mock}

	// Create an artifact update event with nil Parts
	err := fq.Write(t.Context(), &a2a.TaskArtifactUpdateEvent{
		ContextID: "test-context",
		TaskID:    "test-task",
		Append:    true,
		Artifact: &a2a.Artifact{
			ID:    "test-artifact",
			Parts: nil,
		},
	})
	require.NoError(t, err)
	require.Len(t, mock.events, 1)

	written := mock.events[0].(*a2a.TaskArtifactUpdateEvent)
	assert.NotNil(t, written.Artifact.Parts)
	assert.Empty(t, written.Artifact.Parts)

	// Verify it serializes correctly
	data, err := json.Marshal(written.Artifact)
	require.NoError(t, err)

	assert.JSONEq(t, `{"artifactId":"test-artifact","parts":[]}`, string(data))
}

func TestFixingQueue_WithParts(t *testing.T) {
	t.Parallel()
	mock := &mockQueue{}
	fq := &fixingQueue{queue: mock}

	// Create an artifact update event with actual parts
	event := &a2a.TaskArtifactUpdateEvent{
		ContextID: "test-context",
		TaskID:    "test-task",
		Append:    true,
		Artifact: &a2a.Artifact{
			ID: "test-artifact",
			Parts: []a2a.Part{
				a2a.TextPart{Text: "Hello"},
			},
		},
	}

	// Write the event through the fixing queue
	err := fq.Write(t.Context(), event)
	require.NoError(t, err)

	// Verify the event was written unchanged
	require.Len(t, mock.events, 1)
	written := mock.events[0].(*a2a.TaskArtifactUpdateEvent)

	assert.Len(t, written.Artifact.Parts, 1)
}

func TestFixingQueue_NonArtifactEvent(t *testing.T) {
	t.Parallel()
	mock := &mockQueue{}
	fq := &fixingQueue{queue: mock}

	// Create a different type of event
	reqCtx := &a2asrv.RequestContext{
		TaskID:    "test-task",
		ContextID: "test-context",
	}
	event := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCompleted, nil)

	// Write the event through the fixing queue
	err := fq.Write(t.Context(), event)
	require.NoError(t, err)

	// Verify the event was written unchanged
	require.Len(t, mock.events, 1)
	assert.IsType(t, &a2a.TaskStatusUpdateEvent{}, mock.events[0])
}
