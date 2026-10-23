// Package printing configures network ESC/POS printers, routes documents to
// them and delivers print jobs (ADR 0021). v1 scope: network printers only
// (Wi-Fi/Ethernet, raw TCP, default port 9100) and only in the desktop
// edition, where the server runs inside the establishment and can reach the
// printer. On the server edition (VPS/Docker) every operation that would
// touch a printer returns ErrUnavailable (409 printing_unavailable) and the
// panel falls back to the browser print dialog.
//
// Ownership (ESC/POS builders): this package, the handlers in
// internal/http/admin/printing.go, the repositories on store.Printer,
// store.PrinterRoute and store.PrintJob (new file internal/store/printing.go),
// the job registration in internal/app. Encoding lives in internal/escpos,
// document layouts in internal/printing/templates.
//
// # Rules
//
//   - Address: Printer.Host must be a private/LAN address (RFC 1918,
//     100.64/10, link-local, loopback, IPv6 ULA/link-local) or a hostname
//     that resolves only to such addresses, checked on create/update and
//     again before every dial (DNS may change): ErrAddressNotAllowed.
//   - Routing: rule of the cash register, else the default rule of the
//     document kind (register_id NULL); an explicit printer_id wins. No rule
//     → ErrRouteMissing. Inactive printer → ErrPrinterInactive.
//   - Enqueue: the service renders the payload (templates + escpos) at
//     enqueue time and inserts a print_jobs row (status queued,
//     next_attempt_at = now). The idempotency key is the Idempotency-Key
//     header or "<kind>:<document id>:<printer id>"; the unique index
//     (establishment_id, idempotency_key) makes a concurrent duplicate return
//     the existing job (TestConcurrency_PrintJobOnce: 100 parallel enqueues
//     → one row, printed once).
//   - Deliver: the job runner claims due jobs with a conditional update
//     (queued → printing, locked_until = now + lease) so one worker sends a
//     job; Driver.Send with dial/write timeouts (3 s dial, 10 s write); on
//     success printed + printed_at + printer status reachable; on error
//     attempts+1, backoff (5 s, 15 s, 1 m, 5 m) and back to queued, then
//     failed after max_attempts; printer status unreachable + last_error.
//     retryPrintJob resets a failed job to queued with the same payload.
//   - Privacy: logs carry job id, printer id, kind and the error class only:
//     never the payload, the host is fine (LAN address, not personal data),
//     never customer names, emails or phones. last_error is technical.
//   - Clock: time comes from the injected clock.Clock; tests use clock.Fake
//     and an in-process fake TCP printer.
//
// # Extension point
//
// Driver abstracts how bytes reach a printer. v1 registers only the
// network driver (store.PrinterDriverNetwork). A future bridge driver (the
// desktop app printing for a VPS instance, or the mobile app over
// Bluetooth/USB) implements Driver by handing the payload to the paired
// device through the device API (like the fiscal bridge), adds its value to
// PrinterDriver in api/openapi.yaml and to the printers.driver CHECK in a new
// migration, and is registered in the driver registry in internal/app. The
// service, routing, templates and jobs do not change.
package printing
