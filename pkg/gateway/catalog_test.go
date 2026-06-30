package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCatalog is a self-contained catalog used by all tests, removing the
// dependency on the live Docker MCP catalog and the network.
var testCatalog = Catalog{
	"github-official": {
		Type: "server",
		Secrets: []Secret{
			{Name: "github.personal_access_token", Env: "GITHUB_PERSONAL_ACCESS_TOKEN"},
		},
	},
	"fetch": {
		Type: "server",
	},
	"apify": {
		Type: "remote",
		Secrets: []Secret{
			{Name: "apify.token", Env: "APIFY_TOKEN"},
		},
		Remote: Remote{
			URL:           "https://mcp.apify.com",
			TransportType: "streamable-http",
		},
	},
}

// testContext returns a context carrying a static loader that serves
// testCatalog, so the package functions never hit the network and tests stay
// parallel-safe (no shared global).
func testContext(t *testing.T) context.Context {
	t.Helper()
	return WithLoader(t.Context(), NewStaticLoader(testCatalog))
}

func TestRequiredEnvVars_local(t *testing.T) {
	t.Parallel()
	secrets, err := RequiredEnvVars(testContext(t), "github-official")
	require.NoError(t, err)

	assert.Len(t, secrets, 1)
	assert.Equal(t, "GITHUB_PERSONAL_ACCESS_TOKEN", secrets[0].Env)
	assert.Equal(t, "github.personal_access_token", secrets[0].Name)
}

func TestRequiredEnvVars_remote(t *testing.T) {
	t.Parallel()
	secrets, err := RequiredEnvVars(testContext(t), "apify")
	require.NoError(t, err)

	assert.Empty(t, secrets)
}

func TestServerSpec_local(t *testing.T) {
	t.Parallel()
	server, err := ServerSpec(testContext(t), "fetch")
	require.NoError(t, err)

	assert.Equal(t, "server", server.Type)
}

func TestServerSpec_remote(t *testing.T) {
	t.Parallel()
	server, err := ServerSpec(testContext(t), "apify")
	require.NoError(t, err)

	assert.Equal(t, "remote", server.Type)
	assert.Equal(t, "https://mcp.apify.com", server.Remote.URL)
	assert.Equal(t, "streamable-http", server.Remote.TransportType)
}

func TestServerSpec_notFound(t *testing.T) {
	t.Parallel()
	_, err := ServerSpec(testContext(t), "nonexistent")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "not found in MCP catalog")
}

func TestParseServerRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "github-official", ParseServerRef("docker:github-official"))
	assert.Equal(t, "github-official", ParseServerRef("github-official"))
}
