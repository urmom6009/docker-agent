package config

import (
	"os"
	"testing"

	"github.com/docker/docker-agent/pkg/internal/xdgtest"
)

// TestMain restores $HOME-based isolation for this package's tests. Several of
// them write user-config aliases (including "default") through userconfig while
// isolating only via t.Setenv("HOME", ...); without clearing the XDG variables
// those writes would leak into a shared config dir and pollute other packages'
// test runs (e.g. resolving "default" to an OCI ref). See pkg/internal/xdgtest.
func TestMain(m *testing.M) {
	xdgtest.Clear()
	os.Exit(m.Run())
}
