package waitlist

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/store"
)

// Offers: an offer reserves inventory for an entry with a hold (DB
// constraints decide, never an app-level check) until its expiry. The
// resources are chosen from the advisory availability (freed spots first);
// each candidate is tried in its own transaction, so a conflict (someone
// booked first, another entry got it) simply moves on.

// maxCandidates bounds the resource sets tried for one entry.
const maxCandidates = 4

// errEntryChanged: the entry changed between the read and the attempt.
var errEntryChanged = errors.New("waitlist: entry changed")

// Offer creates an offer now (manual cause), optionally on spotID.
func (s *Service) Offer(ctx context.Context, p *auth.Principal, establishmentID, entryID, spotID string, hold time.Duration, ip string) (*Offer, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	set, err := s.settings(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	if !set.Enabled {
		return nil, ErrDisabled
	}
	if hold == 0 {
		hold = time.Duration(set.OfferHoldMinutes) * time.Minute
	}
	if hold < minHoldMinutes*time.Minute || hold > maxHoldMinutes*time.Minute {
		return nil, configerr.Invalid("hold_minutes", fmt.Sprintf("Tra %d e %d minuti.", minHoldMinutes, maxHoldMinutes))
	}
	e, err := s.entry(ctx, s.d.DB, es, entryID)
	if err != nil {
		return nil, err
	}
	switch e.Status {
	case store.WaitlistWaiting:
	case store.WaitlistOffered:
		return nil, fmt.Errorf("%w: l'iscrizione ha già un'offerta attiva", configerr.ErrConflict)
	default:
		return nil, ErrClosed
	}
	if spotID != "" {
		sp, err := store.BookingSpotsByIDs(ctx, s.d.DB, es, []string{spotID})
		if err != nil || len(sp) == 0 {
			return nil, configerr.Invalid("spot_id", "Postazione inesistente.")
		}
	}
	o, err := s.offerEntry(ctx, es, e, spotID, hold, store.WaitlistCauseManual, nil, p.Actor(ip))
	if err != nil {
		return nil, err
	}
	labels, err := s.spotLabels(ctx, s.d.DB, es, []store.WaitlistOffer{*o})
	if err != nil {
		return nil, err
	}
	return &Offer{WaitlistOffer: *o, SpotLabel: labelOf(labels, o.SpotID)}, nil
}

// RevokeOffer revokes a pending offer and releases its hold; the entry
// goes back to waiting.
func (s *Service) RevokeOffer(ctx context.Context, p *auth.Principal, establishmentID, entryID, offerID, ip string) (*Offer, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		o, err := store.GetWaitlistOffer(ctx, tx, es, offerID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && o.EntryID != entryID) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("waitlist: %w", err)
		}
		if o.Status != store.WaitlistOfferPending {
			return ErrOfferExpired
		}
		if err := s.closeOffer(ctx, tx, es, o, store.WaitlistOfferRevoked, p.Actor(ip)); err != nil {
			return err
		}
		return s.reopenEntry(ctx, tx, es, entryID, today)
	})
	if err != nil {
		return nil, err
	}
	o, err := store.GetWaitlistOffer(ctx, s.d.DB, es, offerID)
	if err != nil {
		return nil, fmt.Errorf("waitlist: %w", err)
	}
	labels, err := s.spotLabels(ctx, s.d.DB, es, []store.WaitlistOffer{*o})
	if err != nil {
		return nil, err
	}
	return &Offer{WaitlistOffer: *o, SpotLabel: labelOf(labels, o.SpotID)}, nil
}

// offerEntry tries the candidate resource sets of e in order and returns
// the first offer created. ErrNoAvailability when none could be held.
func (s *Service) offerEntry(ctx context.Context, es store.EstablishmentScope, e *store.WaitlistEntry, spotID string, ttl time.Duration, cause string, freed []string, actor audit.Actor) (*store.WaitlistOffer, error) {
	sets, err := s.offerItems(ctx, es, e, spotID, freed)
	if err != nil {
		return nil, err
	}
	var last error
	for _, items := range sets {
		var o *store.WaitlistOffer
		err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			var err error
			o, err = s.attempt(ctx, tx, es, e, items, ttl, cause, actor)
			return err
		})
		switch {
		case err == nil:
			return o, nil
		case errors.Is(err, errEntryChanged):
			return nil, fmt.Errorf("%w: l'iscrizione è cambiata, riprova", configerr.ErrConflict)
		case isConflict(err):
			last = err
			continue
		default:
			return nil, err
		}
	}
	if last != nil {
		var ce *availability.ConflictError
		if errors.As(last, &ce) {
			return nil, last
		}
	}
	return nil, ErrNoAvailability
}

// isConflict reports an availability conflict of the hold or a lost race
// on the unique indexes (one pending offer per entry and per hold).
func isConflict(err error) bool {
	var ce *availability.ConflictError
	return errors.As(err, &ce) || errors.Is(err, store.ErrConflict) || errors.Is(err, availability.ErrQuotaExceeded)
}

