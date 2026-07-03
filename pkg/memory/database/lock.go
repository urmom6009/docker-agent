package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const lockRetryInterval = 10 * time.Millisecond

type processLock chan struct{}

func newProcessLock() processLock {
	lock := make(processLock, 1)
	lock <- struct{}{}
	return lock
}

func (l processLock) Lock(ctx context.Context) error {
	select {
	case <-l:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l processLock) Unlock() {
	select {
	case l <- struct{}{}:
	default:
	}
}

// FileLock is an advisory file lock for coordinating memory database writes
// across docker-agent processes.
//
// The lock file is intentionally never deleted. Keeping a stable sentinel file
// avoids a race where different processes lock different inodes for the same
// logical database.
type FileLock struct {
	path        string
	file        *os.File
	processLock processLock
	mu          sync.Mutex
}

var processLocks sync.Map

// NewFileLock returns a lock using path as its persistent sentinel file.
func NewFileLock(path string) *FileLock {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	processLockValue, _ := processLocks.LoadOrStore(absPath, newProcessLock())
	return &FileLock{
		path:        absPath,
		processLock: processLockValue.(processLock),
	}
}

// LockPathForDatabase returns the companion lock-file path for a memory DB.
func LockPathForDatabase(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "memory.lock")
}

// Lock blocks until the exclusive advisory lock is acquired or ctx is canceled.
func (l *FileLock) Lock(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return nil
	}

	if err := l.lockProcess(ctx); err != nil {
		return err
	}
	processLocked := true
	defer func() {
		if processLocked {
			l.processLock.Unlock()
		}
	}()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("creating memory lock directory %q: %w", filepath.Dir(l.path), err)
	}

	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("opening memory lock file %q: %w", l.path, err)
	}

	for {
		err = lockFileExclusive(f)
		if err == nil {
			l.file = f
			processLocked = false
			return nil
		}
		if !isLockUnavailable(err) {
			_ = f.Close()
			return fmt.Errorf("locking memory lock file %q: %w", l.path, err)
		}

		select {
		case <-ctx.Done():
			_ = f.Close()
			return ctx.Err()
		case <-time.After(lockRetryInterval):
		}
	}
}

func (l *FileLock) lockProcess(ctx context.Context) error {
	return l.processLock.Lock(ctx)
}

// Unlock releases the advisory lock and closes the sentinel file descriptor.
func (l *FileLock) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	f := l.file
	l.file = nil

	unlockErr := unlockFile(f)
	closeErr := f.Close()
	l.processLock.Unlock()
	if unlockErr != nil {
		return fmt.Errorf("unlocking memory lock file %q: %w", l.path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing memory lock file %q: %w", l.path, closeErr)
	}
	return nil
}
