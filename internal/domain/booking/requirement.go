package booking

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/store"
)

// Requirement verification on a booking (spec 30.2.2-30.2.3): date and
// operator only, never a copy of the document. Writes a
// store.RequirementVerification (subject booking), the audit entry and a
// booking.modified event in the same transaction.

// VerifyInput is api.RequirementVerificationInput.
type VerifyInput struct {
	ExpectedVersion    *int64
	AgreementID        *string
	CustomerCategoryID *string
	Note               string
}

// VerifyRequirement records the verification of the requirement of the
// booking's agreement (or of the agreement / category given). Requires
// auth.PermBookingsWrite.
func (s *Service) VerifyRequirement(ctx context.Context, p *auth.Principal, establishmentID, id string, in VerifyInput) (*Detail, error) {
	return s.mutate(ctx, p, establishmentID, id, auth.PermBookingsWrite, in.ExpectedVersion,
		func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
			if Status(b.Status).IsTerminal() {
				return nil, ErrNotModifiable
			}
			vin := partner.VerifyInput{AgreementID: in.AgreementID, CustomerCategoryID: in.CustomerCategoryID, Note: in.Note}
			if vin.AgreementID == nil && vin.CustomerCategoryID == nil {
				if b.AgreementID == nil {
					return nil, configerr.Invalid("agreement_id", "La prenotazione non ha una convenzione da verificare.")
				}
				a := *b.AgreementID
				vin.AgreementID = &a
			}
			v, err := insertBookingVerification(ctx, tx, es, b, userOf(p), vin, s.now())
			if err != nil {
				return nil, err
			}
			return &outcome{action: eventModified, diff: map[string]any{
				"requirement_verified": true, "verification_id": v.ID, "agreement_id": v.AgreementID,
				"customer_category_id": v.CustomerCategoryID,
			}}, nil
		})
}

// insertBookingVerification writes a verification with subject booking.
func insertBookingVerification(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, by *string, in partner.VerifyInput, now time.Time) (*store.RequirementVerification, error) {
	req, err := partner.ResolveRequirement(ctx, tx, es, &in)
	if err != nil {
		return nil, err
	}
	bid, cid := b.ID, b.CustomerID
	v := &store.RequirementVerification{
		ID: store.NewID(), Subject: "booking", BookingID: &bid, CustomerID: &cid, AgreementID: in.AgreementID,
		CustomerCategoryID: in.CustomerCategoryID, Requirement: req, Note: in.Note, VerifiedByUserID: store.ExistingUser(ctx, tx, by), VerifiedAt: now,
	}
	if err := store.InsertRequirementVerification(ctx, tx, es, v); err != nil {
		return nil, err
	}
	return v, nil
}

func userOf(p *auth.Principal) *string {
	if p != nil && p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		return &u
	}
	return nil
}
