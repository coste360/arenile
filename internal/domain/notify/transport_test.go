package notify

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMaskEmail_Scenarios(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"mario.rossi@example.it", "m***i@e***.it"},
		{"ab@x.com", "a***@x***.com"},
		{"anna@mail.example.co.uk", "a***a@m***.uk"},
		{"nobody", "***"},
		{"@example.it", "***"},
		{"luca@localhost", "l***a@l***"},
	}
	for _, tt := range tests {
		if got := MaskEmail(tt.in); got != tt.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewTransport_Selection(t *testing.T) {
	tests := []struct {
		smtp, dir, want string
	}{
		{"smtp://u:p@mail:587", "/tmp/mail", TransportSMTP},
		{"", "/tmp/mail", TransportFile},
		{"", "", TransportLog},
	}
	for _, tt := range tests {
		if got := NewTransport(tt.smtp, tt.dir, nil).Kind(); got != tt.want {
			t.Errorf("NewTransport(%q, %q).Kind() = %q, want %q", tt.smtp, tt.dir, got, tt.want)
		}
	}
}

func TestMemoryTransport_KeepsSent(t *testing.T) {
	var m MemoryTransport
	if _, err := m.Send(context.Background(), Email{To: "a@b.it", DedupeID: "1"}); err != nil {
		t.Fatal(err)
	}
	if got := m.Sent(); len(got) != 1 || got[0].DedupeID != "1" {
		t.Fatalf("sent = %+v", got)
	}
}

func TestParseSMTPURL_Scenarios(t *testing.T) {
	tests := []struct {
		in       string
		addr     string
		implicit bool
		user     string
		wantErr  bool
	}{
		{"smtp://mail.example:2525", "mail.example:2525", false, "", false},
		{"smtp://u%40x:p%3Aw@mail.example", "mail.example:587", false, "u@x", false},
		{"smtps://u:p@mail.example", "mail.example:465", true, "u", false},
		{"http://mail.example", "", false, "", true},
		{"smtp://", "", false, "", true},
	}
	for _, tt := range tests {
		got, err := parseSMTPURL(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("%q: err = %v", tt.in, err)
		}
		if err != nil {
			if strings.Contains(err.Error(), "p%3Aw") || strings.Contains(err.Error(), ":p@") {
				t.Fatal("error leaks credentials")
			}
			continue
		}
		if got.addr != tt.addr || got.implicitTLS != tt.implicit || got.user != tt.user {
			t.Errorf("%q: got %+v", tt.in, got)
		}
	}
	if got, _ := parseSMTPURL("smtp://u%40x:p%3Aw@mail.example"); got.password != "p:w" {
		t.Fatalf("password %q", got.password)
	}
}

func sampleEmail() Email {
	return Email{
		From: `"Lido Delfino" <noreply@arenile.example>`, ReplyTo: "info@lido.example", To: "mario.rossi@example.it",
		Subject: "Prenotazione LD-1 confermata – Lido", Text: "Ciao Mario,\nriga due", HTML: "<p>Ciao <strong>Mario</strong></p>",
		DedupeID: "0190-msg", MessageID: "0190-msg@arenile.example", Date: time.Date(2027, 6, 1, 8, 0, 0, 0, time.UTC),
		Headers: map[string]string{headerTemplate: "booking_confirmed\r\nBcc: evil@example.com"},
		Attach: []Attachment{
			{Filename: "qr.png", ContentType: "image/png", ContentID: QRContentID, Inline: true, Data: []byte("\x89PNG fake")},
			{Filename: "prenotazione-LD-1.ics", ContentType: ICSContentType, Data: []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")},
		},
	}
}

func TestBuildMIME_Structure(t *testing.T) {
	raw, err := BuildMIME(sampleEmail())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ReplaceAll(raw, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("bare LF in message")
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	if s, _ := dec.DecodeHeader(msg.Header.Get("Subject")); s != "Prenotazione LD-1 confermata – Lido" {
		t.Fatalf("subject %q", s)
	}
	if msg.Header.Get("Bcc") != "" || !strings.HasPrefix(msg.Header.Get("X-Arenile-Template"), "booking_confirmed Bcc:") {
		t.Fatal("header injection not neutralised")
	}
	for k, want := range map[string]string{
		"Message-Id": "<0190-msg@arenile.example>", "Auto-Submitted": "auto-generated", "Mime-Version": "1.0",
		"Reply-To": "<info@lido.example>", "Date": "Tue, 01 Jun 2027 08:00:00 +0000",
	} {
		if got := msg.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	mt, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if mt != "multipart/mixed" {
		t.Fatalf("content type %s", mt)
	}
	mixed := multipart.NewReader(msg.Body, params["boundary"])
	related, _ := mixed.NextPart()
	rt, rp, _ := mime.ParseMediaType(related.Header.Get("Content-Type"))
	if rt != "multipart/related" {
		t.Fatalf("first part %s", rt)
	}
	rr := multipart.NewReader(related, rp["boundary"])
	alt, _ := rr.NextPart()
	at, ap, _ := mime.ParseMediaType(alt.Header.Get("Content-Type"))
	if at != "multipart/alternative" {
		t.Fatalf("alternative %s", at)
	}
	ar := multipart.NewReader(alt, ap["boundary"])
	text, _ := ar.NextPart() // multipart decodes quoted-printable
	body, _ := io.ReadAll(text)
	if string(body) != "Ciao Mario,\r\nriga due" {
		t.Fatalf("text part %q", body)
	}
	htmlPart, _ := ar.NextPart()
	if !strings.HasPrefix(htmlPart.Header.Get("Content-Type"), "text/html") {
		t.Fatal("html part missing")
	}
	inline, _ := rr.NextPart()
	if inline.Header.Get("Content-Id") != "<"+QRContentID+">" || !strings.HasPrefix(inline.Header.Get("Content-Disposition"), "inline") {
		t.Fatalf("inline part %v", inline.Header)
	}
	ics, _ := mixed.NextPart()
	if ics.FileName() != "prenotazione-LD-1.ics" || !strings.HasPrefix(ics.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("ics part %v", ics.Header)
	}
	// Deterministic for the same email.
	again, _ := BuildMIME(sampleEmail())
	if !bytes.Equal(raw, again) {
		t.Fatal("BuildMIME is not deterministic")
	}
}

func TestFileTransport_WritesEML(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mail")
	tr := &FileTransport{Dir: dir}
	id, err := tr.Send(context.Background(), sampleEmail())
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != id || !strings.HasSuffix(id, ".eml") {
		t.Fatalf("files %v, id %q", entries, id)
	}
	info, _ := entries[0].Info()
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	raw, _ := os.ReadFile(filepath.Join(dir, id))
	if _, err := mail.ReadMessage(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestLogTransport_NoPII(t *testing.T) {
	var buf bytes.Buffer
	tr := &LogTransport{Log: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	e := sampleEmail()
	e.Text = "link https://x/b/lido/m/SECRET_TOKEN"
	if _, err := tr.Send(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "mario.rossi") || strings.Contains(buf.String(), "SECRET_TOKEN") {
		t.Fatalf("log leaks data: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "m***i@e***.it") {
		t.Fatalf("masked recipient missing: %s", buf.String())
	}
}

// fakeSMTP is a minimal SMTP server: rcptCode answers RCPT TO.
type fakeSMTP struct {
	ln       net.Listener
	rcptCode string
	mu       sync.Mutex
	data     []string
	rcpts    []string
}

func newFakeSMTP(t *testing.T, rcptCode string) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, rcptCode: rcptCode}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
	w("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w("250-fake")
			w("250 8BITMIME")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			w("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			f.mu.Lock()
			f.rcpts = append(f.rcpts, strings.TrimSpace(line))
			f.mu.Unlock()
			if f.rcptCode == "250" {
				w("250 OK")
			} else {
				w(f.rcptCode + " 5.1.1 <mario.rossi@example.it>: Recipient rejected")
			}
		case cmd == "DATA":
			w("354 go ahead")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				sb.WriteString(l)
			}
			f.mu.Lock()
			f.data = append(f.data, sb.String())
			f.mu.Unlock()
			w("250 queued")
		case cmd == "QUIT":
			w("221 bye")
			return
		default:
			w("250 OK")
		}
	}
}

func TestSMTPTransport_Delivers(t *testing.T) {
	f := newFakeSMTP(t, "250")
	tr := &SMTPTransport{URL: "smtp://" + f.ln.Addr().String()}
	id, err := tr.Send(context.Background(), sampleEmail())
	if err != nil {
		t.Fatal(err)
	}
	if id != "0190-msg@arenile.example" {
		t.Fatalf("provider id %q", id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.data) != 1 || !strings.Contains(f.data[0], "Subject: =?utf-8?q?") || len(f.rcpts) != 1 || !strings.Contains(f.rcpts[0], "<mario.rossi@example.it>") {
		t.Fatalf("server saw %v / %v", f.rcpts, f.data)
	}
}

func TestSMTPTransport_ClassifiesReplies(t *testing.T) {
	tests := []struct {
		code      string
		permanent bool
	}{
		{"550", true},
		{"451", false},
	}
	for _, tt := range tests {
		f := newFakeSMTP(t, tt.code)
		tr := &SMTPTransport{URL: "smtp://" + f.ln.Addr().String()}
		_, err := tr.Send(context.Background(), sampleEmail())
		if err == nil {
			t.Fatalf("%s: expected an error", tt.code)
		}
		if errors.Is(err, ErrPermanent) != tt.permanent {
			t.Fatalf("%s: permanent = %v (%v)", tt.code, errors.Is(err, ErrPermanent), err)
		}
		if red := redactError(err); strings.Contains(red, "mario.rossi@example.it") || !strings.Contains(red, "m***i@e***.it") {
			t.Fatalf("redacted error %q", red)
		}
	}
}

func TestSMTPTransport_UnreachableIsRetryable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	_, err := (&SMTPTransport{URL: "smtp://user:secret@" + addr}).Send(context.Background(), sampleEmail())
	if err == nil || errors.Is(err, ErrPermanent) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestBackoff_Exponential(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{0, time.Minute},
		{1, time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{8, 128 * time.Minute},
		{9, 256 * time.Minute},
		{10, 6 * time.Hour},
		{50, 6 * time.Hour},
	}
	for _, tt := range tests {
		if got := Backoff(tt.attempts); got != tt.want {
			t.Errorf("Backoff(%d) = %v, want %v", tt.attempts, got, tt.want)
		}
	}
}
