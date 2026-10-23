package apikey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the apikey package (mapped through the configerr kinds).
var (
	// ErrRevoked: the key is revoked and cannot be changed (409).
	ErrRevoked = fmt.Errorf("%w: chiave API revocata", configerr.ErrConflict)
)

// MaxNameRunes bounds the key name.
const MaxNameRunes = 100

// Status is the derived state of a key (API enum ApiKeyStatus).
type Status string

// Key states.
const (
	StatusActive  Status = "active"
	StatusExpired Status = "expired"
	StatusRevoked Status = "revoked"
)

// Key is the management view of an API key (never the secret or its hash).
type Key struct {
	ID              string
	EstablishmentID string
	Name            string
	// DisplayPrefix is "ar_live_<prefix>".
	DisplayPrefix   string
	Scopes          []auth.Permission
	Status          Status
	CreatedByUserID string
	CreatedByName   string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
	ExpiresAt       *time.Time
	RevokedAt       *time.Time
}

// CreateInput describes a new key.
type CreateInput struct {
	Name      string
	Scopes    []auth.Permission
	ExpiresAt *time.Time
	IP        string
}

// UpdateInput changes a key; nil fields are unchanged. ClearExpiry removes
// the expiry and is incompatible with ExpiresAt.
type UpdateInput struct {
	Name        *string
	Scopes      []auth.Permission // nil = unchanged
	ExpiresAt   *time.Time
	ClearExpiry bool
	IP          string
}

// Deps are the dependencies of the service. Every field is required except
// Log.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
}

// Service implements the API key management use cases.
//
// Handler mapping (internal/http/admin/apikeys.go):
//
//	ListAPIKeys  -> List   -> api.ApiKeyList
//	CreateAPIKey -> Create -> 201 api.ApiKeyCreated (Cache-Control: no-store)
//	GetAPIKey    -> Get    -> api.ApiKey
//	UpdateAPIKey -> Update -> api.ApiKey
//	RevokeAPIKey -> Revoke -> 204
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// authorize refuses API key principals (a key never manages keys) and
// device tokens (a stolen device token must not mint a credential that
// outlives its revocation, INV-56), and requires auth.PermUsersManage on
// the establishment.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string) (store.EstablishmentScope, error) {
	if p != nil && (p.Kind == auth.PrincipalAPIKey || p.DeviceID != "") {
		return store.EstablishmentScope{}, auth.ErrForbidden
	}
	return p.Authorize(ctx, s.d.DB, establishmentID, auth.PermUsersManage)
}

// List returns the keys of the establishment, newest first; revoked keys
// only when includeRevoked.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, includeRevoked bool) ([]Key, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListEstablishmentAPIKeys(ctx, s.d.DB, es, includeRevoked)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, s.d.DB, rows)
}

// Get returns one key.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Key, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	row, err := store.GetAPIKey(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, err
	}
	keys, err := s.views(ctx, s.d.DB, []store.APIKey{*row})
	if err != nil {
		return nil, err
	}
	return &keys[0], nil
}

// Create creates a key and returns it with the plaintext secret (shown once).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*Key, string, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, "", err
	}
	now := s.d.Clock.Now()
	ve := &configerr.ValidationError{}
	name := validateName(ve, in.Name)
	scopes := validateScopes(ve, p, es, in.Scopes)
	validateExpiry(ve, in.ExpiresAt, now)
	if err := ve.OrNil(); err != nil {
		return nil, "", err
	}
	key, prefix, err := auth.NewAPIKey()
	if err != nil {
		return nil, "", err
	}
	row := &store.APIKey{
		ID: store.NewID(), Name: name, Prefix: prefix, KeyHash: auth.HashToken(key), Scopes: scopes,
		ExpiresAt: utc(in.ExpiresAt), CreatedAt: now, UpdatedAt: now,
	}
	if p.UserID != "" {
		row.CreatedByUserID = &p.UserID
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertAPIKey(ctx, tx, es, row); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: "api_key.created",
			EntityType: "api_key", EntityID: row.ID, Diff: map[string]any{
				"name": name, "scopes": scopes, "prefix": auth.DisplayPrefix(prefix), "expires_at": row.ExpiresAt,
			},
		})
	})
	if err != nil {
		return nil, "", fmt.Errorf("apikey: create: %w", err)
	}
	v := s.view(row, map[string]string{p.UserID: p.Name})
	return &v, key, nil
}

