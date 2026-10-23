// Package notify sends the transactional notifications of Arenile (spec
// 19) and owns the customer-facing artefacts that travel with them: magic
// links of the manage area (spec 12.4), QR codes and .ics calendar files.
// Transactional only: marketing messages are not part of this package.
//
// Ownership (M4): the "notifications" builder owns this package, the
// handlers SendTestEmail and ListMessages in internal/http/admin/email.go,
// the repository in internal/store/notify.go and the tables
// magic_link_tokens, notification_templates and message_log. The service
// methods behind resend-confirmation and magic-link regeneration
// (ResendConfirmation, RegenerateLink) are called from
// internal/http/admin/online.go.
//
// # Architecture
//
// Functional core / imperative shell. Pure, golden-tested code: Markdown
// subset (markdown.go), rendering into subject + text/plain + text/html
// with the establishment branding (render.go, layout.html, branding.go),
// locale formatting (format.go, i18n.go), booking summary and policy lines
// (bookingview.go), iCalendar (ics.go), QR (qr.go), MIME (mime.go). Shell:
// the outbox sink (sink.go), the delivery job (deliver.go, compose.go),
// magic links (magiclink.go) and the admin use cases (service.go).
// Transports are a strategy (transport.go): SMTP, file, log, memory, or an
// extension ext.Notifier of channel "email".
//
// # Pipeline (outbox -> message_log -> transport)
//
//  1. Sink (ext.EventSink "notify", wired in internal/app/m4.go) receives
//     booking.confirmed / modified / cancelled / rejected /
//     approval_requested, payment.refunded / failed, message.failed and
//     customer.anonymized after the outbox and queues message_log rows in
//     one transaction. DedupeKey "<event type>:<event id>:<template>" is
//     unique per establishment: redelivered events queue nothing. The
//     per-establishment toggles of online.EmailConfig decide who gets what;
//     booking.modified sends an email only when what the customer sees
//     changed (fingerprint of dates, slots, spots, lines and total).
//  2. Deliver (job notify.deliver, every 30 s) claims due rows with a
//     conditional UPDATE (one worker per message, also across instances),
//     renders them with the data read at send time in the message locale
//     and sends them. Failures retry with exponential backoff (1 min
//     doubling, capped at 6 h); after MaxAttempts (8), or at once for a
//     permanent error (SMTP 5xx), the row stays "failed" without
//     next_attempt_at (dead letter) and message.failed is written to the
//     outbox in the same transaction, which notifies the manager.
//  3. Templates: built-in Markdown per locale (templates/<locale>/<code>.md
//     with subject and preheader front matter; Italian source, en/de/fr),
//     overridable per establishment (notification_templates). Variables
//     ({{booking.code}}, {{customer.first_name}}, {{manage_url}}, ...) are
//     substituted after parsing and HTML-escaped; partials ({{> summary}},
//     {{> qr}}, {{> manage_button}}, {{> policy}}, {{> ics_note}}) render
//     nothing when their data is missing. Labels shared by every template
//     live in the catalogs i18n/<locale>.json. No tracking pixels, no
//     remote images except the https logo of the theme.
//
// The confirmation and modification emails issue a NEW magic link at send
// time (the raw token exists only in the email; it is revoked if the
// transport fails) and carry the QR inline (PNG, Content-ID qr@arenile)
// and the .ics file.
//
// # Personal data (spec 22, 23)
//
// Recipients are stored as RecipientHash (SHA-256 of establishment id and
// normalised address) plus key ring ciphertext used only for delivery;
// payloads hold ids and amounts only. Logs and stored errors mask
// addresses (MaskEmail, redactError) and never contain bodies or tokens.
// Anonymized customers, the online placeholder customer and customers
// without email receive nothing; customer.anonymized cancels their pending
// messages and erases the ciphertexts.
//
// # Magic links (spec 12.4)
//
// 256-bit random tokens (crypto/rand, base64url, 43 chars), stored as
// SHA-256 hex in magic_link_tokens, valid until the end of the booking (in
// the establishment timezone) + 30 days, revocable; regeneration and the
// "send me a new link" email revoke the previous links of the booking.
// ResolveMagicLink compares hashes only and answers ErrLinkNotFound for
// unknown, expired, revoked and foreign tokens alike. RequestLink (email +
// booking code) always succeeds from the caller's point of view (no
// enumeration), is limited to 3 requests per booking code and per address
// per hour and to one email per booking every 10 minutes; per-IP limits
// are applied by the public HTTP middleware.
//
// # Transports
//
// SMTPTransport (ARENILE_SMTP_URL: smtps:// implicit TLS, smtp:// with
// STARTTLS, mandatory when credentials are present; TLS 1.2+ with
// verification), FileTransport (ARENILE_MAIL_DIR: one .eml per message,
// used by e2e tests and local development), LogTransport (no SMTP: masked
// recipient and message id only), MemoryTransport (tests). compose.yaml
// runs Mailpit for local development.
//
// # QR and .ics
//
// QR encodes "ARENILE:<booking code>" (QRPayload) with error correction M
// via github.com/skip2/go-qrcode (MIT, pure Go, no dependencies), rendered
// as PNG (emails) or compact SVG (public pages); RenderQR is stateless and
// is what checkout.Notifier.QR calls. .ics follows RFC 5545 (VCALENDAR
// with one VEVENT per booking, all-day DATE values with exclusive DTEND,
// or timed events with TZID + generated VTIMEZONE and RRULE for a slot
// over several days; UID "<booking id>@<host>", SEQUENCE = version - 1,
// STATUS:CANCELLED for cancelled bookings, CRLF, 75-octet folding).
package notify
