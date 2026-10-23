package desktop

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestActivation_ShowsRunningInstance(t *testing.T) {
	shown := make(chan struct{}, 4)
	s, err := ListenActivation(func() { shown <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Token()) != 64 {
		t.Fatalf("token length %d", len(s.Token()))
	}
	ctx := context.Background()
	if err := SendActivation(ctx, s.Addr(), s.Token()); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	select {
	case <-shown:
	case <-time.After(2 * time.Second):
		t.Fatal("onShow not called")
	}

	if err := SendActivation(ctx, s.Addr(), "wrong"); !errors.Is(err, ErrActivationRefused) {
		t.Fatalf("wrong token: %v", err)
	}
	// Garbage and a silent client are dropped without calling onShow.
	c, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
	_ = c.Close()
	select {
	case <-shown:
		t.Fatal("onShow called for an invalid message")
	case <-time.After(200 * time.Millisecond):
	}

	if err := SendActivation(ctx, "192.0.2.1:9", s.Token()); err == nil {
		t.Fatal("non-loopback address accepted")
	}
	addr := s.Addr()
	_ = s.Close()
	if err := SendActivation(ctx, addr, s.Token()); err == nil {
		t.Fatal("closed listener answered")
	}
}

func TestInstanceInfo_ActivationRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := InstanceInfo{PID: 7, BaseURL: "http://127.0.0.1:5", OpenURL: "http://127.0.0.1:5/admin/", ActivateAddr: "127.0.0.1:6", ActivateToken: "abc"}
	if err := WriteInstanceInfo(dir, in); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstanceInfo(dir)
	if err != nil || got != in {
		t.Fatalf("%+v %v", got, err)
	}
}
