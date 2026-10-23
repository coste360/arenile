package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coste360/arenile/pkg/ext"
)

// Transports (strategy pattern): the delivery job renders an Email and
// hands it to the Transport chosen by the server configuration
// (NewTransport). Transports never log recipients in clear nor bodies
// (they may contain a magic link).

// Transport kinds.
const (
	TransportSMTP = "smtp"
	TransportFile = "file"
	TransportLog  = "log"
)

// ErrPermanent marks a delivery error that retrying cannot fix (SMTP 5xx,
// malformed address): the message goes straight to the dead letter state.
var ErrPermanent = errors.New("notify: permanent delivery failure")

// smtpTimeout bounds one SMTP session.
const smtpTimeout = 60 * time.Second

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

// SMTPTransport sends through an SMTP server with net/smtp:
//
//   - smtps://user:pass@host[:465]  implicit TLS (RFC 8314);
//   - smtp://user:pass@host[:587]   STARTTLS when the server offers it,
//     mandatory when credentials are present (PLAIN auth never travels in
//     clear, except to localhost as net/smtp allows, e.g. Mailpit);
//   - smtp://host:1025              plain local relay (development).
//
// TLS is at least 1.2 with certificate verification. The URL carries the
// credentials: it is never logged nor included in errors.
type SMTPTransport struct {
	URL string
	// Dialer is optional (tests); TLSConfig overrides the TLS settings (tests).
	Dialer    *net.Dialer
	TLSConfig *tls.Config
}

// Kind implements Transport.
func (*SMTPTransport) Kind() string { return TransportSMTP }

type smtpTarget struct {
	host, addr     string
	implicitTLS    bool
	user, password string
}

func parseSMTPURL(raw string) (smtpTarget, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return smtpTarget{}, errors.New("notify: invalid ARENILE_SMTP_URL")
	}
	t := smtpTarget{host: u.Hostname()}
	port := u.Port()
	switch u.Scheme {
	case "smtps":
		t.implicitTLS = true
		if port == "" {
			port = "465"
		}
	case "smtp":
		if port == "" {
			port = "587"
		}
	default:
		return smtpTarget{}, errors.New("notify: ARENILE_SMTP_URL scheme must be smtp or smtps")
	}
	t.addr = net.JoinHostPort(t.host, port)
	if u.User != nil {
		t.user = u.User.Username()
		t.password, _ = u.User.Password()
	}
	return t, nil
}

