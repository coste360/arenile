// Package setup implements the guided setup (spec 15.2): the four templates
// ("Lido classico a file", "Spiaggia attrezzata a quantità", "Lido con
// piscina", "Vuoto"), applying a template (season, rate periods, map with
// generated rows, zones, pools, components, packs, extras, default booking
// policies) and the quick price list (day price per price category x rate
// period -> base price rules), plus the demo configuration of `arenile seed
// demo` (spec 27.10). Acceptance (spec 28 M1): a lido of 8 rows x 15 spots
// configured in < 15 minutes from a template.
//
// Permissions: Apply and QuickPricelist require
// auth.PermEstablishmentConfigure (owner, manager); ListTemplates and Status
// require auth.PermBookingsRead (every role). SeedDemo is a system (CLI)
// operation.
//
// Transactions: a template is applied in ONE transaction, which also writes
// the audit entry ("setup.template_applied") and the outbox events
// config.map_changed and config.pricing_changed. To keep that single
// transaction while the other configuration services were being written
// concurrently, this package writes the rows through the store models
// (bun query builder on store.* models, always filtered by the
// establishment of an authorized store.EstablishmentScope) instead of calling
// the season/mapping/catalog/pricingconfig services. The integrator may move
// these writes behind tx-aware repository functions of internal/store.
//
// Idempotency: applying a template is refused (ErrAlreadyConfigured, 409)
// when the establishment already has maps or zones, unless ApplyInput.Force
// is set (not exposed by the API) or the template is "empty". The season of
// the year and its rate periods are reused when they exist; components,
// packs and extras are reused by code; zone codes get a numeric suffix.
//
// Ownership (M1): the "setup" builder owns this package, the handlers in
// internal/http/admin/setup.go and the `seed demo` wiring in
// cmd/arenile/commands.go.
package setup
