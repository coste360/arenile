package pricingconfig

// PolicyService implements the booking policies use cases (spec 9). It shares
// Deps with Service but is owned by a different builder.
type PolicyService struct {
	d Deps
}

// NewPolicyService returns a PolicyService.
func NewPolicyService(d Deps) *PolicyService { return &PolicyService{d: d} }
