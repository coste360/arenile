// Package pricing is the pure price engine of Arenile (spec section 8).
//
// Quote(input, catalog, rules) returns a PriceBreakdown that serialises to the
// price snapshot of spec 8.6. The function does no I/O, never reads the clock
// (the current date is QuoteInput.Today) and uses integer arithmetic only:
// money is money.Cents, factors and percentages are exact Decimals parsed
// from the JSON text (1.2 becomes 12/10, 12.5% becomes 1250 basis points) and
// every product is rounded half-even to the cent. Prices are VAT-inclusive.
//
// # Conditions (8.2)
//
// A rule applies when every present condition holds; an empty list counts as
// absent. List conditions match when the value (or, for spot attributes and
// customer tags, any of the values) is in the list. Each condition is
// evaluated against one of three levels:
//
//   - booking: formulas, channels, min_days/max_days (number of days of the
//     range), lead_time_days (days from Today to the start date), customer_tags,
//     agreement_ids (the agreement in the input, if active), slots;
//   - line: price_categories (spot override, else zone category), zone_ids,
//     spot_attributes (false when the line has no spot); line conditions never
//     match booking-level extras;
//   - day: rate_periods (false on a day outside every period), date_ranges,
//     weekdays (ISO 1-7), holidays (true = only national/local holidays,
//     false = only non-holidays), occupancy_percent (zone occupancy of the day
//     from QuoteInput.Occupancy; false when no value is provided).
//
// slots matches when every slot of the priced context is listed: a FULL day
// matches only ["AM","PM"], an AM half day matches ["AM"] and ["AM","PM"].
//
// # Pipeline (8.1) and conflict resolution (8.4)
//
// For each line:
//
//  1. Fixed duration: for week/fortnight/month/season, the fixed price of the
//     pack, formula and price category whose rate period is the period of the
//     start date wins; otherwise the one without period. For AM/PM bookings
//     it is multiplied by half_day_factor. The fixed amount replaces steps 2-3
//     for the pack (Days is empty, Fixed is set). Without a fixed price the
//     line falls through to per-day pricing.
//  2. Base per day: base rules may only "set". One rule wins per day: highest
//     priority, then most specific (number of conditions, +1 when target_ids
//     is not empty), then most recent (highest Seq), then lowest ID. For AM/PM
//     bookings, base rules with a slots condition matching the slot are
//     considered first; if none matches, the full-day winner is multiplied by
//     half_day_factor (recorded as the pseudo-rule "half_day_factor"). No
//     winning rule means ErrNoPrice (*NoPriceError).
//  3. Modifiers per day: all matching modifiers are partitioned into
//     stackable and non-stackable ones. All stackable ones apply. Among the
//     non-stackable ones exactly one applies: the one whose inclusion yields
//     the lowest day price (ties: higher priority, more specific, more recent,
//     lower ID). The selected rules are then applied in priority order
//     (ascending: a higher priority is evaluated later, then older Seq first)
//     on the running price, which is clamped at zero after every step. This
//     is how "non-stackable rules replace the previous ones of the same group"
//     (8.1) and "only the most favourable non-stackable applies" (8.4) are
//     reconciled: the non-stackable group is all non-stackable modifiers.
//  4. Extras: the same steps 2-3 with target_type extra. per_day: unit price
//     of each day (with the half-day logic) x quantity. per_slot: the price
//     of each booked slot is evaluated separately with a single-slot context
//     (so slot conditions can price AM and PM differently, no factor) and
//     summed, x quantity. per_booking: one price evaluated on the start date
//     with the booking slots (no factor) x quantity. Extras in
//     QuoteInput.Extras are booking-level and appear in PriceBreakdown.Extras.
//
// Booking level:
//
//  5. Discounts, applied in this order: agreement, promo code, automatic
//     discount rules (priority ascending, then Seq). The same stackable /
//     non-stackable selection of step 3 is used with "lowest booking total"
//     as the criterion (ties: agreement, then promo, then rules by priority).
//     Each discount applies to its eligible amounts: the remaining (already
//     discounted) per-day pack and extra amounts that pass its target and its
//     conditions (day conditions are checked on the unit's date; fixed-price
//     packs and per_booking extras use the start date). Promo codes and
//     agreements target packs and extras; rules target their target_type.
//     percent_off and multiply (factor <= 1) take a share of the eligible
//     amount; amount_off takes amount x 1 (per booking), x number of eligible
//     lines (per line; booking-level extras count as one group) or x number
//     of eligible days (per day), capped at the eligible amount; free_days
//     makes, for every line, the cheapest F of every N eligible days free
//     (fixed-price packs and per_booking extras are excluded). Each discount
//     is spread over its eligible amounts by largest remainder, so no amount
//     ever goes below zero and the total is never negative. A promo code that
//     is inactive, outside [valid_from, valid_to] (compared with Today) or
//     with no eligible amount returns ErrPromoNotApplicable; usage counters
//     are enforced by the booking service. An inactive agreement is ignored.
//  6. Rounding: none, euro or fifty_cents, half-even, on the final total only;
//     the delta is recorded in Rounding.
//  7. The deposit is computed by the policy engine, not here.
//
// VAT: amounts are grouped by the VAT rate of their pack/extra (default from
// the settings). Discounts are already allocated to the amounts they reduce;
// the rounding delta is spread across rates by largest remainder; each rate's
// gross is split with money.VATSplit, so taxable + tax sums exactly to Total.
//
// # Panel tools (8.7)
//
// Matrix and Coverage run the same engine (see their docs); CopySeason
// duplicates a price list to another year (see ShiftDate for the date rule).
package pricing
