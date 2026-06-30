package teamloader

import (
	"context"
	"testing"

	"github.com/docker/docker-agent/pkg/gateway"
)

// testCatalog seeds a fake MCP catalog so teamloader tests that load configs
// with MCP `ref:` toolsets run without a live network call.
var testCatalog = gateway.Catalog{
	// A local (subprocess-based) server entry.
	"local-server": {
		Type: "server",
	},
	// A remote (no subprocess) server entry — used to test that
	// working_dir is rejected at runtime for ref-based remote MCPs.
	"remote-server": {
		Type: "remote",
		Remote: gateway.Remote{
			URL:           "https://mcp.example.com/sse",
			TransportType: "sse",
		},
	},
}

// catalogContext returns a context carrying a static gateway loader serving
// testCatalog, so calls reaching gateway.ServerSpec / RequiredEnvVars resolve
// against it instead of fetching the live Docker catalog. This replaces the
// former package-global override and keeps the tests free of shared mutable
// state.
func catalogContext(t *testing.T) context.Context {
	t.Helper()
	return gateway.WithLoader(t.Context(), gateway.NewStaticLoader(testCatalog))
}