// attempt creates one offer in tx: entry waiting -> offered (conditional
// on its version, new link token), hold on items, offer row, audit and
// waitlist.offered, then the offer email.
func (s *Service) attempt(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, e *store.WaitlistEntry, items []store.HoldItem, ttl time.Duration, cause string, actor audit.Actor) (*store.WaitlistOffer, error) {
	token, hash, err := checkout.NewToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	next := *e
	next.Status, next.AccessTokenHash = store.WaitlistOffered, &hash
	next.Version, next.UpdatedAt = e.Version+1, now
	ok, err := store.UpdateWaitlistEntry(ctx, tx, es, &next, e.Version)
	if err != nil {
		return nil, fmt.Errorf("waitlist: offer: %w", err)
	}
	if !ok {
		return nil, errEntryChanged
	}
	h, err := s.d.Holds.CreateHoldInTx(ctx, tx, es, inventory.HoldInput{
		Channel: e.Channel, StartsOn: e.StartsOn, EndsOn: e.EndsOn, Slots: e.Slots, Items: items,
		Note: "Lista d'attesa", TTL: ttl,
	})
	if err != nil {
		return nil, err
	}
	o := &store.WaitlistOffer{
		ID: store.NewID(), EntryID: e.ID, Status: store.WaitlistOfferPending, HoldID: h.ID, Cause: cause,
		ExpiresAt: h.ExpiresAt, CreatedAt: now, UpdatedAt: now,
	}
	for _, it := range items {
		if it.SpotID != "" {
			sp := it.SpotID
			o.SpotID = &sp
			break
		}
	}
	if err := store.InsertWaitlistOffer(ctx, tx, es, o); err != nil {
		return nil, fmt.Errorf("waitlist: offer: %w", err)
	}
	ch := entryChange(e)
	ch.OfferID, ch.HoldID, ch.Cause, ch.ExpiresAt = o.ID, h.ID, cause, o.ExpiresAt.Format(time.RFC3339)
	if err := s.record(ctx, tx, es, actor, ActionOffered, e.ID, map[string]any{
		"offer_id": o.ID, "hold_id": h.ID, "cause": cause, "expires_at": o.ExpiresAt,
	}, event.WaitlistOffered, ch); err != nil {
		return nil, err
	}
	if err := s.sendLink(ctx, tx, es, &next, notify.TemplateWaitlistOffer, "waitlist_offer:"+o.ID, token, o.ExpiresAt); err != nil {
		return nil, err
	}
	return o, nil
}

// sendLink queues a waitlist email with the personal link (skipped for
// customers without a usable email: the staff contacts them).
func (s *Service) sendLink(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, e *store.WaitlistEntry, template, dedupe, token string, expires time.Time) error {
	if s.d.Notify == nil {
		return nil
	}
	c, err := store.GetCustomer(ctx, tx, es, e.CustomerID)
	if err != nil {
		return fmt.Errorf("waitlist: customer: %w", err)
	}
	if c.AnonymizedAt != nil || c.Email == nil || *c.Email == "" {
		return nil
	}
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return fmt.Errorf("waitlist: establishment: %w", err)
	}
	_, err = s.d.Notify.EnqueueWaitlist(ctx, tx, es, notify.WaitlistMessage{
		Template: template, CustomerID: c.ID, Recipient: *c.Email, Locale: e.Locale, DedupeKey: dedupe,
		PackID: e.PackID, StartsOn: e.StartsOn, EndsOn: e.EndsOn, ExpiresAt: expires,
		URL: s.linkURL(est.Slug, token),
	})
	if err != nil {
		return fmt.Errorf("waitlist: email: %w", err)
	}
	return nil
}

// linkURL is the personal waitlist page of the public site.
func (s *Service) linkURL(slug, token string) string {
	return s.d.BaseURL + "/b/" + slug + "/m/w/" + token
}

// closeOffer moves a pending offer to status (declined, expired, revoked),
// releases its hold (hold.released lets the next entry have it) and writes
// audit and waitlist.offer_expired with the reason. The entry is left to
// the caller. ErrOfferExpired when it was no longer pending.
func (s *Service) closeOffer(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, o *store.WaitlistOffer, status string, actor audit.Actor) error {
	ok, err := store.CloseWaitlistOffer(ctx, tx, es, o.ID, status, nil, s.now())
	if err != nil {
		return fmt.Errorf("waitlist: close offer: %w", err)
	}
	if !ok {
		return ErrOfferExpired
	}
	if err := s.d.Holds.ReleaseHoldInTx(ctx, tx, es, o.HoldID); err != nil {
		return fmt.Errorf("waitlist: release hold: %w", err)
	}
	return s.record(ctx, tx, es, actor, ActionOfferClosed, o.EntryID, map[string]any{"offer_id": o.ID, "status": status},
		event.WaitlistOfferExpired, EntryChange{EntryID: o.EntryID, OfferID: o.ID, HoldID: o.HoldID, Reason: status})
}

