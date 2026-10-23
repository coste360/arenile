package desktop

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestInstanceLock_SingleInstance(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireInstanceLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireInstanceLock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second lock: %v", err)
	}
	// Another data directory is another instance.
	other, err := AcquireInstanceLock(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = other.Release()
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil { // idempotent
		t.Fatal(err)
	}
	again, err := AcquireInstanceLock(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = again.Release()
}

func TestInstanceInfo_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadInstanceInfo(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	in := InstanceInfo{PID: 42, BaseURL: "http://127.0.0.1:5000", OpenURL: "http://127.0.0.1:5000/admin/"}
	if err := WriteInstanceInfo(dir, in); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(filepath.Join(dir, instanceFile)); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	got, err := ReadInstanceInfo(dir)
	if err != nil || got != in {
		t.Fatalf("read %+v %v", got, err)
	}
	RemoveInstanceInfo(dir)
	if _, err := ReadInstanceInfo(dir); err == nil {
		t.Fatal("not removed")
	}
}

func TestReserveLoopbackPort_PrefersLastPort(t *testing.T) {
	p, err := ReserveLoopbackPort(0)
	if err != nil || p == 0 {
		t.Fatal(p, err)
	}
	if got, err := ReserveLoopbackPort(p); err != nil || got != p {
		t.Fatalf("preferred free port not reused: %d %v", got, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Skip("port taken meanwhile")
	}
	defer ln.Close()
	if got, err := ReserveLoopbackPort(p); err != nil || got == p {
		t.Fatalf("busy preferred port returned: %d %v", got, err)
	}
	dir := t.TempDir()
	if err := saveState(dir, launcherState{Port: p}); err != nil || loadState(dir).Port != p {
		t.Fatal("state not persisted")
	}
}
