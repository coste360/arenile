package partner

import "errors"

// Sentinel errors of the partner domain (problem types in
// internal/http/problem/m8.go).
var (
	// ErrPartnerInUse: agreements, bookings or payments reference the
	// partner; deactivate it instead -> 409 partner_in_use.
	ErrPartnerInUse = errors.New("partner: in use")
	// ErrCategoryInUse: agreements reference the category -> 409
	// customer_category_in_use.
	ErrCategoryInUse = errors.New("partner: customer category in use")
)
