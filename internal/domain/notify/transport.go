package notify

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/coste360/arenile/internal/domain/configerr"
)

// Transport kinds.
const (
	TransportSMTP = "smtp"
	TransportFile = "file"
	TransportLog  = "log"
)

// NewTransport picks the transport from the server configuration:
// ARENILE_SMTP_URL (smtp:// or smtps://) wins, then ARENILE_MAIL_DIR, then
// the log transport.
func NewTransport(smtpURL, mailDir string, log *slog.Logger) Transport {
	switch {
	case smtpURL != "":
		return &SMTPTransport{URL: smtpURL}
	case mailDir != "":
		return &FileTransport{Dir: mailDir}
	default:
		return &LogTransport{Log: log}
	}
}

// SMTPTransport sends through an SMTP server (net/smtp, STARTTLS or
// implicit TLS for smtps://). The URL carries the credentials: never log it.
type SMTPTransport struct {
	URL string
}

// Kind implements Transport.
func (*SMTPTransport) Kind() string { return TransportSMTP }

// Send implements Transport.
func (t *SMTPTransport) Send(ctx context.Context, e Email) (string, error) {
	return "", configerr.ErrNotImplemented
}

// FileTransport writes one RFC 5322 .eml file per message into Dir (local
// development and end-to-end tests read the magic links from there).
type FileTransport struct {
	Dir string
}

// Kind implements Transport.
func (*FileTransport) Kind() string { return TransportFile }

// Send implements Transport.
func (t *FileTransport) Send(ctx context.Context, e Email) (string, error) {
	return "", configerr.ErrNotImplemented
}

// LogTransport only logs that a message would be sent: template subject
// and masked recipient, never the body (it may contain a magic link).
type LogTransport struct {
	Log *slog.Logger
}

// Kind implements Transport.
func (*LogTransport) Kind() string { return TransportLog }

// Send implements Transport.
func (t *LogTransport) Send(ctx context.Context, e Email) (string, error) {
	if t.Log != nil {
		t.Log.InfoContext(ctx, "email not sent (no ARENILE_SMTP_URL)",
			slog.String("to", MaskEmail(e.To)), slog.String("message_id", e.DedupeID))
	}
	return "", nil
}

// MemoryTransport keeps the sent emails in memory (unit tests).
type MemoryTransport struct {
	mu   sync.Mutex
	sent []Email
}

// Kind implements Transport.
func (*MemoryTransport) Kind() string { return TransportLog }

// Send implements Transport.
func (t *MemoryTransport) Send(_ context.Context, e Email) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sent = append(t.sent, e)
	return e.DedupeID, nil
}

// Sent returns a copy of the emails sent so far.
func (t *MemoryTransport) Sent() []Email {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Email(nil), t.sent...)
}

// MaskEmail masks an address for logs and the admin UI:
// "mario.rossi@example.it" -> "m***i@e***.it". Never log addresses in clear.
func MaskEmail(addr string) string {
	local, domain, ok := strings.Cut(strings.TrimSpace(addr), "@")
	if !ok || local == "" || domain == "" {
		return "***"
	}
	mask := func(s string) string {
		r := []rune(s)
		if len(r) <= 2 {
			return string(r[:1]) + "***"
		}
		return string(r[0]) + "***" + string(r[len(r)-1])
	}
	if i := strings.LastIndex(domain, "."); i > 0 {
		return mask(local) + "@" + string([]rune(domain)[:1]) + "***" + domain[i:]
	}
	return mask(local) + "@" + string([]rune(domain)[:1]) + "***"
}
