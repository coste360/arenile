package testmode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Event types and audit actions of the package. The events carry ids and
// counts only (no personal data).
const (
	EventChanged      = "establishment.test_mode_changed"
	EventPurged       = "booking.test_bookings_purged"
	ActionChanged     = "establishment.test_mode_changed"
	ActionPurged      = "test_bookings.purged"
	EntityTestMode    = "test_mode"
	PreviewTTL        = 2 * time.Hour
	previewAADPrefix  = "arenile:test-mode-preview:"
	stripeProvider    = "stripe"
	maxPreviewTokenSz = 512
)

// Errors of the package.
var (
	// ErrDisabled: the operation needs test mode on (preview link).
	ErrDisabled = fmt.Errorf("%w: la modalità prova non è attiva", configerr.ErrConflict)
	// ErrPreviewRequired: a public creation while test mode is on without a
	// valid preview token (403 test_mode_preview_required).
	ErrPreviewRequired = errors.New("testmode: preview token required")
)

// Releaser frees the allocations of a booking and fixes the pool counters
// (implemented by *inventory.Service).
type Releaser interface {
	Release(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req inventory.ReleaseRequest) (*inventory.Allocation, error)
}

// Deps are the dependencies of the service; every field is required except
// Log.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Keys      *crypto.KeyRing
	Inventory Releaser
	// BaseURL is config.EffectiveBaseURL() (preview link of the public page).
	BaseURL string
	Log     *slog.Logger
}

// Service implements the test-mode use cases.
type Service struct{ d Deps }

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	d.BaseURL = strings.TrimRight(d.BaseURL, "/")
	return &Service{d: d}
}

// Status is the test-mode state of an establishment.
type Status struct {
	Enabled              bool
	PublicEnabled        bool
	EnabledAt            *time.Time
	EnabledByUserID      *string
	TestBookings         int64
	StripeTestConfigured bool
	Version              int64
}

// Input switches test mode.
type Input struct {
	Enabled       bool
	PublicEnabled bool
}

// PreviewLink is the signed link of the public page in test mode.
type PreviewLink struct {
	URL       string
	Token     string
	ExpiresAt time.Time
}

// PurgeResult counts the purged (or, with a dry run, purgeable) rows.
type PurgeResult struct {
	Bookings int64
	Payments int64
	Deleted  bool
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// checkCredits is ErrTestCreditRedeemed when a test credit was spent by a
// real booking (INV-76).
func (s *Service) checkCredits(ctx context.Context, db bun.IDB, es store.EstablishmentScope) error {
	n, err := store.TestCreditShortfall(ctx, db, es)
	if err != nil {
		return fmt.Errorf("testmode: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w (%d clienti)", ErrTestCreditRedeemed, n)
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	return p.Authorize(ctx, s.d.DB, establishmentID, perm)
}

// Get returns the test-mode state (auth.PermBookingsRead: every staff member
// sees the banner).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID string) (*Status, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, s.d.DB, es)
}

func (s *Service) status(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*Status, error) {
	out := &Status{Version: 1}
	row, err := store.GetTestModeSettings(ctx, db, es)
	switch {
	case err == nil:
		out.Enabled, out.PublicEnabled, out.Version = row.Enabled, row.PublicEnabled, row.Version
		if row.Enabled {
			out.EnabledAt, out.EnabledByUserID = row.EnabledAt, row.EnabledByUserID
		}
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("testmode: %w", err)
	}
	counts, err := store.CountTestRows(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("testmode: %w", err)
	}
	out.TestBookings = counts.Bookings
	if out.StripeTestConfigured, err = StripeTestConfigured(ctx, db, es); err != nil {
		return nil, err
	}
	return out, nil
}

// StripeTestConfigured reports whether es has Stripe credentials in mode
// test (used automatically while test mode is on).
func StripeTestConfigured(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (bool, error) {
	row, err := store.GetPaymentProviderSettings(ctx, db, es, stripeProvider, store.ProviderModeTest)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("testmode: %w", err)
	}
	return row.Credentials != "", nil
}

// Update switches test mode with optimistic concurrency (expected = If-Match
// version, nil = any) (auth.PermEstablishmentConfigure). public_enabled is
// forced off when test mode is off (CHECK of the table). The audit entry and
// the establishment.test_mode_changed event are written in the same
// transaction.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in Input) (*Status, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if !in.Enabled {
		in.PublicEnabled = false
	}
	var out *Status
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		row, err := store.EnsureTestModeSettings(ctx, tx, es, now)
		if err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		if err := configerr.CheckVersion(expected, row.Version); err != nil {
			return err
		}
		before := *row
		switch {
		case in.Enabled && !row.Enabled:
			row.EnabledAt = &now
			if p.Kind != auth.PrincipalAPIKey && p.UserID != "" {
				uid := p.UserID
				row.EnabledByUserID = &uid
			} else {
				row.EnabledByUserID = nil
			}
		case !in.Enabled:
			row.EnabledAt, row.EnabledByUserID = nil, nil
		}
		row.Enabled, row.PublicEnabled, row.UpdatedAt = in.Enabled, in.PublicEnabled, now
		old := row.Version
		ok, err := store.UpdateTestModeSettingsVersioned(ctx, tx, es, row, old)
		if err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		if !ok {
			cur, err := store.GetTestModeSettings(ctx, tx, es)
			if err != nil {
				return fmt.Errorf("testmode: %w", err)
			}
			return &configerr.VersionConflictError{Current: cur.Version}
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(reqctx.From(ctx).ClientIP), Action: ActionChanged,
			EntityType: EntityTestMode, EntityID: es.ID(),
			Diff: map[string]any{
				"enabled":        map[string]any{"before": before.Enabled, "after": row.Enabled},
				"public_enabled": map[string]any{"before": before.PublicEnabled, "after": row.PublicEnabled},
				"version":        row.Version,
			},
		}); err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		if _, err := s.d.Outbox.Write(ctx, tx, EventChanged, es.ID(), 1, Changed{
			Enabled: row.Enabled, PublicEnabled: row.PublicEnabled, Version: row.Version,
		}); err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		out, err = s.status(ctx, tx, es)
		return err
	})
	return out, err
}

