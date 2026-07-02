package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
)

// overridable holds an optional directory override backed by an atomic pointer.
// A nil pointer (the zero value) means "use the default".
type overridable struct{ p atomic.Pointer[string] }

// Set stores an override directory. An empty value clears the override.
func (o *overridable) Set(dir string) {
	if dir == "" {
		o.p.Store(nil)
	} else {
		o.p.Store(&dir)
	}
}

// isSet reports whether an override is currently in effect.
func (o *overridable) isSet() bool { return o.p.Load() != nil }

// get returns the override if set, or falls back to the result of defaultFn.
func (o *overridable) get(defaultFn func() string) string {
	if p := o.p.Load(); p != nil {
		return filepath.Clean(*p)
	}
	return defaultFn()
}

var (
	cacheDirOverride  overridable
	configDirOverride overridable
	dataDirOverride   overridable
)

// SetCacheDir overrides the default cache directory returned by [GetCacheDir].
// An empty value restores the default behaviour.
// This should be called early (e.g. during CLI flag processing) before any
// goroutine calls the corresponding getter.
func SetCacheDir(dir string) { cacheDirOverride.Set(dir) }

// SetConfigDir overrides the default config directory returned by [GetConfigDir].
// An empty value restores the default behaviour.
func SetConfigDir(dir string) { configDirOverride.Set(dir) }

// SetDataDir overrides the default data directory returned by [GetDataDir].
// An empty value restores the default behaviour.
func SetDataDir(dir string) { dataDirOverride.Set(dir) }

// SetRoot re-homes all docker-agent state under one directory: data,
// config, and cache land in the "data", "config", and "cache"
// subdirectories of root. It is the one-call override for embedders that
// must keep their embedded agent's state isolated from a docker-agent
// installation on the same machine (e.g. the Gordon assistant in Docker
// Sandboxes homes everything under ~/.sbx/gordon). An empty root restores
// the per-directory defaults.
func SetRoot(root string) {
	if root == "" {
		SetDataDir("")
		SetConfigDir("")
		SetCacheDir("")
		return
	}
	SetDataDir(filepath.Join(root, "data"))
	SetConfigDir(filepath.Join(root, "config"))
	SetCacheDir(filepath.Join(root, "cache"))
}

// GetCacheDir returns the user's cache directory for docker agent.
//
// If an override has been set via [SetCacheDir] it is returned instead.
//
// The default follows the XDG Base Directory Specification, honouring
// $XDG_CACHE_HOME on every platform and otherwise using the OS-native
// location:
//   - $XDG_CACHE_HOME/cagent, default ~/.cache/cagent (Linux)
//   - ~/Library/Caches/cagent (macOS)
//   - %LocalAppData%\cagent (Windows)
func GetCacheDir() string {
	return cacheDirOverride.get(func() string {
		return filepath.Clean(xdgCacheDir())
	})
}

// GetConfigDir returns the user's config directory for docker agent
// (config.yaml, aliases, user id, MCP OAuth tokens, sandbox tokens).
//
// If an override has been set via [SetConfigDir] it is returned instead.
//
// The default follows the XDG Base Directory Specification, honouring
// $XDG_CONFIG_HOME on every platform and otherwise using the OS-native
// location:
//   - $XDG_CONFIG_HOME/cagent, default ~/.config/cagent (Linux)
//   - ~/Library/Application Support/cagent (macOS)
//   - %AppData%\cagent (Windows)
//
// Until the new location exists, an existing legacy ~/.config/cagent is used so
// installs keep working until [MigrateLegacy] relocates it.
func GetConfigDir() string {
	return configDirOverride.get(func() string {
		return resolveDefault(xdgConfigDir(), legacyConfigDir())
	})
}

// GetDataDir returns the user's data directory for docker agent (sessions,
// history, installed tools, OCI store, memory, worktrees, snapshots, ...).
//
// If an override has been set via [SetDataDir] it is returned instead.
//
// The default follows the XDG Base Directory Specification, honouring
// $XDG_DATA_HOME on every platform and otherwise using the OS-native
// location:
//   - $XDG_DATA_HOME/cagent, default ~/.local/share/cagent (Linux)
//   - ~/Library/Application Support/cagent (macOS)
//   - %LocalAppData%\cagent (Windows)
//
// Until the new location exists, an existing legacy ~/.cagent is used so
// installs keep working until [MigrateLegacy] relocates it.
func GetDataDir() string {
	return dataDirOverride.get(func() string {
		return resolveDefault(xdgDataDir(), legacyDataDir())
	})
}

// GetHomeDir returns the user's home directory.
//
// Returns an empty string if the home directory cannot be determined.
func GetHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Clean(homeDir)
}

// --- default directory resolution ---
//
// XDG_* env vars are honoured on every platform (not just Linux); otherwise the
// OS-native dir is used. Mirrors github.com/adrg/xdg.

func xdgConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "cagent")
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "cagent")
	}
	return filepath.Join(os.TempDir(), ".cagent-config")
}

func xdgCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "cagent")
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "cagent")
	}
	return filepath.Join(os.TempDir(), ".cagent-cache")
}

// xdgDataDir derives the data dir per platform since Go has no os.UserDataDir.
func xdgDataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "cagent")
	}
	home := GetHomeDir()
	if home == "" {
		return filepath.Join(os.TempDir(), ".cagent")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "cagent")
	case "windows":
		if dir := os.Getenv("LocalAppData"); dir != "" {
			return filepath.Join(dir, "cagent")
		}
		return filepath.Join(home, "AppData", "Local", "cagent")
	default:
		return filepath.Join(home, ".local", "share", "cagent")
	}
}

func legacyConfigDir() string {
	if home := GetHomeDir(); home != "" {
		return filepath.Join(home, ".config", "cagent")
	}
	return ""
}

func legacyDataDir() string {
	if home := GetHomeDir(); home != "" {
		return filepath.Join(home, ".cagent")
	}
	return ""
}

// resolveDefault returns dst, except while dst does not yet exist and the
// legacy location does, in which case legacy is returned so existing installs
// keep reading their state until [MigrateLegacy] relocates it.
func resolveDefault(dst, legacy string) string {
	if legacy != "" && legacy != dst && !pathExists(dst) && dirExists(legacy) {
		return filepath.Clean(legacy)
	}
	return filepath.Clean(dst)
}

// dirExists reports whether p exists and is a directory.
func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// pathExists reports whether p exists (file, directory or symlink).
func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
