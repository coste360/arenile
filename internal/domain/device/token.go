package device

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/coste360/arenile/internal/domain/auth"
)

// Device tokens have the form ar_dev_<prefix>_<secret>: the prefix (8
// lowercase base32 chars) is stored in clear for display, the full token
// only as SHA-256 hex (devices.token_hash, unique).
const (
	TokenScheme    = "ar_dev_"
	tokenPrefixLen = 8
	tokenSecretLen = 32 // bytes, 256 bits
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewToken returns a new device token and its visible prefix.
func NewToken() (token, prefix string, err error) {
	pb := make([]byte, 5) // 5 bytes -> 8 base32 chars
	sb := make([]byte, tokenSecretLen)
	if _, err := rand.Read(pb); err != nil {
		return "", "", fmt.Errorf("device: token prefix: %w", err)
	}
	if _, err := rand.Read(sb); err != nil {
		return "", "", fmt.Errorf("device: token secret: %w", err)
	}
	prefix = strings.ToLower(b32.EncodeToString(pb))
	secret := strings.ToLower(b32.EncodeToString(sb))
	return TokenScheme + prefix + "_" + secret, prefix, nil
}

// IsToken reports whether s uses the device token scheme (cheap check used
// by the bearer strategy; ParseToken validates the shape).
func IsToken(s string) bool { return strings.HasPrefix(s, TokenScheme) }

// ParseToken validates the shape of a device token and returns its prefix.
func ParseToken(token string) (prefix string, ok bool) {
	rest, found := strings.CutPrefix(token, TokenScheme)
	if !found {
		return "", false
	}
	prefix, secret, found := strings.Cut(rest, "_")
	if !found || len(prefix) != tokenPrefixLen || len(secret) < 40 || len(secret) > 64 {
		return "", false
	}
	for _, c := range prefix + secret {
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return "", false
		}
	}
	return prefix, true
}

// HashToken is the stored form of a device token (SHA-256 hex).
func HashToken(token string) string { return auth.HashToken(token) }

// DisplayPrefix is the visible part of a token ("ar_dev_<prefix>").
func DisplayPrefix(prefix string) string { return TokenScheme + prefix }

// Pairing codes: 8 characters of the Crockford base32 alphabet (no I, L,
// O, U: no ambiguity when typed), 40 bits, shown as ABCD-EFGH. With a 5
// minute lifetime, single use and 10 attempts per minute per IP, guessing
// is not practical.
const (
	pairingCodeLen = 8
	crockford      = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// NewPairingCode returns a random code in display form (ABCD-EFGH).
func NewPairingCode() (string, error) {
	b := make([]byte, pairingCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("device: pairing code: %w", err)
	}
	out := make([]byte, 0, pairingCodeLen+1)
	for i, v := range b {
		if i == pairingCodeLen/2 {
			out = append(out, '-')
		}
		out = append(out, crockford[int(v)%len(crockford)])
	}
	return string(out), nil
}

// NormalizePairingCode turns user input into the canonical 8-character
// form: case-insensitive, spaces and dashes ignored, Crockford aliases
// (O->0, I/L->1) applied. ok is false for any other shape.
func NormalizePairingCode(in string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(in) {
		switch r {
		case ' ', '-':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if !strings.ContainsRune(crockford, r) {
			return "", false
		}
		b.WriteRune(r)
	}
	if b.Len() != pairingCodeLen {
		return "", false
	}
	return b.String(), true
}

// HashPairingCode is the stored form of a pairing code: SHA-256 hex of the
// normalised code. ok is false when the input is not a code.
func HashPairingCode(in string) (hash string, ok bool) {
	c, ok := NormalizePairingCode(in)
	if !ok {
		return "", false
	}
	return auth.HashToken("pairing:" + c), true
}
