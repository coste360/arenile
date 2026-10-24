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
//     100.64/10, link-local, IPv6 ULA/link-local) or a hostname that
//     resolves only to such addresses, checked on create/update and again
//     before every dial (DNS may change): ErrAddressNotAllowed. Loopback is
//     admitted only by AddressPolicy.AllowLoopback (development and tests,
//     ARENILE_PRINTERS_ALLOW_LOOPBACK); cloud metadata addresses never.
//     Printers can be added or changed only where they can print (desktop
//     edition): ErrUnavailable otherwise.
//   - Routing: rule of the cash register, else the default rule of the
//     document kind (register_id NULL); an explicit printer_id wins. No rule
//     → ErrRouteMissing. Inactive printer → ErrPrinterInactive. ResolveRoute
//     re-probes a printer whose status is stale (unreachable > 10 s,
//     reachable > 1 min, probe capped at 1.5 s) so print buttons fall back
//     to the browser only when the printer is really off.
//   - Enqueue: the service renders the payload (templates + escpos) at
//     enqueue time and inserts a print_jobs row (status queued,
//     next_attempt_at = now). The idempotency key is the Idempotency-Key
//     header, bound to the request (kind, document, printer): the same key
//     returns the existing job, the same key for another request is
//     ErrIdempotencyKeyReused, no key is a new job (a deliberate reprint).
//     The unique index (establishment_id, idempotency_key) makes a
//     concurrent duplicate return the existing job
//     (TestConcurrency_PrintJobOnce: 100 parallel enqueues → one row, sent
//     once).
//   - Deliver: the job runner claims due jobs with a conditional update
//     (queued → printing, locked_until = now + lease) so one worker sends a
//     job at a time; jobs are grouped by printer and printers served in
//     parallel, a printer that does not answer skips its remaining jobs for
//     the run. Driver.Send with dial/write timeouts (3 s dial, 10 s write);
//     on success printed + printed_at + printer status reachable; on error
//     attempts+1, backoff (5 s, 15 s, 1 m, 5 m) and back to queued, then
//     failed after max_attempts; printer status unreachable + last_error.
//     Delivery is at-least-once: a job whose lease expired mid-send is
//     reclaimed (attempt counted) and resent without the drawer kick.
//     RetryJob resets a failed job to queued with the same payload. The
//     payload of finished jobs is nulled after PayloadRetention
//     (printing.retention job).
//   - Privacy: logs carry job id, printer id, kind and the error class only:
//     never the payload, the host is fine (LAN address, not personal data),
//     never customer names, emails or phones. last_error is technical.
//   - Clock: time comes from the injected clock.Clock; tests use clock.Fake
//     and an in-process fake TCP printer.
//
// # Extension points
//
// Documents: payment receipts and cash Z summaries are built here from the
// store; daily-sale tickets come from a TicketSource (Deps.Tickets) that
// the daily-sale feature implements with templates.TicketDoc. The
// establishment logo is fetched through a LogoLoader (HTTPLogoLoader:
// https URLs or instance paths), dithered once per printer and paper width
// and cached in printers.logo_raster.
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
