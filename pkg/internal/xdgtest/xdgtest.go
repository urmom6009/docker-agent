// Package xdgtest neutralises the XDG base-directory environment variables so
// that tests isolating docker-agent state via a temporary $HOME keep working.
//
// Tests point $HOME at a t.TempDir() and expect paths.Get{Config,Data,Cache}Dir
// to resolve under it. That only holds while those getters derive from $HOME;
// they now also honour $XDG_CONFIG_HOME / $XDG_DATA_HOME / $XDG_CACHE_HOME (via
// os.UserConfigDir and friends). On a machine that exports any of them (GitHub
// runners do), config/data/cache resolve to the real, shared directory instead
// of the test's $HOME, so writes leak out (and reads pick up unrelated state) —
// e.g. an alias written by one package's test surfaces in another package's run.
//
// Call Clear from a package's TestMain to restore $HOME-based isolation for the
// whole test binary.
package xdgtest

import "os"

// Clear unsets the XDG base-directory variables for the current process.
func Clear() {
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		_ = os.Unsetenv(v)
	}
}
