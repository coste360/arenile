// Package policy is the pure booking policy engine (spec section 9 and the
// refund computation of section 13.4).
//
// The package has no I/O: callers pass the instant "now" and the establishment
// *time.Location explicitly, so every function is deterministic and testable
// with clock.Fake.
//
// # Resolution and inheritance
//
// A Policy has a Scope (channels, zones, packs, rate periods) and Rules.
// Resolve picks, among the policies whose scope matches the booking, the one
// with the highest priority (ties: more constrained dimensions first, then the
// lowest ID, so the choice is deterministic) and merges its Rules over the
// establishment default. Every Rules field is a pointer: nil means "not
// specified" and is inherited from the default. Nested objects (min_lead_time,
// duration, payment, ...) are merged field by field; the cancellation schedule
// and allowed_formulas are replaced as a whole when specified.
//
// # Multi-zone / multi-pack bookings
//
// A booking may contain lines in several zones and with several packs. The
// Target passed to Resolve lists every zone, pack and rate period touched by
// the booking, and a non-empty scope dimension matches when it intersects the
// target ("any" semantics). Rationale: policies typically add constraints to a
// specific product ("gazebos require approval", "August is week-only"), and a
// booking that includes such a product must be subject to them; the highest
// priority policy then decides for the whole booking, which keeps a single,
// explainable policy_snapshot per booking. Managers who need a stricter policy
// to win over a more generic one give it a higher priority. Zones and packs can
// be referenced by ID or by code: the Target should carry both.
//
// # Validation
//
// Validate returns violations with stable codes (see Code) that the API maps to
// problem+json and the UI to i18n keys; approval_required is reported as a
// flag, not as a violation. Deposit, BalanceDueDate, CancellationOutcome,
// ModificationAllowed, WeatherOutcome and SeasonReleaseCreditAmount compute the
// money consequences of the rules with integer cents and half-even rounding.
// Snapshot is the JSON stored in bookings.policy_snapshot.
package policy
