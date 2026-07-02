//go:build windows

package paths

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// maxRange asks LockFileEx / UnlockFileEx to cover the whole file by
// passing 0xFFFFFFFF for both the low and high 32 bits of the range.
const maxRange = ^uint32(0)

// tryLockExclusive attempts a non-blocking exclusive LockFileEx on f. It
// returns false without error when another process already holds the lock.
func tryLockExclusive(f *os.File) (bool, error) {
	var ol windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		maxRange,
		maxRange,
		&ol,
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

// unlockFile releases the lock previously acquired with tryLockExclusive.
func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0,
		maxRange,
		maxRange,
		&ol,
	)
}