// Changed is the payload (version 1) of establishment.test_mode_changed.
type Changed struct {
	Enabled       bool  `json:"enabled"`
	PublicEnabled bool  `json:"public_enabled"`
	Version       int64 `json:"version"`
}

// Purged is the payload (version 1) of booking.test_bookings_purged (topic
// booking.changed: open panels refresh their booking views).
type Purged struct {
	Bookings int64 `json:"bookings"`
	Payments int64 `json:"payments"`
}

// PreviewLink returns the signed link of the public page that may book in
// test mode for PreviewTTL (auth.PermEstablishmentConfigure). ErrDisabled
// when test mode is off.
func (s *Service) PreviewLink(ctx context.Context, p *auth.Principal, establishmentID string) (*PreviewLink, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	on, err := store.TestModeEnabled(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("testmode: %w", err)
	}
	if !on {
		return nil, ErrDisabled
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("testmode: %w", err)
	}
	exp := s.now().Add(PreviewTTL).Truncate(time.Second)
	token, err := s.signPreview(es.ID(), exp)
	if err != nil {
		return nil, err
	}
	return &PreviewLink{
		URL:   s.d.BaseURL + "/b/" + est.Slug + "?preview=" + url.QueryEscape(token),
		Token: token, ExpiresAt: exp,
	}, nil
}

// signPreview seals "<establishment id>|<expiry unix>" with the key ring
// (AEAD with the establishment id as additional data): the token cannot be
// forged, moved to another establishment or extended.
func (s *Service) signPreview(establishmentID string, exp time.Time) (string, error) {
	if s.d.Keys == nil {
		return "", errors.New("testmode: key ring not configured")
	}
	pt := establishmentID + "|" + strconv.FormatInt(exp.Unix(), 10)
	tok, err := s.d.Keys.Encrypt([]byte(pt), []byte(previewAADPrefix+establishmentID))
	if err != nil {
		return "", fmt.Errorf("testmode: sign preview: %w", err)
	}
	return tok, nil
}

// validPreview reports whether token is a preview token of establishmentID
// not yet expired.
func (s *Service) validPreview(establishmentID, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxPreviewTokenSz || s.d.Keys == nil {
		return false
	}
	pt, err := s.d.Keys.Decrypt(token, []byte(previewAADPrefix+establishmentID))
	if err != nil {
		return false
	}
	id, expRaw, ok := strings.Cut(string(pt), "|")
	if !ok || id != establishmentID {
		return false
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		return false
	}
	return s.now().Before(time.Unix(exp, 0))
}

