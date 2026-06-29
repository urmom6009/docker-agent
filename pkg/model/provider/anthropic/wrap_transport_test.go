package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/chat"
	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/environment"
	"github.com/docker/docker-agent/pkg/model/provider/options"
)

// countingTransport wraps a base RoundTripper and counts how many times
// RoundTrip is called.
type countingTransport struct {
	base  http.RoundTripper
	calls atomic.Int64
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.base.RoundTrip(req)
}

// writeMinimalAnthropicSSE writes a bare-minimum valid Anthropic SSE stream
// so that the streaming client does not error before we can observe transport invocation.
func writeMinimalAnthropicSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)

	writeEvent := func(eventType string, payload any) {
		data, _ := json.Marshal(payload)
		_, _ = w.Write([]byte("event: " + eventType + "\n"))
		_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	writeEvent("message_start", map[string]any{
		"type":    "message_start",
		"message": map[string]any{"id": "msg_test", "model": "claude-test", "role": "assistant", "type": "message", "content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": 5, "output_tokens": 0}},
	})
	writeEvent("content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]any{
			"type": "text",
			"text": "",
		},
	})
	writeEvent("content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "text_delta", "text": "hi"},
	})
	writeEvent("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeEvent("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": 1},
	})
	writeEvent("message_stop", map[string]any{"type": "message_stop"})
}

func TestNewClient_TransportWrapperInvokedDirectPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeMinimalAnthropicSSE(w)
	}))
	defer server.Close()

	var counter countingTransport

	cfg := &latest.ModelConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-latest",
		BaseURL:  server.URL,
	}
	env := environment.NewMapEnvProvider(map[string]string{
		"ANTHROPIC_API_KEY": "test-key",
	})

	client, err := NewClient(t.Context(), cfg, env,
		options.WithHTTPTransportWrapper(func(base http.RoundTripper) http.RoundTripper {
			counter.base = base
			return &counter
		}),
	)
	require.NoError(t, err)

	stream, err := client.CreateChatCompletionStream(t.Context(), []chat.Message{
		{Role: chat.MessageRoleUser, Content: "hello"},
	}, nil)
	require.NoError(t, err)
	defer stream.Close()

	// Drain the stream so RoundTrip has been fully exercised.
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}

	assert.Positive(t, counter.calls.Load(), "transport wrapper RoundTrip should have been called at least once")
}

func TestNewClient_TransportWrapperInvokedGatewayPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeMinimalAnthropicSSE(w)
	}))
	defer server.Close()

	var counter countingTransport

	cfg := &latest.ModelConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-latest",
	}
	// server.URL is 127.0.0.1 which IsTrustedDockerURL considers trusted,
	// so we must supply the Docker Desktop token.
	env := environment.NewMapEnvProvider(map[string]string{
		environment.DockerDesktopTokenEnv: "test-dd-token",
	})

	client, err := NewClient(t.Context(), cfg, env,
		options.WithGateway(server.URL),
		options.WithHTTPTransportWrapper(func(base http.RoundTripper) http.RoundTripper {
			counter.base = base
			return &counter
		}),
	)
	require.NoError(t, err)

	stream, err := client.CreateChatCompletionStream(t.Context(), []chat.Message{
		{Role: chat.MessageRoleUser, Content: "hello"},
	}, nil)
	require.NoError(t, err)
	defer stream.Close()

	// Drain the stream so RoundTrip has been fully exercised.
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}

	assert.Positive(t, counter.calls.Load(), "transport wrapper RoundTrip should have been called at least once in gateway path")
}
