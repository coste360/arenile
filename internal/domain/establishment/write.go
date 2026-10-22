package establishment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the configuration use cases.
var (
	// ErrSlugTaken: another establishment already uses the slug (409).
	ErrSlugTaken = fmt.Errorf("%w: lo slug è già usato da un altro stabilimento", configerr.ErrConflict)
	// ErrOrganizationRequired: the caller owns several organizations and did
	// not say which one the new establishment belongs to.
	ErrOrganizationRequired = configerr.Invalid("organization_id", "Indica l'organizzazione: sei proprietario di più organizzazioni.")
)

// Audit actions of the establishment configuration.
const (
	ActionCreated  = "establishment.created"
	ActionUpdated  = "establishment.updated"
	ActionArchived = "establishment.archived"
	ActionRestored = "establishment.restored"
)

// WithDB returns a copy of the service that runs on db, typically the
// transaction of a composing use case (guided setup). Nested transactions
// become savepoints.
func (s *Service) WithDB(db bun.IDB) *Service {
	c := *s
	c.db = db
	return &c
}

// CreateInput is a new establishment. Empty optional fields take the
// defaults of spec 6.1 (Europe/Rome, EUR, Italian).
type CreateInput struct {
	// OrganizationID is required only when the caller owns several
	// organizations.
	OrganizationID string
	Slug           string
	Name           string
	Timezone       string
	Currency       string
	DefaultLocale  string
	EnabledLocales []string
	ContactEmail   string
	ContactPhone   string
	Address        store.Address
	// IP is recorded in the audit log.
	IP string
}

// Create creates an establishment in an organization where the caller holds
// an organization-wide owner membership. Organization-wide memberships
// (every owner, manager... of the organization) grant access to the new
// establishment immediately; no membership row is needed.
func (s *Service) Create(ctx context.Context, p *auth.Principal, in CreateInput) (*store.Establishment, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	orgID, err := ownedOrganization(p, in.OrganizationID)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if err := ValidateSlug(slug); err != nil {
		ve.Add("slug", "Usa lettere minuscole, cifre e trattini (non all'inizio o alla fine); alcune parole sono riservate.")
	}
	name := validateName("name", in.Name, ve)
	tz := orDefault(strings.TrimSpace(in.Timezone), DefaultTimezone)
	if err := ValidateTimezone(tz); err != nil {
		ve.Add("timezone", "Fuso orario sconosciuto (es. Europe/Rome).")
	}
	currency := orDefault(strings.ToUpper(strings.TrimSpace(in.Currency)), DefaultCurrency)
	if !currencyRe.MatchString(currency) {
		ve.Add("currency", "Codice valuta ISO 4217 di tre lettere (es. EUR).")
	}
	def := orDefault(strings.TrimSpace(in.DefaultLocale), DefaultLocale)
	enabled := in.EnabledLocales
	if enabled == nil {
		enabled = []string{def}
	}
	enabled = validateLocales(def, enabled, ve)
	email, phone := validateContacts(in.ContactEmail, in.ContactPhone, ve)
	addr := validateAddress(in.Address, ve)
	if err := ve.OrNil(); err != nil {
		return nil, err
	}

	now := s.w.Clock.Now()
	e := store.NewEstablishmentDefaults(orgID, slug, name, now)
	e.Timezone, e.Currency, e.DefaultLocale, e.EnabledLocales = tz, currency, def, enabled
	e.ContactEmail, e.ContactPhone, e.Address = email, phone, addr
	settings, err := store.MarshalJSONValue(storedSettings(DefaultSettings()))
	if err != nil {
		return nil, err
	}
	e.Settings = settings

	err = store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if taken, err := store.EstablishmentSlugTaken(ctx, tx, slug, ""); err != nil {
			return err
		} else if taken {
			return ErrSlugTaken
		}
		if err := store.InsertEstablishment(ctx, tx, e); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrSlugTaken
			}
			return err
		}
		return s.w.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: e.ID, Actor: p.Actor(in.IP), Action: ActionCreated, EntityType: "establishment", EntityID: e.ID,
			Diff: map[string]any{"organization_id": orgID, "slug": slug, "name": name, "timezone": tz, "currency": currency},
		})
	})
	if err != nil {
		if errors.Is(err, ErrSlugTaken) {
			return nil, err
		}
		return nil, fmt.Errorf("establishment: create: %w", err)
	}
	return e, nil
}

// ownedOrganization picks the organization of a new establishment among the
// organizations where p is an organization-wide owner.
func ownedOrganization(p *auth.Principal, requested string) (string, error) {
	if p == nil {
		return "", auth.ErrUnauthenticated
	}
	if p.TOTPEnrolmentRequired {
		return "", auth.ErrTOTPEnrolmentNeeded
	}
	if p.Kind != auth.PrincipalUser {
		return "", auth.ErrForbidden
	}
	var owned []string
	for _, g := range p.Grants {
		if g.Role == auth.RoleOwner && g.EstablishmentID == "" && !slices.Contains(owned, g.OrganizationID) {
			owned = append(owned, g.OrganizationID)
		}
	}
	switch {
	case requested != "":
		if !slices.Contains(owned, requested) {
			return "", auth.ErrForbidden
		}
		return requested, nil
	case len(owned) == 1:
		return owned[0], nil
	case len(owned) == 0:
		return "", auth.ErrForbidden
	default:
		return "", ErrOrganizationRequired
	}
}