// Send implements Transport.
func (t *SMTPTransport) Send(ctx context.Context, e Email) (string, error) {
	target, err := parseSMTPURL(t.URL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	from, err := mail.ParseAddress(e.From)
	if err != nil {
		return "", fmt.Errorf("%w: sender: %w", ErrPermanent, err)
	}
	to, err := mail.ParseAddress(e.To)
	if err != nil {
		return "", fmt.Errorf("%w: recipient address", ErrPermanent)
	}
	msg, err := BuildMIME(e)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()
	conn, err := t.dial(ctx, target)
	if err != nil {
		return "", fmt.Errorf("notify: smtp connect %s: %w", target.host, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, target.host)
	if err != nil {
		_ = conn.Close()
		return "", classifySMTP("greeting", err)
	}
	defer func() { _ = c.Close() }()
	if !target.implicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(t.tlsConfig(target.host)); err != nil {
				return "", classifySMTP("starttls", err)
			}
		} else if target.user != "" && !isLocalhost(target.host) {
			return "", fmt.Errorf("%w: smtp server %s does not offer STARTTLS, refusing to send credentials", ErrPermanent, target.host)
		}
	}
	if target.user != "" {
		if err := c.Auth(smtp.PlainAuth("", target.user, target.password, target.host)); err != nil {
			return "", classifySMTP("auth", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return "", classifySMTP("mail from", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return "", classifySMTP("rcpt to", err)
	}
	w, err := c.Data()
	if err != nil {
		return "", classifySMTP("data", err)
	}
	if _, err := w.Write(msg); err != nil {
		return "", classifySMTP("data", err)
	}
	if err := w.Close(); err != nil {
		return "", classifySMTP("data", err)
	}
	_ = c.Quit()
	return e.MessageID, nil
}

func (t *SMTPTransport) dial(ctx context.Context, target smtpTarget) (net.Conn, error) {
	d := t.Dialer
	if d == nil {
		d = &net.Dialer{Timeout: 15 * time.Second}
	}
	if target.implicitTLS {
		td := &tls.Dialer{NetDialer: d, Config: t.tlsConfig(target.host)}
		return td.DialContext(ctx, "tcp", target.addr)
	}
	return d.DialContext(ctx, "tcp", target.addr)
}

func (t *SMTPTransport) tlsConfig(host string) *tls.Config {
	if t.TLSConfig != nil {
		c := t.TLSConfig.Clone()
		if c.ServerName == "" {
			c.ServerName = host
		}
		return c
	}
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

func isLocalhost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// classifySMTP marks 5xx replies as permanent (RFC 5321 4.2.1); other
// errors (4xx, network) are retried.
func classifySMTP(stage string, err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) && tp.Code >= 500 {
		return fmt.Errorf("%w: smtp %s: %d %s", ErrPermanent, stage, tp.Code, tp.Msg)
	}
	return fmt.Errorf("notify: smtp %s: %w", stage, err)
}

// FileTransport writes one RFC 5322 .eml file per message into Dir (local
// development and end-to-end tests read the magic links from there).
// Files are written atomically (temp file + rename) with mode 0600.
type FileTransport struct {
	Dir string
}

// Kind implements Transport.
func (*FileTransport) Kind() string { return TransportFile }

// Send implements Transport.
func (t *FileTransport) Send(_ context.Context, e Email) (string, error) {
	msg, err := BuildMIME(e)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	if err := os.MkdirAll(t.Dir, 0o700); err != nil {
		return "", fmt.Errorf("notify: mail dir: %w", err)
	}
	name := sanitizeFilename(e.DedupeID)
	if name == "" {
		name = sanitizeFilename(e.MessageID)
	}
	date := e.Date
	if date.IsZero() {
		date = time.Now()
	}
	final := filepath.Join(t.Dir, date.UTC().Format("20060102T150405Z")+"-"+name+".eml")
	tmp, err := os.CreateTemp(t.Dir, ".tmp-*.eml")
	if err != nil {
		return "", fmt.Errorf("notify: mail file: %w", err)
	}
	if _, err := tmp.Write(msg); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("notify: mail file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("notify: mail file: %w", err)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("notify: mail file: %w", err)
	}
	return filepath.Base(final), nil
}

func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return -1
	}, s)
}

// LogTransport only logs that a message would be sent: masked recipient
// and message id, never the body (it may contain a magic link).
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

// MemoryTransport keeps the sent emails in memory (unit tests). Fail, when
// set, is called before each send and its error is returned.
type MemoryTransport struct {
	Fail func(Email) error

	mu   sync.Mutex
	sent []Email
}

// Kind implements Transport.
func (*MemoryTransport) Kind() string { return TransportLog }

// Send implements Transport.
func (t *MemoryTransport) Send(_ context.Context, e Email) (string, error) {
	if t.Fail != nil {
		if err := t.Fail(e); err != nil {
			return "", err
		}
	}
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

// notifierTransport adapts an extension ext.Notifier of channel "email"
// (e.g. a transactional email API) to Transport: when registered it
// replaces the server transport.
type notifierTransport struct{ n ext.Notifier }

func (t notifierTransport) Kind() string { return TransportSMTP }

func (t notifierTransport) Send(ctx context.Context, e Email) (string, error) {
	msg := ext.Message{
		Channel: MessageChannel, To: e.To, Subject: e.Subject, BodyText: e.Text, BodyHTML: e.HTML,
		IdempotencyKey: e.DedupeID, TemplateCode: e.Headers[headerTemplate],
	}
	for _, a := range e.Attach {
		msg.Attachments = append(msg.Attachments, ext.Attachment{Filename: a.Filename, ContentType: a.ContentType, Data: a.Data, Inline: a.Inline, ContentID: a.ContentID})
	}
	res, err := t.n.Send(ctx, msg)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	return res.ProviderRef, nil
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
