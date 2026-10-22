package pricingconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// FixedPriceInput is the full content of a fixed duration price.
type FixedPriceInput struct {
	SeasonID      string
	PackID        string
	Formula       pricing.Formula
	PriceCategory string
	// RatePeriodCode nil means valid in every period.
	RatePeriodCode *string
	Amount         money.Cents
}

// FixedPriceListFilter narrows ListFixedPrices.
type FixedPriceListFilter struct {
	SeasonID string
	PackID   string
}

func validateFixed(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in *FixedPriceInput) error {
	v := &configerr.ValidationError{}
	in.PriceCategory = strings.TrimSpace(in.PriceCategory)
	if in.PriceCategory == "" {
		v.Add("price_category", "Campo obbligatorio.")
	}
	if !in.Formula.IsFixedDuration() {
		v.Add("formula", "Usa week, fortnight, month o season.")
	}
	if in.Amount < 0 {
		v.Add("amount", "Deve essere maggiore o uguale a zero.")
	}
	if in.RatePeriodCode != nil {
		c := strings.TrimSpace(*in.RatePeriodCode)
		if c == "" {
			in.RatePeriodCode = nil
		} else {
			in.RatePeriodCode = &c
		}
	}
	if err := v.OrNil(); err != nil {
		return err
	}
	sn, err := seasonOf(ctx, db, es, in.SeasonID)
	if err != nil {
		return err
	}
	packs, err := store.PricingListPacks(ctx, db, es)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	found := false
	for _, p := range packs {
		if p.ID == in.PackID {
			found = true
		}
	}
	if !found {
		v.Add("pack_id", "Pack inesistente.")
	}
	if in.RatePeriodCode != nil {
		periods, err := store.PricingListRatePeriods(ctx, db, es, sn.ID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		ok := false
		for _, rp := range periods {
			if rp.Code == *in.RatePeriodCode {
				ok = true
			}
		}
		if !ok {
			v.Add("rate_period_code", "Periodo tariffario inesistente nella stagione.")
		}
	}
	return v.OrNil()
}

func applyFixed(row *store.FixedDurationPrice, in FixedPriceInput) {
	row.SeasonID = in.SeasonID
	row.PackID = in.PackID
	row.Formula = string(in.Formula)
	row.PriceCategory = in.PriceCategory
	row.RatePeriodCode = in.RatePeriodCode
	row.Amount = int64(in.Amount)
}

// ListFixedPrices lists fixed prices (view permission).
func (s *Service) ListFixedPrices(ctx context.Context, p *auth.Principal, establishmentID string, f FixedPriceListFilter, cursor string, limit int) ([]store.FixedDurationPrice, string, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	page, err := store.ListFixedPrices(ctx, s.d.DB, es, store.FixedPriceFilter(f), cursor, limit)
	if err != nil {
		return nil, "", mapStoreErr(err, nil)
	}
	return page.Items, page.NextCursor, nil
}

// GetFixedPrice returns one fixed price (view permission).
func (s *Service) GetFixedPrice(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.FixedDurationPrice, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	fp, err := store.GetFixedPrice(ctx, s.d.DB, es, id)
	return fp, mapStoreErr(err, nil)
}

// CreateFixedPrice stores a fixed price.
func (s *Service) CreateFixedPrice(ctx context.Context, p *auth.Principal, establishmentID string, in FixedPriceInput) (*store.FixedDurationPrice, error) {
	var out *store.FixedDurationPrice
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		if err := validateFixed(ctx, tx, es, &in); err != nil {
			return err
		}
		now := s.now()
		row := &store.FixedDurationPrice{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
		applyFixed(row, in)
		if err := store.InsertFixedPrice(ctx, tx, es, row); err != nil {
			return mapStoreErr(err, ErrDuplicateFixed)
		}
		out = row
		return change(ctx, tx, s.d, p, es, EntityFixedPrice, row.ID, ChangeCreated, row.SeasonID, row)
	})
	return out, err
}

// UpdateFixedPrice replaces a fixed price.
func (s *Service) UpdateFixedPrice(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in FixedPriceInput) (*store.FixedDurationPrice, error) {
	var out *store.FixedDurationPrice
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetFixedPrice(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		if err := validateFixed(ctx, tx, es, &in); err != nil {
			return err
		}
		before := *row
		old := row.Version
		applyFixed(row, in)
		row.Version, row.UpdatedAt = old+1, s.now()
		ok, err := store.UpdateFixedPrice(ctx, tx, es, row, old)
		if err != nil {
			return mapStoreErr(err, ErrDuplicateFixed)
		}
		if !ok {
			return staleOrMissing(old, true)
		}
		out = row
		return change(ctx, tx, s.d, p, es, EntityFixedPrice, id, ChangeUpdated, row.SeasonID, diffOf(before, *row))
	})
	return out, err
}

// DeleteFixedPrice deletes a fixed price.
func (s *Service) DeleteFixedPrice(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetFixedPrice(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		if err := store.DeleteFixedPrice(ctx, tx, es, id); err != nil {
			return mapStoreErr(err, nil)
		}
		return change(ctx, tx, s.d, p, es, EntityFixedPrice, id, ChangeDeleted, row.SeasonID, row)
	})
}
