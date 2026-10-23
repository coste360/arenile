package inventory

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// Panel holds (spec 7.4, ADR 0010 §3): a selection occupied with an expiry
// before the booking exists. Allocations are kind hold with hold_id; POST
// /bookings with hold_id converts them (ConvertHold) in the booking
// transaction.

// Hold statuses (computed).
const (
	HoldActive    = "active"
	HoldReleased  = "released"
	HoldConverted = "converted"
	HoldExpired   = "expired"
)

// Hold TTL bounds for ttl_seconds.
const (
	MinHoldTTL = 30 * time.Second
	MaxHoldTTL = 24 * time.Hour
	// maxHoldDays bounds the interval of a panel hold (a season).
	maxHoldDays = 200
	maxNoteLen  = 500
)

// HoldInput creates a panel hold.
type HoldInput struct {
	Channel  string
	StartsOn calendar.Date
	EndsOn   calendar.Date
	Slots    calendar.Slots
	Items    []store.HoldItem
	Note     string
	// TTL overrides Deps.HoldTTL (bounded by MinHoldTTL..MaxHoldTTL).
	TTL time.Duration
	IP  string
}

// HoldView is a hold with its computed status.
type HoldView struct {
	store.Hold
	Status string
}

func (s *Service) view(h *store.Hold) *HoldView {
	return &HoldView{Hold: *h, Status: holdStatus(h, s.d.Clock.Now())}
}

// holdStatus derives the public status of a hold (pure). A hold closed at or
// after its expiry (by the sweeper, or by a late release of a hold that had
// already lapsed) reports "expired": the operator did not free it in time.
func holdStatus(h *store.Hold, now time.Time) string {
	switch {
	case h.ConvertedBookingID != nil:
		return HoldConverted
	case h.ReleasedAt != nil && h.ReleasedAt.Before(h.ExpiresAt):
		return HoldReleased
	case h.ReleasedAt != nil, !h.ExpiresAt.After(now):
		return HoldExpired
	default:
		return HoldActive
	}
}

func validateHold(in *HoldInput) error {
	var ve configerr.ValidationError
	if in.Channel == "" {
		in.Channel = store.ChannelDesk
	}
	switch in.Channel {
	case store.ChannelDesk, store.ChannelOnline, store.ChannelAPI:
	default:
		ve.Add("channel", "Canale non valido.")
	}
	if in.StartsOn.IsZero() || in.EndsOn.IsZero() || in.EndsOn.Before(in.StartsOn) {
		ve.Add("ends_on", "La data finale non può precedere quella iniziale.")
	} else if in.StartsOn.DaysUntil(in.EndsOn) >= maxHoldDays {
		ve.Add("ends_on", fmt.Sprintf("Un hold copre al massimo %d giorni.", maxHoldDays))
	}
	if !in.Slots.Valid() {
		ve.Add("slots", "Fasce non valide.")
	}
	if len(in.Items) == 0 {
		ve.Add("items", "Selezionare almeno una risorsa.")
	}
	for i, it := range in.Items {
		f := fmt.Sprintf("items[%d]", i)
		set := 0
		for _, v := range []string{it.SpotID, it.ZoneID, it.PoolID} {
			if v != "" {
				set++
			}
		}
		if set == 0 || (it.PoolID != "" && set > 1) {
			ve.Add(f, "Indicare una postazione, una zona o un pool.")
		}
		if it.Quantity < 0 || it.Quantity > 1000 {
			ve.Add(f+".quantity", "Quantità non valida.")
		}
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > maxNoteLen {
		ve.Add("note", fmt.Sprintf("Al massimo %d caratteri.", maxNoteLen))
	}
	if in.TTL != 0 && (in.TTL < MinHoldTTL || in.TTL > MaxHoldTTL) {
		ve.Add("ttl_seconds", "Durata dell'hold non valida.")
	}
	return ve.OrNil()
}

// holdLines turns hold items into allocation lines.
func holdLines(items []store.HoldItem) []LineRequest {
	out := make([]LineRequest, len(items))
	for i, it := range items {
		out[i] = LineRequest{SpotID: it.SpotID, ZoneID: it.ZoneID, PoolID: it.PoolID, Quantity: max(it.Quantity, 1)}
	}
	return out
}

// CreateHold occupies a selection for TTL (bookings.write). Conflicts are
// *availability.ConflictError (409 spot_unavailable / pool_exhausted /
// quota_exceeded).
func (s *Service) CreateHold(ctx context.Context, p *auth.Principal, establishmentID string, in HoldInput) (*HoldView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	in.Channel = auth.ChannelFor(p, in.Channel) // API keys always hold on "api" (ADR 0011)
	if err := validateHold(&in); err != nil {
		return nil, err
	}
	ttl := s.d.HoldTTL
	if in.TTL > 0 {
		ttl = in.TTL
	}
	now := s.d.Clock.Now()
	h := &store.Hold{
		ID: store.NewID(), Channel: in.Channel, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots,
		Items: in.Items, Note: in.Note, ExpiresAt: now.Add(ttl).UTC(), CreatedAt: now, UpdatedAt: now,
	}
	if p != nil && p.UserID != "" {
		h.CreatedByUserID = ptr(p.UserID)
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertHold(ctx, tx, es, h); err != nil {
			return err
		}
		a, err := s.Allocate(ctx, tx, es, AllocateRequest{
			HoldID: h.ID, Channel: h.Channel, StartsOn: h.StartsOn, EndsOn: h.EndsOn, Slots: h.Slots,
			Kind: KindHold, ExpiresAt: &h.ExpiresAt, Lines: holdLines(h.Items),
		})
		if err != nil {
			return err
		}
		if err := s.record(ctx, tx, p, es, in.IP, ActionHoldCreated, "hold", h.ID, map[string]any{
			"starts_on": h.StartsOn, "ends_on": h.EndsOn, "slots": h.Slots, "items": h.Items, "expires_at": h.ExpiresAt,
		}); err != nil {
			return err
		}
		return s.writeEvent(ctx, tx, es, eventHoldCreated, allocationChange("hold_created", a, func(c *changeIDs) { c.holdID = h.ID }))
	})
	if err != nil {
		return nil, wrap("create hold", err)
	}
	return s.view(h), nil
}

