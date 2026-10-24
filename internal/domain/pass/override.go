package pass

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the manual price override.
var (
	// ErrOverrideNotAllowed: the pass is in a terminal status.
	ErrOverrideNotAllowed = fmt.Errorf("%w: il prezzo di un abbonamento chiuso o annullato non può essere modificato", configerr.ErrConflict)
	// ErrOverrideBelowPaid: the new total is lower than what was already paid.
	ErrOverrideBelowPaid = fmt.Errorf("%w: il nuovo prezzo è inferiore a quanto già pagato: registra prima un rimborso", configerr.ErrConflict)
	// ErrNoOverride: clearing a pass that has no manual price.
	ErrNoOverride = fmt.Errorf("%w: l'abbonamento non ha un prezzo manuale", configerr.ErrValidation)
)

// SetPriceOverride replaces the engine total of a season pass with amount
// (cents) and a mandatory reason (ADR 0019 E). Permission
// auth.PermEstablishmentConfigure (owner and manager, never the desk);
// ifMatch is the pass version (required by the handler). One transaction:
// booking row lock, price_current with the override line (price_snapshot
// untouched, INV-21), total and deposit, override columns, audit
// season_pass.price_overridden (before/after, reason) and booking.modified.
func (s *Service) SetPriceOverride(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, amount money.Cents, reason string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if err := validateOverride(amount, reason); err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		v, err := lockPass(ctx, tx, es, id)
		if err != nil {
			return err
		}
		b := v.Booking
		if err := configerr.CheckVersion(ifMatch, b.Version); err != nil {
			return err
		}
		if !overridableStatuses[b.Status] {
			return ErrOverrideNotAllowed
		}
		if amount < b.PaidAmount {
			return ErrOverrideBelowPaid
		}
		engine, prev, err := engineOf(&b)
		if err != nil {
			return err
		}
		cur, err := applyOverride(engine, prev, amount)
		if err != nil {
			return err
		}
		before := b.TotalAmount
		var prevOverride any
		if b.PriceOverrideAmount != nil {
			prevOverride = int64(*b.PriceOverrideAmount)
		}
		now := s.now()
		ea := engine.Total
		b.PriceCurrent, b.TotalAmount = cur, amount
		b.DepositDue = depositFor(&b, amount)
		b.PriceOverrideAmount, b.PriceOverrideEngineAmount = &amount, &ea
		b.PriceOverrideReason, b.PriceOverrideAt = &reason, &now
		b.PriceOverrideByUserID = userOf(p)
		if err := s.save(ctx, tx, es, &b, now); err != nil {
			return err
		}
		if out, err = s.detail(ctx, tx, es, id); err != nil {
			return err
		}
		nb := out.Pass.Booking
		return s.record(ctx, tx, p, ip, es, "season_pass.price_overridden", id, map[string]any{
			"previous_total": int64(before), "new_total": int64(amount), "engine_total": int64(ea),
			"previous_override": prevOverride, "reason": reason,
		}, event.BookingModified, bookingChange(&nb, out.Pass.SpotID))
	})
	return out, err
}

// ClearPriceOverride goes back to the engine price of a season pass: the
// breakdown and total computed by the engine before the override (same
// permission, version check, audit and event as SetPriceOverride).
func (s *Service) ClearPriceOverride(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		v, err := lockPass(ctx, tx, es, id)
		if err != nil {
			return err
		}
		b := v.Booking
		if err := configerr.CheckVersion(ifMatch, b.Version); err != nil {
			return err
		}
		if b.PriceOverrideAmount == nil {
			return ErrNoOverride
		}
		if !overridableStatuses[b.Status] {
			return ErrOverrideNotAllowed
		}
		cur, total, err := restoreEngine(&b)
		if err != nil {
			return err
		}
		if total < b.PaidAmount {
			return ErrOverrideBelowPaid
		}
		before, reason := b.TotalAmount, ""
		if b.PriceOverrideReason != nil {
			reason = *b.PriceOverrideReason
		}
		now := s.now()
		b.PriceCurrent, b.TotalAmount = cur, total
		b.DepositDue = depositFor(&b, total)
		b.PriceOverrideAmount, b.PriceOverrideEngineAmount, b.PriceOverrideReason = nil, nil, nil
		b.PriceOverrideByUserID, b.PriceOverrideAt = nil, nil
		if err := s.save(ctx, tx, es, &b, now); err != nil {
			return err
		}
		if out, err = s.detail(ctx, tx, es, id); err != nil {
			return err
		}
		nb := out.Pass.Booking
		return s.record(ctx, tx, p, ip, es, "season_pass.price_override_cleared", id, map[string]any{
			"previous_total": int64(before), "new_total": int64(total), "previous_reason": reason,
		}, event.BookingModified, bookingChange(&nb, out.Pass.SpotID))
	})
	return out, err
}

// save writes the booking row with optimistic concurrency (the row is
// locked, so a mismatch means a bug or a concurrent writer without lock).
func (s *Service) save(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, now time.Time) error {
	b.UpdatedAt = now
	ok, err := store.UpdateBookingVersioned(ctx, tx, es, b, b.Version)
	if err != nil {
		return fmt.Errorf("pass: price override: %w", err)
	}
	if !ok {
		cur, err := loadPass(ctx, tx, es, b.ID)
		if err != nil {
			return err
		}
		return &configerr.VersionConflictError{Current: cur.Booking.Version}
	}
	return nil
}

// depositFor is the deposit of b for total: before the confirmation the
// rules of the policy snapshot, after it the same rule on the new total
// (as booking.applyTotals does).
func depositFor(b *store.Booking, total money.Cents) money.Cents {
	snap, err := policy.ParseSnapshot(b.PolicySnapshot)
	if err != nil {
		return min(b.DepositDue, total)
	}
	return policy.Deposit(total, snap.Rules)
}

// userOf returns the user id of a panel principal (nil for API keys).
func userOf(p *auth.Principal) *string {
	if p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		return &u
	}
	return nil
}
