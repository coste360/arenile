package desktop

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SecretKeyBytes is the size of the generated ARENILE_SECRET_KEY.
const SecretKeyBytes = 32

// ErrSecretNotFound: the store holds no secret yet.
var ErrSecretNotFound = errors.New("desktop: secret not found")

// SecretStore keeps one secret (strategy: OS keyring or file).
type SecretStore interface {
	// Name identifies the store in logs ("keyring", "file").
	Name() string
	Get(ctx context.Context) ([]byte, error)
	Set(ctx context.Context, secret []byte) error
}

// FileSecretStore keeps the secret base64-encoded in a 0600 file (fallback
// when the OS keyring is unavailable, e.g. Linux without Secret Service).
type FileSecretStore struct{ Path string }

// Name implements SecretStore.
func (FileSecretStore) Name() string { return "file" }

// Get implements SecretStore.
func (f FileSecretStore) Get(_ context.Context) ([]byte, error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("desktop: read secret file: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("desktop: secret file is corrupted: %w", err)
	}
	return key, nil
}

// Set implements SecretStore (atomic write, 0600).
func (f FileSecretStore) Set(_ context.Context, secret []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(base64.StdEncoding.EncodeToString(secret)+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}

// LoadOrCreateSecretKey returns the secret from the first store that has
// it; when none has it, it generates a new key and saves it in the first
// store that accepts it (keyring first, file as fallback). The key is never
// logged. A key found only in a later store is not copied forward, so an
// unavailable keyring never forks the key.
func LoadOrCreateSecretKey(ctx context.Context, stores ...SecretStore) (key []byte, source string, err error) {
	if len(stores) == 0 {
		return nil, "", errors.New("desktop: no secret store")
	}
	for _, s := range stores {
		k, err := s.Get(ctx)
		if err == nil && len(k) >= SecretKeyBytes {
			return k, s.Name(), nil
		}
	}
	k := make([]byte, SecretKeyBytes)
	if _, err := rand.Read(k); err != nil {
		return nil, "", fmt.Errorf("desktop: generate secret key: %w", err)
	}
	var errs []error
	for _, s := range stores {
		if err := s.Set(ctx, k); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
			continue
		}
		return k, s.Name(), nil
	}
	return nil, "", fmt.Errorf("desktop: save secret key: %w", errors.Join(errs...))
}