// GetHold returns a hold with its status (bookings.read).
func (s *Service) GetHold(ctx context.Context, p *auth.Principal, establishmentID, id string) (*HoldView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	h, err := store.GetHold(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, wrap("get hold", err)
	}
	return s.view(h), nil
}

// ReleaseHold frees a hold (bookings.write). Idempotent: an already
// released, converted or swept hold is a no-op.
func (s *Service) ReleaseHold(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return err
	}
	if _, err := store.GetHold(ctx, s.d.DB, es, id); err != nil {
		return wrap("release hold", err)
	}
	_, err = s.closeHold(ctx, p, es, id, ip, "released")
	return wrap("release hold", err)
}

// closeHold releases an open hold and its allocations in one transaction
// and reports whether this call closed it: the conditional close makes
// concurrent closers act once.
func (s *Service) closeHold(ctx context.Context, p *auth.Principal, es store.EstablishmentScope, id, ip, why string) (bool, error) {
	closed := false
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ok, err := store.CloseHold(ctx, tx, es, id, "", s.d.Clock.Now())
		if err != nil || !ok {
			return err
		}
		closed = true
		a, err := s.releaseOwner(ctx, tx, es, store.AllocationOwner{HoldID: id})
		if err != nil {
			return err
		}
		if err := s.record(ctx, tx, p, es, ip, ActionHoldReleased, "hold", id, map[string]any{"reason": why}); err != nil {
			return err
		}
		return s.writeEvent(ctx, tx, es, eventHoldReleased, allocationChange("hold_released", a, func(c *changeIDs) { c.holdID = id }))
	})
	return closed && err == nil, err
}

// sweepBatch bounds one run of the expired hold sweeper.
const sweepBatch = 200

// SweepExpiredHolds releases the panel holds past their expiry (all
// establishments) and returns how many it closed. Idempotent: holds closed
// meanwhile by a user or another runner are skipped by the conditional
// close. Booking holds are expired by the booking service, which owns the
// status change.
func (s *Service) SweepExpiredHolds(ctx context.Context) (int, error) {
	closed := 0
	for {
		batch, err := store.ListExpiredOpenHolds(ctx, s.d.DB, s.d.Clock.Now(), sweepBatch)
		if err != nil {
			return closed, fmt.Errorf("inventory: sweep holds: %w", err)
		}
		for _, h := range batch {
			es, err := store.SystemEstablishmentScope(ctx, s.d.DB, h.EstablishmentID)
			if err != nil {
				return closed, fmt.Errorf("inventory: sweep holds: %w", err)
			}
			done, err := s.closeHold(ctx, nil, es, h.ID, "", "expired")
			if err != nil {
				return closed, fmt.Errorf("inventory: sweep holds: %w", err)
			}
			if done {
				closed++
			}
		}
		if len(batch) < sweepBatch {
			return closed, nil
		}
	}
}

// Jobs returns the background jobs of the inventory (registered by the app):
// the expired panel hold sweeper every 30 s (spec 7.4).
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name: "inventory.sweep_expired_holds", Interval: 30 * time.Second, RunOnStart: true,
		Run: func(ctx context.Context) error {
			n, err := s.SweepExpiredHolds(ctx)
			if n > 0 {
				s.d.Log.InfoContext(ctx, "expired holds released", "count", n)
			}
			return err
		},
	}}
}
