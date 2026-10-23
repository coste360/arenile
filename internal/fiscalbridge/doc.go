// Package fiscalbridge lets a remote Arenile instance (VPS, private server)
// issue fiscal documents on the printer / registratore telematico attached
// to the cash desk PC (spec 29.1 "Ponte verso le periferiche locali", 14.2).
//
// Ownership (M6): the "fiscal bridge" builder owns this package, the
// handlers in internal/http/device (fiscal-bridge operations) and
// internal/http/admin/fiscalbridge.go, the repository functions in
// internal/store/fiscalbridge.go and the models fiscal_bridges,
// fiscal_bridge_jobs (internal/store/models_device.go).
//
// # Design (no inbound ports on the cash desk PC)
//
// The desktop app in remote mode is paired as a device (internal/domain/device)
// and opens an outbound, authenticated long-poll to the instance:
//
//	desktop Agent                         instance (this package)
//	PUT  /api/device/v1/fiscal-bridge  -> Service.Register (one active bridge per establishment)
//	GET  /api/device/v1/fiscal-bridge/jobs?wait=25
//	                                   <- Service.Poll: leased jobs (lease 60 s)
//	ext.FiscalDriver.Issue / Void on the local driver
//	POST /api/device/v1/fiscal-bridge/jobs/{id}/result
//	                                   -> Service.Submit (idempotent)
//
// Long-poll instead of SSE: it works through every proxy and VPN, needs
// no keep-alive tuning against the server WriteTimeout (60 s) and maps 1:1
// to the request/ack model; a job is delivered at least once and the local
// driver deduplicates with the request IdempotencyKey.
//
// # Integration with the fiscal module (strategy/registry)
//
// Driver is an ext.FiscalDriver with code "bridge" registered in the
// app.Registry; an establishment selects it with settings.fiscal.driver =
// "bridge". fiscal.Service.IssuePending (every minute, outside any
// transaction) calls Driver.Issue, which enqueues a job idempotently on
// (establishment, kind, idempotency key) and returns a *DeferredError until
// the device has submitted the outcome; fiscal leaves the document pending
// on a deferred error and stores the outcome on the call that finds it.
// Failed jobs with retryable=true are re-queued with backoff (max 5
// attempts); otherwise the document becomes failed.
//
// # Delivery semantics (exactly-once issuing)
//
//   - Enqueue is idempotent on (establishment, kind, idempotency key); the
//     fiscal job asks again every minute and gets the stored outcome.
//   - Every job transition is a conditional UPDATE (store/fiscalbridge.go):
//     concurrent polls never share a lease, and a result completes a job
//     once (TestConcurrency_FiscalBridgeAck). A duplicate submission of the
//     stored outcome answers 204; a different one 409.
//   - A lease that expires (agent crashed, network down) is offered again
//     with attempt+1; after MaxAttempts deliveries without an answer the
//     job fails (a poison job must not loop). A late result of the same
//     device is still accepted: the receipt was printed.
//   - The agent remembers each outcome by idempotency key before
//     submitting it (ResultStore, a file in the data dir), so a redelivered
//     job is acknowledged without printing again.
//   - Retryable device errors (RetryableError, Temporary()) go back to the
//     queue with Backoff (15 s doubling, max 10 min).
//
// # Files
//
//   - types.go (pure, tested): Document / Job / Result / DeviceStatus, the
//     mapping from ext.FiscalDocumentRequest, Decide, Backoff, Validate.
//   - service.go (shell): registration, lease-based queue, results,
//     status, lease expiry job; DeactivateDevice for device revocation.
//   - notifier.go: wakes waiting long-polls on Enqueue (observer).
//   - driver.go: the "bridge" ext.FiscalDriver on top of the Queue port.
//   - agent.go: the desktop side (internal/desktop/remote): register, poll,
//     execute on the local ext.FiscalDriver, submit with retries.
//   - local.go: registry of local drivers (strategy) with the noop driver,
//     RetryableError.
//   - results.go: ResultStore (memory, file).
//
// The selection stays explicit (strategy/registry): an establishment uses
// the bridge when settings.fiscal.driver = "bridge"; any other driver is
// called locally by the instance as before. Jobs enqueued while no bridge
// is registered wait for one (the panel shows them as pending).
//
// # Permissions
//
// Register / poll / submit: device principal whose user has
// auth.PermCashOperate on the device establishment. Status (admin):
// auth.PermReportsRead or auth.PermCashOperate.
package fiscalbridge