// UpdateInput is a partial update: nil fields are left unchanged. Settings
// and Theme, when present, replace the whole document.
type UpdateInput struct {
	Name           *string
	Slug           *string
	Timezone       *string
	Currency       *string
	DefaultLocale  *string
	EnabledLocales *[]string
	ContactEmail   *string
	ContactPhone   *string
	Address        *store.Address
	Geo            *store.Geo
	// ConcessionRef: an empty string removes it.
	ConcessionRef       *string
	BookingEnabled      *bool
	AllowedEmbedOrigins *[]string
	Theme               *store.Theme
	Settings            *Settings
	// Archived true archives, false restores (owners only).
	Archived *bool
	// IfMatch is the expected version (nil = no precondition).
	IfMatch *int64
	IP      string
}

// Update changes an establishment (auth.PermEstablishmentConfigure; archiving
// and restoring require the owner role). It bumps the version, writes the
// audit log and, when the settings document changes (pricing settings, local
// holidays, default policy), emits config.pricing_changed in the same
// transaction.
func (s *Service) Update(ctx context.Context, p *auth.Principal, id string, in UpdateInput) (*store.Establishment, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	es, err := p.Authorize(ctx, s.db, id, auth.PermEstablishmentConfigure)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if in.Archived != nil && !isOwner(p, es) {
		return nil, auth.ErrForbidden
	}
	var out *store.Establishment
	err = store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(in.IfMatch, cur.Version); err != nil {
			return err
		}
		next := *cur
		changes, settingsChanged, err := applyUpdate(&next, cur, in)
		if err != nil {
			return err
		}
		if next.Slug != cur.Slug {
			if taken, err := store.EstablishmentSlugTaken(ctx, tx, next.Slug, cur.ID); err != nil {
				return err
			} else if taken {
				return ErrSlugTaken
			}
		}
		now := s.w.Clock.Now()
		next.Version = cur.Version + 1
		next.UpdatedAt = now
		action := ActionUpdated
		if in.Archived != nil && *in.Archived != (cur.ArchivedAt != nil) {
			if *in.Archived {
				next.ArchivedAt, action = &now, ActionArchived
			} else {
				next.ArchivedAt, action = nil, ActionRestored
			}
			changes["archived"] = map[string]any{"from": cur.ArchivedAt != nil, "to": *in.Archived}
		}
		ok, err := store.UpdateEstablishmentIfVersion(ctx, tx, es, &next, cur.Version)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrSlugTaken
			}
			return err
		}
		if !ok {
			return s.conflict(ctx, tx, es)
		}
		if err := s.w.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: action, EntityType: "establishment", EntityID: es.ID(),
			Diff: map[string]any{"version": next.Version, "changes": changes},
		}); err != nil {
			return err
		}
		if settingsChanged {
			if _, err := s.w.Outbox.Write(ctx, tx, event.ConfigPricingChanged, es.ID(), 1, map[string]any{
				"resource": "establishment_settings", "resource_id": es.ID(), "action": "updated", "version": next.Version,
			}); err != nil {
				return err
			}
		}
		out = &next
		return nil
	})
	if err != nil {
		var ve *configerr.ValidationError
		var vc *configerr.VersionConflictError
		switch {
		case errors.As(err, &ve), errors.As(err, &vc), errors.Is(err, ErrSlugTaken):
			return nil, err
		case errors.Is(err, store.ErrNotFound):
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("establishment: update: %w", err)
	}
	return out, nil
}

// writable checks that the write dependencies were configured.
func (s *Service) writable() error {
	if s.w.Clock == nil || s.w.Audit == nil || s.w.Outbox == nil {
		return errors.New("establishment: write dependencies not configured")
	}
	return nil
}

// conflict reports the current version after a lost optimistic update.
func (s *Service) conflict(ctx context.Context, db bun.IDB, es store.EstablishmentScope) error {
	cur, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return err
	}
	return &configerr.VersionConflictError{Current: cur.Version}
}

