package waitlist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Customer side of the waitlist: the public join (no enumeration: the
// handler always answers 202) and the personal link (256-bit token, only
// its SHA-256 stored). The link is rotated by every email that carries it
// (joined, offer): the latest email is the valid one.

// ErrTermsOutdated: the accepted terms are not the current ones (409).
var ErrTermsOutdated = fmt.Errorf("%w: le condizioni sono state aggiornate, rileggile e accettale di nuovo", configerr.ErrConflict)

const maxName = 100

// Join registers a public request (ErrDisabled when public join is off).
func (s *Service) Join(ctx context.Context, slug string, in PublicJoinInput) error {
	es, err := s.public(ctx, slug)
	if err != nil {
		return err
	}
	set, err := s.settings(ctx, s.d.DB, es)
	if err != nil {
		return err
	}
	if !set.Enabled || !set.PublicJoin {
		return ErrDisabled
	}
	if err := validateJoin(&in); err != nil {
		return err
	}
	if s.d.Terms != nil {
		cur, err := s.d.Terms.CurrentTerms(ctx, s.d.DB, es)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("waitlist: terms: %w", err)
		}
		if cur != nil && strings.TrimSpace(in.AcceptTermsVersion) != cur.Version {
			return ErrTermsOutdated
		}
	}
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return err
	}
	req := Input{
		StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, PackID: in.PackID, ZoneID: in.ZoneID,
		Quantity: in.Quantity, Channel: store.ChannelOnline, IP: in.IP,
	}
	if err := s.validate(ctx, s.d.DB, es, &req, today); err != nil {
		return err
	}
	if err := s.sellsOnline(ctx, es, req.PackID, req.ZoneID); err != nil {
		return err
	}
	// One open request per address, pack and period: a repeated join (or a
	// flood aimed at an address) writes nothing and sends nothing more; the
	// caller still gets 202 (no enumeration).
	if dup, err := store.HasOpenWaitlistEntryForEmail(ctx, s.d.DB, es, in.Email, req.PackID, req.StartsOn, req.EndsOn); err != nil {
		return fmt.Errorf("waitlist: %w", err)
	} else if dup {
		s.d.Log.InfoContext(ctx, "waitlist: public join ignored, open request exists", slog.String("establishment_id", es.ID()))
		return nil
	}
	actor := audit.Actor{Type: audit.ActorCustomer, IP: in.IP}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		c, err := s.d.Customers.ResolveOnline(ctx, tx, actor, es, "", "", customer.Input{
			FirstName: in.FirstName, LastName: in.LastName, Email: in.Email, Phone: in.Phone, Locale: in.Locale,
		})
		if err != nil {
			return err
		}
		req.CustomerID = c.ID
		token, hash, err := checkout.NewToken()
		if err != nil {
			return err
		}
		row, err := s.insertEntry(ctx, tx, es, req, firstNonEmpty(in.Locale, c.Locale), &hash, audit.Actor{Type: audit.ActorCustomer, ID: c.ID, IP: in.IP})
		if err != nil {
			return err
		}
		return s.sendLink(ctx, tx, es, row, notify.TemplateWaitlistJoined, "waitlist_joined:"+row.ID, token, time.Time{})
	})
}

func validateJoin(in *PublicJoinInput) error {
	var ve configerr.ValidationError
	in.FirstName, in.LastName = strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName)
	in.Email = customer.NormalizeEmail(in.Email)
	if in.FirstName == "" || utf8.RuneCountInString(in.FirstName) > maxName {
		ve.Add("customer.first_name", "Campo obbligatorio (massimo 100 caratteri).")
	}
	if in.LastName == "" || utf8.RuneCountInString(in.LastName) > maxName {
		ve.Add("customer.last_name", "Campo obbligatorio (massimo 100 caratteri).")
	}
	if !customer.ValidEmail(in.Email) {
		ve.Add("customer.email", "Indirizzo email non valido.")
	}
	if len(in.Phone) > 32 {
		ve.Add("customer.phone", "Telefono non valido.")
	}
	return ve.OrNil()
}

