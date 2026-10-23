package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"

	"github.com/coste360/arenile/internal/desktop"
)

// TokenStore keeps the device token of the remote mode: the OS keyring
// first (Keychain, Credential Manager, Secret Service), a 0600 file in the
// data dir when the keyring is unavailable (same strategy as the secret
// key, desktop.SecretStore). The token is never logged.
type TokenStore interface {
	Get(ctx context.Context) (string, error)
	Set(ctx context.Context, token string) error
	Delete(ctx context.Context) error
}

// ErrNoToken: no device token is stored.
var ErrNoToken = errors.New("remote: no device token")

// SecretTokenStore is a TokenStore over a chain of desktop.SecretStore.
type SecretTokenStore struct {
	Stores []desktop.SecretStore
	// deleteKeyring removes the keyring entry (nil in tests).
	deleteKeyring func() error
	filePath      string
}

// NewTokenStore returns the keyring + file token store of dataDir.
func NewTokenStore(dataDir string) *SecretTokenStore {
	sum := sha256.Sum256([]byte(filepath.Clean(dataDir)))
	kr := desktop.KeyringSecretStore{Service: desktop.BundleID, Account: "device-token-" + hex.EncodeToString(sum[:6])}
	path := filepath.Join(dataDir, "remote-token")
	return &SecretTokenStore{
		Stores:        []desktop.SecretStore{kr, desktop.FileSecretStore{Path: path}},
		deleteKeyring: func() error { return keyring.Delete(kr.Service, kr.Account) },
		filePath:      path,
	}
}

// Get implements TokenStore: the first store that has a token wins.
func (s *SecretTokenStore) Get(ctx context.Context) (string, error) {
	for _, st := range s.Stores {
		b, err := st.Get(ctx)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", ErrNoToken
}

// Set implements TokenStore: the first store that accepts the token wins,
// and stale copies in the later stores are removed.
func (s *SecretTokenStore) Set(ctx context.Context, token string) error {
	var errs []error
	for _, st := range s.Stores {
		if err := st.Set(ctx, []byte(token)); err != nil {
			errs = append(errs, err)
			continue
		}
		if f, isFile := st.(desktop.FileSecretStore); s.filePath != "" && (!isFile || f.Path != s.filePath) {
			_ = os.Remove(s.filePath) // the keyring took it: drop a stale file copy
		}
		return nil
	}
	return errors.Join(errs...)
}

// Delete implements TokenStore (idempotent).
func (s *SecretTokenStore) Delete(context.Context) error {
	if s.deleteKeyring != nil {
		// A missing entry or an unavailable keyring is fine: the token is
		// then only in the file, removed below.
		_ = s.deleteKeyring()
	}
	if s.filePath != "" {
		if err := os.Remove(s.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, st := range s.Stores {
		if f, ok := st.(desktop.FileSecretStore); ok && f.Path != s.filePath {
			_ = os.Remove(f.Path)
		}
	}
	return nil
}
