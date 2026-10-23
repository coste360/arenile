//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION (not exported by syscall).
const errSharingViolation syscall.Errno = 32

// lockFileExclusive opens path with share mode 0: while this handle is
// open every other open fails with ERROR_SHARING_VIOLATION. Windows closes
// the handle when the process exits.
func lockFileExclusive(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, errSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("desktop: lock: %w", err)
	}
	return os.NewFile(uintptr(h), path), nil
}

func unlockFile(f *os.File) error { return f.Close() }
