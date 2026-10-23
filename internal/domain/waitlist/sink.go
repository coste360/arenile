package waitlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Trigger: the service consumes the inventory release events after they
// leave the outbox (at-least-once). For the freed dates it scans the
// compatible waiting entries in queue order and offers what can be held;
// re-processing an event finds the entries already offered and does
// nothing more (idempotent). customer.anonymized removes the customer's
// entries (spec 23).

// candidatesPerEvent bounds the entries examined for one event.
const candidatesPerEvent = 50

// dueBatch bounds the offers expired by one job run.
const dueBatch = 200

// Name implements ext.EventSink.
func (s *Service) Name() string { return SinkName }

// releaseData is the subset of the release payloads (event.BookingChange,
// event.AllocationChange, event.CustomerChange) the sink reads.
type releaseData struct {
	Dates      []string `json:"dates"`
	StartsOn   string   `json:"starts_on"`
	EndsOn     string   `json:"ends_on"`
	HoldID     string   `json:"hold_id"`
	SpotIDs    []string `json:"spot_ids"`
	CustomerID string   `json:"customer_id"`
}

// causeOf maps a release event to the cause of the offers it triggers
// ("" = not a trigger).
func causeOf(eventType string) string {
	switch eventType {
	case event.BookingCancelled:
		return store.WaitlistCauseCancellation
	case event.BookingExpired, event.HoldReleased:
		return store.WaitlistCauseExpiry
	case event.SeasonDayReleased, event.AllocationReleased:
		return store.WaitlistCauseRelease
	}
	return ""
}

// Handle implements ext.EventSink: offers freed inventory to waiting
// entries and forgets anonymised customers.
func (s *Service) Handle(ctx context.Context, ev ext.Event) error {
	cause := causeOf(ev.Type)
	if cause == "" && ev.Type != event.CustomerAnonymized {
		return nil
	}
	var data releaseData
	if len(ev.Data) > 0 {
		if err := json.Unmarshal(ev.Data, &data); err != nil {
			return fmt.Errorf("waitlist: decode %s: %w", ev.Type, err)
		}
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, ev.EstablishmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("waitlist: %w", err)
	}
	if ev.Type == event.CustomerAnonymized {
		return s.forgetCustomer(ctx, es, data.CustomerID)
	}
	set, err := s.settings(ctx, s.d.DB, es)
	if err != nil {
		return err
	}
	if !set.Enabled || !set.AutoOffer {
		return nil
	}
	from, to, ok := freedRange(data)
	if !ok {
		return nil
	}
	exclude := ""
	if ev.Type == event.HoldReleased && data.HoldID != "" {
		// The hold of an offer that was declined, revoked or expired: the
		// inventory goes to the next entry, not back to the same one.
		if o, err := store.GetWaitlistOfferByHold(ctx, s.d.DB, es, data.HoldID); err == nil {
			exclude = o.EntryID
		}
	}
	_, err = s.match(ctx, es, from, to, cause, data.SpotIDs, exclude, time.Duration(set.OfferHoldMinutes)*time.Minute)
	return err
}

// freedRange is the date interval of a release payload.
func freedRange(d releaseData) (calendar.Date, calendar.Date, bool) {
	var from, to calendar.Date
	for _, v := range d.Dates {
		dt, err := calendar.Parse(v)
		if err != nil {
			continue
		}
		if from.IsZero() || dt.Before(from) {
			from = dt
		}
		if to.IsZero() || dt.After(to) {
			to = dt
		}
	}
	if from.IsZero() {
		f, ferr := calendar.Parse(d.StartsOn)
		t, terr := calendar.Parse(d.EndsOn)
		if ferr != nil || terr != nil {
			return from, to, false
		}
		from, to = f, t
	}
	return from, to, true
}

