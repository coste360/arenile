package booking

import (
	"context"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Requirement verification on a booking (spec 30.2.2-30.2.3): date and
// operator only, never a copy of the document. Writes a
// store.RequirementVerification (subject booking), the audit entry and a
// booking.modified event in the same transaction. Owner: the agreements
// builder.

// VerifyInput is api.RequirementVerificationInput.
type VerifyInput struct {
	ExpectedVersion    *int64
	AgreementID        *string
	CustomerCategoryID *string
	Note               string
}

// VerifyRequirement records the verification of the requirement of the
// booking's agreement. Requires auth.PermBookingsWrite.
func (s *Service) VerifyRequirement(ctx context.Context, p *auth.Principal, establishmentID, id string, in VerifyInput) (*store.RequirementVerification, error) {
	_, _, _, _, _ = ctx, p, establishmentID, id, in
	return nil, configerr.ErrNotImplemented
}
