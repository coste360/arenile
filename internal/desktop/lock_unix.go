//go:build !windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockFileExclusive opens path and takes a non-blocking exclusive flock.
// flock locks belong to the open file description, so a second open in
// the same process conflicts too (the tests rely on it).
func lockFileExclusive(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: data directory path
	if err != nil {
		return nil, fmt.Errorf("desktop: lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // G115: file descriptors fit in int
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("desktop: lock: %w", err)
	}
	return f, nil
}

func unlockFile(f *os.File) error {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:gosec // G115: file descriptors fit in int
	return f.Close()
}
