package useragent

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/version"
)

func TestSetIdentity(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody)
	require.NoError(t, err)

	SetIdentity(req)

	assert.Equal(t, Header, req.Header.Get("User-Agent"))
	assert.Equal(t, version.Version, req.Header.Get(HeaderAgentVersion))
}
