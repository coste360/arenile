package checkout

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// TokenBytes is the entropy of checkout session tokens (256 bit, spec 12.2).
const TokenBytes = 32

// NewToken returns a random URL-safe token (43 characters, no padding) and
// its storage hash. The raw token is returned to the client once and never
// stored or logged.
func NewToken() (token, hash string, err error) {
	b := make([]byte, TokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("checkout: token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is the value stored in checkout_sessions.token_hash: SHA-256
// hex of the raw token. Lookups are by hash, so timing attacks on the
// comparison are not possible.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
