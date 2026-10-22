package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default algorithm, required by authenticator apps
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (RFC 6238 defaults understood by every authenticator app).
const (
	totpPeriod = 30 * time.Second
	totpDigits = 6
	totpSkew   = 1 // accepted steps before/after the current one
)

// NewTOTPSecret returns a random 160-bit secret in base32 (no padding).
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: totp secret: %w", err)
	}
	return b32.EncodeToString(b), nil
}

// TOTPCode computes the code for a secret at time step counter.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return "", fmt.Errorf("auth: totp secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1_000_000), nil
}

// TOTPStep returns the time step of t.
func TOTPStep(t time.Time) int64 { return t.Unix() / int64(totpPeriod/time.Second) }

// VerifyTOTP checks code against secret at time now (±1 step) and returns the
// matched step, which callers must persist to prevent replays.
func VerifyTOTP(secret, code string, now time.Time) (int64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	matched := int64(0)
	ok := 0
	for d := -totpSkew; d <= totpSkew; d++ {
		want, err := TOTPCode(secret, cur+int64(d))
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			matched = cur + int64(d)
			ok = 1
		}
	}
	return matched, ok == 1
}

// TOTPURL builds the otpauth:// URI shown as a QR code.
func TOTPURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(int(totpPeriod/time.Second)))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
