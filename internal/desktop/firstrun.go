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
// line, never logged). Consume is constant-time and single-use, and the
// token is burnt only when the step it protects succeeded.
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

// Consume checks token, runs complete (the owner creation) while holding
// the token, and invalidates the token only when complete returned nil.
// A failed creation (database busy, I/O error) therefore keeps the link
// usable; concurrent calls are serialised, so the second one finds the
// token gone once the first succeeded. A nil complete only checks and
// invalidates.
func (t *SetupToken) Consume(token string, complete func() error) error {
	if t == nil {
		return control.ErrSetupTokenInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.value == "" || subtle.ConstantTimeCompare([]byte(token), []byte(t.value)) != 1 {
		return control.ErrSetupTokenInvalid
	}
	if complete != nil {
		if err := complete(); err != nil {
			return err
		}
	}
	t.value = ""
	return nil
}
