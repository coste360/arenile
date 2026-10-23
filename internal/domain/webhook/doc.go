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
// This package never sends HTTP itself. M5A extensions of event (done):
//
//   - SignAll(secrets, body, at): the Deliverer signs with the current
//     secret and, while previous_secret_expires_at is in the future, the
//     previous one ("t=<ts>,v1=<new>,v1=<old>"; VerifySignature accepts any).
//   - Deliverer.AttemptNow(ctx, deliveryID): one synchronous attempt of a
//     pending delivery, claimed with the same lease as DeliverDue; every
//     attempt records last_delivery_at on the endpoint. A TestEventType
//     delivery is never retried and never counts towards the auto-disable.
//   - EgressPolicy / NewEgressClient: SSRF protection of every delivery
//     (resolve once, check every address, connect to the checked IP, no
//     redirects, no environment proxy); private targets only with
//     ARENILE_WEBHOOKS_ALLOW_PRIVATE. ValidateURL applies the same policy to
//     IP literals at configuration time.
//   - DeliveryOptions.MaxPayload (256 KiB): larger envelopes fail unsent.
//   - Secrets stay key ring ciphertexts with AAD = endpoint id, the previous
//     secret with AAD event.PreviousSecretAAD(id) = "<endpoint id>:previous".
//   - Auto-disable writes the audit entry "webhook.disabled" and the panel
//     shows status "disabled" with disabled_reason; an email to the owners
//     is not sent yet (gap, notify has no such template).
//
// Retention: Service.Jobs returns "webhooks.retention" (hourly), which
// deletes finished deliveries older than Deps.Retention (30 days), fails
// test deliveries never attempted after an hour and drops unreferenced
// webhook.test outbox rows.
//
// # Rules
//
//   - Permission auth.PermUsersManage (owner); a principal of Kind
//     PrincipalAPIKey is always refused with auth.ErrForbidden.
//   - URL: https only (http allowed only for localhost, 127.0.0.1 and ::1,
//     for development), no userinfo, no fragment, at most 2048 bytes, IP
//     literal allowed by the egress policy. Event filters (at least one):
//     known types (EventTypes), "<group>.*" with a known group, or "*".
//     Matching is store.WebhookEndpoint.Matches.
//   - Status: active; paused (active = false, disabled_at nil); disabled
//     (active = false, disabled_at set by the Deliverer). Update with
//     active = true resets consecutive_failures, disabled_at and
//     disabled_reason.
//   - Test event: an outbox row of type TestEventType written with
//     dispatched_at already set (so the Dispatcher ignores it and no other
//     endpoint or sink receives it), one delivery row for this endpoint,
//     one synchronous attempt, failed_at set on failure (no retries). It
//     works on paused endpoints (test before activating).
//     Payload: {"message": "Evento di prova da Arenile", "endpoint_id": ...}.
//   - Redeliver: only failed or delivered deliveries of an active endpoint;
//     clears delivered_at, failed_at, last_error, sets next_attempt_at =
//     now and created_at = now (created_at is the age reference of the 24h
//     retry window, see store.RequeueWebhookDelivery); attempts keep
//     counting. The log orders by id (UUIDv7), so a redelivery does not
//     move in the list.
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