// reopenEntry puts an offered entry back to waiting (expired when its
// period has ended before today).
func (s *Service) reopenEntry(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, entryID string, today calendar.Date) error {
	cur, err := s.entry(ctx, tx, es, entryID)
	if err != nil {
		return err
	}
	if cur.Status != store.WaitlistOffered {
		return nil
	}
	next := *cur
	next.Status = store.WaitlistWaiting
	if cur.EndsOn.Before(today) {
		next.Status, next.AccessTokenHash = store.WaitlistExpired, nil
	}
	next.Version, next.UpdatedAt = cur.Version+1, s.now()
	ok, err := store.UpdateWaitlistEntry(ctx, tx, es, &next, cur.Version)
	if err != nil {
		return fmt.Errorf("waitlist: reopen: %w", err)
	}
	if !ok {
		return ErrVersionMismatch
	}
	return nil
}

// offerItems returns the candidate resource sets (hold items) of an
// entry, best first: the given or requested spot; otherwise the available
// spots of the compatible seat zones (spots freed by the triggering event
// first) or the zone/quantity zones with enough units. Every set carries
// the tracked components of the pack (pools other than the zone's own).
func (s *Service) offerItems(ctx context.Context, es store.EstablishmentScope, e *store.WaitlistEntry, spotID string, freed []string) ([][]store.HoldItem, error) {
	pack, err := store.GetPackForInventory(ctx, s.d.DB, es, e.PackID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNoAvailability
		}
		return nil, fmt.Errorf("waitlist: pack: %w", err)
	}
	codes := make([]string, 0, len(pack.Composition))
	for c := range pack.Composition {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	compPools, err := store.TrackedComponentPools(ctx, s.d.DB, es, codes)
	if err != nil {
		return nil, fmt.Errorf("waitlist: components: %w", err)
	}
	qty := max(e.Quantity, 1)
	if spotID == "" && e.SpotID != nil {
		spotID = *e.SpotID
	}
	res, err := s.d.Holds.OfferAvailability(ctx, es, inventory.AvailabilityQuery{
		From: e.StartsOn, To: e.EndsOn, Slots: e.Slots, Channel: e.Channel, ZoneID: deref(e.ZoneID), PackID: e.PackID,
	})
	if err != nil {
		if errors.Is(err, configerr.ErrNotFound) {
			return nil, ErrNoAvailability
		}
		return nil, fmt.Errorf("waitlist: availability: %w", err)
	}
	poolZone := map[string]string{}
	for _, pl := range res.Pools {
		if pl.ZoneID != nil {
			poolZone[pl.PoolID] = *pl.ZoneID
		}
	}
	components := func(zoneID string) []store.HoldItem {
		var out []store.HoldItem
		for _, code := range codes {
			pool, ok := compPools[code]
			if !ok || pool == "" || poolZone[pool] == zoneID {
				continue
			}
			if n := pack.Composition[code] * qty; n > 0 {
				out = append(out, store.HoldItem{PoolID: pool, Quantity: n})
			}
		}
		return out
	}
	if spotID != "" {
		zoneID := ""
		for _, z := range res.Zones {
			for _, sp := range z.Spots {
				if sp.SpotID == spotID {
					zoneID = z.ZoneID
				}
			}
		}
		if zoneID == "" {
			sp, err := store.BookingSpotsByIDs(ctx, s.d.DB, es, []string{spotID})
			if err != nil || len(sp) == 0 {
				return nil, ErrNoAvailability
			}
			zoneID = sp[0].ZoneID
		}
		return [][]store.HoldItem{append([]store.HoldItem{{SpotID: spotID}}, components(zoneID)...)}, nil
	}
	isFreed := map[string]bool{}
	for _, id := range freed {
		isFreed[id] = true
	}
	zones := slices.Clone(res.Zones)
	freedIn := func(z inventory.ZoneAvailability) bool {
		for _, sp := range z.Spots {
			if isFreed[sp.SpotID] {
				return true
			}
		}
		return false
	}
	slices.SortStableFunc(zones, func(a, b inventory.ZoneAvailability) int {
		fa, fb := freedIn(a), freedIn(b)
		switch {
		case fa && !fb:
			return -1
		case fb && !fa:
			return 1
		}
		return 0
	})
	var sets [][]store.HoldItem
	for _, z := range zones {
		if len(sets) >= maxCandidates {
			break
		}
		switch z.Mode {
		case "seat":
			var avail []string
			for _, sp := range z.Spots {
				if sp.Available {
					avail = append(avail, sp.SpotID)
				}
			}
			slices.SortStableFunc(avail, func(a, b string) int {
				fa, fb := isFreed[a], isFreed[b]
				switch {
				case fa && !fb:
					return -1
				case fb && !fa:
					return 1
				}
				return 0
			})
			for k := 0; (k+1)*qty <= len(avail) && len(sets) < maxCandidates; k++ {
				var items []store.HoldItem
				for _, id := range avail[k*qty : (k+1)*qty] {
					items = append(items, store.HoldItem{SpotID: id})
				}
				sets = append(sets, append(items, components(z.ZoneID)...))
			}
		default: // zone (assigned later) and quantity zones: units of the pool
			if z.Available >= qty {
				sets = append(sets, append([]store.HoldItem{{ZoneID: z.ZoneID, Quantity: qty}}, components(z.ZoneID)...))
			}
		}
	}
	return sets, nil
}
