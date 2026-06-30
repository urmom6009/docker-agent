package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/paths"
)

// These tests mutate process-global state (HOME/XDG env and the directory
// overrides), so they must not run in parallel.

func TestDefaultDirsHonourXDGEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	paths.SetConfigDir("")
	paths.SetDataDir("")
	paths.SetCacheDir("")

	assert.Equal(t, filepath.Join(home, "cfg", "cagent"), paths.GetConfigDir())
	assert.Equal(t, filepath.Join(home, "data", "cagent"), paths.GetDataDir())
	assert.Equal(t, filepath.Join(home, "cache", "cagent"), paths.GetCacheDir())
}

func TestGetDataDirLegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	paths.SetDataDir("")

	// While the new location is absent, an existing ~/.cagent is still used.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cagent"), 0o755))
	assert.Equal(t, filepath.Join(home, ".cagent"), paths.GetDataDir())

	// Once the new location exists it takes over.
	require.NoError(t, os.MkdirAll(filepath.Join(home, "data", "cagent"), 0o755))
	assert.Equal(t, filepath.Join(home, "data", "cagent"), paths.GetDataDir())
}

func TestOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		set    func(string)
		get    func() string
		custom string
	}{
		{"CacheDir", paths.SetCacheDir, paths.GetCacheDir, "/custom/cache"},
		{"ConfigDir", paths.SetConfigDir, paths.GetConfigDir, "/custom/config"},
		{"DataDir", paths.SetDataDir, paths.GetDataDir, "/custom/data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Restore default after the test.
			t.Cleanup(func() { tt.set("") })

			original := tt.get()
			assert.NotEmpty(t, original)

			tt.set(tt.custom)
			assert.Equal(t, tt.custom, tt.get())

			// Empty string restores the default.
			tt.set("")
			assert.Equal(t, original, tt.get())
		})
	}
}

func TestGetHomeDir(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, paths.GetHomeDir())
}

func TestSetRoot(t *testing.T) {
	t.Cleanup(func() { paths.SetRoot("") })

	defaultData := paths.GetDataDir()
	defaultConfig := paths.GetConfigDir()
	defaultCache := paths.GetCacheDir()

	paths.SetRoot("/custom/root")
	assert.Equal(t, filepath.Clean("/custom/root/data"), paths.GetDataDir())
	assert.Equal(t, filepath.Clean("/custom/root/config"), paths.GetConfigDir())
	assert.Equal(t, filepath.Clean("/custom/root/cache"), paths.GetCacheDir())

	// Empty root restores the defaults.
	paths.SetRoot("")
	assert.Equal(t, defaultData, paths.GetDataDir())
	assert.Equal(t, defaultConfig, paths.GetConfigDir())
	assert.Equal(t, defaultCache, paths.GetCacheDir())
}
