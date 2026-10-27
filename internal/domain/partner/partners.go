package partner

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Limits of the partner fields.
const (
	maxNameLen  = 200
	maxShortLen = 200
	maxNotesLen = 2000
)

var partnerKinds = map[string]bool{
	store.PartnerKindAccommodation: true, store.PartnerKindPublicBody: true, store.PartnerKindCompany: true,
	store.PartnerKindAssociation: true, store.PartnerKindOther: true,
}

var accommodationTypes = map[string]bool{
	"hotel": true, "bnb": true, "affittacamere": true, "campeggio": true, "casa_vacanze": true, "altro": true,
}

// Input is api.PartnerInput.
type Input struct {
	Name              string
	Kind              string
	AccommodationType *string
	Municipality      string
	Address           string
	ContactName       string
	Email             string
	Phone             string
	VATNumber         string
	TaxCode           string
	Notes             string
	Active            bool
}

// Filter selects partners in List.
type Filter struct {
	Q      string
	Kind   string
	Active *bool
}

// View is a partner with its counters (api.Partner).
type View struct {
	store.Partner
	AgreementCount int
	// Balance is set only for principals with reports.read.
	Balance *money.Cents
}

func validatePartner(in *Input) error {
	v := &configerr.ValidationError{}
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		v.Add("name", "Campo obbligatorio.")
	case len([]rune(in.Name)) > maxNameLen:
		v.Add("name", fmt.Sprintf("Massimo %d caratteri.", maxNameLen))
	}
	if !partnerKinds[in.Kind] {
		v.Add("kind", "Tipo di partner sconosciuto.")
	}
	if in.AccommodationType != nil {
		t := strings.TrimSpace(*in.AccommodationType)
		switch {
		case t == "":
			in.AccommodationType = nil
		case in.Kind != store.PartnerKindAccommodation:
			v.Add("accommodation_type", "Solo per le strutture ricettive.")
		case !accommodationTypes[t]:
			v.Add("accommodation_type", "Tipo di struttura sconosciuto.")
		default:
			in.AccommodationType = &t
		}
	}
	for _, f := range []struct {
		name string
		val  *string
	}{
		{"municipality", &in.Municipality},
		{"address", &in.Address},
		{"contact_name", &in.ContactName},
		{"vat_number", &in.VATNumber},
		{"tax_code", &in.TaxCode},
	} {
		*f.val = strings.TrimSpace(*f.val)
		if len([]rune(*f.val)) > maxShortLen {
			v.Add(f.name, fmt.Sprintf("Massimo %d caratteri.", maxShortLen))
		}
	}
	in.Notes = strings.TrimSpace(in.Notes)
	if len([]rune(in.Notes)) > maxNotesLen {
		v.Add("notes", fmt.Sprintf("Massimo %d caratteri.", maxNotesLen))
	}
	in.Email = customer.NormalizeEmail(in.Email)
	if in.Email != "" && !customer.ValidEmail(in.Email) {
		v.Add("email", "Indirizzo email non valido.")
	}
	if strings.TrimSpace(in.Phone) != "" {
		ph, ok := customer.NormalizePhone(in.Phone)
		if !ok {
			v.Add("phone", "Numero di telefono non valido.")
		}
		in.Phone = ph
	} else {
		in.Phone = ""
	}
	return v.OrNil()
}

func applyPartner(row *store.Partner, in Input) {
	row.Name, row.Kind, row.AccommodationType = in.Name, in.Kind, in.AccommodationType
	row.Municipality, row.Address, row.ContactName = in.Municipality, in.Address, in.ContactName
	row.VATNumber, row.TaxCode, row.Notes, row.Active = in.VATNumber, in.TaxCode, in.Notes, in.Active
	row.Email, row.Phone = optional(in.Email), optional(in.Phone)
	row.SearchText = strings.Join(nonEmpty(customer.Fold(in.Name), customer.Fold(in.Municipality), customer.Fold(in.ContactName)), " ")
}

// changedPartnerFields lists the names of the changed fields (audit diff:
// never the values, which may be personal data).
func changedPartnerFields(a, b *store.Partner) []string {
	var out []string
	add := func(name string, changed bool) {
		if changed {
			out = append(out, name)
		}
	}
	add("name", a.Name != b.Name)
	add("kind", a.Kind != b.Kind)
	add("accommodation_type", deref(a.AccommodationType) != deref(b.AccommodationType))
	add("municipality", a.Municipality != b.Municipality)
	add("address", a.Address != b.Address)
	add("contact_name", a.ContactName != b.ContactName)
	add("email", deref(a.Email) != deref(b.Email))
	add("phone", deref(a.Phone) != deref(b.Phone))
	add("vat_number", a.VATNumber != b.VATNumber)
	add("tax_code", a.TaxCode != b.TaxCode)
	add("notes", a.Notes != b.Notes)
	add("active", a.Active != b.Active)
	return out
}

