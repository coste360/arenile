// Package mapping manages maps, zones, spots, decorations and pools (spec
// 5.2, 6.2, 7.5, 11.2): the map document with transactional, versioned save
// (PUT /maps/{id} with If-Match), the server-side row generator, zones with
// their assignment mode and the implicit capacity pool of `zone` zones.
// Mutations require auth.PermEstablishmentConfigure; reads auth.PermBookingsRead.
//
// Ownership (M1): the "mapping" builder owns this package, the handlers in
// internal/http/admin/maps.go (maps, zones, pools, generate-rows) and the
// repository functions in internal/store/mapping.go, zone.go and pool.go.
// Models live in internal/store/models_mapping.go.
//
// Map document save (SaveMap, GenerateRows with apply): one transaction that
// checks the If-Match version, validates the whole document (labels unique
// per map, finite coordinates inside [0,width]x[0,height], valid shapes and
// kinds, non-negative component quantities, zones of this establishment and
// of this map, never `quantity` zones), diff-applies spots and decorations by
// id (insert / update / delete; spot ids survive label swaps), bumps
// maps.version with a conditional UPDATE (concurrent saves: one wins, the
// others get configerr.VersionConflictError), resyncs the implicit pools,
// writes the audit log and emits event.ConfigMapChanged (payload
// event.ConfigChange, version 1). Zone and pool mutations emit the same
// event with entity zone / pool.
//
// Locking: map saves, zone updates/deletions and map deletions lock the
// zone rows they touch first (store.LockZones, a no-op UPDATE that is a row
// lock on PostgreSQL), then the map row. Spot counts and the implicit pool
// capacity are read after the lock, so a save adding spots to a zone and an
// update turning it into `quantity` can never both commit
// (TestConcurrency_ZoneUpdateVsSaveMap).
//
// Future allocations: deleting a spot (or a map) whose spots have
// allocations from today (establishment timezone) is refused with
// configerr.SpotsAllocatedError; deactivation (active=false) is always
// allowed. The check goes through AllocationChecker, wired to NoAllocations
// until M3 adds spot_allocations.
//
// Implicit pools (spec 7.5): every `zone` zone has exactly one
// system_managed pool (component_code "spot", name = zone name) whose
// capacity is the number of active spots of the zone; it is created,
// updated or removed in the same transaction as any zone change or map save.
// System pools are read-only through the API (ErrSystemPoolReadOnly).
// Hand-made pools may be bound only to `quantity` zones.
//
// Deleting a map deletes its spots, decorations and zones (with their
// implicit pools). A zone with spots, with hand-made pools bound to it or
// referenced by price rules, promo codes, agreements (conditions.zone_ids),
// packs (allowed_zone_ids) or booking policies (scope.zones) cannot be
// deleted, alone or with its map: ZoneReferencedError lists the references
// (409 conflict).
//
// Sea side (M5A, spec 11.2): "top" is the default of new maps and of the
// row generator (row A nearest to the sea). ReorientLayout is the pure
// geometry of a sea side change (mirror for opposite sides, quarter turn
// with width and height swapped for adjacent ones; distances from the sea,
// ids, labels, rows and zones preserved, walkway tiles kept on their
// 60-unit grid, round trip = identity) and ReorientMap persists it through
// the same transactional save as SaveMap (If-Match, one version, audit
// "map.reoriented", config.map_changed with change "reoriented"); spots
// with booking, season or block allocations from today on need an explicit
// acknowledgement (configerr.SpotsAllocatedError otherwise). Dry runs
// return the preview and the counts without writing.
//
// Composition: Service.WithDB(tx) lets the guided setup run these use cases
// inside its own transaction (they become savepoints).
package mapping
