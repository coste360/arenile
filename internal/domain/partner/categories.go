package partner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// categoryCodeRe is the format of a category code ("residenti", "over65").
var categoryCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// RegisterCategories are the categories of the agreements register
// (spec 30.2.5), in the order of the printed register.
var RegisterCategories = []string{
	store.RegisterResidents, store.RegisterAccommodation, store.RegisterProtected,
	store.RegisterCompanies, store.RegisterAssociations, store.RegisterOther,
}

// ValidRegisterCategory reports whether c is a register category.
func ValidRegisterCategory(c string) bool {
	for _, x := range RegisterCategories {
		if x == c {
			return true
		}
	}
	return false
}

// CategoryInput is api.CustomerCategoryInput.
type CategoryInput struct {
	Code             string
	Name             string
	Description      string
	Requirement      string
	RegisterCategory string // default other
	Active           bool
}

// CategoryView is a category with its member count (api.CustomerCategory).
type CategoryView struct {
	store.CustomerCategory
	MemberCount int
}

func validateCategory(in *CategoryInput) error {
	v := &configerr.ValidationError{}
	in.Code = strings.ToLower(strings.TrimSpace(in.Code))
	if !categoryCodeRe.MatchString(in.Code) {
		v.Add("code", "Da 1 a 40 caratteri tra lettere minuscole, cifre, trattino e trattino basso.")
	}
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		v.Add("name", "Campo obbligatorio.")
	case len([]rune(in.Name)) > maxNameLen:
		v.Add("name", fmt.Sprintf("Massimo %d caratteri.", maxNameLen))
	}
	in.Description = strings.TrimSpace(in.Description)
	in.Requirement = strings.TrimSpace(in.Requirement)
	if len([]rune(in.Description)) > maxNotesLen {
		v.Add("description", fmt.Sprintf("Massimo %d caratteri.", maxNotesLen))
	}
	if len([]rune(in.Requirement)) > maxShortLen {
		v.Add("requirement", fmt.Sprintf("Massimo %d caratteri.", maxShortLen))
	}
	if in.RegisterCategory == "" {
		in.RegisterCategory = store.RegisterOther
	}
	if !ValidRegisterCategory(in.RegisterCategory) {
		v.Add("register_category", "Categoria del registro sconosciuta.")
	}
	return v.OrNil()
}

func applyCategory(row *store.CustomerCategory, in CategoryInput) {
	row.Code, row.Name, row.Description = in.Code, in.Name, in.Description
	row.Requirement, row.RegisterCategory, row.Active = in.Requirement, in.RegisterCategory, in.Active
}

// ListCategories lists the customer categories (bookings.read).
func (s *Service) ListCategories(ctx context.Context, p *auth.Principal, establishmentID string, active *bool) ([]CategoryView, error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListCustomerCategories(ctx, s.d.DB, es, active)
	if err != nil {
		return nil, fmt.Errorf("partner: %w", err)
	}
	return s.categoryViews(ctx, s.d.DB, es, rows)
}

// GetCategory returns one category (bookings.read).
func (s *Service) GetCategory(ctx context.Context, p *auth.Principal, establishmentID, id string) (*CategoryView, error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetCustomerCategory(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	v, err := s.categoryViews(ctx, s.d.DB, es, []store.CustomerCategory{*row})
	if err != nil {
		return nil, err
	}
	return &v[0], nil
}

func (s *Service) categoryViews(ctx context.Context, db bun.IDB, es store.EstablishmentScope, rows []store.CustomerCategory) ([]CategoryView, error) {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	counts, err := store.CustomerCategoryMemberCounts(ctx, db, es, ids)
	if err != nil {
		return nil, fmt.Errorf("partner: %w", err)
	}
	out := make([]CategoryView, len(rows))
	for i := range rows {
		out[i] = CategoryView{CustomerCategory: rows[i], MemberCount: counts[rows[i].ID]}
	}
	return out, nil
}

// CreateCategory creates a category (establishment.configure).
func (s *Service) CreateCategory(ctx context.Context, p *auth.Principal, establishmentID string, in CategoryInput) (*CategoryView, error) {
	var out *CategoryView
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		if err := validateCategory(&in); err != nil {
			return err
		}
		now := s.now()
		row := &store.CustomerCategory{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
		applyCategory(row, in)
		if err := store.InsertCustomerCategory(ctx, tx, es, row); err != nil {
			if isConflict(err) {
				return ErrDuplicateCategory
			}
			return fmt.Errorf("partner: create category: %w", err)
		}
		out = &CategoryView{CustomerCategory: *row}
		return s.record(ctx, tx, p, es, EntityCustomerCategory, row.ID, "created", row, true)
	})
	return out, err
}

// UpdateCategory replaces a category (establishment.configure).
func (s *Service) UpdateCategory(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in CategoryInput) (*CategoryView, error) {
	var out *CategoryView
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		cur, err := store.GetCustomerCategory(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		if err := validateCategory(&in); err != nil {
			return err
		}
		next := *cur
		applyCategory(&next, in)
		next.Version, next.UpdatedAt = cur.Version+1, s.now()
		ok, err := store.UpdateCustomerCategory(ctx, tx, es, &next, cur.Version)
		if err != nil {
			if isConflict(err) {
				return ErrDuplicateCategory
			}
			return fmt.Errorf("partner: update category: %w", err)
		}
		if !ok {
			return staleOrMissing(ctx, tx, func(ctx context.Context, db bun.IDB) (int64, error) {
				r, err := store.GetCustomerCategory(ctx, db, es, id)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			})
		}
		v, err := s.categoryViews(ctx, tx, es, []store.CustomerCategory{next})
		if err != nil {
			return err
		}
		out = &v[0]
		return s.record(ctx, tx, p, es, EntityCustomerCategory, id, "updated", map[string]any{"before": cur, "after": next}, true)
	})
	return out, err
}

// DeleteCategory deletes a category no agreement targets
// (establishment.configure); its members are removed with it.
func (s *Service) DeleteCategory(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		cur, err := store.GetCustomerCategory(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		used, err := store.CustomerCategoryInUse(ctx, tx, es, id)
		if err != nil {
			return fmt.Errorf("partner: delete category: %w", err)
		}
		if used {
			return ErrCategoryInUse
		}
		if err := store.DeleteCustomerCategory(ctx, tx, es, id); err != nil {
			return notFound(err)
		}
		return s.record(ctx, tx, p, es, EntityCustomerCategory, id, "deleted", cur, true)
	})
}
