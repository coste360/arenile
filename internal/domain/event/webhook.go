package event

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader is the header carrying the webhook signature.
const SignatureHeader = "Arenile-Signature"

// ErrBadSignature is returned when a webhook signature does not verify.
var ErrBadSignature = errors.New("event: invalid webhook signature")

// Sign returns the Arenile-Signature header value for body:
//
//	t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>
//
// The timestamp is part of the signed payload to prevent replays.
func Sign(secret string, body []byte, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	return "t=" + ts + ",v1=" + mac(secret, ts, body)
}

func mac(secret, ts string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts))
	m.Write([]byte("."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// VerifySignature checks a signature header against body. tolerance bounds
// the accepted clock difference (receivers should use about 5 minutes).
// Multiple v1 values are accepted (secret rotation).
func VerifySignature(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return ErrBadSignature
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if d := now.Sub(time.Unix(sec, 0)); d > tolerance || d < -tolerance {
		return fmt.Errorf("%w: timestamp outside tolerance", ErrBadSignature)
	}
	want := mac(secret, ts, body)
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(want)) {
			return nil
		}
	}
	return ErrBadSignature
}

// NewWebhookSecret returns a random endpoint secret ("whsec_<hex>").
func NewWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("event: webhook secret: %w", err)
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// Backoff returns the delay before attempt n+1 after n failed attempts:
// 30s, 1m, 2m, 4m ... capped at 6h.
func Backoff(failedAttempts int) time.Duration {
	if failedAttempts < 1 {
		failedAttempts = 1
	}
	d := 30 * time.Second
	for i := 1; i < failedAttempts; i++ {
		d *= 2
		if d >= 6*time.Hour {
			return 6 * time.Hour
		}
	}
	return d
}