// applyUpdate validates in and applies it to next. It returns the audit
// changes (field -> {from,to}, or just the field name for documents and
// contacts, which are not copied into the audit log) and whether the
// settings document changed.
func applyUpdate(next, cur *store.Establishment, in UpdateInput) (map[string]any, bool, error) {
	ve := &configerr.ValidationError{}
	changes := map[string]any{}
	track := func(field string, from, to any) { changes[field] = map[string]any{"from": from, "to": to} }
	if in.Name != nil {
		next.Name = validateName("name", *in.Name, ve)
		if next.Name != cur.Name {
			track("name", cur.Name, next.Name)
		}
	}
	if in.Slug != nil {
		next.Slug = strings.ToLower(strings.TrimSpace(*in.Slug))
		if err := ValidateSlug(next.Slug); err != nil {
			ve.Add("slug", "Usa lettere minuscole, cifre e trattini (non all'inizio o alla fine); alcune parole sono riservate.")
		}
		if next.Slug != cur.Slug {
			track("slug", cur.Slug, next.Slug)
		}
	}
	if in.Timezone != nil {
		next.Timezone = strings.TrimSpace(*in.Timezone)
		if err := ValidateTimezone(next.Timezone); err != nil {
			ve.Add("timezone", "Fuso orario sconosciuto (es. Europe/Rome).")
		}
		if next.Timezone != cur.Timezone {
			track("timezone", cur.Timezone, next.Timezone)
		}
	}
	if in.Currency != nil {
		next.Currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
		if !currencyRe.MatchString(next.Currency) {
			ve.Add("currency", "Codice valuta ISO 4217 di tre lettere (es. EUR).")
		}
		if next.Currency != cur.Currency {
			track("currency", cur.Currency, next.Currency)
		}
	}
	if in.DefaultLocale != nil {
		next.DefaultLocale = strings.TrimSpace(*in.DefaultLocale)
	}
	if in.EnabledLocales != nil {
		next.EnabledLocales = *in.EnabledLocales
	}
	if in.DefaultLocale != nil || in.EnabledLocales != nil {
		next.EnabledLocales = validateLocales(next.DefaultLocale, next.EnabledLocales, ve)
		if next.DefaultLocale != cur.DefaultLocale {
			track("default_locale", cur.DefaultLocale, next.DefaultLocale)
		}
		if !slices.Equal(next.EnabledLocales, cur.EnabledLocales) {
			track("enabled_locales", cur.EnabledLocales, next.EnabledLocales)
		}
	}
	if in.ContactEmail != nil || in.ContactPhone != nil {
		email, phone := cur.ContactEmail, cur.ContactPhone
		if in.ContactEmail != nil {
			email = *in.ContactEmail
		}
		if in.ContactPhone != nil {
			phone = *in.ContactPhone
		}
		next.ContactEmail, next.ContactPhone = validateContacts(email, phone, ve)
		if next.ContactEmail != cur.ContactEmail {
			changes["contact_email"] = "changed"
		}
		if next.ContactPhone != cur.ContactPhone {
			changes["contact_phone"] = "changed"
		}
	}
	if in.Address != nil {
		next.Address = validateAddress(*in.Address, ve)
		if next.Address != cur.Address {
			changes["address"] = "changed"
		}
	}
	if in.Geo != nil {
		g := *in.Geo
		validateGeo(&g, ve)
		next.Geo = &g
		changes["geo"] = "changed"
	}
	if in.ConcessionRef != nil {
		ref := strings.TrimSpace(*in.ConcessionRef)
		if len(ref) > MaxNameLength {
			ve.Add("concession_ref", fmt.Sprintf("Al massimo %d caratteri.", MaxNameLength))
		}
		if ref == "" {
			next.ConcessionRef = nil
		} else {
			next.ConcessionRef = &ref
		}
		changes["concession_ref"] = "changed"
	}
	if in.BookingEnabled != nil {
		next.BookingEnabled = *in.BookingEnabled
		if next.BookingEnabled != cur.BookingEnabled {
			track("booking_enabled", cur.BookingEnabled, next.BookingEnabled)
		}
	}
	if in.AllowedEmbedOrigins != nil {
		next.AllowedEmbedOrigins = validateEmbedOrigins(*in.AllowedEmbedOrigins, ve)
		if !slices.Equal(next.AllowedEmbedOrigins, cur.AllowedEmbedOrigins) {
			track("allowed_embed_origins", cur.AllowedEmbedOrigins, next.AllowedEmbedOrigins)
		}
	}
	if in.Theme != nil {
		next.Theme = ValidateTheme(*in.Theme, ve)
		changes["theme"] = "changed"
	}
	settingsChanged := false
	if in.Settings != nil {
		st := ValidateSettings(*in.Settings, ve)
		raw, err := store.MarshalJSONValue(storedSettings(st))
		if err != nil {
			return nil, false, err
		}
		next.Settings = raw
		settingsChanged = !bytes.Equal(raw, cur.Settings)
		if settingsChanged {
			changes["settings"] = "changed"
		}
	}
	if in.Currency != nil && next.Currency != cur.Currency {
		// The currency is part of every price quote.
		settingsChanged = true
	}
	return changes, settingsChanged, ve.OrNil()
}

// storedSettings is the persisted form of the settings document: the
// currency lives on the establishment row and is never duplicated.
func storedSettings(s Settings) Settings {
	s.Pricing.Currency = ""
	return s
}

// isOwner reports whether p holds the owner role on the establishment.
func isOwner(p *auth.Principal, es store.EstablishmentScope) bool {
	if p == nil || p.Kind != auth.PrincipalUser {
		return false
	}
	for _, g := range p.Grants {
		if g.Role == auth.RoleOwner && g.OrganizationID == es.OrganizationID() && (g.EstablishmentID == "" || g.EstablishmentID == es.ID()) {
			return true
		}
	}
	return false
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