// sellsOnline checks that the pack (and the zone) are sold on the public
// page.
func (s *Service) sellsOnline(ctx context.Context, es store.EstablishmentScope, packID, zoneID string) error {
	pk, err := store.GetPackForInventory(ctx, s.d.DB, es, packID)
	if err != nil {
		return configerr.Invalid("pack_id", "Pack inesistente o non attivo.")
	}
	online := false
	for _, ch := range pk.Channels {
		if ch == store.ChannelOnline {
			online = true
		}
	}
	if !online {
		return configerr.Invalid("pack_id", "Pack non prenotabile online.")
	}
	if zoneID != "" {
		zs, err := store.InventoryZones(ctx, s.d.DB, es, []string{zoneID})
		if err != nil || len(zs) == 0 || !zs[0].OnlineVisible {
			return configerr.Invalid("zone_id", "Zona non prenotabile online.")
		}
	}
	return nil
}

// GetByToken returns the entry of a waitlist link.
func (s *Service) GetByToken(ctx context.Context, slug, token string) (*PublicEntry, error) {
	es, row, err := s.byToken(ctx, s.d.DB, slug, token)
	if err != nil {
		return nil, err
	}
	return s.publicEntry(ctx, s.d.DB, es, row)
}

// LeaveByToken removes the entry of a waitlist link.
func (s *Service) LeaveByToken(ctx context.Context, slug, token string) error {
	es, err := s.public(ctx, slug)
	if err != nil {
		return ErrNotFound
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := s.entryByToken(ctx, tx, es, token)
		if err != nil {
			return err
		}
		return s.remove(ctx, tx, es, row, s.customerActor(ctx, row), "customer")
	})
}

// AcceptByToken converts the pending offer into a checkout session: the
// offer hold becomes a held booking of the entry's customer and the
// customer continues in the M4 checkout (confirmation only from the payment
// webhook or the manual driver).
func (s *Service) AcceptByToken(ctx context.Context, slug, token, _ string) (*checkout.Session, error) {
	es, row, err := s.byToken(ctx, s.d.DB, slug, token)
	if err != nil {
		return nil, err
	}
	if row.Status != store.WaitlistOffered {
		return nil, ErrNoOffer
	}
	o, err := store.PendingWaitlistOffer(ctx, s.d.DB, es, row.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNoOffer
	}
	if err != nil {
		return nil, fmt.Errorf("waitlist: %w", err)
	}
	if !o.ExpiresAt.After(s.now()) {
		return nil, ErrOfferExpired
	}
	if s.d.Checkout == nil {
		return nil, configerr.ErrNotImplemented
	}
	actor := s.customerActor(ctx, row)
	sess, err := s.d.Checkout.CreateFromHold(ctx, es, checkout.HoldSession{
		HoldID: o.HoldID, CustomerID: row.CustomerID, PackID: row.PackID, Locale: row.Locale, ActorID: row.ID,
		Within: func(ctx context.Context, tx bun.Tx, d *booking.Detail) error {
			bid := d.Booking.ID
			ok, err := store.CloseWaitlistOffer(ctx, tx, es, o.ID, store.WaitlistOfferAccepted, &bid, s.now())
			if err != nil {
				return fmt.Errorf("waitlist: accept: %w", err)
			}
			if !ok {
				return ErrOfferExpired
			}
			cur, err := s.entry(ctx, tx, es, row.ID)
			if err != nil {
				return err
			}
			next := *cur
			next.Status, next.ConvertedBookingID = store.WaitlistConverted, &bid
			next.Version, next.UpdatedAt = cur.Version+1, s.now()
			if ok, err := store.UpdateWaitlistEntry(ctx, tx, es, &next, cur.Version); err != nil || !ok {
				if err == nil {
					err = ErrOfferExpired
				}
				return err
			}
			ch := entryChange(cur)
			ch.OfferID, ch.HoldID, ch.BookingID = o.ID, o.HoldID, bid
			return s.record(ctx, tx, es, actor, ActionConverted, row.ID, map[string]any{"offer_id": o.ID, "booking_id": bid},
				event.WaitlistConverted, ch)
		},
	})
	if err != nil {
		if errors.Is(err, inventory.ErrHoldClosed) || errors.Is(err, availability.ErrHoldExpired) {
			return nil, ErrOfferExpired
		}
		return nil, err
	}
	return sess, nil
}

