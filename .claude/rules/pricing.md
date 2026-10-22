---
paths:
  - "internal/domain/pricing/**"
---
- The engine is pure: inputs in, `PriceBreakdown` out. No DB, no `time.Now()`.
- Integer arithmetic only, rounding half-even to the cent.
- Every new condition or action needs table-driven cases plus property tests
  (`rapid`): total >= 0, discounts never increase the price, snapshot reproducible.
- Bump `engine_version` when the output for an existing input can change.
- Use `internal/money` helpers (`DivRoundHalfEven`, `MulRat`, `PercentOfBasisPoints`, `VATSplit`).
- No base rule for a day means the pack is not sellable: return `ErrNoPrice`.
