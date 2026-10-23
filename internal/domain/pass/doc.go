// Package pass manages season passes and their releases (spec 7.7, 10.4,
// 15.1 Abbonamenti). A season pass is a booking with is_season_pass = true
// and formula season whose spot allocations have kind season for every day
// and slot of the period (ADR 0010): the pass id is the booking id, and
// cancellation, payments and moves go through the booking and payment
// services.
//
// Ownership (M3): the "passes" builder owns this package, the handlers in
// internal/http/admin/passes.go and the repository functions in
// internal/store/pass.go (season_releases and the season-pass views of
// bookings). Model: store.SeasonRelease (models_booking.go).
//
// Structure (functional core / imperative shell, ports & adapters):
//   - plan.go is pure: pass interval (season bounds, start moved to today
//     when the season already started), release planning (dates inside the
//     pass, not in the past, slots covered by the pass), overlap with active
//     releases, resale share and credit from the policy snapshot;
//   - passes.go, releases.go and resale.go are the shell: authorization,
//     one transaction per use case, the ports of ports.go (Bookings,
//     Inventory, Credits) called with the same tx, audit and outbox.
//
// Creation: Create validates season, spot (active, in the establishment),
// pack and renewal reference, then calls Bookings.CreateInTx with formula
// season, IsSeasonPass and AllocationKind = inventory.KindSeason in its own
// transaction: pricing comes from the pricing engine (fixed season price or
// period rules), allocations for the whole range are created atomically and
// a spot already taken fails with *availability.ConflictError listing the
// half-days (409 spot_unavailable). The booking service writes
// booking.created; this package adds the audit entry season_pass.created.
//
// Releases (INV-10): releasing half-days deletes the season allocations of
// those half-days (Inventory.RemoveSeasonDays) and inserts season_releases
// rows, in one transaction with audit and event.SeasonDayReleased (payload
// event.AllocationChange with spot_ids/dates); the spot becomes bookable.
// The partial unique index on active releases is the arbiter between
// concurrent requests (TestConcurrency_SeasonReleaseSameDay). Revoking a
// release not yet resold sets revoked_at with a conditional UPDATE (the
// arbiter against a concurrent resale), recreates the allocation
// (Inventory.RestoreSeasonDays; a hold or block taken meanwhile fails with
// a ConflictError and rolls back) and emits event.SeasonDayReleaseRevoked;
// a resold half-day cannot be revoked (409).
//
// Resale: when a released half-day is sold, the booking service calls
// Resale.MarkResold in its transaction: resold_booking_line_id is set and,
// when the pass policy snapshot enables season_release_credit, the holder
// gets percent of the resale line's share for that half-day (line total
// spread on its half-days, half-even) as a customer_credits movement
// (source season_release, season_release_id set); credit_amount keeps it on
// the release. UnmarkResold (resale cancelled or moved) clears the link and
// reverses the credit with a negative movement.
//
// Construction: Resale is a separate stateless type so that the booking
// service can depend on it while Service depends on the booking service
// (no construction cycle); see internal/app/m3.go.
//
// Permissions: list/get -> auth.PermBookingsRead; create/update ->
// auth.PermBookingsWrite; releases and revocations ->
// auth.PermOperationsRun (spec 16.1 "rilasci"). Panel staff may always
// release on behalf of the holder; customer_can_release_season_days
// applies to the self-service flow (M4).
//
// Integrator notes: a manual price override of a pass (cents, audited) needs
// a field in api.SeasonPassCreate and in booking.CreateInput (the price is
// computed and snapshotted by the booking service); renewals (spec 10.4)
// are M5 and only the renewal_of_booking_id reference is stored here.
package pass
