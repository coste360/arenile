package customer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// AnonymizedLastName replaces the last name of an anonymized customer.
const AnonymizedLastName = "Anonimizzato"

// RecentBookingsLimit is the number of bookings shown on the customer page.
const RecentBookingsLimit = 10

// ListFilter selects customers in List.
type ListFilter struct {
	// Q is a free text: name, email, phone (any format) or tag.
	Q                 string
	Tag               string
	IncludeAnonymized bool
}

// Detail is the customer page (api.CustomerDetail).
type Detail struct {
	Customer       store.Customer
	CreditBalance  money.Cents
	Credits        []store.CustomerCredit
	RecentBookings []store.Booking
	// SpotLabels maps the ids of RecentBookings to their spot labels.
	SpotLabels   map[string][]string
	SeasonPasses []store.SeasonPassView
	Stats        store.CustomerBookingStats
}

// Export is the GDPR export of a customer (api.CustomerExport): every
// record that references the customer.
type Export struct {
	Customer   store.Customer
	Bookings   []store.Booking
	SpotLabels map[string][]string
	Payments   []store.Payment
	// BookingCodes maps the booking ids of Payments to their codes.
	BookingCodes map[string]string
	Credits      []store.CustomerCredit
	ExportedAt   time.Time
}

// List returns a page of the customers of an establishment.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f ListFilter, cursor string, limit int) (store.Page[store.Customer], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.Customer]{}, err
	}
	page, err := store.ListCustomers(ctx, s.d.DB, es, store.CustomerFilter{
		Terms: SearchTerms(f.Q), Tag: NormalizeTag(f.Tag), IncludeAnonymized: f.IncludeAnonymized,
	}, cursor, limit)
	if err != nil && !errors.Is(err, store.ErrInvalidCursor) {
		err = fmt.Errorf("customer: list: %w", err)
	}
	return page, err
}

// Get returns the customer page: data, credit balance and ledger, recent
// bookings, season passes and booking statistics.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	db := s.d.DB
	c, err := store.GetCustomer(ctx, db, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("customer: get: %w", err)
	}
	d := &Detail{Customer: *c}
	if d.Credits, err = store.ListCustomerCredits(ctx, db, es, id); err != nil {
		return nil, fmt.Errorf("customer: get: %w", err)
	}
	if d.CreditBalance, err = s.CreditBalance(ctx, db, es, id, s.today(est)); err != nil {
		return nil, err
	}
	if d.RecentBookings, err = store.CustomerBookings(ctx, db, es, id, RecentBookingsLimit); err != nil {
		return nil, fmt.Errorf("customer: get: %w", err)
	}
	if d.SpotLabels, err = store.CustomerBookingSpotLabels(ctx, db, es, bookingIDs(d.RecentBookings)); err != nil {
		return nil, fmt.Errorf("customer: get: %w", err)
	}
	if d.SeasonPasses, err = store.SeasonPassesOfCustomer(ctx, db, es, id); err != nil {
		return nil, fmt.Errorf("customer: get: %w", err)
	}
	if d.Stats, err = store.GetCustomerBookingStats(ctx, db, es, id); err != nil {
		return nil, fmt.Errorf("customer: get: %w", err)
	}
	return d, nil
}

// Create creates a customer. Unless force is set, a non-anonymized customer
// with the same email or phone makes it fail with *DuplicateError.
func (s *Service) Create(ctx context.Context, p *auth.Principal, ip, establishmentID string, in Input, force bool) (*store.Customer, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *store.Customer
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		est, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("customer: create: %w", err)
		}
		now := s.now()
		f, err := validate(&in, est.DefaultLocale, now)
		if err != nil {
			return err
		}
		if err := s.checkAgreement(ctx, tx, es, f.AgreementID); err != nil {
			return err
		}
		if !force {
			if err := s.checkDuplicates(ctx, tx, es, &f, ""); err != nil {
				return err
			}
		}
		out, err = s.insert(ctx, tx, p, ip, es, &f, now)
		return err
	})
	return out, err
}

// Update replaces the data of a customer (PUT semantics: an empty field
// clears it, a nil consent withdraws it). ifMatch nil skips the version
// check. An anonymized customer cannot be updated.
func (s *Service) Update(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in Input) (*store.Customer, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *store.Customer
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetCustomer(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if cur.AnonymizedAt != nil {
			return ErrAnonymized
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		est, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("customer: update: %w", err)
		}
		now := s.now()
		f, err := validate(&in, est.DefaultLocale, now)
		if err != nil {
			return err
		}
		if err := s.checkAgreement(ctx, tx, es, f.AgreementID); err != nil {
			return err
		}
		// An unchanged consent keeps its original timestamp.
		if f.MarketingConsent != nil && cur.MarketingConsent != nil && in.MarketingConsent != nil &&
			in.MarketingConsent.GivenAt.IsZero() && f.MarketingConsent.Source == cur.MarketingConsent.Source &&
			f.MarketingConsent.TextVersion == cur.MarketingConsent.TextVersion {
			f.MarketingConsent.GivenAt = cur.MarketingConsent.GivenAt
		}
		next := *cur
		apply(&next, &f)
		changed := changedFields(cur, &next)
		if len(changed) == 0 {
			out = cur
			return nil
		}
		next.Version = cur.Version + 1
		next.UpdatedAt = now
		ok, err := store.UpdateCustomer(ctx, tx, es, &next, cur.Version)
		if err != nil {
			return fmt.Errorf("customer: update: %w", err)
		}
		if !ok {
			return s.lostRace(ctx, tx, es, id)
		}
		out = &next
		return s.record(ctx, tx, p, ip, es, "customer.updated", event.CustomerUpdated, &next, map[string]any{"fields": changed})
	})
	return out, err
}

