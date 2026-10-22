package pricingconfig

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// codeRe is the format of promo and agreement codes (compared ignoring case).
var codeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,39}$`)

// PromoCode is a stored promo code with its decoded engine documents.
type PromoCode struct {
	ID                 string
	Code               string
	Description        string
	Action             pricing.Action
	Conditions         pricing.Conditions
	ValidFrom          *calendar.Date
	ValidTo            *calendar.Date
	MaxUses            *int
	MaxUsesPerCustomer *int
	Uses               int
	Stackable          bool
	Active             bool
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Engine returns the pricing engine view of the code. Usage limits are not
// part of it: they are checked by the caller that owns the counters.
func (pc PromoCode) Engine() pricing.PromoCode {
	return pricing.PromoCode{
		ID: pc.ID, Code: pc.Code, Action: pc.Action, Conditions: pc.Conditions, ValidFrom: pc.ValidFrom,
		ValidTo: pc.ValidTo, Stackable: pc.Stackable, Active: pc.Active,
	}
}

// Exhausted reports whether the global usage limit is reached.
func (pc PromoCode) Exhausted() bool { return pc.MaxUses != nil && pc.Uses >= *pc.MaxUses }

// PromoCodeInput is the full content of a promo code (Uses is read-only).
type PromoCodeInput struct {
	Code               string
	Description        string
	Action             pricing.Action
	Conditions         pricing.Conditions
	ValidFrom          *calendar.Date
	ValidTo            *calendar.Date
	MaxUses            *int
	MaxUsesPerCustomer *int
	Stackable          bool
	Active             bool
}

func promoFromRow(r *store.PromoCode) (PromoCode, error) {
	out := PromoCode{
		ID: r.ID, Code: r.Code, Description: r.Description, ValidFrom: r.ValidFrom, ValidTo: r.ValidTo,
		MaxUses: r.MaxUses, MaxUsesPerCustomer: r.MaxUsesPerCustomer, Uses: r.Uses, Stackable: r.Stackable,
		Active: r.Active, Version: r.Version, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if err := r.Action.Unmarshal(&out.Action); err != nil {
		return PromoCode{}, fmt.Errorf("pricingconfig: promo %s action: %w", r.ID, err)
	}
	if err := r.Conditions.Unmarshal(&out.Conditions); err != nil {
		return PromoCode{}, fmt.Errorf("pricingconfig: promo %s conditions: %w", r.ID, err)
	}
	return out, nil
}

func validatePromo(ctx context.Context, db bun.IDB, es store.EstablishmentScope, selfID string, uses int, in *PromoCodeInput) error {
	v := &configerr.ValidationError{}
	in.Code = strings.TrimSpace(in.Code)
	if !codeRe.MatchString(in.Code) {
		v.Add("code", "Da 2 a 40 caratteri tra lettere, cifre, trattino e trattino basso.")
	}
	in.Description = checkText(v, "description", strings.TrimSpace(in.Description))
	if in.MaxUses != nil && *in.MaxUses < 1 {
		v.Add("max_uses", "Deve essere almeno 1.")
	} else if in.MaxUses != nil && *in.MaxUses < uses {
		v.Add("max_uses", fmt.Sprintf("Il codice è già stato usato %d volte.", uses))
	}
	if in.MaxUsesPerCustomer != nil && *in.MaxUsesPerCustomer < 1 {
		v.Add("max_uses_per_customer", "Deve essere almeno 1.")
	}
	if in.ValidFrom != nil && in.ValidTo != nil && in.ValidTo.Before(*in.ValidFrom) {
		v.Add("valid_to", "Deve essere successiva o uguale a valid_from.")
	}
	if err := v.OrNil(); err != nil {
		return err
	}
	if err := fromEngine(pricing.ValidatePromoCode(pricing.PromoCode{
		ID: selfID, Code: in.Code, Action: in.Action, Conditions: in.Conditions,
		ValidFrom: in.ValidFrom, ValidTo: in.ValidTo, Stackable: in.Stackable, Active: in.Active,
	}), ""); err != nil {
		return err
	}
	existing, err := store.GetPromoCodeByCode(ctx, db, es, in.Code)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return fmt.Errorf("pricingconfig: %w", err)
	case existing.ID != selfID:
		return ErrDuplicatePromo
	}
	return nil
}

func applyPromo(row *store.PromoCode, in PromoCodeInput) error {
	act, err := jsonOf(in.Action)
	if err != nil {
		return err
	}
	cond, err := jsonOf(in.Conditions)
	if err != nil {
		return err
	}
	row.Code, row.Description, row.Action, row.Conditions = in.Code, in.Description, act, cond
	row.ValidFrom, row.ValidTo, row.MaxUses, row.MaxUsesPerCustomer = in.ValidFrom, in.ValidTo, in.MaxUses, in.MaxUsesPerCustomer
	row.Stackable, row.Active = in.Stackable, in.Active
	return nil
}

// ListPromoCodes lists promo codes (view permission).
func (s *Service) ListPromoCodes(ctx context.Context, p *auth.Principal, establishmentID string, active *bool, cursor string, limit int) ([]PromoCode, string, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	page, err := store.ListPromoCodes(ctx, s.d.DB, es, store.PricingActiveFilter{Active: active}, cursor, limit)
	if err != nil {
		return nil, "", mapStoreErr(err, nil)
	}
	out := make([]PromoCode, 0, len(page.Items))
	for i := range page.Items {
		pc, err := promoFromRow(&page.Items[i])
		if err != nil {
			return nil, "", err
		}
		out = append(out, pc)
	}
	return out, page.NextCursor, nil
}

// GetPromoCode returns one promo code (view permission).
func (s *Service) GetPromoCode(ctx context.Context, p *auth.Principal, establishmentID, id string) (*PromoCode, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetPromoCode(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	pc, err := promoFromRow(row)
	return &pc, err
}

// CreatePromoCode stores a promo code; codes are unique ignoring case.
func (s *Service) CreatePromoCode(ctx context.Context, p *auth.Principal, establishmentID string, in PromoCodeInput) (*PromoCode, error) {
	var out *PromoCode
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		id := store.NewID()
		if err := validatePromo(ctx, tx, es, id, 0, &in); err != nil {
			return err
		}
		now := s.now()
		row := &store.PromoCode{ID: id, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := applyPromo(row, in); err != nil {
			return err
		}
		if err := store.InsertPromoCode(ctx, tx, es, row); err != nil {
			return mapStoreErr(err, ErrDuplicatePromo)
		}
		pc, err := promoFromRow(row)
		if err != nil {
			return err
		}
		out = &pc
		return change(ctx, tx, s.d, p, es, EntityPromoCode, id, ChangeCreated, "", pc)
	})
	return out, err
}

// UpdatePromoCode replaces a promo code (the usage counter is preserved).
func (s *Service) UpdatePromoCode(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in PromoCodeInput) (*PromoCode, error) {
	var out *PromoCode
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetPromoCode(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := promoFromRow(row)
		if err != nil {
			return err
		}
		if err := validatePromo(ctx, tx, es, id, row.Uses, &in); err != nil {
			return err
		}
		old := row.Version
		if err := applyPromo(row, in); err != nil {
			return err
		}
		row.Version, row.UpdatedAt = old+1, s.now()
		ok, err := store.UpdatePromoCode(ctx, tx, es, row, old)
		if err != nil {
			return mapStoreErr(err, ErrDuplicatePromo)
		}
		if !ok {
			return staleOrMissing(old, true)
		}
		after, err := promoFromRow(row)
		if err != nil {
			return err
		}
		out = &after
		return change(ctx, tx, s.d, p, es, EntityPromoCode, id, ChangeUpdated, "", diffOf(before, after))
	})
	return out, err
}

// DeletePromoCode deletes a promo code.
func (s *Service) DeletePromoCode(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetPromoCode(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := promoFromRow(row)
		if err != nil {
			return err
		}
		if err := store.DeletePromoCode(ctx, tx, es, id); err != nil {
			return mapStoreErr(err, nil)
		}
		return change(ctx, tx, s.d, p, es, EntityPromoCode, id, ChangeDeleted, "", before)
	})
}

// ---- agreements -------------------------------------------------------------

// Agreement is a stored agreement (convenzione) with decoded documents.
type Agreement struct {
	ID          string
	Code        *string
	Name        string
	PartnerName string
	Notes       string
	Action      pricing.Action
	Conditions  pricing.Conditions
	ValidFrom   *calendar.Date
	ValidTo     *calendar.Date
	Stackable   bool
	Active      bool
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Engine returns the pricing engine view of the agreement.
func (a Agreement) Engine() pricing.Agreement {
	code := ""
	if a.Code != nil {
		code = *a.Code
	}
	return pricing.Agreement{ID: a.ID, Code: code, Action: a.Action, Conditions: a.Conditions, Stackable: a.Stackable, Active: a.Active}
}

// ValidOn reports whether the agreement is active and d is in its window.
func (a Agreement) ValidOn(d calendar.Date) bool {
	if !a.Active {
		return false
	}
	if a.ValidFrom != nil && d.Before(*a.ValidFrom) {
		return false
	}
	return a.ValidTo == nil || !d.After(*a.ValidTo)
}

// AgreementInput is the full content of an agreement.
type AgreementInput struct {
	Code        *string
	Name        string
	PartnerName string
	Notes       string
	Action      pricing.Action
	Conditions  pricing.Conditions
	ValidFrom   *calendar.Date
	ValidTo     *calendar.Date
	Stackable   bool
	Active      bool
}

func agreementFromRow(r *store.Agreement) (Agreement, error) {
	out := Agreement{
		ID: r.ID, Code: r.Code, Name: r.Name, PartnerName: r.PartnerName, Notes: r.Notes, ValidFrom: r.ValidFrom,
		ValidTo: r.ValidTo, Stackable: r.Stackable, Active: r.Active, Version: r.Version,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if err := r.Action.Unmarshal(&out.Action); err != nil {
		return Agreement{}, fmt.Errorf("pricingconfig: agreement %s action: %w", r.ID, err)
	}
	if err := r.Conditions.Unmarshal(&out.Conditions); err != nil {
		return Agreement{}, fmt.Errorf("pricingconfig: agreement %s conditions: %w", r.ID, err)
	}
	return out, nil
}

func validateAgreement(id string, in *AgreementInput) error {
	v := &configerr.ValidationError{}
	in.Name = trimName(v, "name", in.Name, true)
	in.PartnerName = trimName(v, "partner_name", in.PartnerName, false)
	in.Notes = checkText(v, "notes", strings.TrimSpace(in.Notes))
	if in.Code != nil {
		c := strings.TrimSpace(*in.Code)
		switch {
		case c == "":
			in.Code = nil
		case !codeRe.MatchString(c):
			v.Add("code", "Da 2 a 40 caratteri tra lettere, cifre, trattino e trattino basso.")
		default:
			in.Code = &c
		}
	}
	if in.ValidFrom != nil && in.ValidTo != nil && in.ValidTo.Before(*in.ValidFrom) {
		v.Add("valid_to", "Deve essere successiva o uguale a valid_from.")
	}
	if err := v.OrNil(); err != nil {
		return err
	}
	a := Agreement{ID: id, Code: in.Code, Action: in.Action, Conditions: in.Conditions, Active: in.Active}
	return fromEngine(pricing.ValidateAgreement(a.Engine()), "")
}

func applyAgreement(row *store.Agreement, in AgreementInput) error {
	act, err := jsonOf(in.Action)
	if err != nil {
		return err
	}
	cond, err := jsonOf(in.Conditions)
	if err != nil {
		return err
	}
	row.Code, row.Name, row.PartnerName, row.Notes = in.Code, in.Name, in.PartnerName, in.Notes
	row.Action, row.Conditions, row.ValidFrom, row.ValidTo = act, cond, in.ValidFrom, in.ValidTo
	row.Stackable, row.Active = in.Stackable, in.Active
	return nil
}

// ListAgreements lists agreements (view permission).
func (s *Service) ListAgreements(ctx context.Context, p *auth.Principal, establishmentID string, active *bool, cursor string, limit int) ([]Agreement, string, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	page, err := store.ListAgreements(ctx, s.d.DB, es, store.PricingActiveFilter{Active: active}, cursor, limit)
	if err != nil {
		return nil, "", mapStoreErr(err, nil)
	}
	out := make([]Agreement, 0, len(page.Items))
	for i := range page.Items {
		a, err := agreementFromRow(&page.Items[i])
		if err != nil {
			return nil, "", err
		}
		out = append(out, a)
	}
	return out, page.NextCursor, nil
}

// GetAgreement returns one agreement (view permission).
func (s *Service) GetAgreement(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Agreement, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetAgreement(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	a, err := agreementFromRow(row)
	return &a, err
}

// CreateAgreement stores an agreement; codes are unique ignoring case.
func (s *Service) CreateAgreement(ctx context.Context, p *auth.Principal, establishmentID string, in AgreementInput) (*Agreement, error) {
	var out *Agreement
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		id := store.NewID()
		if err := validateAgreement(id, &in); err != nil {
			return err
		}
		now := s.now()
		row := &store.Agreement{ID: id, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := applyAgreement(row, in); err != nil {
			return err
		}
		if err := store.InsertAgreement(ctx, tx, es, row); err != nil {
			return mapStoreErr(err, ErrDuplicateAgree)
		}
		a, err := agreementFromRow(row)
		if err != nil {
			return err
		}
		out = &a
		return change(ctx, tx, s.d, p, es, EntityAgreement, id, ChangeCreated, "", a)
	})
	return out, err
}

// UpdateAgreement replaces an agreement.
func (s *Service) UpdateAgreement(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in AgreementInput) (*Agreement, error) {
	var out *Agreement
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetAgreement(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := agreementFromRow(row)
		if err != nil {
			return err
		}
		if err := validateAgreement(id, &in); err != nil {
			return err
		}
		old := row.Version
		if err := applyAgreement(row, in); err != nil {
			return err
		}
		row.Version, row.UpdatedAt = old+1, s.now()
		ok, err := store.UpdateAgreement(ctx, tx, es, row, old)
		if err != nil {
			return mapStoreErr(err, ErrDuplicateAgree)
		}
		if !ok {
			return staleOrMissing(old, true)
		}
		after, err := agreementFromRow(row)
		if err != nil {
			return err
		}
		out = &after
		return change(ctx, tx, s.d, p, es, EntityAgreement, id, ChangeUpdated, "", diffOf(before, after))
	})
	return out, err
}

// DeleteAgreement deletes an agreement.
func (s *Service) DeleteAgreement(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetAgreement(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := agreementFromRow(row)
		if err != nil {
			return err
		}
		if err := store.DeleteAgreement(ctx, tx, es, id); err != nil {
			return mapStoreErr(err, nil)
		}
		return change(ctx, tx, s.d, p, es, EntityAgreement, id, ChangeDeleted, "", before)
	})
}
