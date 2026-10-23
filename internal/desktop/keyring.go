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

// Keyring call bounds. Get waits long enough for the user to answer the
// macOS "allow access" prompt (shown after every re-signed build) or to
// unlock the Secret Service at autostart: a timeout here is reported as
// ErrSecretStoreUnavailable, never as a lost key. Set is bounded tightly:
// a store that does not answer is skipped and the file store takes over.
const (
	KeyringGetTimeout = 90 * time.Second
	KeyringSetTimeout = 15 * time.Second
)

// ErrSecretStoreUnavailable: a store (the keyring) did not answer or
// failed, so the key may be there but could not be read. The launcher
// stops with a message asking to unlock the keychain (or answer its
// prompt) and open the app again; it never generates a new key.
var ErrSecretStoreUnavailable = errors.New("desktop: the keychain did not answer or is locked: unlock it (or allow the access) and open Arenile again")

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
	v, err := withTimeout(ctx, KeyringGetTimeout, func() (string, error) { return keyring.Get(k.Service, k.Account) })
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: keyring: %w", ErrSecretStoreUnavailable, err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
	if err != nil {
		return nil, fmt.Errorf("desktop: keyring secret is corrupted: %w", err)
	}
	return key, nil
}

// Set implements SecretStore.
func (k KeyringSecretStore) Set(ctx context.Context, secret []byte) error {
	_, err := withTimeout(ctx, KeyringSetTimeout, func() (string, error) {
		return "", keyring.Set(k.Service, k.Account, base64.StdEncoding.EncodeToString(secret))
	})
	if err != nil {
		return fmt.Errorf("desktop: keyring: %w", err)
	}
	return nil
}

// withTimeout runs fn with a deadline (go-keyring has no context).
func withTimeout(ctx context.Context, d time.Duration, fn func() (string, error)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
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

// LoadSecretKey returns the key from the first store that has it. When no
// store has it the error is ErrSecretNotFound if every store answered
// "not found", else ErrSecretStoreUnavailable wrapping the store errors
// (a store that did not answer may still hold the key).
func LoadSecretKey(ctx context.Context, stores ...SecretStore) ([]byte, string, error) {
	var failures []error
	for _, s := range stores {
		k, err := s.Get(ctx)
		if err == nil && len(k) >= SecretKeyBytes {
			return k, s.Name(), nil
		}
		if err != nil && !errors.Is(err, ErrSecretNotFound) {
			failures = append(failures, fmt.Errorf("%s: %w", s.Name(), err))
		}
	}
	if len(failures) > 0 {
		return nil, "", fmt.Errorf("%w: %w", ErrSecretStoreUnavailable, errors.Join(failures...))
	}
	return nil, "", ErrSecretNotFound
}

// ResolveSecretKey is the start-up policy: reuse the key, generate one on a
// fresh installation (keyring first, file fallback), refuse to fork the
// key of an existing database (ErrSecretKeyMissing when every store
// answered "not found", ErrSecretStoreUnavailable when a store did not
// answer: the key may be in a locked keychain, so the user retries).
func ResolveSecretKey(ctx context.Context, dbExists bool, stores ...SecretStore) ([]byte, string, error) {
	k, src, err := LoadSecretKey(ctx, stores...)
	if err == nil {
		return k, src, nil
	}
	if dbExists {
		if errors.Is(err, ErrSecretStoreUnavailable) {
			return nil, "", err
		}
		return nil, "", ErrSecretKeyMissing
	}
	return LoadOrCreateSecretKey(ctx, stores...)
}
