package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var migrateOnce sync.Once

// MigrateLegacy relocates state from the historical layout (~/.cagent for data,
// ~/.config/cagent for config) to the resolved XDG/native directories. It runs
// once per process and is idempotent across runs. Overridden directories
// (via [SetConfigDir]/[SetDataDir]/[SetRoot] or the CLI flags) are skipped, so
// embedders and explicit --data-dir/--config-dir users are untouched.
func MigrateLegacy() {
	migrateOnce.Do(func() {
		if !configDirOverride.isSet() {
			migrateDir(legacyConfigDir(), xdgConfigDir())
		}
		if !dataDirOverride.isSet() {
			migrateDir(legacyDataDir(), xdgDataDir())
		}
	})
}

// migrateTmpPrefix is the reserved name prefix for in-progress copies inside
// the destination directory. An entry only appears under its final name once
// fully copied and synced, so a crash can never leave a truncated entry that
// looks migrated.
const migrateTmpPrefix = ".cagent-migrate-"

// renameEntry is a seam for tests to simulate cross-device rename failures.
var renameEntry = os.Rename

// migrationLock{Wait,Poll} bound how long a process waits for a concurrent
// migration before skipping this run. Vars so tests can shorten them.
var (
	migrationLockWait = 5 * time.Second
	migrationLockPoll = 100 * time.Millisecond
)

// migrateDir moves src's entries into dst. Entries are moved with os.Rename
// when possible, falling back to a crash-safe copy-then-delete when rename
// fails (typically EXDEV, when src and dst are on different filesystems).
// Existing dst entries are never clobbered (on macOS config and data share one
// dir, so it must merge), and src is removed only once empty. A failed move is
// left in place and dst is removed if it was created but stayed empty, so the
// getters' legacy fallback keeps the data reachable, no loss.
func migrateDir(src, dst string) {
	if src == "" || src == dst || !dirExists(src) {
		return
	}

	unlock, acquired := acquireMigrationLock(dst)
	if !acquired {
		return
	}
	defer unlock()

	// Re-check under the lock: a concurrent process may have completed the
	// migration while we waited.
	if !dirExists(src) {
		return
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		slog.Warn("xdg migration: cannot read legacy directory", "dir", src, "error", err)
		return
	}

	dstExisted := pathExists(dst)
	if err := os.MkdirAll(dst, 0o700); err != nil {
		slog.Warn("xdg migration: cannot create destination directory", "dir", dst, "error", err)
		return
	}
	removeStaleTmp(dst)

	var moved int
	for _, e := range entries {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if pathExists(to) {
			continue
		}
		if err := renameEntry(from, to); err == nil {
			moved++
			continue
		}
		if err := copyEntry(from, to); err != nil {
			if !errors.Is(err, errIrregular) {
				slog.Warn("xdg migration: could not move entry, leaving it in place",
					"from", from, "to", to, "error", err)
			}
			continue
		}
		moved++
	}

	if rem, err := os.ReadDir(src); err == nil && len(rem) == 0 {
		_ = os.Remove(src)
	}
	// If dst was created here but nothing could move into it, remove it (only
	// succeeds when empty) so the getters keep resolving the legacy directory
	// and existing state stays reachable.
	if !dstExisted {
		_ = os.Remove(dst)
	}
	if moved > 0 {
		slog.Info("relocated docker-agent state to XDG directory", "from", src, "to", dst, "entries", moved)
	}
}

// acquireMigrationLock serializes concurrent docker-agent processes around the
// migration of dst so entry moves and source deletions cannot interleave. The
// lock file lives in the OS temp dir, keyed by the hashed destination, and is
// deliberately never unlinked: recreating it would let two processes hold
// locks on different inodes of the same path.
//
// The wait is bounded: if another process still holds the lock after
// migrationLockWait, this run skips the migration (acquired=false) and the
// legacy fallback keeps state reachable until the next start retries. If the
// lock file itself cannot be used (e.g. permissions), migration proceeds
// unlocked since skipping would disable it forever.
func acquireMigrationLock(dst string) (unlock func(), acquired bool) {
	path := migrationLockPath(dst)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		slog.Warn("xdg migration: proceeding without cross-process lock", "path", path, "error", err)
		return func() {}, true
	}

	deadline := time.Now().Add(migrationLockWait)
	for {
		locked, err := tryLockExclusive(f)
		if err != nil {
			_ = f.Close()
			slog.Warn("xdg migration: proceeding without cross-process lock", "path", path, "error", err)
			return func() {}, true
		}
		if locked {
			return func() {
				_ = unlockFile(f)
				_ = f.Close()
			}, true
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			slog.Info("xdg migration: another process is migrating, skipping this run", "path", path)
			return nil, false
		}
		time.Sleep(migrationLockPoll)
	}
}

// migrationLockPath keys the cross-process lock file on the hashed
// destination so concurrent migrations of the same dir share one lock.
func migrationLockPath(dst string) string {
	sum := sha256.Sum256([]byte(dst))
	return filepath.Join(os.TempDir(), "cagent-migrate-"+hex.EncodeToString(sum[:8])+".lock")
}

// removeStaleTmp deletes leftover in-progress copies from a previous run that
// crashed mid-copy. Only migration creates names with migrateTmpPrefix in dst.
func removeStaleTmp(dst string) {
	entries, err := os.ReadDir(dst)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), migrateTmpPrefix) {
			_ = os.RemoveAll(filepath.Join(dst, e.Name()))
		}
	}
}

// errIrregular marks a top-level socket/FIFO/device that a cross-filesystem
// copy cannot replicate; the entry is deliberately left in place.
var errIrregular = errors.New("irregular file cannot be copied")

// copyEntry replicates from into to via a temporary sibling so the final name
// only ever appears complete: copy fully (fsynced), rename atomically, flush
// the parent, and only then delete the source. A crash at any point leaves
// either the source intact or both copies, never a partial destination under
// its final name.
func copyEntry(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	// A top-level socket/FIFO/device cannot be copied across filesystems;
	// leave it in place rather than delete something we could not replicate.
	if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular() {
		slog.Warn("xdg migration: leaving irregular file in place", "path", from, "mode", info.Mode().String())
		return errIrregular
	}

	tmp := filepath.Join(filepath.Dir(to), migrateTmpPrefix+filepath.Base(to))
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := copyTree(from, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, to); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	// Durability barrier: the rename must hit disk before the source goes
	// away, or a power loss could drop both copies.
	syncDir(filepath.Dir(to))
	if err := os.RemoveAll(from); err != nil {
		slog.Warn("xdg migration: entry copied but source could not be removed", "path", from, "error", err)
	}
	return nil
}

// copyTree recursively copies a directory tree, preserving permissions and
// symlinks. Irregular files (sockets, FIFOs, devices) cannot be copied and are
// skipped: they are transient rendezvous points their owners recreate.
func copyTree(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	case info.IsDir():
		if err := os.Mkdir(to, 0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(to, info.Mode().Perm())
	case info.Mode().IsRegular():
		return copyFile(from, to, info.Mode().Perm())
	default:
		slog.Warn("xdg migration: skipping irregular file", "path", from, "mode", info.Mode().String())
		return nil
	}
}

func copyFile(from, to string, perm os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	// Flush content before the tmp tree can be renamed to its final name,
	// otherwise a crash could surface a migrated-looking but truncated file.
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// Chmod explicitly: the mode passed to OpenFile is filtered by umask.
	return os.Chmod(to, perm)
}

// syncDir flushes directory metadata so completed renames survive a crash.
// Windows cannot sync directory handles; the error is best-effort ignored.
func syncDir(path string) {
	if d, err := os.Open(path); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
