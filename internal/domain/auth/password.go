package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ErrInvalidHash is returned for malformed PHC strings.
var ErrInvalidHash = errors.New("auth: invalid password hash")

// MinPasswordLength is the minimum accepted password length.
const MinPasswordLength = 10

// Argon2Params are the Argon2id cost parameters.
type Argon2Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params follow current OWASP guidance with a margin.
var DefaultArgon2Params = Argon2Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 2, SaltLength: 16, KeyLength: 32}

// Hasher hashes and verifies passwords with Argon2id in PHC string format:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt b64>$<hash b64>
type Hasher struct {
	Params Argon2Params
}

// NewHasher returns a Hasher with the given parameters.
func NewHasher(p Argon2Params) *Hasher { return &Hasher{Params: p} }

// ValidatePassword checks password policy.
func ValidatePassword(pw string) error {
	if len([]rune(pw)) < MinPasswordLength {
		return fmt.Errorf("%w: at least %d characters", ErrWeakPassword, MinPasswordLength)
	}
	if len(pw) > 1024 {
		return fmt.Errorf("%w: too long", ErrWeakPassword)
	}
	return nil
}

// Hash returns the PHC string of password.
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.Params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	p := h.Params
	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify reports whether password matches the PHC string, in constant time.
func (h *Hasher) Verify(password, phc string) (bool, error) {
	p, salt, key, err := decodePHC(phc)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}

// NeedsRehash reports whether phc was produced with different parameters.
func (h *Hasher) NeedsRehash(phc string) bool {
	p, _, key, err := decodePHC(phc)
	if err != nil {
		return true
	}
	return p.Memory != h.Params.Memory || p.Iterations != h.Params.Iterations ||
		p.Parallelism != h.Params.Parallelism || uint32(len(key)) != h.Params.KeyLength
}

func decodePHC(phc string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(phc, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	if p.Memory == 0 || p.Iterations == 0 || p.Parallelism == 0 || p.Memory > 4*1024*1024 || p.Iterations > 100 {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 128 {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}
