package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// SessionTokenBytes is the entropy of session tokens (256 bits).
const SessionTokenBytes = 32

// NewSessionToken returns a random base64url session token.
func NewSessionToken() (string, error) {
	b := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the hex SHA-256 of a token. Only hashes are stored.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// API keys have the form ar_live_<prefix>_<secret>: the prefix (8 chars) is
// stored in clear for lookup and display, the full key only as SHA-256.
const (
	apiKeyScheme    = "ar_live_"
	apiKeyPrefixLen = 8
	apiKeySecretLen = 32 // bytes
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewAPIKey returns a new key and its public prefix.
func NewAPIKey() (key, prefix string, err error) {
	pb := make([]byte, 5) // 5 bytes -> 8 base32 chars
	sb := make([]byte, apiKeySecretLen)
	if _, err := rand.Read(pb); err != nil {
		return "", "", fmt.Errorf("auth: api key prefix: %w", err)
	}
	if _, err := rand.Read(sb); err != nil {
		return "", "", fmt.Errorf("auth: api key secret: %w", err)
	}
	prefix = strings.ToLower(b32.EncodeToString(pb))
	secret := strings.ToLower(b32.EncodeToString(sb))
	return apiKeyScheme + prefix + "_" + secret, prefix, nil
}

// ParseAPIKey extracts the prefix of a well-formed key.
func ParseAPIKey(key string) (prefix string, ok bool) {
	rest, found := strings.CutPrefix(key, apiKeyScheme)
	if !found {
		return "", false
	}
	prefix, secret, found := strings.Cut(rest, "_")
	if !found || len(prefix) != apiKeyPrefixLen || len(secret) < 40 || len(secret) > 64 {
		return "", false
	}
	for _, c := range prefix + secret {
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return "", false
		}
	}
	return prefix, true
}

// DisplayPrefix is the visible part of a key ("ar_live_<prefix>").
func DisplayPrefix(prefix string) string { return apiKeyScheme + prefix }

// constantTimeEqual compares two hex hashes in constant time.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
