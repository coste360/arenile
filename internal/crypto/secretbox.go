// Package crypto encrypts secrets at rest (payment provider credentials,
// webhook secrets, TOTP seeds) with AES-256-GCM and a rotating key ring.
//
// Ciphertexts are self-describing strings:
//
//	v1:<key id>:<base64url(nonce || ciphertext || tag)>
//
// The key id is derived from the key material, so adding a new active key to
// ARENILE_SECRET_KEY (first position) and keeping the old ones afterwards lets
// existing values be decrypted and progressively re-encrypted (Rotate).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// KeySize is the required size of every key in bytes (AES-256).
const KeySize = 32

const formatVersion = "v1"

var (
	// ErrInvalidKey is returned when key material is malformed.
	ErrInvalidKey = errors.New("crypto: invalid key")
	// ErrNoKeys is returned when a key ring has no keys.
	ErrNoKeys = errors.New("crypto: key ring is empty")
	// ErrUnknownKey is returned when a ciphertext references a key not in the ring.
	ErrUnknownKey = errors.New("crypto: ciphertext encrypted with unknown key")
	// ErrMalformed is returned for ciphertexts that cannot be parsed.
	ErrMalformed = errors.New("crypto: malformed ciphertext")
	// ErrDecrypt is returned when authentication of a ciphertext fails.
	ErrDecrypt = errors.New("crypto: decryption failed")
)

type key struct {
	id   string
	aead cipher.AEAD
}

// KeyRing holds one active key (used to encrypt) and any number of older keys
// (used only to decrypt). It is safe for concurrent use.
type KeyRing struct {
	keys []key
}

// NewKeyRing builds a key ring from raw 32-byte keys. The first key is active.
func NewKeyRing(raw ...[]byte) (*KeyRing, error) {
	if len(raw) == 0 {
		return nil, ErrNoKeys
	}
	kr := &KeyRing{}
	seen := map[string]bool{}
	for i, k := range raw {
		if len(k) != KeySize {
			return nil, fmt.Errorf("%w: key %d has %d bytes, want %d", ErrInvalidKey, i, len(k), KeySize)
		}
		id := KeyID(k)
		if seen[id] {
			continue
		}
		seen[id] = true
		block, err := aes.NewCipher(k)
		if err != nil {
			return nil, fmt.Errorf("crypto: new cipher: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("crypto: new gcm: %w", err)
		}
		kr.keys = append(kr.keys, key{id: id, aead: aead})
	}
	return kr, nil
}

// ParseKeyRing parses a comma-separated list of base64 (standard or URL,
// padded or not) encoded 32-byte keys, as found in ARENILE_SECRET_KEY.
func ParseKeyRing(s string) (*KeyRing, error) {
	raws, err := ParseKeys(s)
	if err != nil {
		return nil, err
	}
	return NewKeyRing(raws...)
}

// ParseKeys decodes a comma-separated list of base64 keys and validates sizes.
func ParseKeys(s string) ([][]byte, error) {
	var out [][]byte
	for i, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		b, err := decodeBase64(part)
		if err != nil {
			return nil, fmt.Errorf("%w: key %d is not valid base64", ErrInvalidKey, i)
		}
		if len(b) != KeySize {
			return nil, fmt.Errorf("%w: key %d has %d bytes, want %d", ErrInvalidKey, i, len(b), KeySize)
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, ErrNoKeys
	}
	return out, nil
}

func decodeBase64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, ErrInvalidKey
}

// GenerateKey returns a new random key encoded in standard base64.
func GenerateKey() (string, error) {
	b := make([]byte, KeySize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// KeyID returns the public identifier of a key: the first 4 bytes of its
// SHA-256, hex encoded. It never reveals the key.
func KeyID(k []byte) string {
	sum := sha256.Sum256(k)
	return hex.EncodeToString(sum[:4])
}

// ActiveKeyID is the id of the key used for new encryptions.
func (kr *KeyRing) ActiveKeyID() string { return kr.keys[0].id }

// Encrypt seals plaintext with the active key. aad is optional additional
// authenticated data (e.g. the row id) that must be supplied again to decrypt.
func (kr *KeyRing) Encrypt(plaintext, aad []byte) (string, error) {
	k := kr.keys[0]
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce: %w", err)
	}
	sealed := k.aead.Seal(nonce, nonce, plaintext, aad)
	return formatVersion + ":" + k.id + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// EncryptString is Encrypt for string values without AAD.
func (kr *KeyRing) EncryptString(plaintext string) (string, error) {
	return kr.Encrypt([]byte(plaintext), nil)
}

// Decrypt opens a ciphertext produced by Encrypt with any key in the ring.
func (kr *KeyRing) Decrypt(ciphertext string, aad []byte) ([]byte, error) {
	kid, payload, err := parse(ciphertext)
	if err != nil {
		return nil, err
	}
	k, ok := kr.find(kid)
	if !ok {
		return nil, ErrUnknownKey
	}
	ns := k.aead.NonceSize()
	if len(payload) < ns+k.aead.Overhead() {
		return nil, ErrMalformed
	}
	pt, err := k.aead.Open(nil, payload[:ns], payload[ns:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// DecryptString is Decrypt for string values without AAD.
func (kr *KeyRing) DecryptString(ciphertext string) (string, error) {
	b, err := kr.Decrypt(ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NeedsRotation reports whether ciphertext was not encrypted with the active key.
func (kr *KeyRing) NeedsRotation(ciphertext string) bool {
	kid, _, err := parse(ciphertext)
	if err != nil {
		return true
	}
	return kid != kr.ActiveKeyID()
}

// Rotate re-encrypts ciphertext with the active key. If it already uses the
// active key it is returned unchanged.
func (kr *KeyRing) Rotate(ciphertext string, aad []byte) (string, error) {
	if !kr.NeedsRotation(ciphertext) {
		return ciphertext, nil
	}
	pt, err := kr.Decrypt(ciphertext, aad)
	if err != nil {
		return "", err
	}
	return kr.Encrypt(pt, aad)
}

func (kr *KeyRing) find(id string) (key, bool) {
	for _, k := range kr.keys {
		if k.id == id {
			return k, true
		}
	}
	return key{}, false
}

func parse(ciphertext string) (kid string, payload []byte, err error) {
	parts := strings.SplitN(ciphertext, ":", 3)
	if len(parts) != 3 || parts[0] != formatVersion || parts[1] == "" {
		return "", nil, ErrMalformed
	}
	payload, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", nil, ErrMalformed
	}
	return parts[1], payload, nil
}
