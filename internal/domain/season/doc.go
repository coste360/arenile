// Package season manages seasons, rate periods and closure days of an
// establishment (spec 6.1). Mutations require auth.PermEstablishmentConfigure
// (owner, manager); reads require auth.PermBookingsRead.
//
// Ownership (M1): the "season" builder owns this package, the handlers in
// internal/http/admin/seasons.go (except CopySeason, which lives in
// pricing.go and belongs to the pricingconfig builder) and the repository
// functions in internal/store/season.go. Models: store.Season,
// store.RatePeriod, store.ClosureDay (internal/store/models_season.go).
//
// Rules: unique(establishment, year); opens_on <= closes_on; rate periods of a
// season never overlap and must lie inside the season; every mutation bumps
// `version`, honours If-Match (ErrVersionConflict) and writes the audit log in
// the same transaction. Changes to rate periods also emit
// event.ConfigPricingChanged in the same transaction.
//
// Decisions (M1):
//   - Status transitions: draft <-> active, active <-> closed (CanTransition);
//     only draft seasons can be deleted (the delete cascades to rate periods,
//     price rules, fixed prices and policies).
//   - Every season and rate period mutation emits config.pricing_changed with
//     payload {"resource","resource_id","season_id","action"}; closure days
//     write only the audit log (no event in spec 18.1).
//   - Rate period writers lock the season row (store.LockSeason) so that
//     concurrent writers cannot create overlapping periods.
//   - Closure days are unique per (establishment, date) and must fall inside
//     a season.
//   - Season copy (spec 8.7): Service.CopySeason creates the target season
//     (draft, dates shifted with pricing.ShiftDate) and copies the rate
//     periods (clipped and de-overlapped, with warnings). The CopySeason
//     endpoint (pricing.go) runs it with WithDB(tx) and copies price rules,
//     fixed prices and policies in the same transaction.
//   - WithDB lets composing services (setup, pricingconfig) run these use
//     cases inside their own transaction (nested transactions = savepoints).
package season
