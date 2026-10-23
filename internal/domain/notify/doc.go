// Package notify sends the transactional notifications of Arenile (spec
// 19) and owns the customer-facing artefacts that travel with them: magic
// links of the manage area (spec 12.4), QR codes and .ics calendar files.
//
// Ownership (M4): the "notifications" builder owns this package, the
// handlers in internal/http/admin/email.go (test email, message log),
// resend-confirmation and magic-link in internal/http/admin/online.go, the
// repository functions in internal/store/notify.go and the models
// magic_link_tokens, notification_templates, message_log.
//
// # Pipeline (outbox -> message_log -> transport)
//
//  1. Sink (ext.EventSink "notify", registered in internal/app/m4.go next
//     to the extension sinks) receives booking.confirmed,
//     booking.approval_requested, booking.cancelled, booking.rejected,
//     payment.refunded, payment.failed, message.failed ... after the
//     outbox. It enqueues message_log rows with DedupeKey
//     "<event type>:<event id>:<template>" (unique per establishment), so
//     at-least-once delivery of events never sends twice.
//  2. Deliver (job every 30 s) picks due rows (queued, or failed with
//     next_attempt_at <= now; exponential backoff, max 8 attempts, then
//     message.failed), renders the template in the booking locale with the
//     booking data read at send time, and sends through Transport.
//  3. Templates: built-in Markdown templates per locale (embedded files
//     templates/<locale>/<template>.md with front matter subject; Italian
//     source, en/de/fr), overridable per establishment
//     (notification_templates); variables {{booking.code}},
//     {{customer.first_name}}, {{manage_url}}, ... rendered with
//     text/template-like escaping into HTML + plain text parts.
//
// The confirmation email issues a NEW magic link at send time (the raw
// token exists only in the email body) and attaches the QR (inline PNG,
// Content-ID) and the .ics file.
//
// # Magic links (spec 12.4)
//
// 256-bit random tokens (crypto/rand, base64url, 43 chars), stored as
// SHA-256 hex in magic_link_tokens, valid until the end of the booking + 30
// days, revocable; regeneration revokes the previous links of the booking.
// ResolveMagicLink compares hashes only and answers ErrLinkNotFound for
// unknown, expired and revoked tokens alike. RequestLink (email + booking
// code) always succeeds from the caller's point of view (no enumeration)
// and is rate limited per IP and per booking.
//
// # Transports
//
// SMTPTransport (ARENILE_SMTP_URL, net/smtp with STARTTLS / implicit TLS
// for smtps://), FileTransport (ARENILE_MAIL_DIR: one .eml per message,
// used by e2e tests and local development), LogTransport (no SMTP: logs
// template and masked recipient only), MemoryTransport (unit tests).
// Recipients are stored as hash + ciphertext (key ring); logs mask them
// (MaskEmail).
//
// # QR and .ics
//
// QR encodes "ARENILE:<booking code>" (QRPayload). Rendering SVG and PNG
// needs a QR encoder: the builder either writes a small encoder (Reed-
// Solomon, byte mode, ECC M) or adds a dependency with an MIT/BSD/Apache
// license and documents the reason. .ics follows RFC 5545 (VCALENDAR,
// one VEVENT per booking period, all-day DATE values in the establishment
// timezone, UID "<booking id>@<host>", CRLF line endings, 75-octet folding).
package notify