// match offers the inventory free on from..to to the compatible waiting
// entries in queue order and returns how many offers it created.
func (s *Service) match(ctx context.Context, es store.EstablishmentScope, from, to calendar.Date, cause string, freed []string, exclude string, ttl time.Duration) (int, error) {
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return 0, err
	}
	list, err := store.WaitlistCandidates(ctx, s.d.DB, es, from, to, today, candidatesPerEvent)
	if err != nil {
		return 0, fmt.Errorf("waitlist: candidates: %w", err)
	}
	offered := 0
	var errs []error
	for i := range list {
		e := &list[i]
		if e.ID == exclude {
			continue
		}
		_, err := s.offerEntry(ctx, es, e, "", ttl, cause, freed, audit.SystemActor)
		switch {
		case err == nil:
			offered++
		case errors.Is(err, ErrNoAvailability), isConflict(err), errors.Is(err, configerr.ErrConflict),
			errors.Is(err, configerr.ErrValidation):
			// Nothing for this entry now (or it changed meanwhile).
		default:
			errs = append(errs, fmt.Errorf("entry %s: %w", e.ID, err))
		}
		if err := ctx.Err(); err != nil {
			return offered, err
		}
	}
	if offered > 0 {
		s.d.Log.InfoContext(ctx, "waitlist offers created", slog.String("establishment_id", es.ID()), slog.Int("offers", offered), slog.String("cause", cause))
	}
	return offered, errors.Join(errs...)
}

// forgetCustomer removes the open entries of an anonymised customer
// (pending offers revoked, holds released) and wipes notes and link
// tokens of all their entries.
func (s *Service) forgetCustomer(ctx context.Context, es store.EstablishmentScope, customerID string) error {
	if customerID == "" {
		return nil
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		rows, err := store.CustomerWaitlistEntries(ctx, tx, es, customerID)
		if err != nil {
			return fmt.Errorf("waitlist: forget: %w", err)
		}
		for i := range rows {
			if err := s.remove(ctx, tx, es, &rows[i], audit.SystemActor, "anonymized"); err != nil {
				return err
			}
		}
		if err := store.WipeCustomerWaitlist(ctx, tx, es, customerID, s.now()); err != nil {
			return fmt.Errorf("waitlist: forget: %w", err)
		}
		return nil
	})
}

// ExpireDue expires pending offers past expires_at (hold released, entry
// back to waiting, the inventory offered to the next entry through
// hold.released) and the waiting entries whose period has ended. It
// returns how many offers and entries it closed.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	n := 0
	due, err := store.DueWaitlistOffers(ctx, s.d.DB, s.now(), dueBatch)
	if err != nil {
		return 0, fmt.Errorf("waitlist: expire: %w", err)
	}
	scopes := map[string]store.EstablishmentScope{}
	scope := func(id string) (store.EstablishmentScope, error) {
		if es, ok := scopes[id]; ok {
			return es, nil
		}
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, id)
		if err == nil {
			scopes[id] = es
		}
		return es, err
	}
	for _, d := range due {
		es, err := scope(d.EstablishmentID)
		if err != nil {
			return n, fmt.Errorf("waitlist: expire: %w", err)
		}
		today, err := s.today(ctx, s.d.DB, es)
		if err != nil {
			return n, err
		}
		err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			o, err := store.GetWaitlistOffer(ctx, tx, es, d.ID)
			if err != nil {
				return fmt.Errorf("waitlist: expire: %w", err)
			}
			if o.Status != store.WaitlistOfferPending {
				return nil
			}
			if err := s.closeOffer(ctx, tx, es, o, store.WaitlistOfferExpired, audit.SystemActor); err != nil {
				return err
			}
			n++
			return s.reopenEntry(ctx, tx, es, o.EntryID, today)
		})
		if err != nil && !errors.Is(err, ErrOfferExpired) {
			return n, err
		}
	}
	ids, err := store.EstablishmentsWithWaitlist(ctx, s.d.DB)
	if err != nil {
		return n, fmt.Errorf("waitlist: expire: %w", err)
	}
	for _, id := range ids {
		es, err := scope(id)
		if err != nil {
			return n, fmt.Errorf("waitlist: expire: %w", err)
		}
		today, err := s.today(ctx, s.d.DB, es)
		if err != nil {
			return n, err
		}
		ended, err := store.ExpireEndedWaitlistEntries(ctx, s.d.DB, es, today, s.now())
		if err != nil {
			return n, fmt.Errorf("waitlist: expire: %w", err)
		}
		n += len(ended)
	}
	return n, nil
}

// Jobs returns the waitlist jobs (registered by internal/app).
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobExpire, Interval: time.Minute, RunOnStart: true, Timeout: 2 * time.Minute, Run: func(ctx context.Context) error {
		_, err := s.ExpireDue(ctx)
		return err
	}}}
}
