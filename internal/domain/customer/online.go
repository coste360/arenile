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
// real customer (ResolveOnline). The placeholder:
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
// (spec 12.3, ADR 0013 §3): the existing customer of es with the same email
// (checked first) or phone, otherwise a new customer. An existing customer
// is never modified from the public page (no data overwrite by whoever
// knows an address), except that a marketing consent given now is added
// when the customer has none. actor is the checkout session (audit).
func (s *Service) ResolveOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, in Input) (*store.Customer, error) {
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
	dups, err := store.FindCustomerDuplicates(ctx, tx, es, deref(f.Email), deref(f.Phone), PlaceholderID(es.ID()))
	if err != nil {
		return nil, fmt.Errorf("customer: resolve online: %w", err)
	}
	var match *store.Customer
	for _, field := range []string{"email", "phone"} {
		for i := range dups {
			d := &dups[i]
			if match == nil && field == "email" && f.Email != nil && deref(d.Email) == *f.Email {
				match = d
			}
			if match == nil && field == "phone" && f.Phone != nil && deref(d.Phone) == *f.Phone {
				match = d
			}
		}
	}
	if match != nil {
		if f.MarketingConsent == nil || match.MarketingConsent != nil {
			return match, nil
		}
		next := *match
		next.MarketingConsent = f.MarketingConsent
		next.Version = match.Version + 1
		next.UpdatedAt = now
		ok, err := store.UpdateCustomer(ctx, tx, es, &next, match.Version)
		if err != nil {
			return nil, fmt.Errorf("customer: resolve online: %w", err)
		}
		if !ok {
			return nil, s.lostRace(ctx, tx, es, match.ID)
		}
		if err := s.recordAs(ctx, tx, actor, es, "customer.updated", event.CustomerUpdated, &next, map[string]any{
			"fields": []string{"marketing_consent"}, "source": ConsentSourceOnline,
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
