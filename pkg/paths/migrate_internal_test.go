package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXDGHelpersHonourEnv(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "cfg")
	data := filepath.Join(base, "data")
	cache := filepath.Join(base, "cache")
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CACHE_HOME", cache)

	assert.Equal(t, filepath.Join(cfg, "cagent"), xdgConfigDir())
	assert.Equal(t, filepath.Join(data, "cagent"), xdgDataDir())
	assert.Equal(t, filepath.Join(cache, "cagent"), xdgCacheDir())
}

// TestNativeDataDirWithoutXDG exercises the OS-native default branch (the
// runtime.GOOS switch) that the XDG-env tests bypass. It pins the concrete
// expected path on the platform the suite runs on.
func TestNativeDataDirWithoutXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "") // force the native fallback

	var want string
	switch runtime.GOOS {
	case "darwin":
		want = filepath.Join(home, "Library", "Application Support", "cagent")
	case "windows":
		// On Windows os.UserHomeDir reads USERPROFILE; LocalAppData drives the
		// data dir. This branch is documented but only asserted when run there.
		t.Skip("native Windows path is environment-specific")
	default:
		want = filepath.Join(home, ".local", "share", "cagent")
	}
	assert.Equal(t, want, xdgDataDir())
}

func TestMigrateDirMovesAndRemovesEmptiedSource(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("A"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "x.txt"), []byte("X"), 0o600))

	migrateDir(src, dst)

	assert.FileExists(t, filepath.Join(dst, "a.txt"))
	assert.FileExists(t, filepath.Join(dst, "sub", "x.txt"))
	assert.False(t, pathExists(src), "fully drained source dir should be removed")
}

func TestMigrateDirMergesWithoutClobbering(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("from-src"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "shared.txt"), []byte("src"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "shared.txt"), []byte("dst"), 0o600))

	migrateDir(src, dst)

	// New entries move over.
	got, err := os.ReadFile(filepath.Join(dst, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "from-src", string(got))

	// Pre-existing destination entries are never overwritten.
	got, err = os.ReadFile(filepath.Join(dst, "shared.txt"))
	require.NoError(t, err)
	assert.Equal(t, "dst", string(got))

	// The un-moved colliding entry stays in src, so src is kept (not removed).
	assert.FileExists(t, filepath.Join(src, "shared.txt"))
	assert.True(t, dirExists(src))
}

func TestMigrateDirNoops(t *testing.T) {
	base := t.TempDir()
	dst := filepath.Join(base, "dst")

	migrateDir("", dst)
	migrateDir(filepath.Join(base, "missing"), dst)
	migrateDir(dst, dst)

	assert.False(t, pathExists(dst), "no-op migrations must not create the destination")
}

func TestMigrateLegacyMovesDefaultLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	legacyData := filepath.Join(home, ".cagent")
	require.NoError(t, os.MkdirAll(legacyData, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyData, "session.db"), []byte("x"), 0o600))
	legacyConfig := filepath.Join(home, ".config", "cagent")
	require.NoError(t, os.MkdirAll(legacyConfig, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyConfig, "config.yaml"), []byte("y"), 0o600))

	resetMigrationState(t)
	MigrateLegacy()

	assert.FileExists(t, filepath.Join(home, "data", "cagent", "session.db"))
	assert.FileExists(t, filepath.Join(home, "config", "cagent", "config.yaml"))
	assert.False(t, pathExists(legacyData), "legacy data dir should be relocated")
	assert.False(t, pathExists(legacyConfig), "legacy config dir should be relocated")
}

// TestMigrateLegacyMergesSharedDestination reproduces the macOS case where the
// config and data directories resolve to the SAME location
// (~/Library/Application Support/cagent). Both legacy dirs must merge into the
// shared destination without either being dropped.
func TestMigrateLegacyMergesSharedDestination(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Same base for config and data => xdgConfigDir() == xdgDataDir().
	shared := filepath.Join(home, "shared")
	t.Setenv("XDG_CONFIG_HOME", shared)
	t.Setenv("XDG_DATA_HOME", shared)
	require.Equal(t, xdgConfigDir(), xdgDataDir(), "test precondition: dirs must collide")

	legacyData := filepath.Join(home, ".cagent")
	require.NoError(t, os.MkdirAll(legacyData, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyData, "session.db"), []byte("S"), 0o600))
	legacyConfig := filepath.Join(home, ".config", "cagent")
	require.NoError(t, os.MkdirAll(legacyConfig, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyConfig, "config.yaml"), []byte("C"), 0o600))

	resetMigrationState(t)
	MigrateLegacy()

	dst := filepath.Join(shared, "cagent")
	assert.FileExists(t, filepath.Join(dst, "session.db"), "data file must survive the merge")
	assert.FileExists(t, filepath.Join(dst, "config.yaml"), "config file must survive the merge")
	assert.False(t, pathExists(legacyData))
	assert.False(t, pathExists(legacyConfig))
}

func TestMigrateLegacySkipsOverriddenDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	legacyData := filepath.Join(home, ".cagent")
	require.NoError(t, os.MkdirAll(legacyData, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyData, "session.db"), []byte("x"), 0o600))

	resetMigrationState(t)
	dataDirOverride.Set(filepath.Join(home, "override"))
	MigrateLegacy()

	// An explicit --data-dir / SetRoot override must leave the legacy dir alone.
	assert.FileExists(t, filepath.Join(legacyData, "session.db"))
	assert.False(t, pathExists(filepath.Join(home, "data", "cagent")))
}

// resetMigrationState clears the once-guard and any directory overrides so a
// test can exercise MigrateLegacy from a clean slate, restoring them after.
func resetMigrationState(t *testing.T) {
	t.Helper()
	migrateOnce = sync.Once{}
	cacheDirOverride.Set("")
	configDirOverride.Set("")
	dataDirOverride.Set("")
	t.Cleanup(func() {
		migrateOnce = sync.Once{}
		cacheDirOverride.Set("")
		configDirOverride.Set("")
		dataDirOverride.Set("")
	})
}
