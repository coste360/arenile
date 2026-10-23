package customer

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Customers of the public channel (M4, ADR 0013 §3). Ownership: the
// "checkout" builder.
//
// A held online booking needs a customer before the visitor types their
// details (bookings.customer_id is NOT NULL): it points to the
// establishment's placeholder customer until step 4 re-points it to the
// customer created for that checkout (ResolveOnline). The placeholder:
//
//   - has a deterministic id per establishment (UUIDv5 of the
//     establishment id), so concurrent first uses insert the same row and
//     converge (INSERT ... ON CONFLICT (id) DO NOTHING, no unique tag index
//     needed);
//   - is born anonymized (anonymized_at set, no email or phone): the
//     existing filters of lists, searches, duplicate hints, exports and
//     retention skip anonymized rows, so it never shows up anywhere and
//     never receives messages;
//   - carries the tag TagOnlinePlaceholder for diagnostics.

// TagOnlinePlaceholder marks the per-establishment placeholder customer of
// held online bookings (see above).
const TagOnlinePlaceholder = "_online_placeholder"

// ConsentSourceOnline is the source of a marketing consent given on the
// public page (spec 23).
const ConsentSourceOnline = "online"

// placeholderNamespace derives the placeholder ids (UUIDv5).
var placeholderNamespace = uuid.MustParse("6f1c2a52-6d3e-4d0e-9a51-2f6b2c0e7a10")

// PlaceholderID is the id of the online placeholder customer of an establishment.
func PlaceholderID(establishmentID string) string {
	return uuid.NewSHA1(placeholderNamespace, []byte(establishmentID)).String()
}

// IsPlaceholder reports whether c is an online placeholder customer.
func IsPlaceholder(c *store.Customer) bool {
	return c != nil && c.ID == PlaceholderID(c.EstablishmentID)
}

// OnlinePlaceholder returns the placeholder customer of es, creating it in
// tx on first use (idempotent under concurrency: deterministic id and
// INSERT ... ON CONFLICT DO NOTHING).
func (s *Service) OnlinePlaceholder(ctx context.Context, tx bun.IDB, es store.EstablishmentScope) (*store.Customer, error) {
	id := PlaceholderID(es.ID())
	c, err := store.GetCustomer(ctx, tx, es, id)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("customer: placeholder: %w", err)
	}
	now := s.now()
	c = &store.Customer{
		ID: id, EstablishmentID: es.ID(), FirstName: "Prenotazione online", LastName: "in corso", Locale: "it",
		Tags: []string{TagOnlinePlaceholder}, SearchText: "|" + TagOnlinePlaceholder + "|",
		AnonymizedAt: &now, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.NewInsert().Model(c).On("CONFLICT (id) DO NOTHING").Exec(ctx); err != nil {
		return nil, fmt.Errorf("customer: placeholder: %w", err)
	}
	return store.GetCustomer(ctx, tx, es, id)
}

// ResolveOnline returns the customer of step 4 of the public flow in tx
// (spec 12.3, 23; ADR 0013 §3 as amended): the details typed by the
// visitor are stored on a customer that belongs to this checkout only.
// They are NEVER matched to an existing customer of es by email or phone:
// whoever knows an address could otherwise read that customer's stored
// name, email and phone back from the session (stateToAPI), book under
// their record (emails to the victim, booking in their history) and be
// priced with their tags (residents, agreements). Duplicates are a staff
// concern (the panel shows the duplicate hints on the customer card).
//
// currentID is the customer of the booking before this step: the
// placeholder (first time) or the customer a previous PATCH of the same
// session created, which is updated in place (typo corrections) when no
// other booking points to it; a customer with other bookings (linked by
// the staff) is never rewritten from the public page: a new one is created.
// actor is the checkout session (audit).
func (s *Service) ResolveOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, currentID string, in Input) (*store.Customer, error) {
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("customer: resolve online: %w", err)
	}
	now := s.now()
	in.Tags, in.AgreementID, in.Notes = nil, "", ""
	f, err := validate(&in, est.DefaultLocale, now)
	if err != nil {
		return nil, err
	}
	if own, err := s.ownCustomer(ctx, tx, es, bookingID, currentID); err != nil {
		return nil, err
	} else if own != nil {
		next := *own
		apply(&next, &f)
		if f.MarketingConsent == nil && own.MarketingConsent != nil {
			// A consent given earlier in this session is not withdrawn by
			// leaving the box unticked on a later submit (spec 23: explicit).
			next.MarketingConsent = own.MarketingConsent
		}
		fields := changedFields(own, &next)
		if len(fields) == 0 {
			return own, nil
		}
		next.Version = own.Version + 1
		next.UpdatedAt = now
		ok, err := store.UpdateCustomer(ctx, tx, es, &next, own.Version)
		if err != nil {
			return nil, fmt.Errorf("customer: resolve online: %w", err)
		}
		if !ok {
			return nil, s.lostRace(ctx, tx, es, own.ID)
		}
		if err := s.recordAs(ctx, tx, actor, es, "customer.updated", event.CustomerUpdated, &next, map[string]any{
			"fields": fields, "source": ConsentSourceOnline,
		}); err != nil {
			return nil, err
		}
		return &next, nil
	}
	c := &store.Customer{ID: store.NewID(), EstablishmentID: es.ID(), Version: 1, CreatedAt: now, UpdatedAt: now}
	apply(c, &f)
	if err := store.InsertCustomer(ctx, tx, es, c); err != nil {
		return nil, fmt.Errorf("customer: create: %w", err)
	}
	if err := s.recordAs(ctx, tx, actor, es, "customer.created", event.CustomerCreated, c, map[string]any{
		"fields": changedFields(&store.Customer{}, c), "source": ConsentSourceOnline,
	}); err != nil {
		return nil, err
	}
	return c, nil
}

// ownCustomer returns the customer currentID when it was created by the
// checkout of bookingID (not the placeholder, not anonymized, no other
// booking of es points to it), nil otherwise.
func (s *Service) ownCustomer(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID, currentID string) (*store.Customer, error) {
	if currentID == "" || currentID == PlaceholderID(es.ID()) {
		return nil, nil
	}
	c, err := store.GetCustomer(ctx, tx, es, currentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("customer: resolve online: %w", err)
	}
	if c.AnonymizedAt != nil {
		return nil, nil
	}
	n, err := store.CountCustomerBookings(ctx, tx, es, c.ID, bookingID)
	if err != nil {
		return nil, fmt.Errorf("customer: resolve online: %w", err)
	}
	if n > 0 {
		return nil, nil
	}
	return c, nil
}

// recordAs is record with an explicit actor (public channel, no principal).
func (s *Service) recordAs(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, action, eventType string, c *store.Customer, diff any) error {
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "customer", EntityID: c.ID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, event.CustomerChange{CustomerID: c.ID, Version: c.Version}); err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	return nil
}
