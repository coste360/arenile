// Package setup implements the guided setup (spec 15.2): the four templates
// ("Lido classico a file", "Spiaggia attrezzata a quantità", "Lido con
// piscina", "Vuoto"), applying a template (season, rate periods, map with
// generated rows, zones, pools, components, packs, extras) and the quick price
// list (day price per price category x rate period -> base price rules).
// Acceptance (spec 28 M1): a lido of 8 rows x 15 spots configured in < 15
// minutes from a template. Requires auth.PermEstablishmentConfigure.
//
// Ownership (M1): the "setup" builder owns this package and the handlers in
// internal/http/admin/setup.go. It composes the other domain services (their
// exported methods) rather than writing SQL itself; if a single transaction
// is needed it may add tx-aware helpers to its own files only and must not
// edit the other packages.
package setup
