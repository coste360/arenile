// Package pushrelay is the HTTP client of the Coste360 push relay (ADR 0027):
// the instance sends one signed POST per device to notify, the relay
// forwards it to APNs or FCM with the credentials of the published app.
//
// The request body is the closed Payload type: an opaque native token, the
// platform, the APNs environment and a message key of a closed catalogue
// with its locale. There is no free-text field, so personal data cannot be
// sent by construction.
//
// Authentication: Arenile-Relay-Key carries the key id, Arenile-Signature
// the HMAC-SHA256 of "<unix time>.<body>" with the key secret (the
// signature scheme of outgoing webhooks, event.Sign): the secret never
// travels. The package depends only on the standard library so that
// internal/config can parse keys without pulling the domain in.
package pushrelay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is the endpoint of the Coste360 relay (ARENILE_PUSH_RELAY_URL).
const DefaultURL = "https://push.coste360.it/v1/notifications"

// ProtocolVersion is Payload.V.
const ProtocolVersion = 1

// Headers of a relay request.
const (
	KeyHeader       = "Arenile-Relay-Key"
	SignatureHeader = "Arenile-Signature"
)

// DefaultTTL is how long APNs/FCM keep an undelivered notification.
const DefaultTTL = time.Hour

// maxRetryAfter caps a Retry-After answered by the relay.
const maxRetryAfter = time.Hour

// ErrInvalidKey is returned by ParseKey; it never contains the secret.
var ErrInvalidKey = errors.New("pushrelay: key must be <id>.<secret> with an id of 4-64 characters [a-z0-9_-] and a secret of at least 32 characters")

var keyIDPattern = regexp.MustCompile(`^[a-z0-9_-]{4,64}$`)

// Key is the instance key issued by Coste360 (ARENILE_PUSH_RELAY_KEY). Its
// String, GoString and LogValue never reveal the secret.
type Key struct {
	ID     string
	secret string
}

// ParseKey parses "<id>.<secret>". The empty string is the zero Key (push
// not configured).
func ParseKey(s string) (Key, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Key{}, nil
	}
	id, secret, ok := strings.Cut(s, ".")
	if !ok || !keyIDPattern.MatchString(id) || len(secret) < 32 || strings.ContainsAny(secret, " \t\r\n") {
		return Key{}, ErrInvalidKey
	}
	return Key{ID: id, secret: secret}, nil
}

// Configured reports whether the key is set.
func (k Key) Configured() bool { return k.ID != "" && k.secret != "" }

// String implements fmt.Stringer without the secret.
func (k Key) String() string {
	if !k.Configured() {
		return ""
	}
	return k.ID + ".***"
}

// GoString keeps %#v from printing the secret.
func (k Key) GoString() string { return "pushrelay.Key{ID:" + strconv.Quote(k.ID) + "}" }

// LogValue implements slog.LogValuer without the secret.
func (k Key) LogValue() slog.Value { return slog.StringValue(k.String()) }

// Sign returns the Arenile-Signature value of body at the given time:
// t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>.
func (k Key) Sign(body []byte, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	m := hmac.New(sha256.New, []byte(k.secret))
	m.Write([]byte(ts))
	m.Write([]byte("."))
	m.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

// Message is a key of the closed catalogue of the relay and the locale it
// is rendered in.
type Message struct {
	Key    string `json:"key"`
	Locale string `json:"locale"`
}

// Payload is the request body, protocol version 1. Every field is listed
// here: adding one is a protocol change (new ADR).
type Payload struct {
	V            int     `json:"v"`
	ID           string  `json:"id"`
	DevicePushID string  `json:"device_push_id"`
	Platform     string  `json:"platform"`
	Environment  string  `json:"environment"`
	Message      Message `json:"message"`
	TTLSeconds   int     `json:"ttl_seconds"`
}

// Notification is what the domain asks to send.
type Notification struct {
	// ID identifies the delivery (deduplication on the relay).
	ID string
	// DevicePushID is the native APNs/FCM token.
	DevicePushID string
	Platform     string
	Environment  string
	Message      Message
	// TTL defaults to DefaultTTL.
	TTL time.Duration
}

// BuildPayload returns the body of n.
func BuildPayload(n Notification) Payload {
	ttl := n.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return Payload{
		V: ProtocolVersion, ID: n.ID, DevicePushID: n.DevicePushID, Platform: n.Platform,
		Environment: n.Environment, Message: n.Message, TTLSeconds: int(ttl / time.Second),
	}
}

// Outcome classifies a relay answer.
type Outcome int

// Outcomes of Send.
const (
	// Delivered: 2xx, the relay accepted the notification.
	Delivered Outcome = iota + 1
	// Gone: 410, the native token is no longer valid; forget it.
	Gone
	// Rejected: any other 4xx; retrying would not help.
	Rejected
	// Retry: 429, 5xx or a network error; try again later.
	Retry
)

// Result is the outcome of one request. Status is the HTTP status (0 on a
// network error); RetryAfter is the relay's Retry-After, capped at an hour.
// Error is a short description without tokens or secrets.
type Result struct {
	Outcome    Outcome
	Status     int
	RetryAfter time.Duration
	Error      string
}

// Client sends notifications to the relay.
type Client struct {
	URL     string
	Key     Key
	Version string
	HTTP    *http.Client
	// Now defaults to time.Now (signature timestamp only).
	Now func() time.Time
}

// Configured reports whether the client can send.
func (c Client) Configured() bool { return c.URL != "" && c.Key.Configured() }

// Send posts n to the relay. It never returns the token or the secret in
// Result.Error.
func (c Client) Send(ctx context.Context, n Notification) Result {
	if !c.Configured() {
		return Result{Outcome: Rejected, Error: "relay not configured"}
	}
	body, err := json.Marshal(BuildPayload(n))
	if err != nil {
		return Result{Outcome: Rejected, Error: "encode payload"}
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return Result{Outcome: Rejected, Error: "build request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent(c.Version))
	req.Header.Set(KeyHeader, c.Key.ID)
	req.Header.Set(SignatureHeader, c.Key.Sign(body, now()))
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Result{Outcome: Retry, Error: "cancelled"}
		}
		return Result{Outcome: Retry, Error: "network error"}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return classify(resp)
}

func classify(resp *http.Response) Result {
	r := Result{Status: resp.StatusCode}
	switch s := resp.StatusCode; {
	case s >= 200 && s < 300:
		r.Outcome = Delivered
	case s == http.StatusGone:
		r.Outcome, r.Error = Gone, "token gone"
	case s == http.StatusTooManyRequests || s >= 500:
		r.Outcome, r.Error = Retry, fmt.Sprintf("relay status %d", s)
		r.RetryAfter = retryAfter(resp.Header.Get("Retry-After"))
	default:
		r.Outcome, r.Error = Rejected, fmt.Sprintf("relay status %d", s)
	}
	return r
}

func retryAfter(v string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0
	}
	return min(time.Duration(n)*time.Second, maxRetryAfter)
}

func userAgent(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if v == "" {
		v = "dev"
	}
	return "arenile/" + v
}
