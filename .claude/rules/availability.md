---
paths:
  - "internal/domain/availability/**"
  - "internal/domain/booking/**"
---
Read `docs/invariants.md` before changing anything here.
- A spot is occupied iff a row exists in `spot_allocations` for (spot, date, slot).
- Holds expire; expired holds must be ignored and cleaned on conflict.
- Any change here needs a concurrency test (`make test-race`) with at least
  100 parallel attempts on the same resource.
- Pools are consumed only with the conditional `UPDATE pool_usage ... WHERE used + held + :qty <= capacity`.
- Booking state transitions follow spec 10.2; each one writes audit log + outbox event in the same transaction.
- Confirmation only from the payment webhook or the manual driver.