// Export returns every record linked to a customer (GDPR, spec 23) and
// records the request in the audit log. Requires bookings.write.
func (s *Service) Export(ctx context.Context, p *auth.Principal, ip, establishmentID, id string) (*Export, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *Export
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		c, err := store.GetCustomer(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		x := &Export{Customer: *c, ExportedAt: s.now()}
		if x.Bookings, err = store.CustomerBookings(ctx, tx, es, id, 0); err != nil {
			return fmt.Errorf("customer: export: %w", err)
		}
		if x.SpotLabels, err = store.CustomerBookingSpotLabels(ctx, tx, es, bookingIDs(x.Bookings)); err != nil {
			return fmt.Errorf("customer: export: %w", err)
		}
		if x.Payments, err = store.CustomerPayments(ctx, tx, es, id); err != nil {
			return fmt.Errorf("customer: export: %w", err)
		}
		var ids []string
		for i := range x.Payments {
			if b := x.Payments[i].BookingID; b != nil {
				ids = append(ids, *b)
			}
		}
		if x.BookingCodes, err = store.CustomerBookingCodes(ctx, tx, es, ids); err != nil {
			return fmt.Errorf("customer: export: %w", err)
		}
		if x.Credits, err = store.ListCustomerCredits(ctx, tx, es, id); err != nil {
			return fmt.Errorf("customer: export: %w", err)
		}
		out = x
		return s.record(ctx, tx, p, ip, es, "customer.exported", "", c, map[string]any{
			"bookings": len(x.Bookings), "payments": len(x.Payments), "credits": len(x.Credits),
		})
	})
	return out, err
}

// Anonymize irreversibly replaces the personal data of a customer (GDPR,
// spec 23): names, email, phone, notes, tags and marketing consent, and the
// free-text notes of its bookings. Bookings, lines, payments and credits are
// kept for accounting. Emits customer.anonymized. Anonymizing twice is a
// conflict.
func (s *Service) Anonymize(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) (*store.Customer, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *store.Customer
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetCustomer(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if cur.AnonymizedAt != nil {
			return ErrAnonymized
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		now := s.now()
		next := *cur
		next.FirstName, next.LastName = "", AnonymizedLastName
		next.Email, next.Phone = nil, nil
		next.Notes, next.Tags, next.MarketingConsent = "", []string{}, nil
		next.SearchText = ""
		next.AnonymizedAt = &now
		next.Version = cur.Version + 1
		next.UpdatedAt = now
		ok, err := store.UpdateCustomer(ctx, tx, es, &next, cur.Version)
		if err != nil {
			return fmt.Errorf("customer: anonymize: %w", err)
		}
		if !ok {
			return s.lostRace(ctx, tx, es, id)
		}
		wiped, err := store.CustomerWipeFreeText(ctx, tx, es, id, now)
		if err != nil {
			return fmt.Errorf("customer: anonymize: %w", err)
		}
		out = &next
		return s.record(ctx, tx, p, ip, es, "customer.anonymized", event.CustomerAnonymized, &next, map[string]any{
			"fields": changedFields(cur, &next), "bookings_notes_wiped": wiped.Bookings,
			"payment_notes_wiped": wiped.Payments, "credit_notes_wiped": wiped.Credits,
		})
	})
	return out, err
}

// ---- internals ---------------------------------------------------------------

// insert creates the customer row of f with audit and customer.created.
func (s *Service) insert(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, f *fields, now time.Time) (*store.Customer, error) {
	c := &store.Customer{ID: store.NewID(), EstablishmentID: es.ID(), Version: 1, CreatedAt: now, UpdatedAt: now}
	apply(c, f)
	if err := store.InsertCustomer(ctx, tx, es, c); err != nil {
		return nil, fmt.Errorf("customer: create: %w", err)
	}
	set := changedFields(&store.Customer{}, c)
	if err := s.record(ctx, tx, p, ip, es, "customer.created", event.CustomerCreated, c, map[string]any{"fields": set}); err != nil {
		return nil, err
	}
	return c, nil
}

// checkDuplicates returns *DuplicateError for the oldest customer sharing
// the email (checked first) or the phone of f.
func (s *Service) checkDuplicates(ctx context.Context, db bun.IDB, es store.EstablishmentScope, f *fields, excludeID string) error {
	dups, err := store.FindCustomerDuplicates(ctx, db, es, deref(f.Email), deref(f.Phone), excludeID)
	if err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	for _, field := range []string{"email", "phone"} {
		for i := range dups {
			if field == "email" && f.Email != nil && deref(dups[i].Email) == *f.Email {
				return &DuplicateError{ExistingID: dups[i].ID, Field: field}
			}
			if field == "phone" && f.Phone != nil && deref(dups[i].Phone) == *f.Phone {
				return &DuplicateError{ExistingID: dups[i].ID, Field: field}
			}
		}
	}
	return nil
}

// lostRace builds the version conflict of an optimistic update that
// matched no row.
func (s *Service) lostRace(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) error {
	c, err := store.GetCustomer(ctx, db, es, id)
	if err != nil {
		return notFound(err)
	}
	return &configerr.VersionConflictError{Current: c.Version}
}

// apply copies validated fields onto c and recomputes search_text.
func apply(c *store.Customer, f *fields) {
	c.FirstName, c.LastName = f.FirstName, f.LastName
	c.Email, c.Phone = f.Email, f.Phone
	c.Locale, c.Notes = f.Locale, f.Notes
	c.Tags = append([]string{}, f.Tags...)
	c.AgreementID = f.AgreementID
	c.MarketingConsent = f.MarketingConsent
	c.SearchText = searchText(f)
}

func bookingIDs(bs []store.Booking) []string {
	out := make([]string, len(bs))
	for i := range bs {
		out[i] = bs[i].ID
	}
	return out
}
