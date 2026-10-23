package desktop

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type brokenStore struct{}

func (brokenStore) Name() string                        { return "keyring" }
func (brokenStore) Get(context.Context) ([]byte, error) { return nil, errors.New("no keyring") }
func (brokenStore) Set(context.Context, []byte) error   { return errors.New("no keyring") }

func TestLoadOrCreateSecretKey_FallbackAndReuse(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "secret.key")
	file := FileSecretStore{Path: path}
	k1, src, err := LoadOrCreateSecretKey(ctx, brokenStore{}, file)
	if err != nil || src != "file" || len(k1) != SecretKeyBytes {
		t.Fatalf("first run: %v %q %d", err, src, len(k1))
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode %v %v", st, err)
	}
	k2, _, err := LoadOrCreateSecretKey(ctx, brokenStore{}, file)
	if err != nil || !bytes.Equal(k1, k2) {
		t.Fatal("second run must reuse the key")
	}
	if _, _, err := LoadOrCreateSecretKey(ctx, brokenStore{}); err == nil {
		t.Fatal("no writable store must fail")
	}
}
