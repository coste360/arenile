package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// KeyringTimeout bounds a keyring call: on Linux a locked or missing
// Secret Service can block on D-Bus; the file store then takes over.
const KeyringTimeout = 5 * time.Second

// KeyringSecretStore keeps the secret in the OS keychain (macOS Keychain,
// Windows Credential Manager, Secret Service on Linux) through
// github.com/zalando/go-keyring (MIT). The account is derived from the
// data directory, so two data directories (portable installs, tests) never
// share a key.
type KeyringSecretStore struct {
	Service string
	Account string
}

// NewKeyringSecretStore returns the store of the ARENILE_SECRET_KEY of
// dataDir.
func NewKeyringSecretStore(dataDir string) KeyringSecretStore {
	sum := sha256.Sum256([]byte(filepath.Clean(dataDir)))
	return KeyringSecretStore{Service: BundleID, Account: "secret-key-" + hex.EncodeToString(sum[:6])}
}

// Name implements SecretStore.
func (KeyringSecretStore) Name() string { return "keyring" }

// Get implements SecretStore.
func (k KeyringSecretStore) Get(ctx context.Context) ([]byte, error) {
	v, err := withTimeout(ctx, func() (string, error) { return keyring.Get(k.Service, k.Account) })
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("desktop: keyring: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
	if err != nil {
		return nil, fmt.Errorf("desktop: keyring secret is corrupted: %w", err)
	}
	return key, nil
}

// Set implements SecretStore.
func (k KeyringSecretStore) Set(ctx context.Context, secret []byte) error {
	_, err := withTimeout(ctx, func() (string, error) {
		return "", keyring.Set(k.Service, k.Account, base64.StdEncoding.EncodeToString(secret))
	})
	if err != nil {
		return fmt.Errorf("desktop: keyring: %w", err)
	}
	return nil
}

// withTimeout runs fn with KeyringTimeout (go-keyring has no context).
func withTimeout(ctx context.Context, fn func() (string, error)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, KeyringTimeout)
	defer cancel()
	type result struct {
		v   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ErrSecretKeyMissing: the database exists but no store holds its key
// (e.g. the keyring is locked or was reset). A new key would make the
// encrypted columns unreadable, so the launcher stops instead.
var ErrSecretKeyMissing = errors.New("desktop: the secret key of the existing database was not found in the keychain or in the data folder")

// LoadSecretKey returns the key from the first store that has it, or
// ErrSecretNotFound.
func LoadSecretKey(ctx context.Context, stores ...SecretStore) ([]byte, string, error) {
	for _, s := range stores {
		k, err := s.Get(ctx)
		if err == nil && len(k) >= SecretKeyBytes {
			return k, s.Name(), nil
		}
	}
	return nil, "", ErrSecretNotFound
}

// ResolveSecretKey is the start-up policy: reuse the key, generate one on a
// fresh installation (keyring first, file fallback), refuse to fork the
// key of an existing database (ErrSecretKeyMissing).
func ResolveSecretKey(ctx context.Context, dbExists bool, stores ...SecretStore) ([]byte, string, error) {
	if k, src, err := LoadSecretKey(ctx, stores...); err == nil {
		return k, src, nil
	}
	if dbExists {
		return nil, "", ErrSecretKeyMissing
	}
	return LoadOrCreateSecretKey(ctx, stores...)
}
