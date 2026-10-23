package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// ErrAlreadyRunning: another launcher holds the instance lock of the data
// directory (the second launch only opens the panel of the first).
var ErrAlreadyRunning = errors.New("desktop: Arenile is already running")

const (
	lockFile     = "arenile.lock"
	instanceFile = "instance.json"
	stateFile    = "state.json"
)

// InstanceLock is the single-instance lock of a data directory: an OS
// advisory lock (flock on Unix, an exclusive share mode on Windows) on
// <data dir>/arenile.lock, released by the kernel when the process dies,
// so a crash never leaves a stale lock behind.
type InstanceLock struct {
	f *os.File
}

// AcquireInstanceLock takes the lock or returns ErrAlreadyRunning.
func AcquireInstanceLock(dataDir string) (*InstanceLock, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("desktop: data dir: %w", err)
	}
	f, err := lockFileExclusive(filepath.Join(dataDir, lockFile))
	if err != nil {
		return nil, err
	}
	return &InstanceLock{f: f}, nil
}

// Release frees the lock (idempotent).
func (l *InstanceLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := unlockFile(l.f)
	l.f = nil
	return err
}

// InstanceInfo tells a second launch where the running panel is. It is
// written 0600 in the data directory once the server listens and removed
// at exit. OpenURL carries the one-time first-run token (in the fragment)
// while no owner exists: whoever can read the user's data directory can
// read the database anyway.
type InstanceInfo struct {
	PID     int    `json:"pid"`
	BaseURL string `json:"base_url"`
	OpenURL string `json:"open_url"`
}

// WriteInstanceInfo writes <data dir>/instance.json atomically.
func WriteInstanceInfo(dataDir string, info InstanceInfo) error {
	return writeJSON0600(filepath.Join(dataDir, instanceFile), info)
}

// ReadInstanceInfo reads <data dir>/instance.json.
func ReadInstanceInfo(dataDir string) (InstanceInfo, error) {
	var info InstanceInfo
	b, err := os.ReadFile(filepath.Join(dataDir, instanceFile))
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return info, fmt.Errorf("desktop: instance file: %w", err)
	}
	return info, nil
}

// RemoveInstanceInfo deletes <data dir>/instance.json.
func RemoveInstanceInfo(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, instanceFile))
}

// launcherState is what the launcher remembers between runs.
type launcherState struct {
	// Port is the last loopback port: reusing it keeps the panel URL (and
	// the browser's per-origin storage) stable.
	Port int `json:"port"`
}

func loadState(dataDir string) launcherState {
	var s launcherState
	if b, err := os.ReadFile(filepath.Join(dataDir, stateFile)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func saveState(dataDir string, s launcherState) error {
	return writeJSON0600(filepath.Join(dataDir, stateFile), s)
}

// ReserveLoopbackPort returns a free 127.0.0.1 port, preferring preferred.
// The port is released before returning: app.Run listens on it right away
// (a lost race only fails the start with a clear error).
func ReserveLoopbackPort(preferred int) (int, error) {
	if preferred > 1023 && preferred < 65536 {
		if ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(preferred))); err == nil {
			_ = ln.Close()
			return preferred, nil
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("desktop: no free loopback port: %w", err)
	}
	defer ln.Close()
	a, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("desktop: unexpected listener address %v", ln.Addr())
	}
	return a.Port, nil
}

func writeJSON0600(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
