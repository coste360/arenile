// Package findspot is "Trova posto" (spec 30.6, ADR 0028): the cash desk
// search of availability with a maximum number of spot changes, and the
// booking of a chosen proposal.
//
// Search loads the occupancy of the candidate spots through the Occupancy
// port (implemented by internal/domain/inventory: same snapshot as the
// operations day and auto-assignment, expired holds ignored), runs the pure
// availability.FindSpots and prices every proposal through the Quoter port
// (the booking pricer: agreement included; for a split stay each day is
// priced with the price category of the spot occupied that day, ADR 0028).
//
// Book creates the booking through internal/domain/booking with one
// allocation per segment in a single transaction: if a half-day was taken
// meanwhile the DB constraints refuse it (409 spot_unavailable with the
// conflicts, the panel searches again). No app-level availability check
// ever replaces the constraints (INV-01, INV-102).
//
// The decision to keep the search here and the algorithm in availability:
// availability stays pure and reusable (auto-assignment, public split stay),
// this package owns the I/O orchestration and permissions.
package findspot
