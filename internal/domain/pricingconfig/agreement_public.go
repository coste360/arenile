package pricingconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Agreement link and code on the public page (spec 30.2.2, ADR 0028): an
// agreement is visible online only when it is active, valid today and sold
// with its code online (an explicit online_code or online_link channel; an
// agreement without channels is desk only). Anything else is "not found",
// so the codes are never revealed.

// ErrAgreementCodeUnknown: no agreement sold online with this code.
var ErrAgreementCodeUnknown = errors.New("pricingconfig: agreement code unknown")

// PublicAgreement is an agreement as the public page shows it.
type PublicAgreement struct {
	Agreement
	PartnerName string
	Summary     string // short condition, Italian
}

// OnlineAgreement resolves code for the public page of es on today.
// ErrAgreementCodeUnknown when no agreement is sold online with it,
// ErrAgreementNotApplicable when it exists but is not usable today.
func OnlineAgreement(ctx context.Context, db bun.IDB, es store.EstablishmentScope, code string, today calendar.Date) (*PublicAgreement, error) {
	code = strings.TrimSpace(code)
	if code == "" || !codeRe.MatchString(code) {
		return nil, ErrAgreementCodeUnknown
	}
	row, err := store.GetAgreementByCode(ctx, db, es, code)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrAgreementCodeUnknown
	}
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	a, err := agreementFromRow(row)
	if err != nil {
		return nil, err
	}
	if !a.M8.OnlineChannel() {
		return nil, ErrAgreementCodeUnknown
	}
	out := &PublicAgreement{Agreement: a, PartnerName: a.PartnerName}
	if err := fillAgreementOutput(ctx, db, es, []*Agreement{&out.Agreement}); err != nil {
		return nil, err
	}
	if p := out.M8.Partner; p != nil {
		out.PartnerName = p.Name
	}
	out.Summary = shortCondition(a)
	if !a.ValidOn(today) {
		return out, ErrAgreementNotApplicable
	}
	return out, nil
}

// shortCondition is the condition in a few words ("-15%").
func shortCondition(a Agreement) string {
	switch a.Action.Type {
	case pricing.ActionPercentOff:
		if a.Action.Percent != nil {
			if bp, _ := a.Action.Percent.BasisPoints(); bp > 0 {
				return "-" + strings.ReplaceAll(a.Action.Percent.String(), ".", ",") + "%"
			}
		}
	case pricing.ActionAmountOff:
		if a.Action.Amount > 0 {
			return fmt.Sprintf("-%d,%02d €", a.Action.Amount/100, a.Action.Amount%100)
		}
	}
	return ""
}
