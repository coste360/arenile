---
paths:
  - "api/**"
  - "internal/http/**"
---
- Edit `api/openapi.yaml` first, then `make gen`, then the handler.
- Errors are `application/problem+json` with a stable `type` slug.
- Collections use cursor pagination; creation endpoints accept `Idempotency-Key`.
- Public endpoints never return customer data except via the booking session
  token or the manage token.
- Paths are versioned: `/api/admin/v1/...`, `/api/public/v1/{slug}/...`.
- Configuration and map resources use `ETag` / `If-Match` optimistic concurrency.
- Calendar dates are `YYYY-MM-DD` (establishment timezone); instants RFC 3339 UTC; money in integer cents.
- Handlers are thin: decode, call the domain service, encode. Permission checks live in the domain service.
- Generated server code is `internal/http/api/api.gen.go`; never edit it.
