package database

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileLockRoundTripPersistsLockFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "memory.lock")
	lock := NewFileLock(lockPath)

	require.NoError(t, lock.Lock(t.Context()))
	require.FileExists(t, lockPath)
	require.NoError(t, lock.Unlock())
	require.FileExists(t, lockPath)

	require.NoError(t, lock.Lock(t.Context()))
	require.NoError(t, lock.Unlock())
	require.FileExists(t, lockPath)
}

func TestFileLockSerializesAcrossProcesses(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "memory.lock")
	lock := NewFileLock(lockPath)
	require.NoError(t, lock.Lock(t.Context()))

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=TestFileLockHelperProcess", "--", lockPath)
	cmd.Env = append(os.Environ(), "MEMORY_LOCK_HELPER=1")

	done := make(chan error, 1)
	require.NoError(t, cmd.Start())
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		require.NoError(t, err)
		t.Fatal("helper acquired the lock before the parent released it")
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, lock.Unlock())

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper did not acquire the lock after the parent released it")
	}
}

func TestFileLockHelperProcess(t *testing.T) {
	if os.Getenv("MEMORY_LOCK_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			lock := NewFileLock(args[i+1])
			if err := lock.Lock(t.Context()); err != nil {
				os.Exit(2)
			}
			if err := lock.Unlock(); err != nil {
				os.Exit(3)
			}
			os.Exit(0)
		}
	}
	os.Exit(4)
}
