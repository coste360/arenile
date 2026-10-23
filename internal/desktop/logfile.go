package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Log rotation defaults: 4 files of at most 5 MiB in <data dir>/logs.
const (
	DefaultLogMaxBytes = 5 << 20
	DefaultLogKeep     = 3
	LogFileName        = "arenile-desktop.log"
)

// RotatingFile is an io.Writer that appends to Path and, when a write
// would exceed MaxBytes, renames Path to Path.1 (shifting Path.1 to Path.2
// and so on, dropping the ones beyond Keep). Safe for concurrent use.
// Logs never contain secrets or personal data (AGENTS.md); the rotation
// bounds the disk use of a PC that stays on all season.
type RotatingFile struct {
	Path     string
	MaxBytes int64
	Keep     int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// OpenRotatingFile opens (or creates, 0600) the log file.
func OpenRotatingFile(path string, maxBytes int64, keep int) (*RotatingFile, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultLogMaxBytes
	}
	if keep < 1 {
		keep = DefaultLogKeep
	}
	r := &RotatingFile{Path: path, MaxBytes: maxBytes, Keep: keep}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

// Write implements io.Writer.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > r.MaxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *RotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	r.f = nil
	_ = os.Remove(fmt.Sprintf("%s.%d", r.Path, r.Keep))
	for i := r.Keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.Path, i), fmt.Sprintf("%s.%d", r.Path, i+1))
	}
	if err := os.Rename(r.Path, r.Path+".1"); err != nil {
		return err
	}
	return r.open()
}

// Close closes the file.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
