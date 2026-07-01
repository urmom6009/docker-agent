package kit

import (
	"os"
	"testing"

	"github.com/docker/docker-agent/pkg/internal/xdgtest"
)

// TestMain restores $HOME-based isolation for this package's tests. They stage
// kits for the "default" agent, which resolves the agent config through
// userconfig; clearing the XDG variables keeps that resolution anchored to the
// test's temporary $HOME instead of a real, shared config dir. See
// pkg/internal/xdgtest.
func TestMain(m *testing.M) {
	xdgtest.Clear()
	os.Exit(m.Run())
}
