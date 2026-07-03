//go:build js && wasm

package database

import "os"

func lockFileExclusive(_ *os.File) error {
	return nil
}

func unlockFile(_ *os.File) error {
	return nil
}

func isLockUnavailable(error) bool {
	return false
}
