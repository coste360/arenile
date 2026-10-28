package availability

import "errors"

// M8 agreement quota (spec 30.2.2, ADR 0028, INV-103). The quota is a
// capacity reservation like the online quota (spec 7.6): it is enforced in
// internal/domain/inventory by the same database mechanisms (conditional
// UPDATE on agreement_quota_usage for the agreement cap; zone lock and
// recount after the insert for the capacity reserved to an agreement and
// not yet released), never by an application pre-check.

// ErrAgreementQuotaExceeded: an agreement sale exceeds max_per_day, or a
// sale outside the agreement would take capacity still reserved to it.
// Returned as *ConflictError with ReasonAgreementQuota or
// ReasonAgreementReserved conflicts.
var ErrAgreementQuotaExceeded = errors.New("availability: agreement quota exceeded")

// Conflict reasons of the agreement quota (mirrored by the API
// AllocationConflict.reason).
const (
	// ReasonAgreementQuota: the agreement cap (max_per_day) is reached.
	ReasonAgreementQuota = "agreement_quota"
	// ReasonAgreementReserved: the free capacity left is reserved to an
	// agreement until its release date.
	ReasonAgreementReserved = "agreement_reserved"
)

// ReservedRemaining is the capacity an agreement still holds back from
// general sale on one half-day: max(reserved - used, 0) before releaseOn,
// 0 from releaseOn on. Pure helper shared by inventory and the reports.
func ReservedRemaining(reserved, used int, released bool) int {
	if released || used >= reserved {
		return 0
	}
	return reserved - used
}