// Update changes name, scopes or expiry of a non-revoked key. An expired key
// may be updated (e.g. to extend its expiry).
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, in UpdateInput) (*Key, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	var out *Key
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := store.GetAPIKey(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if row.RevokedAt != nil {
			return ErrRevoked
		}
		ve := &configerr.ValidationError{}
		diff := map[string]any{}
		var cols []string
		if in.Name != nil {
			if name := validateName(ve, *in.Name); name != "" && name != row.Name {
				diff["name"] = change(row.Name, name)
				row.Name = name
				cols = append(cols, "name")
			}
		}
		if in.Scopes != nil {
			if scopes := validateScopes(ve, p, es, in.Scopes); len(scopes) > 0 && !slices.Equal(scopes, row.Scopes) {
				diff["scopes"] = change(row.Scopes, scopes)
				row.Scopes = scopes
				cols = append(cols, "scopes")
			}
		}
		switch {
		case in.ClearExpiry && in.ExpiresAt != nil:
			ve.Add("clear_expiry", "Non indicare insieme clear_expiry ed expires_at.")
		case in.ClearExpiry:
			if row.ExpiresAt != nil {
				diff["expires_at"] = change(row.ExpiresAt, nil)
				row.ExpiresAt = nil
				cols = append(cols, "expires_at")
			}
		case in.ExpiresAt != nil:
			validateExpiry(ve, in.ExpiresAt, now)
			diff["expires_at"] = change(row.ExpiresAt, utc(in.ExpiresAt))
			row.ExpiresAt = utc(in.ExpiresAt)
			cols = append(cols, "expires_at")
		}
		if err := ve.OrNil(); err != nil {
			return err
		}
		if len(cols) > 0 {
			row.UpdatedAt = now
			if err := store.UpdateAPIKey(ctx, tx, es, row, cols...); err != nil {
				return err
			}
			diff["prefix"] = auth.DisplayPrefix(row.Prefix)
			if err := s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: "api_key.updated",
				EntityType: "api_key", EntityID: row.ID, Diff: diff,
			}); err != nil {
				return err
			}
		}
		keys, err := s.views(ctx, tx, []store.APIKey{*row})
		if err != nil {
			return err
		}
		out = &keys[0]
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Revoke revokes a key (idempotent: revoking a revoked key succeeds and
// writes nothing). It takes effect on the next request: authentication only
// loads non-revoked keys.
func (s *Service) Revoke(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := store.GetAPIKey(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if row.RevokedAt != nil {
			return nil
		}
		if err := store.RevokeAPIKey(ctx, tx, es, id, now); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil // revoked concurrently
			}
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: "api_key.revoked",
			EntityType: "api_key", EntityID: row.ID, Diff: map[string]any{"name": row.Name, "prefix": auth.DisplayPrefix(row.Prefix)},
		})
	})
}

// ---- helpers --------------------------------------------------------------------

func (s *Service) views(ctx context.Context, db bun.IDB, rows []store.APIKey) ([]Key, error) {
	var ids []string
	for i := range rows {
		if u := rows[i].CreatedByUserID; u != nil && !slices.Contains(ids, *u) {
			ids = append(ids, *u)
		}
	}
	names, err := store.UserNames(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Key, len(rows))
	for i := range rows {
		out[i] = s.view(&rows[i], names)
	}
	return out, nil
}

func (s *Service) view(k *store.APIKey, names map[string]string) Key {
	v := Key{
		ID: k.ID, EstablishmentID: k.EstablishmentID, Name: k.Name, DisplayPrefix: auth.DisplayPrefix(k.Prefix),
		Scopes: make([]auth.Permission, 0, len(k.Scopes)), Status: StatusOf(k, s.d.Clock.Now()),
		CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, ExpiresAt: k.ExpiresAt, RevokedAt: k.RevokedAt,
	}
	for _, sc := range k.Scopes {
		v.Scopes = append(v.Scopes, auth.Permission(sc))
	}
	if k.CreatedByUserID != nil {
		v.CreatedByUserID = *k.CreatedByUserID
		v.CreatedByName = names[*k.CreatedByUserID]
	}
	return v
}

// StatusOf derives the state of a key at now (pure): revoked wins over
// expired; a key expires at its expires_at instant.
func StatusOf(k *store.APIKey, now time.Time) Status {
	switch {
	case k.RevokedAt != nil:
		return StatusRevoked
	case k.ExpiresAt != nil && !now.Before(*k.ExpiresAt):
		return StatusExpired
	default:
		return StatusActive
	}
}

func change(from, to any) map[string]any { return map[string]any{"from": from, "to": to} }

func validateName(ve *configerr.ValidationError, raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > MaxNameRunes {
		ve.Add("name", fmt.Sprintf("Il nome è obbligatorio e al massimo di %d caratteri.", MaxNameRunes))
		return ""
	}
	return name
}

// validateScopes checks the requested scopes: at least one, known, never
// users.manage, and held by the creator on the establishment (no privilege
// escalation through a key). It returns them deduplicated in matrix order.
func validateScopes(ve *configerr.ValidationError, p *auth.Principal, es store.EstablishmentScope, in []auth.Permission) []string {
	if len(in) == 0 {
		ve.Add("scopes", "Indica almeno un permesso.")
		return nil
	}
	for _, sc := range in {
		switch {
		case !sc.Valid():
			ve.Add("scopes", fmt.Sprintf("Permesso sconosciuto: %q.", sc))
			return nil
		case sc == auth.PermUsersManage:
			ve.Add("scopes", "Una chiave API non può gestire utenti, chiavi o webhook.")
			return nil
		case !p.Can(es, sc):
			ve.Add("scopes", fmt.Sprintf("Non puoi concedere un permesso che non hai: %q.", sc))
			return nil
		}
	}
	out := make([]string, 0, len(in))
	for _, perm := range auth.Permissions {
		if slices.Contains(in, perm) {
			out = append(out, string(perm))
		}
	}
	return out
}

func validateExpiry(ve *configerr.ValidationError, at *time.Time, now time.Time) {
	if at != nil && !at.After(now) {
		ve.Add("expires_at", "La scadenza deve essere nel futuro.")
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
