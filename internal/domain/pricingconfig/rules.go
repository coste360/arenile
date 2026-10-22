package pricingconfig

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// PriceRule is a stored price rule with its decoded engine documents.
type PriceRule struct {
	ID         string
	SeasonID   string
	Name       string
	TargetType pricing.TargetType
	// TargetIDs nil means every pack/extra.
	TargetIDs  []string
	Conditions pricing.Conditions
	Action     pricing.Action
	Priority   int
	Stage      pricing.Stage
	Stackable  bool
	Active     bool
	Seq        int64
	Version    int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Engine returns the pricing engine view of the rule.
func (r PriceRule) Engine() pricing.Rule {
	return pricing.Rule{
		ID: r.ID, Name: r.Name, TargetType: r.TargetType, TargetIDs: slices.Clone(r.TargetIDs),
		Conditions: r.Conditions, Action: r.Action, Priority: r.Priority, Stage: r.Stage,
		Stackable: r.Stackable, Active: r.Active, Seq: r.Seq,
	}
}

// RuleInput is the full (create or replace) content of a price rule.
type RuleInput struct {
	SeasonID   string
	Name       string
	TargetType pricing.TargetType
	TargetIDs  []string
	Conditions pricing.Conditions
	Action     pricing.Action
	Priority   int
	Stage      pricing.Stage
	Stackable  bool
	Active     bool
}

// RuleListFilter narrows ListRules.
type RuleListFilter struct {
	SeasonID   string
	Stage      string
	TargetType string
}

func ruleFromRow(r *store.PriceRule) (PriceRule, error) {
	out := PriceRule{
		ID: r.ID, SeasonID: r.SeasonID, Name: r.Name, TargetType: pricing.TargetType(r.TargetType),
		TargetIDs: r.TargetIDs, Priority: r.Priority, Stage: pricing.Stage(r.Stage), Stackable: r.Stackable,
		Active: r.Active, Seq: r.Seq, Version: r.Version, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if err := r.Conditions.Unmarshal(&out.Conditions); err != nil {
		return PriceRule{}, fmt.Errorf("pricingconfig: rule %s conditions: %w", r.ID, err)
	}
	if err := r.Action.Unmarshal(&out.Action); err != nil {
		return PriceRule{}, fmt.Errorf("pricingconfig: rule %s action: %w", r.ID, err)
	}
	return out, nil
}

// validateRule checks in against the engine and the establishment data; id
// is the rule ID (new or existing).
func validateRule(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string, in *RuleInput) error {
	v := &configerr.ValidationError{}
	in.Name = trimName(v, "name", in.Name, true)
	in.TargetIDs = cleanIDs(in.TargetIDs)
	if err := v.OrNil(); err != nil {
		return err
	}
	if _, err := seasonOf(ctx, db, es, in.SeasonID); err != nil {
		return err
	}
	rule := pricing.Rule{
		ID: id, Name: in.Name, TargetType: in.TargetType, TargetIDs: in.TargetIDs, Conditions: in.Conditions,
		Action: in.Action, Priority: in.Priority, Stage: in.Stage, Stackable: in.Stackable, Active: in.Active,
	}
	if err := fromEngine(pricing.ValidateRule(rule), ""); err != nil {
		return err
	}
	if len(in.TargetIDs) > 0 {
		known := map[string]bool{}
		if in.TargetType == pricing.TargetPack {
			packs, err := store.PricingListPacks(ctx, db, es)
			if err != nil {
				return fmt.Errorf("pricingconfig: %w", err)
			}
			for _, p := range packs {
				known[p.ID] = true
			}
		} else {
			extras, err := store.PricingListExtras(ctx, db, es)
			if err != nil {
				return fmt.Errorf("pricingconfig: %w", err)
			}
			for _, x := range extras {
				known[x.ID] = true
			}
		}
		for i, tid := range in.TargetIDs {
			if !known[tid] {
				v.Add(fmt.Sprintf("target_ids[%d]", i), "Elemento del catalogo inesistente.")
			}
		}
	}
	return v.OrNil()
}

func applyRuleInput(row *store.PriceRule, in RuleInput) error {
	cond, err := jsonOf(in.Conditions)
	if err != nil {
		return err
	}
	act, err := jsonOf(in.Action)
	if err != nil {
		return err
	}
	row.SeasonID = in.SeasonID
	row.Name = in.Name
	row.TargetType = string(in.TargetType)
	row.TargetIDs = in.TargetIDs
	row.Conditions = cond
	row.Action = act
	row.Priority = in.Priority
	row.Stage = string(in.Stage)
	row.Stackable = in.Stackable
	row.Active = in.Active
	return nil
}

// ListRules lists the price rules of the establishment (view permission).
func (s *Service) ListRules(ctx context.Context, p *auth.Principal, establishmentID string, f RuleListFilter, cursor string, limit int) ([]PriceRule, string, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	page, err := store.ListPriceRules(ctx, s.d.DB, es, store.PriceRuleFilter(f), cursor, limit)
	if err != nil {
		return nil, "", mapStoreErr(err, nil)
	}
	out := make([]PriceRule, 0, len(page.Items))
	for i := range page.Items {
		r, err := ruleFromRow(&page.Items[i])
		if err != nil {
			return nil, "", err
		}
		out = append(out, r)
	}
	return out, page.NextCursor, nil
}

// GetRule returns one price rule (view permission).
func (s *Service) GetRule(ctx context.Context, p *auth.Principal, establishmentID, id string) (*PriceRule, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetPriceRule(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	r, err := ruleFromRow(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateRule validates and stores a price rule; seq = max(seq)+1.
func (s *Service) CreateRule(ctx context.Context, p *auth.Principal, establishmentID string, in RuleInput) (*PriceRule, error) {
	var out *PriceRule
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		r, err := s.createRuleTx(ctx, tx, p, es, in)
		out = r
		return err
	})
	return out, err
}

func (s *Service) createRuleTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in RuleInput) (*PriceRule, error) {
	id := store.NewID()
	if err := validateRule(ctx, tx, es, id, &in); err != nil {
		return nil, err
	}
	seq, err := store.NextPriceRuleSeq(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	now := s.now()
	row := &store.PriceRule{ID: id, Seq: seq, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := applyRuleInput(row, in); err != nil {
		return nil, err
	}
	if err := store.InsertPriceRule(ctx, tx, es, row); err != nil {
		return nil, mapStoreErr(err, nil)
	}
	r, err := ruleFromRow(row)
	if err != nil {
		return nil, err
	}
	if err := change(ctx, tx, s.d, p, es, EntityPriceRule, id, ChangeCreated, row.SeasonID, r); err != nil {
		return nil, err
	}
	return &r, nil
}

// UpdateRule replaces a price rule. expected is the If-Match version (nil =
// unconditional). The rule gets a new seq (it becomes the most recent).
func (s *Service) UpdateRule(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in RuleInput) (*PriceRule, error) {
	var out *PriceRule
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetPriceRule(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := ruleFromRow(row)
		if err != nil {
			return err
		}
		if err := validateRule(ctx, tx, es, id, &in); err != nil {
			return err
		}
		seq, err := store.NextPriceRuleSeq(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		old := row.Version
		if err := applyRuleInput(row, in); err != nil {
			return err
		}
		row.Seq, row.Version, row.UpdatedAt = seq, old+1, s.now()
		ok, err := store.UpdatePriceRule(ctx, tx, es, row, old)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if !ok {
			return staleOrMissing(old, true)
		}
		after, err := ruleFromRow(row)
		if err != nil {
			return err
		}
		out = &after
		return change(ctx, tx, s.d, p, es, EntityPriceRule, id, ChangeUpdated, row.SeasonID, diffOf(before, after))
	})
	return out, err
}

// DeleteRule deletes a price rule.
func (s *Service) DeleteRule(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		row, err := store.GetPriceRule(ctx, tx, es, id)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before, err := ruleFromRow(row)
		if err != nil {
			return err
		}
		if err := store.DeletePriceRule(ctx, tx, es, id); err != nil {
			return mapStoreErr(err, nil)
		}
		return change(ctx, tx, s.d, p, es, EntityPriceRule, id, ChangeDeleted, row.SeasonID, before)
	})
}

// ReplaceRulesResult is the outcome of ReplaceRules.
type ReplaceRulesResult struct {
	Rules    []PriceRule
	Replaced int
}

// ReplaceRules creates rules in one transaction, after deleting the rules of
// the season whose IDs are in replaceIDs (e.g. the base rules generated by a
// previous quick price list). It is meant for the guided setup (spec 15.2):
// one audit entry per rule, one config.pricing_changed event per rule.
func (s *Service) ReplaceRules(ctx context.Context, p *auth.Principal, establishmentID, seasonID string, replaceIDs []string, inputs []RuleInput) (*ReplaceRulesResult, error) {
	out := &ReplaceRulesResult{Rules: []PriceRule{}}
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		for _, id := range cleanIDs(replaceIDs) {
			row, err := store.GetPriceRule(ctx, tx, es, id)
			if err != nil {
				return mapStoreErr(err, nil)
			}
			if row.SeasonID != seasonID {
				return configerr.Invalid("replace_ids", "La regola appartiene a un'altra stagione.")
			}
			before, err := ruleFromRow(row)
			if err != nil {
				return err
			}
			if err := store.DeletePriceRule(ctx, tx, es, id); err != nil {
				return mapStoreErr(err, nil)
			}
			if err := change(ctx, tx, s.d, p, es, EntityPriceRule, id, ChangeDeleted, seasonID, before); err != nil {
				return err
			}
			out.Replaced++
		}
		for i := range inputs {
			in := inputs[i]
			in.SeasonID = seasonID
			r, err := s.createRuleTx(ctx, tx, p, es, in)
			if err != nil {
				return prefixValidation(err, fmt.Sprintf("rules[%d]", i))
			}
			out.Rules = append(out.Rules, *r)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// prefixValidation prefixes the fields of a validation error.
func prefixValidation(err error, prefix string) error {
	ve, ok := err.(*configerr.ValidationError) //nolint:errorlint // only the direct type is rewritten
	if !ok {
		return err
	}
	out := &configerr.ValidationError{}
	for _, f := range ve.Fields {
		name := prefix
		if f.Field != "" {
			name += "." + f.Field
		}
		out.Add(name, f.Message)
	}
	return out
}
