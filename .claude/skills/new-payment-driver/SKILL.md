---
name: new-payment-driver
description: Implement a new payment provider driver (ext.PaymentProvider) with signed webhook tests, registry registration and configuration docs. Use when adding a payment gateway.
argument-hint: "[provider code]"
---
# New payment driver: $ARGUMENTS

Context to read first: `docs/spec/13-pagamenti.md`, `docs/spec/18-eventi-webhook-e-punti-di-estensione.md`,
`docs/spec/22-sicurezza.md`, payment INV entries in `docs/invariants.md`, the existing `manual` and
`stripe` drivers.

1. Create the driver package (core drivers under `internal/domain/payment/<code>`; third-party
   drivers in their own module importing `pkg/ext`). Do not change `pkg/ext` interfaces.
2. Implement `ext.PaymentProvider`: `Code()`, `Capabilities()`, `CreatePayment`, `ParseWebhook`, `Refund`.
   - Amounts are `int64` cents; currency from the request.
   - Pass `IdempotencyKey` (`booking_id + attempt`) to the provider.
   - Put `establishment_id`, `booking_id`, `booking_code` in provider metadata.
   - `ParseWebhook` must verify the signature and reject (error) unsigned or stale payloads; map events to
     `ext.PaymentEvent` with the provider event ID for deduplication.
3. Credentials are per establishment and stored encrypted (AES-GCM with `ARENILE_SECRET_KEY`); never log them.
4. Register the driver in the extension registry in `internal/app` (core drivers) or document how a
   custom `main` registers it via `arenile.WithExtensions`.
5. Tests (no network):
   - `CreatePayment` against an `httptest` fake of the provider API;
   - signed webhook fixtures (valid, bad signature, replayed/duplicate event, unknown type);
   - refund full/partial according to `Capabilities()`;
   - integration: a webhook confirms the booking exactly once, in one transaction (payment, booking status,
     allocations hold -> booking), and a duplicate delivery is a no-op.
6. Document configuration (fields, webhook URL `POST /webhooks/payments/{provider}/{establishment_id}`,
   events to enable) in `docs/` and add a `CHANGELOG.md` entry.
7. Run `make check-full`.

## Verification criteria
- Booking confirmation happens only via `ParseWebhook` (never via return URL).
- Duplicate webhook events do not double-count `paid_amount`.
- No secret appears in logs or error messages (grep the test output).
