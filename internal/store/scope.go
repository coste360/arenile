package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/uptrace/bun"
)

// Scope is the set of establishments an authenticated principal may access:
// every establishment of the listed organizations plus the listed
// establishments. The zero Scope grants nothing.
type Scope struct {
	organizationIDs  []string
	establishmentIDs []string
}

// NewScope builds a Scope from organization-wide grants and single
// establishment grants.
func NewScope(organizationIDs, establishmentIDs []string) Scope {
	return Scope{
		organizationIDs:  dedupe(organizationIDs),
		establishmentIDs: dedupe(establishmentIDs),
	}
}

// Empty reports whether the scope grants nothing.
func (s Scope) Empty() bool { return len(s.organizationIDs) == 0 && len(s.establishmentIDs) == 0 }

// OrganizationIDs returns the organization-wide grants.
func (s Scope) OrganizationIDs() []string { return slices.Clone(s.organizationIDs) }

// EstablishmentIDs returns the single-establishment grants.
func (s Scope) EstablishmentIDs() []string { return slices.Clone(s.establishmentIDs) }

// Allows reports whether an establishment (with its organization) is in scope.
func (s Scope) Allows(establishmentID, organizationID string) bool {
	return slices.Contains(s.establishmentIDs, establishmentID) || slices.Contains(s.organizationIDs, organizationID)
}

// apply restricts a select on establishments (aliased as "e") to the scope.
func (s Scope) applyToEstablishments(q *bun.SelectQuery) *bun.SelectQuery {
	if s.Empty() {
		return q.Where("1 = 0")
	}
	return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		if len(s.establishmentIDs) > 0 {
			q = q.WhereOr("e.id IN (?)", bun.List(s.establishmentIDs))
		}
		if len(s.organizationIDs) > 0 {
			q = q.WhereOr("e.organization_id IN (?)", bun.List(s.organizationIDs))
		}
		return q
	})
}

// EstablishmentScope proves that the caller may operate on one establishment.
// Operational repository functions take it instead of a bare ID, so that a
// query can never run without an establishment filter coming from an
// authenticated scope (spec 22). Build it with ResolveEstablishment or, for
// trusted system code only (jobs, CLI), SystemEstablishmentScope.
type EstablishmentScope struct {
	establishmentID string
	organizationID  string
}

// ID is the establishment id.
func (s EstablishmentScope) ID() string { return s.establishmentID }

// OrganizationID is the organization owning the establishment.
func (s EstablishmentScope) OrganizationID() string { return s.organizationID }

// Valid reports whether the scope was built by this package.
func (s EstablishmentScope) Valid() bool { return s.establishmentID != "" }

func (s EstablishmentScope) mustBeValid() error {
	if !s.Valid() {
		return errors.New("store: invalid establishment scope")
	}
	return nil
}

// ResolveEstablishment checks that establishmentID exists and belongs to scope.
// It returns ErrNotFound both for unknown and for out-of-scope establishments.
func ResolveEstablishment(ctx context.Context, db bun.IDB, scope Scope, establishmentID string) (EstablishmentScope, error) {
	if establishmentID == "" || scope.Empty() {
		return EstablishmentScope{}, ErrNotFound
	}
	var row struct {
		ID             string `bun:"id"`
		OrganizationID string `bun:"organization_id"`
	}
	err := db.NewSelect().TableExpr("establishments AS e").
		ColumnExpr("e.id, e.organization_id").
		Where("e.id = ?", establishmentID).
		Scan(ctx, &row)
	if err != nil {
		if isNoRows(err) {
			return EstablishmentScope{}, ErrNotFound
		}
		return EstablishmentScope{}, fmt.Errorf("store: resolve establishment: %w", err)
	}
	if !scope.Allows(row.ID, row.OrganizationID) {
		return EstablishmentScope{}, ErrNotFound
	}
	return EstablishmentScope{establishmentID: row.ID, organizationID: row.OrganizationID}, nil
}

// SystemEstablishmentScope builds a scope without an authorization check. It
// is reserved for trusted internal callers (jobs, CLI, public flows that
// resolved the establishment from its slug).
func SystemEstablishmentScope(ctx context.Context, db bun.IDB, establishmentID string) (EstablishmentScope, error) {
	var orgID string
	err := db.NewSelect().TableExpr("establishments AS e").ColumnExpr("e.organization_id").
		Where("e.id = ?", establishmentID).Scan(ctx, &orgID)
	if err != nil {
		if isNoRows(err) {
			return EstablishmentScope{}, ErrNotFound
		}
		return EstablishmentScope{}, fmt.Errorf("store: system scope: %w", err)
	}
	return EstablishmentScope{establishmentID: establishmentID, organizationID: orgID}, nil
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
