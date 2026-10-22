// Package mapping manages maps, zones, spots, decorations and pools (spec
// 6.2, 11.2): the map document with transactional, versioned save
// (PUT /maps/{id} with If-Match), the server-side row generator, zones with
// their assignment mode and the implicit capacity pool of `zone` zones
// (spec 7.5: one system_managed pool per zone, capacity = active spots,
// created/updated/removed by this service whenever zones or spots change).
// Mutations require auth.PermEstablishmentConfigure; reads auth.PermBookingsRead.
//
// Ownership (M1): the "mapping" builder owns this package, the handlers in
// internal/http/admin/maps.go (maps, zones, pools, generate-rows) and the
// repository functions in internal/store/mapping.go. Models live in
// internal/store/models_mapping.go.
//
// Rules: unique(map_id, label); a spot with future allocations cannot be
// deleted, only deactivated (AllocationChecker, ErrSpotHasFutureAllocations);
// every save bumps maps.version, writes the audit log and emits
// event.ConfigMapChanged in the same transaction.
package mapping
