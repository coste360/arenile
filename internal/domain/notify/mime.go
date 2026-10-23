package notify

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

// RFC 5322 / MIME composition of an Email (shared by the SMTP and file
// transports):
//
//	multipart/mixed
//	├── multipart/related
//	│   ├── multipart/alternative
//	│   │   ├── text/plain; charset=utf-8 (quoted-printable)
//	│   │   └── text/html; charset=utf-8 (quoted-printable)
//	│   └── inline parts with Content-ID (QR image)
//	└── attachments (.ics)
//
// Boundaries derive from the message id, so the output is deterministic
// for a given email (golden-testable). Headers are encoded with RFC 2047
// when not ASCII and never contain CR/LF from user data.

// BuildMIME renders e as an RFC 5322 message with CRLF line endings.
func BuildMIME(e Email) ([]byte, error) {
	from, err := mail.ParseAddress(e.From)
	if err != nil {
		return nil, fmt.Errorf("notify: from address: %w", err)
	}
	to, err := mail.ParseAddress(e.To)
	if err != nil {
		return nil, fmt.Errorf("notify: recipient address: %w", err)
	}
	var buf bytes.Buffer
	hdr := func(k, v string) { buf.WriteString(k + ": " + v + "\r\n") }
	hdr("From", from.String())
	hdr("To", to.String())
	if e.ReplyTo != "" {
		if rt, err := mail.ParseAddress(e.ReplyTo); err == nil {
			hdr("Reply-To", rt.String())
		}
	}
	hdr("Subject", mime.QEncoding.Encode("utf-8", headerSafe(e.Subject)))
	date := e.Date
	if date.IsZero() {
		date = time.Unix(0, 0)
	}
	hdr("Date", date.UTC().Format(time.RFC1123Z))
	if e.MessageID != "" {
		hdr("Message-ID", "<"+e.MessageID+">")
	}
	hdr("MIME-Version", "1.0")
	// Transactional, machine-generated (RFC 3834): no auto-replies.
	hdr("Auto-Submitted", "auto-generated")
	keys := make([]string, 0, len(e.Headers))
	for k := range e.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		hdr(textproto.CanonicalMIMEHeaderKey(headerSafe(k)), mime.QEncoding.Encode("utf-8", headerSafe(e.Headers[k])))
	}

	seed := e.MessageID + e.DedupeID
	mixed := multipart.NewWriter(&buf)
	_ = mixed.SetBoundary(boundary(seed, "mixed"))
	hdr("Content-Type", `multipart/mixed; boundary="`+mixed.Boundary()+`"`)
	buf.WriteString("\r\n")

	// related (alternative + inline images)
	var related bytes.Buffer
	rw := multipart.NewWriter(&related)
	_ = rw.SetBoundary(boundary(seed, "related"))
	var alt bytes.Buffer
	aw := multipart.NewWriter(&alt)
	_ = aw.SetBoundary(boundary(seed, "alt"))
	if err := writeQP(aw, "text/plain; charset=utf-8", e.Text); err != nil {
		return nil, err
	}
	if err := writeQP(aw, "text/html; charset=utf-8", e.HTML); err != nil {
		return nil, err
	}
	if err := aw.Close(); err != nil {
		return nil, err
	}
	pw, err := rw.CreatePart(textproto.MIMEHeader{"Content-Type": {`multipart/alternative; boundary="` + aw.Boundary() + `"`}})
	if err != nil {
		return nil, err
	}
	if _, err := pw.Write(alt.Bytes()); err != nil {
		return nil, err
	}
	for _, a := range e.Attach {
		if !a.Inline {
			continue
		}
		if err := writeAttachment(rw, a); err != nil {
			return nil, err
		}
	}
	if err := rw.Close(); err != nil {
		return nil, err
	}
	part, err := mixed.CreatePart(textproto.MIMEHeader{"Content-Type": {`multipart/related; boundary="` + rw.Boundary() + `"`}})
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(related.Bytes()); err != nil {
		return nil, err
	}
	for _, a := range e.Attach {
		if a.Inline {
			continue
		}
		if err := writeAttachment(mixed, a); err != nil {
			return nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func boundary(seed, kind string) string {
	sum := sha256.Sum256([]byte(seed + ":" + kind))
	return "arenile-" + kind + "-" + hex.EncodeToString(sum[:12])
}

func writeQP(w *multipart.Writer, contentType, body string) error {
	p, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	qp := quotedprintable.NewWriter(p)
	if _, err := qp.Write([]byte(crlf(body))); err != nil {
		return err
	}
	return qp.Close()
}

// crlf normalises line endings to CRLF (RFC 5322 2.3).
func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

func writeAttachment(w *multipart.Writer, a Attachment) error {
	disp := "attachment"
	if a.Inline {
		disp = "inline"
	}
	h := textproto.MIMEHeader{
		"Content-Type":              {a.ContentType},
		"Content-Transfer-Encoding": {"base64"},
		"Content-Disposition":       {mime.FormatMediaType(disp, map[string]string{"filename": a.Filename})},
	}
	if a.ContentID != "" {
		h.Set("Content-ID", "<"+a.ContentID+">")
	}
	p, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	enc := base64.StdEncoding.EncodeToString(a.Data)
	for len(enc) > 76 {
		if _, err := p.Write([]byte(enc[:76] + "\r\n")); err != nil {
			return err
		}
		enc = enc[76:]
	}
	_, err = p.Write([]byte(enc + "\r\n"))
	return err
}
