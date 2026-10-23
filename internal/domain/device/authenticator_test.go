package device

import (
	"context"
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
)

type fakeAuth struct{ calls []string }

func (f *fakeAuth) AuthenticateSession(context.Context, string) (*auth.Principal, error) {
	f.calls = append(f.calls, "session")
	return &auth.Principal{Kind: auth.PrincipalUser}, nil
}

func (f *fakeAuth) AuthenticateAPIKey(context.Context, string) (*auth.Principal, error) {
	f.calls = append(f.calls, "api_key")
	return &auth.Principal{Kind: auth.PrincipalAPIKey}, nil
}

func (f *fakeAuth) AuthenticateDevice(context.Context, string) (*auth.Principal, error) {
	f.calls = append(f.calls, "device")
	return &auth.Principal{Kind: auth.PrincipalUser, DeviceID: "d1"}, nil
}

func TestBearerAuthenticator_Dispatch(t *testing.T) {
	ctx := context.Background()
	f := &fakeAuth{}
	b := BearerAuthenticator{Auth: f, Devices: f}
	if p, _ := b.AuthenticateAPIKey(ctx, "ar_dev_x"); p.DeviceID != "d1" {
		t.Fatal("device token not dispatched to devices")
	}
	if p, _ := b.AuthenticateAPIKey(ctx, "ar_live_x"); p.Kind != auth.PrincipalAPIKey {
		t.Fatal("api key not dispatched to auth")
	}
	if _, err := (BearerAuthenticator{Auth: f}).AuthenticateAPIKey(ctx, "ar_dev_x"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("device token without device service must fail closed")
	}
	d := DeviceOnly{Devices: f}
	if _, err := d.AuthenticateAPIKey(ctx, "ar_live_x"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("api keys must not authenticate on /api/device")
	}
	if _, err := d.AuthenticateSession(ctx, "cookie"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("cookies must not authenticate on /api/device")
	}
}

func TestService_AuthenticateDeviceRejectsMalformed(t *testing.T) {
	s := NewService(Deps{})
	if _, err := s.AuthenticateDevice(context.Background(), "ar_dev_bad"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("err = %v", err)
	}
}
