//go:build unix

package paths

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockExclusive attempts a non-blocking exclusive flock(2) on f. It returns
// false without error when another process already holds the lock.
func tryLockExclusive(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

// unlockFile releases the lock previously acquired with tryLockExclusive.
func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