// DeclineByToken declines the pending offer (leave = also remove the entry).
func (s *Service) DeclineByToken(ctx context.Context, slug, token string, leave bool) (*PublicEntry, error) {
	es, err := s.public(ctx, slug)
	if err != nil {
		return nil, ErrNotFound
	}
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	var out *PublicEntry
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := s.entryByToken(ctx, tx, es, token)
		if err != nil {
			return err
		}
		o, err := store.PendingWaitlistOffer(ctx, tx, es, row.ID)
		if errors.Is(err, store.ErrNotFound) {
			return ErrNoOffer
		}
		if err != nil {
			return fmt.Errorf("waitlist: %w", err)
		}
		actor := s.customerActor(ctx, row)
		if err := s.closeOffer(ctx, tx, es, o, store.WaitlistOfferDeclined, actor); err != nil {
			return err
		}
		if leave {
			if err := s.remove(ctx, tx, es, row, actor, "customer"); err != nil {
				return err
			}
		} else if err := s.reopenEntry(ctx, tx, es, row.ID, today); err != nil {
			return err
		}
		cur, err := s.entry(ctx, tx, es, row.ID)
		if err != nil {
			return err
		}
		out, err = s.publicEntry(ctx, tx, es, cur)
		return err
	})
	return out, err
}

// ---- helpers ----------------------------------------------------------------------

// public resolves the establishment of a public slug (no dependency on
// online booking being switched on: links keep working).
func (s *Service) public(ctx context.Context, slug string) (store.EstablishmentScope, error) {
	est, err := store.GetEstablishmentBySlug(ctx, s.d.DB, strings.ToLower(strings.TrimSpace(slug)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.EstablishmentScope{}, ErrNotFound
		}
		return store.EstablishmentScope{}, fmt.Errorf("waitlist: %w", err)
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, est.ID)
	if err != nil {
		return store.EstablishmentScope{}, ErrNotFound
	}
	return es, nil
}

func (s *Service) byToken(ctx context.Context, db bun.IDB, slug, token string) (store.EstablishmentScope, *store.WaitlistEntry, error) {
	es, err := s.public(ctx, slug)
	if err != nil {
		return es, nil, ErrNotFound
	}
	row, err := s.entryByToken(ctx, db, es, token)
	return es, row, err
}

func (s *Service) entryByToken(ctx context.Context, db bun.IDB, es store.EstablishmentScope, token string) (*store.WaitlistEntry, error) {
	if !checkout.ValidToken(token) {
		return nil, ErrNotFound
	}
	row, err := store.GetWaitlistEntryByToken(ctx, db, es, checkout.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("waitlist: %w", err)
	}
	return row, nil
}

// publicEntry builds the customer view of an entry (no personal data).
func (s *Service) publicEntry(ctx context.Context, db bun.IDB, es store.EstablishmentScope, row *store.WaitlistEntry) (*PublicEntry, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("waitlist: establishment: %w", err)
	}
	out := &PublicEntry{Entry: *row}
	if pk, err := store.GetPackForInventory(ctx, db, es, row.PackID); err == nil {
		out.PackName = i18n(pk.Name, row.Locale, est.DefaultLocale)
	}
	if row.ZoneID != nil {
		if zs, err := store.InventoryZones(ctx, db, es, []string{*row.ZoneID}); err == nil && len(zs) == 1 {
			out.ZoneName = i18n(zs[0].Name, row.Locale, est.DefaultLocale)
		}
	}
	if row.Status == store.WaitlistOffered {
		o, err := store.PendingWaitlistOffer(ctx, db, es, row.ID)
		if err == nil {
			if !o.ExpiresAt.After(s.now()) {
				o.Status = store.WaitlistOfferExpired
			}
			labels, err := s.spotLabels(ctx, db, es, []store.WaitlistOffer{*o})
			if err != nil {
				return nil, err
			}
			out.Offer = &Offer{WaitlistOffer: *o, SpotLabel: labelOf(labels, o.SpotID)}
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("waitlist: %w", err)
		}
	}
	return out, nil
}

func (s *Service) customerActor(ctx context.Context, row *store.WaitlistEntry) audit.Actor {
	return audit.Actor{Type: audit.ActorCustomer, ID: row.CustomerID, IP: reqctx.From(ctx).ClientIP}
}

func i18n(t store.I18nText, locale, fallback string) string {
	for _, l := range []string{locale, fallback, "it"} {
		if v := strings.TrimSpace(t[l]); v != "" {
			return v
		}
	}
	for _, v := range t {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
