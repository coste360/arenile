// Package availability is the availability engine (spec 7).
//
// Correctness against double booking is enforced by the database
// (unique(spot_id, date, slot) on spot_allocations and the conditional UPDATE
// on pool_usage, see docs/invariants.md INV-01..INV-10); the store-backed
// parts (holds, allocation, pool consumption) live in internal/domain/
// inventory, which wraps this package.
//
// The files assign.go, quota.go, aggregate.go and errors.go are pure: they
// take the current occupancy as input (FreeSpots, Quantities, QuotaState) and
// never perform I/O or read the clock:
//
//   - AutoAssign: greedy assignment of zone-mode lines to spots (7.5);
//   - OnlineAvailable: online quota per half-day, ignored by the desk (7.6);
//   - SeatAvailability, MinAvailable, ZoneFree: interval aggregation for the
//     public availability query (7.8);
//   - SpotUnavailableError, ConflictError (with Conflict), ErrPoolExhausted,
//     ErrQuotaExceeded, ErrHoldExpired: domain errors.
package availability
