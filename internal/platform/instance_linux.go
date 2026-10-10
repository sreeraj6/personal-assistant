package platform

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// LockInstance keeps the descriptor open for the app's lifetime. Kernel locks
// are released automatically on crashes; the lock file must never be deleted.
func LockInstance(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another tracker may already be running: %w", err)
	}
	return file, nil
}
