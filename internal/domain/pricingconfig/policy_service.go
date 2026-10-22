package pricingconfig

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// PolicyService implements the booking policies use cases (spec 9).
//
// Default mechanism: the establishment default policy is
// establishments.settings.default_policy (edited with the establishment
// settings, initialised with policy.DefaultRules()). Rows of booking_policies
// are scoped overrides of one season: for a booking, the matching active
// policy with the highest priority is merged over the default (policy.Resolve);
// with no match, the default applies as is. So every season always has
// exactly one default, and a policy with an empty scope acts as a
// season-wide override of it.
type PolicyService struct {
	d Deps
}

// NewPolicyService returns a PolicyService.
func NewPolicyService(d Deps) *PolicyService { return &PolicyService{d: d} }

// Policy is a stored booking policy with decoded documents.
type Policy struct {
	ID        string
	SeasonID  string
	Name      string
	Scope     policy.Scope
	Rules     policy.Rules
	Priority  int
	Active    bool
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Engine returns the policy engine view.
func (p Policy) Engine() policy.Policy {
	return policy.Policy{ID: p.ID, Scope: p.Scope, Rules: p.Rules.Clone(), Priority: p.Priority}
}

// PolicyInput is the full content of a booking policy.
type PolicyInput struct {
	SeasonID string
	Name     string
	Scope    policy.Scope
	Rules    policy.Rules
	Priority int
	Active   bool
}

func policyFromRow(r *store.BookingPolicy) (Policy, error) {
	out := Policy{
		ID: r.ID, SeasonID: r.SeasonID, Name: r.Name, Priority: r.Priority, Active: r.Active, Version: r.Version,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if err := r.Scope.Unmarshal(&out.Scope); err != nil {
		return Policy{}, fmt.Errorf("pricingconfig: policy %s scope: %w", r.ID, err)
	}
	if err := r.Rules.Unmarshal(&out.Rules); err != nil {
		return Policy{}, fmt.Errorf("pricingconfig: policy %s rules: %w", r.ID, err)
	}
	return out, nil
}

func validatePolicy(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in *PolicyInput) error {
	v := &configerr.ValidationError{}
	in.Name = trimName(v, "name", in.Name, true)
	for i, ch := range in.Scope.Channels {
		if !ch.Valid() {
			v.Add(fmt.Sprintf("scope.channels[%d]", i), "Canale sconosciuto.")
		}
	}
	in.Scope.Zones = cleanIDs(in.Scope.Zones)
	in.Scope.Packs = cleanIDs(in.Scope.Packs)
	in.Scope.RatePeriods = cleanIDs(in.Scope.RatePeriods)
	if err := in.Rules.CheckConfig(); err != nil {
		for _, e := range unjoin(err) {
			v.Add("rules", strings.TrimPrefix(e.Error(), policy.ErrInvalidRules.Error()+": "))
		}
	}
	if err := v.OrNil(); err != nil {
		return err
	}
	sn, err := seasonOf(ctx, db, es, in.SeasonID)
	if err != nil {
		return err
	}
	zones, err := store.PricingListZones(ctx, db, es)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	known := map[string]bool{}
	for _, z := range zones {
		known[z.ID], known[z.Code] = true, true
	}
	for i, z := range in.Scope.Zones {
		if !known[z] {
			v.Add(fmt.Sprintf("scope.zones[%d]", i), "Zona inesistente.")
		}
	}
	packs, err := store.PricingListPacks(ctx, db, es)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	known = map[string]bool{}
	for _, p := range packs {
		known[p.ID], known[p.Code] = true, true
	}
	for i, p := range in.Scope.Packs {
		if !known[p] {
			v.Add(fmt.Sprintf("scope.packs[%d]", i), "Pack inesistente.")
		}
	}
	periods, err := store.PricingListRatePeriods(ctx, db, es, sn.ID)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	known = map[string]bool{}
	for _, rp := range periods {
		known[rp.Code] = true
	}
	for i, c := range in.Scope.RatePeriods {
		if !known[c] {
			v.Add(fmt.Sprintf("scope.rate_periods[%d]", i), "Periodo tariffario inesistente nella stagione.")
		}
	}
	return v.OrNil()
}

func unjoin(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok { //nolint:errorlint // errors.Join result
		return j.Unwrap()
	}
	return []error{err}
}

func applyPolicy(row *store.BookingPolicy, in PolicyInput) error {
	scope, err := jsonOf(in.Scope)
	if err != nil {
		return err
	}
	rules, err := jsonOf(in.Rules)
	if err != nil {
		return err
	}
	row.SeasonID, row.Name, row.Scope, row.Rules = in.SeasonID, in.Name, scope, rules
	row.Priority, row.Active = in.Priority, in.Active
	return nil
}

// List lists booking policies (view permission).
func (s *PolicyService) List(ctx context.Context, p *auth.Principal, establishmentID, seasonID, cursor string, limit int) ([]Policy, string, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	page, err := store.ListBookingPolicies(ctx, s.d.DB, es, store.BookingPolicyFilter{SeasonID: seasonID}, cursor, limit)
	if err != nil {
		return nil, "", mapStoreErr(err, nil)
	}
	out := make([]Policy, 0, len(page.Items))
	for i := range page.Items {
		bp, err := policyFromRow(&page.Items[i])
		if err != nil {
			return nil, "", err
		}
		out = append(out, bp)
	}
	return out, page.NextCursor, nil
}

// Get returns one booking policy (view permission).
func (s *PolicyService) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Policy, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetBookingPolicy(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	bp, err := policyFromRow(row)
	return &bp, err
}

// Create stores a booking policy.
func (s *PolicyService) Create(ctx context.Context, p *auth.Principal, establishmentID string, in PolicyInput) (*Policy, error) {
	var out *Policy
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		bp, err := s.createTx(ctx, tx, p, es, in)
		out = bp
		return err
	})
	return out, err
}

