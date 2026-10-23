package desktop

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringSecretStore_MockRoundTrip(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	dir := t.TempDir()
	ks := NewKeyringSecretStore(dir)
	if ks.Service != BundleID || ks == NewKeyringSecretStore(t.TempDir()) {
		t.Fatalf("account must depend on the data dir: %+v", ks)
	}
	if _, err := ks.Get(ctx); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("empty keyring: %v", err)
	}
	file := FileSecretStore{Path: filepath.Join(dir, "secret.key")}
	k, src, err := LoadOrCreateSecretKey(ctx, ks, file)
	if err != nil || src != "keyring" {
		t.Fatalf("keyring first: %v %q", err, src)
	}
	if _, err := file.Get(ctx); !errors.Is(err, ErrSecretNotFound) {
		t.Fatal("the key must not be written to the file when the keyring works")
	}
	k2, src2, err := ResolveSecretKey(ctx, true, ks, file)
	if err != nil || src2 != "keyring" || !bytes.Equal(k, k2) {
		t.Fatalf("reuse: %v %q", err, src2)
	}
}

func TestKeyringSecretStore_FallbackToFile(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	t.Cleanup(keyring.MockInit)
	ctx := context.Background()
	dir := t.TempDir()
	ks := NewKeyringSecretStore(dir)
	file := FileSecretStore{Path: filepath.Join(dir, "secret.key")}
	k, src, err := ResolveSecretKey(ctx, false, ks, file)
	if err != nil || src != "file" || len(k) != SecretKeyBytes {
		t.Fatalf("fallback: %v %q", err, src)
	}
	k2, src2, err := ResolveSecretKey(ctx, true, ks, file)
	if err != nil || src2 != "file" || !bytes.Equal(k, k2) {
		t.Fatalf("file reuse: %v %q", err, src2)
	}
}

func TestResolveSecretKey_NeverForksAnExistingDatabase(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	dir := t.TempDir()
	stores := []SecretStore{NewKeyringSecretStore(dir), FileSecretStore{Path: filepath.Join(dir, "secret.key")}}
	if _, _, err := ResolveSecretKey(ctx, true, stores...); !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("existing database without key: %v", err)
	}
	if _, _, err := LoadSecretKey(ctx, stores...); !errors.Is(err, ErrSecretNotFound) {
		t.Fatal("nothing must have been generated")
	}
}