// PublicState tells the public page of slug whether test mode is on and
// whether this request (preview token, possibly empty) may book: Active is
// true when the page must show the test banner; Allowed is false only when
// test mode is on, public_enabled is off and the token is not valid.
type PublicState struct {
	Active  bool
	Allowed bool
}

// Public resolves the public test-mode state of the establishment of slug.
// An unknown slug answers inactive/allowed: the caller's own lookup reports
// the 404.
func (s *Service) Public(ctx context.Context, slug, previewToken string) (PublicState, error) {
	est, err := store.GetEstablishmentBySlug(ctx, s.d.DB, slug)
	if errors.Is(err, store.ErrNotFound) {
		return PublicState{Allowed: true}, nil
	}
	if err != nil {
		return PublicState{}, fmt.Errorf("testmode: %w", err)
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, est.ID)
	if err != nil {
		return PublicState{}, fmt.Errorf("testmode: %w", err)
	}
	row, err := store.GetTestModeSettings(ctx, s.d.DB, es)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !row.Enabled) {
		return PublicState{Allowed: true}, nil
	}
	if err != nil {
		return PublicState{}, fmt.Errorf("testmode: %w", err)
	}
	if row.PublicEnabled || s.validPreview(est.ID, previewToken) {
		return PublicState{Active: true, Allowed: true}, nil
	}
	return PublicState{}, nil
}

// CheckPublicCreate returns ErrPreviewRequired when a public creation for
// slug is not allowed in test mode (see Public).
func (s *Service) CheckPublicCreate(ctx context.Context, slug, previewToken string) error {
	st, err := s.Public(ctx, slug, previewToken)
	if err != nil {
		return err
	}
	if !st.Allowed {
		return ErrPreviewRequired
	}
	return nil
}

// Purge deletes every test row of the establishment in one transaction
// (auth.PermEstablishmentConfigure): allocations are released through the
// inventory first (pool counters), then store.DeleteTestRows removes the
// bookings and everything attached to them. audit_log rows stay; one audit
// entry test_bookings.purged and one event record the counts. dryRun only
// counts.
// ErrTestCreditRedeemed: a credit granted by a test booking was spent on a
// real booking; the purge would leave real money paid with credit that
// never existed (INV-76). Fix the credits of those customers first.
var ErrTestCreditRedeemed = fmt.Errorf("%w: un credito generato da una prenotazione di prova è stato usato su una prenotazione reale: correggi i crediti di quei clienti prima di eliminare le prove", configerr.ErrConflict)

func (s *Service) Purge(ctx context.Context, p *auth.Principal, establishmentID string, dryRun bool) (*PurgeResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if dryRun {
		c, err := store.CountTestRows(ctx, s.d.DB, es)
		if err != nil {
			return nil, fmt.Errorf("testmode: %w", err)
		}
		if err := s.checkCredits(ctx, s.d.DB, es); err != nil {
			return nil, err
		}
		return &PurgeResult{Bookings: c.Bookings, Payments: c.Payments}, nil
	}
	out := &PurgeResult{}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ids, err := store.ListTestBookingIDs(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		if err := store.LockBookings(ctx, tx, es, ids); err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		if err := s.checkCredits(ctx, tx, es); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := s.d.Inventory.Release(ctx, tx, es, inventory.ReleaseRequest{BookingID: id}); err != nil {
				return fmt.Errorf("testmode: release %s: %w", id, err)
			}
		}
		c, err := store.DeleteTestRows(ctx, tx, es, ids, s.now())
		if err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		out.Bookings, out.Payments, out.Deleted = c.Bookings, c.Payments, true
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(reqctx.From(ctx).ClientIP), Action: ActionPurged,
			EntityType: EntityTestMode, EntityID: es.ID(),
			Diff: map[string]any{"bookings": c.Bookings, "payments": c.Payments},
		}); err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		if _, err := s.d.Outbox.Write(ctx, tx, EventPurged, es.ID(), 1, Purged{Bookings: c.Bookings, Payments: c.Payments}); err != nil {
			return fmt.Errorf("testmode: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
