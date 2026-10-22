// Package catalog manages components, packs and extras (spec 6.3).
// Mutations require auth.PermEstablishmentConfigure; reads auth.PermBookingsRead.
//
// Ownership (M1): the "catalog" builder owns this package, the handlers in
// internal/http/admin/catalog.go and the repository functions in
// internal/store/catalog.go. Models live in internal/store/models_catalog.go.
//
// Rules: unique(establishment, code) per table; a tracked component must
// reference a pool of the same establishment; vat_rate_bp nil = establishment
// default; every mutation bumps `version`, honours If-Match, writes the audit
// log and emits event.ConfigPricingChanged (packs and extras change what the
// price list covers) in the same transaction.
package catalog
