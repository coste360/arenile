package partner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// MaxVerificationNoteLen bounds the free note of a verification (never a
// document number, spec 23).
const MaxVerificationNoteLen = 200

// VerifyInput is api.RequirementVerificationInput.
type VerifyInput struct {
	AgreementID        *string
	CustomerCategoryID *string
	Note               string
}

// ResolveRequirement validates the agreement or category of a
// verification and returns the requirement text at the time of the check.
// Shared with internal/domain/booking (verification on a booking).
func ResolveRequirement(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in *VerifyInput) (string, error) {
	v := &configerr.ValidationError{}
	in.Note = strings.TrimSpace(in.Note)
	if len([]rune(in.Note)) > MaxVerificationNoteLen {
		v.Add("note", fmt.Sprintf("Massimo %d caratteri.", MaxVerificationNoteLen))
	}
	in.AgreementID, in.CustomerCategoryID = blankNil(in.AgreementID), blankNil(in.CustomerCategoryID)
	if in.AgreementID == nil && in.CustomerCategoryID == nil {
		v.Add("agreement_id", "Indica la convenzione o la categoria del requisito.")
	}
	for field, p := range map[string]*string{"agreement_id": in.AgreementID, "customer_category_id": in.CustomerCategoryID} {
		if p != nil {
			if _, err := uuid.Parse(*p); err != nil {
				v.Add(field, "Identificativo non valido.")
			}
		}
	}
	if err := v.OrNil(); err != nil {
		return "", err
	}
	var parts []string
	if in.AgreementID != nil {
		ag, err := store.GetAgreement(ctx, db, es, *in.AgreementID)
		if errors.Is(err, store.ErrNotFound) {
			return "", configerr.Invalid("agreement_id", "Convenzione inesistente.")
		}
		if err != nil {
			return "", fmt.Errorf("partner: %w", err)
		}
		if ag.Requirement != "" {
			parts = append(parts, ag.Requirement)
		}
	}
	if in.CustomerCategoryID != nil {
		cc, err := store.GetCustomerCategory(ctx, db, es, *in.CustomerCategoryID)
		if errors.Is(err, store.ErrNotFound) {
			return "", configerr.Invalid("customer_category_id", "Categoria inesistente.")
		}
		if err != nil {
			return "", fmt.Errorf("partner: %w", err)
		}
		if cc.Requirement != "" {
			parts = append(parts, cc.Requirement)
		}
	}
	return strings.Join(parts, " · "), nil
}

// ListCustomerVerifications lists the verifications recorded on a customer
// (bookings.read), newest first.
func (s *Service) ListCustomerVerifications(ctx context.Context, p *auth.Principal, establishmentID, customerID string) ([]store.RequirementVerification, map[string]string, error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, nil, err
	}
	if _, err := store.GetCustomer(ctx, s.d.DB, es, customerID); err != nil {
		return nil, nil, notFound(err)
	}
	rows, err := store.CustomerRequirementVerifications(ctx, s.d.DB, es, customerID)
	if err != nil {
		return nil, nil, fmt.Errorf("partner: %w", err)
	}
	names, err := s.verifierNames(ctx, s.d.DB, rows)
	return rows, names, err
}

// VerifyCustomer records the verification of a requirement on a customer
// (bookings.write): date and operator, never a copy of the document.
func (s *Service) VerifyCustomer(ctx context.Context, p *auth.Principal, establishmentID, customerID string, in VerifyInput) (*store.RequirementVerification, map[string]string, error) {
	var out *store.RequirementVerification
	var names map[string]string
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermBookingsWrite)
		if err != nil {
			return err
		}
		c, err := store.GetCustomer(ctx, tx, es, customerID)
		if err != nil {
			return notFound(err)
		}
		if c.AnonymizedAt != nil {
			return configerr.Invalid("customer_id", "Cliente anonimizzato.")
		}
		req, err := ResolveRequirement(ctx, tx, es, &in)
		if err != nil {
			return err
		}
		cid := customerID
		row := &store.RequirementVerification{
			ID: store.NewID(), Subject: "customer", CustomerID: &cid, AgreementID: in.AgreementID,
			CustomerCategoryID: in.CustomerCategoryID, Requirement: req, Note: in.Note,
			VerifiedByUserID: store.ExistingUser(ctx, tx, userOf(p)), VerifiedAt: s.now(),
		}
		if err := store.InsertRequirementVerification(ctx, tx, es, row); err != nil {
			return fmt.Errorf("partner: verify: %w", err)
		}
		out = row
		if names, err = s.verifierNames(ctx, tx, []store.RequirementVerification{*row}); err != nil {
			return err
		}
		return s.record(ctx, tx, p, es, EntityVerification, row.ID, "created", map[string]any{
			"subject": "customer", "customer_id": customerID, "agreement_id": in.AgreementID,
			"customer_category_id": in.CustomerCategoryID,
		}, false)
	})
	return out, names, err
}

func (s *Service) verifierNames(ctx context.Context, db bun.IDB, rows []store.RequirementVerification) (map[string]string, error) {
	var ids []string
	for _, r := range rows {
		if r.VerifiedByUserID != nil {
			ids = append(ids, *r.VerifiedByUserID)
		}
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	names, err := store.UserNames(ctx, db, ids)
	if err != nil {
		return nil, fmt.Errorf("partner: %w", err)
	}
	return names, nil
}

func blankNil(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

func isConflict(err error) bool { return errors.Is(err, store.ErrConflict) }
