// Package pricingconfig is the DB-backed configuration of prices and booking
// policies (spec 6.4, 8, 9). It persists price rules, fixed duration prices,
// promo codes, agreements and booking policies, and runs the panel tools
// (simulator, price matrix, coverage check, season copy) by loading data and
// calling the PURE engines internal/domain/pricing and internal/domain/policy,
// which must stay free of DB, clock and randomness.
//
// Ownership (M2): two builders share this package through separate files:
//   - "pricing" builder: Service (service.go and files named pricing_*.go /
//     rules.go / promo.go ...), handlers in internal/http/admin/pricing.go
//     (price rules, fixed prices, promo codes, agreements, simulate, matrix,
//     coverage, CopySeason), repository functions in internal/store/pricing.go.
//   - "policies" builder: PolicyService (policy_service.go and files named
//     policy_*.go), handlers in internal/http/admin/policies.go, repository
//     functions in internal/store/policies.go.
//
// Rules: mutations require auth.PermEstablishmentConfigure; reads
// auth.PermBookingsRead; every mutation bumps `version`, honours If-Match,
// writes the audit log and emits event.ConfigPricingChanged in the same
// transaction. price_rules.seq = max(seq)+1 of the establishment on every
// insert/update (recency tie-break of spec 8.4).
package pricingconfig
