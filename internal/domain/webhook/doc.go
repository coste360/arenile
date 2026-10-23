// Package webhook manages the outgoing webhook endpoints of an
// establishment from the panel (spec 18.2): CRUD of endpoints with event
// filters, signing secret shown once and rotation with a grace period,
// pause/resume, delivery log, redelivery and a synchronous test event.
//
// Ownership (M5A): the "integrations backend" builder owns this package,
// the handlers in internal/http/admin/webhookendpoints.go, the repository
// functions in internal/store/webhook.go (new file) and the minimal
// extensions of internal/domain/event listed below.
//
// # Division of labour with internal/domain/event (extend, don't duplicate)
//
// event keeps the outbox, the Dispatcher (fan-out of outbox events to
// matching active endpoints as webhook_deliveries rows), the Deliverer
// (signed HTTP attempts, exponential Backoff up to 24 hours, automatic
// disable after DeliveryOptions.DisableAfter consecutive failures with
// audit "webhook.disabled"), Sign/VerifySignature and NewWebhookSecret.
// This package never sends HTTP itself. Required extensions of event:
//
//   - Deliverer.send signs with every active secret: the current one and,
//     while previous_secret_expires_at is in the future, the previous one
//     ("t=<ts>,v1=<new>,v1=<old>"; VerifySignature already accepts several
//     v1 values). Add SignAll(secrets []string, body, at) next to Sign.
//   - Deliverer.AttemptNow(ctx, deliveryID) performs one attempt of a given
//     delivery synchronously, without retry scheduling when the delivery is
//     a test (used by SendTest); attempt() records last_delivery_at on the
//     endpoint.
//   - Deliverer.CreateEndpoint (M0) moves here as Service.Create (or is
//     called by it); secrets stay key ring ciphertexts with AAD = endpoint
//     id, the previous secret with AAD "<endpoint id>:previous".
//   - Auto-disable notifies the owner (spec 18.2 "avviso al proprietario"):
//     write an audit entry (exists) and, when notify supports it, an email
//     to the owners; at minimum the panel shows status "disabled" with
//     disabled_reason.
//
// # Rules
//
//   - Permission auth.PermUsersManage (owner); a principal of Kind
//     PrincipalAPIKey is always refused with auth.ErrForbidden.
//   - URL: https only (http allowed only for localhost, 127.0.0.1 and ::1,
//     for development), no userinfo, at most 2048 bytes. Event filters:
//     known types (EventTypes), "<group>.*" with a known group, or "*".
//     Matching is store.WebhookEndpoint.Matches.
//   - Status: active; paused (active = false, disabled_at nil); disabled
//     (active = false, disabled_at set by the Deliverer). Update with
//     active = true resets consecutive_failures, disabled_at and
//     disabled_reason.
//   - Test event: an outbox row of type TestEventType written with
//     dispatched_at already set (so the Dispatcher ignores it and no other
//     endpoint or sink receives it), one delivery row for this endpoint,
//     one synchronous attempt, failed_at set on failure (no retries).
//     Payload: {"message": "Evento di prova da Arenile", "endpoint_id": ...}.
//   - Redeliver: only failed or delivered deliveries of an active endpoint;
//     clears delivered_at, failed_at, last_error, sets next_attempt_at =
//     now and created_at stays (the 24h window restarts from now: store a
//     reset of the age reference, e.g. by updating created_at, documented
//     in the store function); attempts keep counting.
//   - Delivery log: newest first (index webhook_deliveries_endpoint_created_idx),
//     cursor pagination, optional status filter (pending: neither delivered
//     nor failed; delivered; failed). The detail rebuilds the envelope from
//     the outbox row (event.FromRow) — the same bytes every attempt sends.
//   - Secrets are returned only by Create and RotateSecret; never logged.
//     No Idempotency-Key on these operations.
//   - Every change writes the audit log in the same transaction:
//     webhook.created, webhook.updated, webhook.deleted,
//     webhook.secret_rotated, webhook.redelivered, webhook.tested (diffs
//     carry url, events, active, never secrets).
package webhook
