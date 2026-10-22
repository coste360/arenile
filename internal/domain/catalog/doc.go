// Package catalog manages components, packs and extras (spec 6.3).
// Mutations require auth.PermEstablishmentConfigure; reads auth.PermBookingsRead.
//
// Ownership (M1): the "catalog" builder owns this package, the handlers in
// internal/http/admin/catalog.go and the repository functions in
// internal/store/catalog.go. Models live in internal/store/models_catalog.go.
//
// Rules: unique(establishment, code) per table; a tracked component must
// reference a pool of the same establishment holding the same component code
// (an untracked one has no pool); pack compositions and extra component codes
// must name existing components; allowed zones/packs must belong to the
// establishment; vat_rate_bp nil = establishment default; every mutation
// bumps `version`, honours If-Match, writes the audit log
// (component.*, pack.*, extra.*) and, for packs and extras, emits
// event.ConfigPricingChanged with ChangeData in the same transaction.
//
// Deletion: components have no archived state, so a component used by a
// pack, an extra or a pool cannot be deleted or renamed (ErrComponentInUse).
// A pack referenced by price rules, fixed prices, extras or booking policies,
// and an extra targeted by price rules, is archived (active = false) instead
// of deleted. M3 must extend these checks to booking lines.
//
// The pricing engine view of the catalog is Service.PricingCatalog (with
// authorization) or LoadPricingCatalog (for callers holding a scope, also
// inside their own transaction).
package catalog
