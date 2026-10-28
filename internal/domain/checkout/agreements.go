package checkout

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
)

// Agreement link and code on the public page (spec 30.2.2, ADR 0028): the
// page reads the agreement of /b/{slug}?c={code} (GetAgreement) and sends
// the code with the quote and the booking (Selection.AgreementCode). The
// quote reports the outcome (Quote.Agreement); the booking applies it in
// internal/domain/booking (CreateOnline), where the quota is enforced.

// ErrAgreementNotFound: no agreement sold online with this code (404,
// never revealing whether the code exists).
var ErrAgreementNotFound = fmt.Errorf("%w: agreement", configerr.ErrNotFound)

// Agreement statuses of a quote (api.AgreementStatus.status).
const (
	AgreementApplied        = "applied"
	AgreementInvalid        = "invalid"
	AgreementNotApplicable  = "not_applicable"
	AgreementQuotaExhausted = "quota_exhausted"
)

// AgreementStatus is api.AgreementStatus.
type AgreementStatus struct {
	Code        string
	Name        string
	Requirement string
	Status      string
}

// GetAgreement returns the agreement of a public link (active, valid today
// and sold online), ErrAgreementNotFound otherwise.
func (s *Service) GetAgreement(ctx context.Context, slug, code string) (*pricingconfig.PublicAgreement, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	a, err := pricingconfig.OnlineAgreement(ctx, s.d.DB, pe.Scope, code, s.today(pe.Establishment))
	switch {
	case errors.Is(err, pricingconfig.ErrAgreementCodeUnknown), errors.Is(err, pricingconfig.ErrAgreementNotApplicable):
		return nil, ErrAgreementNotFound
	case err != nil:
		return nil, err
	}
	return a, nil
}

// quoteAgreement resolves the agreement code of a selection: the agreement
// id to price with ("" when not applicable) and its status for the quote.
func (s *Service) quoteAgreement(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, in Selection) (string, *AgreementStatus, error) {
	if in.AgreementCode == "" {
		return "", nil, nil
	}
	st := &AgreementStatus{Code: in.AgreementCode, Status: AgreementApplied}
	a, err := pricingconfig.OnlineAgreement(ctx, db, pe.Scope, in.AgreementCode, s.today(pe.Establishment))
	switch {
	case errors.Is(err, pricingconfig.ErrAgreementCodeUnknown), errors.Is(err, pricingconfig.ErrAgreementNotApplicable):
		// Unknown, desk-only, inactive or expired codes are all "invalid"
		// without a name: the public quote never confirms that a code
		// exists (ADR 0028). not_applicable is kept for the pricing refusals
		// of a valid online agreement (season, scope): priceWithAgreement.
		st.Status = AgreementInvalid
		return "", st, nil
	case err != nil:
		return "", nil, err
	}
	st.Name, st.Requirement = a.Name, a.M8.Requirement
	if q := a.M8.Quota; q != nil && q.MaxPerDay != nil {
		units := 0
		for _, l := range in.Lines {
			units += max(l.Quantity, 1)
		}
		used, err := store.AgreementUsedOn(ctx, db, pe.Scope, []string{a.ID}, in.StartsOn, in.EndsOn)
		if err != nil {
			return "", nil, fmt.Errorf("checkout: %w", err)
		}
		for _, d := range calendar.Range(in.StartsOn, in.EndsOn) {
			for _, sl := range in.Slots.Expand() {
				if used[store.QuotaKey{AgreementID: a.ID, Date: d, Slot: sl}]+units > *q.MaxPerDay {
					st.Status = AgreementQuotaExhausted
					return "", st, nil
				}
			}
		}
	}
	return a.ID, st, nil
}

// priceWithAgreement prices sim with the agreement of the selection and,
// when the pricing refuses it (season, channel), without it.
func (s *Service) priceWithAgreement(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, in Selection, sim *pricingconfig.SimulateInput, out *Quote) (*pricingconfig.SimulateResult, error) {
	id, st, err := s.quoteAgreement(ctx, db, pe, in)
	if err != nil {
		return nil, err
	}
	out.Agreement, sim.AgreementID = st, id
	res, err := s.d.Pricer.Quote(ctx, db, pe.Scope, *sim)
	if id != "" && errors.Is(err, pricingconfig.ErrAgreementNotApplicable) {
		st.Status, sim.AgreementID = AgreementNotApplicable, ""
		res, err = s.d.Pricer.Quote(ctx, db, pe.Scope, *sim)
	}
	return res, err
}
