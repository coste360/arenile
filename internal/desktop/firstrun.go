package desktop

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/coste360/arenile/internal/desktop/control"
)

// SetupToken is the one-time token of the first owner creation. The
// launcher generates one per start while the database has no users and
// opens /admin/first-run#token=<token> (fragment: never sent in a request
// line, never logged). Consume is constant-time and single-use.
type SetupToken struct {
	mu    sync.Mutex
	value string
}

// NewSetupToken returns a random 256-bit token.
func NewSetupToken() (*SetupToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("desktop: setup token: %w", err)
	}
	return &SetupToken{value: base64.RawURLEncoding.EncodeToString(b)}, nil
}

// Value is the token to put in the browser URL fragment.
func (t *SetupToken) Value() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.value
}

// Consume checks token and invalidates it on success.
func (t *SetupToken) Consume(token string) error {
	if t == nil {
		return control.ErrSetupTokenInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.value == "" || subtle.ConstantTimeCompare([]byte(token), []byte(t.value)) != 1 {
		return control.ErrSetupTokenInvalid
	}
	t.value = ""
	return nil
}
