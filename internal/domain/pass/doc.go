// Package pass manages season passes and their releases (spec 7.7, 10.4,
// 15.1 Abbonamenti). A season pass is a booking with is_season_pass = true
// and formula season whose spot allocations have kind season for every day
// and slot of the period: the pass id is the booking id, and cancellation,
// payments and moves go through the booking and payment services.
//
// Ownership (M3): the "passes" builder owns this package, the handlers in
// internal/http/admin/passes.go and the repository functions in
// internal/store/pass.go (season_releases and the season-pass views of
// bookings). Model: store.SeasonRelease (models_booking.go).
//
// Releases (INV-10): releasing half-days deletes the season allocations of
// those half-days (Inventory.RemoveSeasonDays) and inserts season_releases
// rows, in one transaction with audit and event.SeasonDayReleased (payload
// event.AllocationChange with spot_ids/dates); the spot becomes bookable.
// Revoking a release not yet resold recreates the allocations
// (Inventory.RestoreSeasonDays) and sets revoked_at
// (event.SeasonDayReleaseRevoked); a resold half-day cannot be revoked (409).
// When a released half-day is sold, the booking service calls
// Resale.MarkResold in its transaction: resold_booking_line_id is set and,
// per policy season_release_credit, a credit is granted to the holder
// (Credits.GrantCredit, source season_release).
//
// Construction: Resale is a separate stateless type so that the booking
// service can depend on it while Service depends on the booking service
// (no construction cycle); see internal/app/m3.go.
//
// Permissions: list/get -> auth.PermBookingsRead; create/update ->
// auth.PermBookingsWrite; releases and revocations ->
// auth.PermOperationsRun (spec 16.1 "rilasci").
package pass
