package openai

import (
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

// oaiCountingTransport wraps a base RoundTripper and counts RoundTrip calls.
type oaiCountingTransport struct {
	base  http.RoundTripper
	calls atomic.Int64
}

func (c *oaiCountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.base.RoundTrip(req)
}

func TestNewClient_TransportWrapperInvokedDirectPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSSEResponse(w)
	}))
	defer server.Close()

	var counter oaiCountingTransport

	cfg := &latest.ModelConfig{
		Provider: "openai",
		Model:    "gpt-4o",
		BaseURL:  server.URL,
		TokenKey: "OPENAI_API_KEY",
	}
	env := environment.NewMapEnvProvider(map[string]string{
		"OPENAI_API_KEY": "test-key",
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

	// Drain the stream so RoundTrip is fully exercised.
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
		writeSSEResponse(w)
	}))
	defer server.Close()

	var counter oaiCountingTransport

	cfg := &latest.ModelConfig{
		Provider: "openai",
		Model:    "gpt-4o",
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

	// Drain the stream so RoundTrip is fully exercised.
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}

	assert.Positive(t, counter.calls.Load(), "transport wrapper RoundTrip should have been called at least once in gateway path")
}

// TestNewClient_WebSocketFallsBackToSSEWhenTransportWrapperSet verifies that
// configuring transport=websocket together with a transport wrapper causes the
// client to fall back to SSE (no wsPool created). gorilla/websocket bypasses
// http.RoundTripper, so websocket would silently drop the wrapper; the fallback
// ensures the wrapper covers every outbound request.
func TestNewClient_WebSocketFallsBackToSSEWhenTransportWrapperSet(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSSEResponse(w)
	}))
	defer server.Close()

	var counter oaiCountingTransport

	cfg := &latest.ModelConfig{
		Provider:     "openai",
		Model:        "gpt-4o-realtime-preview",
		BaseURL:      server.URL,
		TokenKey:     "OPENAI_API_KEY",
		ProviderOpts: map[string]any{"transport": "websocket"},
	}
	env := environment.NewMapEnvProvider(map[string]string{
		"OPENAI_API_KEY": "test-key",
	})

	client, err := NewClient(t.Context(), cfg, env,
		options.WithHTTPTransportWrapper(func(base http.RoundTripper) http.RoundTripper {
			counter.base = base
			return &counter
		}),
	)
	require.NoError(t, err)

	// wsPool must not be created when a transport wrapper is registered.
	assert.Nil(t, client.wsPool, "wsPool should be nil when a transport wrapper is set")
}
