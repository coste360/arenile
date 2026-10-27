// Package partner manages the counterparts of agreements and the customer
// categories (spec 30.2, ADR 0028):
//
//   - partners (hotels, B&B, municipality, companies, associations), kept
//     separate from customers; contact data never logged;
//   - customer categories (residents, over 65...) and their members;
//   - requirement verifications on a customer (date and operator, never a
//     copy of the document, spec 23); the verification on a booking goes
//     through internal/domain/booking (VerifyRequirement), which owns every
//     booking change;
//   - the partner account: payments received from a partner and the
//     statement (balance = charged - paid, INV-104). The charged amount is
//     bookings.partner_amount, maintained by internal/domain/booking with
//     the booking total.
//
// Agreements themselves stay in internal/domain/pricingconfig (extended
// fields, versions, quota configuration); the quota is enforced by
// internal/domain/inventory (INV-103). Reports (per partner, agreements
// register) live in internal/domain/report.
//
// Cross-package ports implemented here: Lookup (used by booking to validate
// partner_id and to build PartnerRef badges).
package partner
