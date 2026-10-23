// Package rollover implements the end-of-season passage (spec 8.7, 10.4,
// 28; M5B): from a source season it previews (dry run) and applies the
// creation of the next season with its configuration copied in ONE
// transaction: rate periods, price rules, fixed prices and policies (via
// pricingconfig, which owns those tables), closure days inside the source
// season shifted to the new year (pricing.ShiftDate, de-duplicated against
// existing closures), and a season_rollovers row. Catalog, zones, spots and
// maps are not season-scoped and stay as they are.
//
// Idempotency: season_rollovers is unique on (establishment_id,
// source_season_id, target_year). Apply first looks the row up and, when
// found, returns the recorded result with AlreadyApplied = true (HTTP 200)
// without writing; a season of target_year that was not created by a
// rollover of this source is ErrTargetExists (409 season_rollover_conflict).
// Concurrent applies serialise on the unique index (the loser re-reads).
//
// Audit "season.rolled_over" and outbox event season.rolled_over (ids,
// years and counts only) are written in the same transaction. Permission
// establishment.configure.
//
// The renewal campaign (internal/domain/renewal) is the next step of the
// M5 acceptance cycle and reads SeasonPasses from the preview.
package rollover
