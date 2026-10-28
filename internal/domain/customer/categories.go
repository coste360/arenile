package customer

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Customer categories and M8 personal data (spec 30.2.2, 30.3, 23). The
// categories themselves are managed by internal/domain/partner; here the
// membership of a customer is written in the customer's transaction.

// setCategories replaces the categories of c when ids is not nil and fills
// c.CategoryIDs; it reports whether the set changed. Unknown ids are a
// validation error on category_ids.
func (s *Service) setCategories(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.Customer, ids *[]string) (bool, error) {
	if ids == nil {
		return false, s.fillCategories(ctx, tx, es, c)
	}
	var clean []string
	seen := map[string]bool{}
	for _, id := range *ids {
		id = strings.TrimSpace(id)
		if _, err := uuid.Parse(id); id != "" && err != nil {
			return false, configerr.Invalid("category_ids", "Categoria inesistente.")
		}
		if id != "" && !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	n, err := store.CountCustomerCategories(ctx, tx, es, clean)
	if err != nil {
		return false, fmt.Errorf("customer: categories: %w", err)
	}
	if n != len(clean) {
		return false, configerr.Invalid("category_ids", "Categoria inesistente.")
	}
	changed, err := store.SetCustomerCategories(ctx, tx, es, c.ID, clean, s.now())
	if err != nil {
		return false, fmt.Errorf("customer: categories: %w", err)
	}
	return changed, s.fillCategories(ctx, tx, es, c)
}

// fillCategories loads the category ids of c.
func (s *Service) fillCategories(ctx context.Context, db bun.IDB, es store.EstablishmentScope, cs ...*store.Customer) error {
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	m, err := store.CustomerCategoryIDs(ctx, db, es, ids)
	if err != nil {
		return fmt.Errorf("customer: categories: %w", err)
	}
	for _, c := range cs {
		c.CategoryIDs = m[c.ID]
		if c.CategoryIDs == nil {
			c.CategoryIDs = []string{}
		}
	}
	return nil
}

func (s *Service) fillPage(ctx context.Context, db bun.IDB, es store.EstablishmentScope, items []store.Customer) error {
	ptrs := make([]*store.Customer, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}
	return s.fillCategories(ctx, db, es, ptrs...)
}
