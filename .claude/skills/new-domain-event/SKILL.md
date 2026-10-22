---
name: new-domain-event
description: Add a new domain event type with a versioned payload, written to the outbox in the same transaction as the change, and documented for outgoing webhooks. Use when a state change must be observable by sinks, webhooks or notifications.
argument-hint: "[event.type]"
---
# New domain event: $ARGUMENTS

Context to read first: `docs/spec/18-eventi-webhook-e-punti-di-estensione.md`, `docs/architecture.md`
(event/outbox flow), outbox INV entries in `docs/invariants.md`.

1. Name the event `<entity>.<past_tense_verb>` (e.g. `booking.confirmed`), consistent with spec 18.1.
2. Add the type constant and a payload struct in `internal/domain/event` with `version: 1`.
   Payload contains IDs and the data a consumer needs; no secrets, no tokens; personal data only
   where strictly needed (customer IDs rather than emails).
3. Emit the event from the domain service method that performs the change, writing to `outbox_events`
   through the same transaction (`bun.Tx`) as the state change and the audit log entry.
   Never publish directly to sinks or HTTP from the service.
4. If an existing event's payload changes incompatibly, add a new `version` instead of mutating it.
5. Make sure the dispatcher (job runner) delivers it to `ext.EventSink`s, outgoing webhooks
   (signed `Arenile-Signature`) and the SSE broker if relevant (`{type, spot_ids, dates}` message).
6. Tests:
   - unit: payload serialization golden test (stable JSON field names);
   - integration (`//go:build integration`): the change and the outbox row are both committed, and both
     are absent if the transaction fails after the event write.
7. Document the event and its payload in the webhook documentation in `docs/` and add a `CHANGELOG.md` entry.
8. Run `make check`.

## Verification criteria
- Rolling back the transaction leaves no outbox row (test proves it).
- Event appears in the docs list with an example payload.
