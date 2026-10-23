package inventory

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Waitlist offer holds (M5B, spec 10.5, ADR 0017): the waitlist service
// reserves the freed inventory for the entry it offers it to with an
// ordinary hold (allocations kind hold, DB-enforced, INV-01), created and
// released inside its own transaction together with the offer row, audit
// and outbox. Offer holds last up to 7 days (waitlist_settings
// offer_hold_minutes), longer than a panel hold (MaxHoldTTL).

// MaxOfferHoldTTL bounds the hold of a waitlist offer (10080 minutes).
const MaxOfferHoldTTL = 7 * 24 * time.Hour

// CreateHoldInTx is CreateHold without principal and own transaction:
// the caller (the waitlist service) has already authorised the action and
// runs it in tx. Conflicts are *availability.ConflictError. The TTL is
// required and bounded by MinHoldTTL..MaxOfferHoldTTL; the audit actor is
// the system (the offer writes its own audit entry).
func (s *Service) CreateHoldInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in HoldInput) (*HoldView, error) {
	ttl := in.TTL
	in.TTL = 0 // validated below against the waitlist bound
	if err := validateHold(&in); err != nil {
		return nil, err
	}
	if ttl < MinHoldTTL || ttl > MaxOfferHoldTTL {
		return nil, configerr.Invalid("hold_minutes", "Durata dell'offerta non valida.")
	}
	now := s.d.Clock.Now()
	h := &store.Hold{
		ID: store.NewID(), Channel: in.Channel, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots,
		Items: in.Items, Note: in.Note, ExpiresAt: now.Add(ttl).UTC(), CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertHold(ctx, tx, es, h); err != nil {
		return nil, wrap("create hold", err)
	}
	a, err := s.Allocate(ctx, tx, es, AllocateRequest{
		HoldID: h.ID, Channel: h.Channel, StartsOn: h.StartsOn, EndsOn: h.EndsOn, Slots: h.Slots,
		Kind: KindHold, ExpiresAt: &h.ExpiresAt, Lines: holdLines(h.Items),
	})
	if err != nil {
		return nil, err
	}
	if err := s.recordAs(ctx, tx, es, audit.SystemActor, ActionHoldCreated, "hold", h.ID, map[string]any{
		"starts_on": h.StartsOn, "ends_on": h.EndsOn, "slots": h.Slots, "items": h.Items, "expires_at": h.ExpiresAt, "source": "waitlist",
	}); err != nil {
		return nil, err
	}
	if err := s.writeEvent(ctx, tx, es, eventHoldCreated, allocationChange("hold_created", a, func(c *changeIDs) { c.holdID = h.ID })); err != nil {
		return nil, err
	}
	return s.view(h), nil
}

// ReleaseHoldInTx frees an open hold in tx (idempotent: an already
// released, converted or swept hold is a no-op) and writes audit and
// hold.released, the event that lets the waitlist offer the inventory to
// the next entry.
func (s *Service) ReleaseHoldInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, holdID string) error {
	ok, err := store.CloseHold(ctx, tx, es, holdID, "", s.d.Clock.Now())
	if err != nil || !ok {
		return wrap("release hold", err)
	}
	a, err := s.releaseOwner(ctx, tx, es, store.AllocationOwner{HoldID: holdID})
	if err != nil {
		return wrap("release hold", err)
	}
	if err := s.recordAs(ctx, tx, es, audit.SystemActor, ActionHoldReleased, "hold", holdID, map[string]any{"reason": "waitlist"}); err != nil {
		return err
	}
	return s.writeEvent(ctx, tx, es, eventHoldReleased, allocationChange("hold_released", a, func(c *changeIDs) { c.holdID = holdID }))
}

// OfferAvailability is the availability the waitlist reads to choose the
// resources of an offer: channel as given (online and api apply the online
// quota, ADR 0011), no permission check (the caller authorised es),
// expired holds ignored. Advisory only: the hold itself is decided by the
// database constraints.
func (s *Service) OfferAvailability(ctx context.Context, es store.EstablishmentScope, q AvailabilityQuery) (*AvailabilityResult, error) {
	if q.Slots == "" {
		q.Slots = calendar.SlotsFull
	}
	if q.Channel == "" {
		q.Channel = store.ChannelDesk
	}
	if err := validateRange(q.From, q.To, MaxAvailabilityDays); err != nil {
		return nil, err
	}
	res, err := s.availability(ctx, es, q)
	return res, wrap("offer availability", err)
}

func (s *Service) recordAs(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, actor audit.Actor, action, entity, id string, diff any) error {
	if s.d.Audit == nil {
		return nil
	}
	return s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: entity, EntityID: id, Diff: diff,
	})
}
