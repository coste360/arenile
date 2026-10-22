// Package establishment exposes establishments to the admin panel (scoped by
// the caller's memberships) and their public data to the booking page.
package establishment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the establishment package.
var (
	ErrNotFound      = errors.New("establishment: not found")
	ErrInvalidCursor = errors.New("establishment: invalid cursor")
	ErrInvalidSlug   = errors.New("establishment: invalid slug")
)

// MaxPageSize bounds list pages.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// ReservedSlugs cannot be used by establishments because they collide with
// static asset paths under /b/.
var ReservedSlugs = []string{"assets", "static", "embed", "api", "admin", "b"}

// ValidateSlug checks the slug format and reserved words.
func ValidateSlug(slug string) error {
	if !slugRe.MatchString(slug) {
		return ErrInvalidSlug
	}
	for _, r := range ReservedSlugs {
		if slug == r {
			return fmt.Errorf("%w: %q is reserved", ErrInvalidSlug, slug)
		}
	}
	return nil
}

// Service implements establishment use cases.
type Service struct {
	db bun.IDB
	w  WriteDeps
}

// WriteDeps are the dependencies of the configuration use cases (create,
// update, archive: M1), which write the audit log and outbox events.
type WriteDeps struct {
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Option configures a Service.
type Option func(*Service)

// WithWriteDeps sets the dependencies of the write use cases.
func WithWriteDeps(d WriteDeps) Option { return func(s *Service) { s.w = d } }

// NewService returns a Service. The read use cases need only db.
func NewService(db bun.IDB, opts ...Option) *Service {
	s := &Service{db: db}
	for _, o := range opts {
		o(s)
	}
	return s
}

// List returns the non-archived establishments visible to p.
func (s *Service) List(ctx context.Context, p *auth.Principal, cursor string, limit int) (store.Page[store.Establishment], error) {
	if p == nil {
		return store.Page[store.Establishment]{}, auth.ErrUnauthenticated
	}
	if p.TOTPEnrolmentRequired {
		return store.Page[store.Establishment]{}, auth.ErrTOTPEnrolmentNeeded
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	page, err := store.ListEstablishments(ctx, s.db, p.Scope(), cursor, limit)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return page, ErrInvalidCursor
		}
		return page, fmt.Errorf("establishment: list: %w", err)
	}
	// API keys may only list if they can read their establishment.
	if p.Kind == auth.PrincipalAPIKey && !hasScope(p, auth.PermBookingsRead) {
		return store.Page[store.Establishment]{}, auth.ErrForbidden
	}
	return page, nil
}

// Get returns one establishment visible to p (view permission required).
func (s *Service) Get(ctx context.Context, p *auth.Principal, id string) (*store.Establishment, error) {
	es, err := p.Authorize(ctx, s.db, id, auth.PermBookingsRead)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e, err := store.GetEstablishment(ctx, s.db, es)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("establishment: get: %w", err)
	}
	return e, nil
}

// Public is the public view of an establishment for the booking page.
type Public struct {
	Slug           string
	Name           string
	Timezone       string
	Currency       string
	DefaultLocale  string
	EnabledLocales []string
	BookingEnabled bool
	ContactEmail   string
	ContactPhone   string
	Address        store.Address
	Theme          store.Theme
}

// GetPublic returns the public data of a non-archived establishment.
func (s *Service) GetPublic(ctx context.Context, slug string) (*Public, error) {
	slug = strings.ToLower(slug)
	if !slugRe.MatchString(slug) {
		return nil, ErrNotFound
	}
	e, err := store.GetEstablishmentBySlug(ctx, s.db, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("establishment: get public: %w", err)
	}
	if e.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	return &Public{
		Slug: e.Slug, Name: e.Name, Timezone: e.Timezone, Currency: e.Currency, DefaultLocale: e.DefaultLocale,
		EnabledLocales: e.EnabledLocales, BookingEnabled: e.BookingEnabled, ContactEmail: e.ContactEmail,
		ContactPhone: e.ContactPhone, Address: e.Address, Theme: e.Theme,
	}, nil
}

// EmbedOrigins returns the origins allowed to frame the public page of slug
// (spec 12.2). Unknown slugs yield no origins.
func (s *Service) EmbedOrigins(ctx context.Context, slug string) ([]string, error) {
	slug = strings.ToLower(slug)
	// Reserved slugs (e.g. "assets" for the bundles under /b/assets/) and
	// malformed ones can never match an establishment: skip the query, which
	// otherwise runs for every static file of the public page.
	if ValidateSlug(slug) != nil {
		return nil, nil
	}
	e, err := store.GetEstablishmentBySlug(ctx, s.db, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("establishment: embed origins: %w", err)
	}
	return e.AllowedEmbedOrigins, nil
}

func hasScope(p *auth.Principal, perm auth.Permission) bool {
	for _, s := range p.Scopes {
		if s == perm {
			return true
		}
	}
	return false
}