// List returns a page of partners (bookings.read). Balances are filled for
// principals with reports.read.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f Filter, cursor string, limit int) ([]View, string, error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	page, err := store.ListPartners(ctx, s.d.DB, es, store.PartnerFilter{
		Terms: strings.Fields(customer.Fold(f.Q)), Kind: f.Kind, Active: f.Active,
	}, cursor, limit)
	if err != nil {
		return nil, "", fmt.Errorf("partner: list: %w", err)
	}
	views, err := s.views(ctx, s.d.DB, p, es, page.Items)
	return views, page.NextCursor, err
}

// Get returns one partner (bookings.read).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*View, error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetPartner(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	views, err := s.views(ctx, s.d.DB, p, es, []store.Partner{*row})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

func (s *Service) views(ctx context.Context, db bun.IDB, p *auth.Principal, es store.EstablishmentScope, rows []store.Partner) ([]View, error) {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	counts, err := store.PartnerAgreementCounts(ctx, db, es, ids)
	if err != nil {
		return nil, fmt.Errorf("partner: %w", err)
	}
	var balances map[string]store.PartnerBalance
	if p.Can(es, auth.PermReportsRead) {
		if balances, err = store.PartnerBalances(ctx, db, es, ids); err != nil {
			return nil, fmt.Errorf("partner: %w", err)
		}
	}
	out := make([]View, len(rows))
	for i := range rows {
		out[i] = View{Partner: rows[i], AgreementCount: counts[rows[i].ID]}
		if balances != nil {
			b := balances[rows[i].ID].Balance()
			out[i].Balance = &b
		}
	}
	return out, nil
}

// Create creates a partner (establishment.configure).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in Input) (*View, error) {
	var out *View
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		if err := validatePartner(&in); err != nil {
			return err
		}
		now := s.now()
		row := &store.Partner{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
		applyPartner(row, in)
		if err := store.InsertPartner(ctx, tx, es, row); err != nil {
			return fmt.Errorf("partner: create: %w", err)
		}
		out = &View{Partner: *row}
		return s.record(ctx, tx, p, es, EntityPartner, row.ID, "created",
			map[string]any{"fields": changedPartnerFields(&store.Partner{}, row)}, true)
	})
	return out, err
}

// Update replaces a partner (establishment.configure).
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in Input) (*View, error) {
	var out *View
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		cur, err := store.GetPartner(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		if err := validatePartner(&in); err != nil {
			return err
		}
		next := *cur
		applyPartner(&next, in)
		changed := changedPartnerFields(cur, &next)
		if len(changed) == 0 {
			views, err := s.views(ctx, tx, p, es, []store.Partner{*cur})
			if err == nil {
				out = &views[0]
			}
			return err
		}
		next.Version, next.UpdatedAt = cur.Version+1, s.now()
		ok, err := store.UpdatePartner(ctx, tx, es, &next, cur.Version)
		if err != nil {
			return fmt.Errorf("partner: update: %w", err)
		}
		if !ok {
			return staleOrMissing(ctx, tx, func(ctx context.Context, db bun.IDB) (int64, error) {
				r, err := store.GetPartner(ctx, db, es, id)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			})
		}
		views, err := s.views(ctx, tx, p, es, []store.Partner{next})
		if err != nil {
			return err
		}
		out = &views[0]
		return s.record(ctx, tx, p, es, EntityPartner, id, "updated", map[string]any{"fields": changed}, true)
	})
	return out, err
}

// Delete deletes a partner nobody references (establishment.configure);
// otherwise ErrPartnerInUse (deactivate it instead).
func (s *Service) Delete(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		cur, err := store.GetPartner(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		used, err := store.PartnerInUse(ctx, tx, es, id)
		if err != nil {
			return fmt.Errorf("partner: delete: %w", err)
		}
		if used {
			return ErrPartnerInUse
		}
		if err := store.DeletePartner(ctx, tx, es, id); err != nil {
			return notFound(err)
		}
		return s.record(ctx, tx, p, es, EntityPartner, id, "deleted", map[string]any{"name": cur.Name}, true)
	})
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nonEmpty(in ...string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
