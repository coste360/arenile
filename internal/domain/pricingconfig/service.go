package pricingconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the pricing configuration service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements price rules, fixed prices, promo codes, agreements and
// the pricing tools (simulate, matrix, coverage, season copy).
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service { return &Service{d: d} }

// Errors of the pricingconfig package. They wrap the configerr kinds, which
// internal/http/problem maps to stable problem types.
var (
	ErrNotFound          = fmt.Errorf("%w: pricing configuration", configerr.ErrNotFound)
	ErrDuplicatePromo    = fmt.Errorf("%w: a promo code with the same code (ignoring case) exists", configerr.ErrConflict)
	ErrDuplicateAgree    = fmt.Errorf("%w: an agreement with the same code (ignoring case) exists", configerr.ErrConflict)
	ErrDuplicateFixed    = fmt.Errorf("%w: a fixed price for the same season, pack, formula, category and period exists", configerr.ErrConflict)
	ErrTargetSeasonInUse = fmt.Errorf("%w: the target season already has a price list", configerr.ErrConflict)
)

// Audit entity types and the entity values of PricingChanged.
const (
	EntityPriceRule  = "price_rule"
	EntityFixedPrice = "fixed_price"
	EntityPromoCode  = "promo_code"
	EntityAgreement  = "agreement"
	EntityPolicy     = "booking_policy"
	EntitySeasonCopy = "season_copy"
)

// Change kinds of PricingChanged.
const (
	ChangeCreated = event.ChangeCreated
	ChangeUpdated = event.ChangeUpdated
	ChangeDeleted = event.ChangeDeleted
	ChangeCopied  = event.ChangeCopied
)

// PricingChanged is the payload (version 1) of event.ConfigPricingChanged
// written by this package: the shared event.ConfigChange with the entity
// that changed, how, and its season. Consumers (caches, the public booking
// page) reload the price list of the season; the event carries no prices.
type PricingChanged = event.ConfigChange

// Maximum lengths of free-text fields.
const (
	maxNameLen = 200
	maxTextLen = 2000
)

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// authorize resolves the establishment for p and checks perm.
func authorize(ctx context.Context, db bun.IDB, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, db, establishmentID, perm)
	if err != nil {
		return store.EstablishmentScope{}, fmt.Errorf("pricingconfig: %w", err)
	}
	return es, nil
}

func actorOf(ctx context.Context, p *auth.Principal) audit.Actor {
	return p.Actor(reqctx.From(ctx).ClientIP)
}

// change writes the audit entry and the config.pricing_changed event of a
// mutation, in tx.
func change(ctx context.Context, tx bun.IDB, d Deps, p *auth.Principal, es store.EstablishmentScope, entity, id, kind, seasonID string, diff any) error {
	if err := d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(),
		Actor:           actorOf(ctx, p),
		Action:          entity + "." + kind,
		EntityType:      entity,
		EntityID:        id,
		Diff:            diff,
	}); err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	if _, err := d.Outbox.Write(ctx, tx, event.ConfigPricingChanged, es.ID(), 1,
		PricingChanged{Entity: entity, EntityID: id, Change: kind, SeasonID: seasonID}); err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	return nil
}

// diffOf is the audit diff of an update.
func diffOf(before, after any) map[string]any {
	return map[string]any{"before": before, "after": after}
}

// mapStoreErr turns store errors into configuration errors.
func mapStoreErr(err error, conflict error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrConflict) && conflict != nil:
		return conflict
	}
	return fmt.Errorf("pricingconfig: %w", err)
}

// staleOrMissing is the error of an optimistic update that matched no row:
// a version conflict with the current version, or not found.
func staleOrMissing(current int64, found bool) error {
	if !found {
		return ErrNotFound
	}
	return &configerr.VersionConflictError{Current: current}
}

// fromEngine converts an error of the pure engines into a validation error
// on prefix (the JSON path of the request object), when it is one.
func fromEngine(err error, prefix string) error {
	if err == nil {
		return nil
	}
	var re *pricing.RuleError
	if errors.As(err, &re) {
		field := re.Field
		if prefix != "" {
			field = prefix + "." + field
		}
		return configerr.Invalid(field, engineReason(re.Reason))
	}
	if errors.Is(err, pricing.ErrInvalidRule) || errors.Is(err, pricing.ErrInvalidInput) {
		return configerr.Invalid(prefix, err.Error())
	}
	return err
}

// engineReason makes the engine reason (English, technical) readable in the
// Italian panel while keeping it precise.
func engineReason(r string) string {
	return "Valore non valido: " + r + "."
}

func trimName(v *configerr.ValidationError, field, s string, required bool) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "" && required:
		v.Add(field, "Campo obbligatorio.")
	case len(s) > maxNameLen:
		v.Add(field, fmt.Sprintf("Massimo %d caratteri.", maxNameLen))
	}
	return s
}

func checkText(v *configerr.ValidationError, field, s string) string {
	if len(s) > maxTextLen {
		v.Add(field, fmt.Sprintf("Massimo %d caratteri.", maxTextLen))
	}
	return s
}

// cleanIDs drops blanks and duplicates; an empty result is nil ("no
// restriction"), never an empty list.
func cleanIDs(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func seasonOf(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*store.Season, error) {
	if strings.TrimSpace(id) == "" {
		return nil, configerr.Invalid("season_id", "Campo obbligatorio.")
	}
	sn, err := store.PricingGetSeason(ctx, db, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, configerr.Invalid("season_id", "Stagione inesistente.")
	}
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	return sn, nil
}

func jsonOf(v any) (store.JSON, error) {
	j, err := store.MarshalJSONValue(v)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	return j, nil
}
