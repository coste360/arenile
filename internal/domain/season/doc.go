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
package season