func (s *PolicyService) createTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in PolicyInput) (*Policy, error) {
	if err := validatePolicy(ctx, tx, es, &in); err != nil {
		return nil, err
	}
	now := s.d.Clock.Now().UTC()
	row := &store.BookingPolicy{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := applyPolicy(row, in); err != nil {
		return nil, err
	}
	if err := store.InsertBookingPolicy(ctx, tx, es, row); err != nil {
		return nil, mapStoreErr(err, nil)
	}
	bp, err := policyFromRow(row)
	if err != nil {
		return nil, err
	}
	if err := change(ctx, tx, s.d, p, es, EntityPolicy, row.ID, ChangeCreated, row.SeasonID, bp); err != nil {
		return nil, err
	}
	return &bp, nil
}

// Update replaces a booking policy.
func (s *PolicyService) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in PolicyInput) (*Policy, error) {
	var out *Policy
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetBookingPolicy(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := policyFromRow(row)
		if err != nil {
			return err
		}
		if err := validatePolicy(ctx, tx, es, &in); err != nil {
			return err
		}
		old := row.Version
		if err := applyPolicy(row, in); err != nil {
			return err
		}
		row.Version, row.UpdatedAt = old+1, s.d.Clock.Now().UTC()
		ok, err := store.UpdateBookingPolicy(ctx, tx, es, row, old)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if !ok {
			return staleOrMissing(old, true)
		}
		after, err := policyFromRow(row)
		if err != nil {
			return err
		}
		out = &after
		return change(ctx, tx, s.d, p, es, EntityPolicy, id, ChangeUpdated, row.SeasonID, diffOf(before, after))
	})
	return out, err
}

// Delete deletes a booking policy.
func (s *PolicyService) Delete(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetBookingPolicy(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := policyFromRow(row)
		if err != nil {
			return err
		}
		if err := store.DeleteBookingPolicy(ctx, tx, es, id); err != nil {
			return mapStoreErr(err, nil)
		}
		return change(ctx, tx, s.d, p, es, EntityPolicy, id, ChangeDeleted, row.SeasonID, before)
	})
}
