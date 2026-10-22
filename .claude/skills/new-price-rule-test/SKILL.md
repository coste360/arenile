---
name: new-price-rule-test
description: Turn a pricing scenario described in words (season, pack, dates, expected result) into table-driven and property test cases for the pure pricing engine. Use when asked to cover or reproduce a pricing behaviour.
argument-hint: "[scenario in words]"
---
# New pricing test: $ARGUMENTS

Context to read first: `docs/spec/08-motore-dei-prezzi.md`, `.claude/rules/pricing.md`,
`docs/adr/0004-pricing-pure-function.md`, INV entries on pricing in `docs/invariants.md`.

1. Restate the scenario as explicit inputs: establishment settings (half_day_factor, rounding),
   rate periods with dates, rules (stage, priority, stackable, conditions, action), fixed-duration prices,
   channel, dates, slots, formula, lines (pack, zone/price category, spot), extras, agreement, promo code.
2. Compute the expected `PriceBreakdown` by hand, step by step following the pipeline in spec 8.1
   (fixed duration, base per day, modifiers, extras, discounts, rounding, deposit), in integer cents
   with half-even rounding. Write the arithmetic in a comment on the test case.
3. Open the existing table-driven suite in `internal/domain/pricing/*_test.go`, reuse its fixture
   builders, and add the case(s) with a descriptive name (`TestQuote_<Scenario>`).
   Include the nearest edge cases: day before/after the period boundary, same priority tie-break,
   AM/PM slot, stackable vs non-stackable.
4. If the scenario exposes a new condition or action, extend the property tests (`rapid`):
   total >= 0, adding a discount never increases the total, re-quoting the snapshot inputs with the
   same `engine_version` yields the same total.
5. Do not change production code unless the task says so. If the expected result differs from the
   engine output, report it as a finding with the hand computation.
6. Run `make test` (and `make check` before finishing).

## Verification criteria
- New cases pass (or a documented failing case is reported as a bug with the exact diff).
- Every expected value is in cents and justified by the comment.
- No test uses `time.Now()`, the database or randomness outside `rapid`.
