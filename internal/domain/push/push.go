package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/pushrelay"
	"github.com/coste360/arenile/internal/store"
)

// Domain errors (mapped by internal/http/problem).
var (
	// ErrUnavailable: no relay key on the instance or push turned off by
	// the manager (409 push_unavailable).
	ErrUnavailable = errors.New("push: notifications unavailable")
	// ErrVersionMismatch: If-Match does not match (412).
	ErrVersionMismatch = configerr.ErrVersionConflict
)

// Audit actions.
const (
	ActionSettingsUpdated = "push_settings.updated"
	ActionRegistered      = "device.push_registered"
	ActionUnregistered    = "device.push_unregistered"
)

// Token bounds (api.PushRegistrationInput.token).
const (
	minTokenLen = 16
	maxTokenLen = 4096
)

// Locales of the relay catalogue (api.PushLocale).
var locales = []string{"it", "en", "de", "fr"}

// topicPermission is the permission a role needs to receive a topic.
var topicPermission = map[string]auth.Permission{
	store.PushTopicNewOnlineBooking:  auth.PermBookingsRead,
	store.PushTopicApprovalRequested: auth.PermBookingsWrite,
	store.PushTopicPaymentFailed:     auth.PermRefundsIssue,
}

// Relay sends one notification (pushrelay.Client).
type Relay interface {
	Configured() bool
	Send(ctx context.Context, n pushrelay.Notification) pushrelay.Result
}

// Principals builds the principal of a device (auth.Service): the deliverer
// re-checks the user's role at send time with the same rules as a request.
type Principals interface {
	PrincipalForDevice(ctx context.Context, d auth.DeviceIdentity) (*auth.Principal, error)
}

// Deps are the dependencies of Service.
type Deps struct {
	DB         *store.DB
	Clock      clock.Clock
	Audit      *audit.Writer
	Log        *slog.Logger
	Keys       *crypto.KeyRing
	Principals Principals
	// Relay is nil when push is not configured (no relay key, desktop
	// edition): nothing is queued and no request is ever made.
	Relay Relay
	// IdleExpired reports whether a device token expired for inactivity
	// (device.IdleExpired); nil means never.
	IdleExpired func(d *store.Device, now time.Time) bool
}

// Service implements the push use cases. Construct with NewService.
type Service struct {
	d    Deps
	wake chan struct{}
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d, wake: make(chan struct{}, 1)}
}

// RelayConfigured reports whether the instance can send push
// notifications (/meta capabilities.push_notifications).
func (s *Service) RelayConfigured() bool { return s.d.Relay != nil && s.d.Relay.Configured() }

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// kick wakes the delivery job without blocking.
func (s *Service) kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// ---- settings ---------------------------------------------------------------------

// Events are the notifications an establishment sends (api.PushEvents).
type Events struct {
	NewOnlineBooking  bool
	ApprovalRequested bool
	PaymentFailed     bool
}

// Settings is api.PushSettings.
type Settings struct {
	Enabled         bool
	Events          Events
	RelayConfigured bool
	Version         int64
}

// SettingsInput is api.PushSettingsInput.
type SettingsInput struct {
	Enabled bool
	Events  Events
}

// topicOn reports whether the settings send topic.
func (st *Settings) topicOn(topic string) bool {
	if !st.Enabled {
		return false
	}
	switch topic {
	case store.PushTopicNewOnlineBooking:
		return st.Events.NewOnlineBooking
	case store.PushTopicApprovalRequested:
		return st.Events.ApprovalRequested
	case store.PushTopicPaymentFailed:
		return st.Events.PaymentFailed
	}
	return false
}

// GetSettings returns the push settings of establishmentID (bookings.read).
func (s *Service) GetSettings(ctx context.Context, p *auth.Principal, establishmentID string) (*Settings, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	return s.settings(ctx, s.d.DB, es)
}

// UpdateSettings replaces the push settings of establishmentID
// (establishment.configure). expected is the If-Match version (nil = any).
// Audit "push_settings.updated".
func (s *Service) UpdateSettings(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in SettingsInput, ip string) (*Settings, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *Settings
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		row := &store.PushSettings{
			Enabled: in.Enabled, NewOnlineBooking: in.Events.NewOnlineBooking,
			ApprovalRequested: in.Events.ApprovalRequested, PaymentFailed: in.Events.PaymentFailed, UpdatedAt: now,
		}
		cur, err := store.GetPushSettings(ctx, tx, es)
		switch {
		case errors.Is(err, store.ErrNotFound):
			if expected != nil && *expected != 1 {
				return ErrVersionMismatch
			}
			row.Version, row.CreatedAt = 2, now
			if err := store.InsertPushSettings(ctx, tx, es, row); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return ErrVersionMismatch
				}
				return fmt.Errorf("push: settings: %w", err)
			}
		case err != nil:
			return fmt.Errorf("push: settings: %w", err)
		default:
			if expected != nil && *expected != cur.Version {
				return ErrVersionMismatch
			}
			row.Version, row.CreatedAt = cur.Version+1, cur.CreatedAt
			ok, err := store.UpdatePushSettings(ctx, tx, es, row, cur.Version)
			if err != nil {
				return fmt.Errorf("push: settings: %w", err)
			}
			if !ok {
				return ErrVersionMismatch
			}
		}
		out = s.settingsOf(row)
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: ActionSettingsUpdated,
			EntityType: "establishment", EntityID: es.ID(),
			Diff: map[string]any{
				"enabled": row.Enabled, "booking_new_online": row.NewOnlineBooking,
				"booking_approval_requested": row.ApprovalRequested, "payment_failed": row.PaymentFailed,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// settings returns the settings of es (defaults when no row: disabled,
// every event on, version 1).
func (s *Service) settings(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*Settings, error) {
	row, err := store.GetPushSettings(ctx, db, es)
	if errors.Is(err, store.ErrNotFound) {
		return &Settings{
			Events:          Events{NewOnlineBooking: true, ApprovalRequested: true, PaymentFailed: true},
			RelayConfigured: s.RelayConfigured(), Version: 1,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("push: settings: %w", err)
	}
	return s.settingsOf(row), nil
}

func (s *Service) settingsOf(r *store.PushSettings) *Settings {
	return &Settings{
		Enabled: r.Enabled, Events: Events{NewOnlineBooking: r.NewOnlineBooking, ApprovalRequested: r.ApprovalRequested, PaymentFailed: r.PaymentFailed},
		RelayConfigured: s.RelayConfigured(), Version: r.Version,
	}
}

// ---- registration -----------------------------------------------------------------

// RegisterInput is api.PushRegistrationInput.
type RegisterInput struct {
	Token       string
	Environment string // default store.PushEnvironmentProduction
	Locale      string
	Topics      []string
}

// Status is api.PushRegistrationStatus.
type Status struct {
	Available            bool
	RelayConfigured      bool
	EstablishmentEnabled bool
	Registered           bool
	Environment          string
	Locale               string
	Topics               []string
	AllowedTopics        []string
	UpdatedAt            *time.Time
}

// deviceContext resolves the establishment and device of a device
// principal.
func (s *Service) deviceContext(ctx context.Context, db bun.IDB, p *auth.Principal) (store.EstablishmentScope, *store.Device, error) {
	if p == nil || p.DeviceID == "" {
		return store.EstablishmentScope{}, nil, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, db, p.DeviceEstablishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.EstablishmentScope{}, nil, err
	}
	d, err := store.GetDevice(ctx, db, es, p.DeviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.EstablishmentScope{}, nil, auth.ErrUnauthenticated
		}
		return store.EstablishmentScope{}, nil, fmt.Errorf("push: device: %w", err)
	}
	return es, d, nil
}

// roleTopics lists the topics p's role may receive on es, in catalogue
// order.
func roleTopics(p *auth.Principal, es store.EstablishmentScope) []string {
	out := []string{}
	for _, t := range store.PushTopics {
		if p.Can(es, topicPermission[t]) {
			out = append(out, t)
		}
	}
	return out
}

// Status returns the push state of the device of p.
func (s *Service) Status(ctx context.Context, p *auth.Principal) (*Status, error) {
	es, _, err := s.deviceContext(ctx, s.d.DB, p)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, s.d.DB, p, es)
}

func (s *Service) status(ctx context.Context, db bun.IDB, p *auth.Principal, es store.EstablishmentScope) (*Status, error) {
	st, err := s.settings(ctx, db, es)
	if err != nil {
		return nil, err
	}
	out := &Status{
		RelayConfigured: st.RelayConfigured, EstablishmentEnabled: st.Enabled,
		Available: st.RelayConfigured && st.Enabled, Topics: []string{}, AllowedTopics: []string{},
	}
	for _, t := range roleTopics(p, es) {
		if st.topicOn(t) {
			out.AllowedTopics = append(out.AllowedTopics, t)
		}
	}
	reg, err := store.GetPushRegistrationByDevice(ctx, db, es, p.DeviceID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return out, nil
	case err != nil:
		return nil, fmt.Errorf("push: registration: %w", err)
	}
	updated := reg.UpdatedAt
	out.Registered, out.Environment, out.Locale, out.UpdatedAt = true, reg.Environment, reg.Locale, &updated
	out.Topics = append(out.Topics, reg.Topics...)
	return out, nil
}

// validateRegistration normalises in and checks it against the topics the
// role of p allows.
func validateRegistration(in *RegisterInput, allowed []string) error {
	var ve configerr.ValidationError
	if n := len(in.Token); n < minTokenLen || n > maxTokenLen {
		ve.Fields = append(ve.Fields, configerr.FieldError{Field: "token", Message: fmt.Sprintf("Token tra %d e %d caratteri.", minTokenLen, maxTokenLen)})
	} else {
		for i := 0; i < len(in.Token); i++ {
			if c := in.Token[i]; c < 0x21 || c > 0x7e {
				ve.Fields = append(ve.Fields, configerr.FieldError{Field: "token", Message: "Token non valido: solo caratteri ASCII stampabili, senza spazi."})
				break
			}
		}
	}
	if in.Environment == "" {
		in.Environment = store.PushEnvironmentProduction
	}
	if in.Environment != store.PushEnvironmentProduction && in.Environment != store.PushEnvironmentSandbox {
		ve.Fields = append(ve.Fields, configerr.FieldError{Field: "environment", Message: "Usa production o sandbox."})
	}
	if !slices.Contains(locales, in.Locale) {
		ve.Fields = append(ve.Fields, configerr.FieldError{Field: "locale", Message: "Lingua non supportata: it, en, de o fr."})
	}
	topics := make([]string, 0, len(in.Topics))
	for _, t := range in.Topics {
		switch {
		case !slices.Contains(store.PushTopics, t):
			ve.Fields = append(ve.Fields, configerr.FieldError{Field: "topics", Message: "Argomento sconosciuto."})
		case !slices.Contains(allowed, t):
			ve.Fields = append(ve.Fields, configerr.FieldError{Field: "topics", Message: "Argomento non consentito al tuo ruolo."})
		case !slices.Contains(topics, t):
			topics = append(topics, t)
		}
	}
	// Catalogue order, so a registration reads the same whatever the order
	// sent by the app.
	slices.SortFunc(topics, func(a, b string) int { return slices.Index(store.PushTopics, a) - slices.Index(store.PushTopics, b) })
	in.Topics = topics
	if len(ve.Fields) > 0 {
		return &ve
	}
	return nil
}

// Register stores the native token of the device of p (user consent),
// replacing its previous registration and any registration of the same
// token by another device of the establishment. Audit
// "device.push_registered" (never the token).
func (s *Service) Register(ctx context.Context, p *auth.Principal, in RegisterInput) (*Status, error) {
	es, d, err := s.deviceContext(ctx, s.d.DB, p)
	if err != nil {
		return nil, err
	}
	if d.Platform != store.DevicePlatformIOS && d.Platform != store.DevicePlatformAndroid {
		return nil, configerr.Invalid("platform", "Le notifiche push sono solo per l'app mobile (iOS o Android).")
	}
	if err := validateRegistration(&in, roleTopics(p, es)); err != nil {
		return nil, err
	}
	st, err := s.settings(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	if !st.RelayConfigured || !st.Enabled {
		return nil, ErrUnavailable
	}
	if s.d.Keys == nil {
		return nil, errors.New("push: no key ring")
	}
	ciphertext, err := s.d.Keys.Encrypt([]byte(in.Token), tokenAAD(d.ID))
	if err != nil {
		return nil, fmt.Errorf("push: encrypt token: %w", err)
	}
	var out *Status
	register := func() error {
		return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			now := s.now()
			created := now
			if prev, err := store.GetPushRegistrationByDevice(ctx, tx, es, d.ID); err == nil {
				created = prev.CreatedAt
			} else if !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("push: registration: %w", err)
			}
			reg := &store.PushRegistration{
				ID: store.NewID(), DeviceID: d.ID, UserID: d.UserID, Platform: d.Platform, Environment: in.Environment,
				Locale: in.Locale, Topics: in.Topics, TokenCiphertext: ciphertext, TokenHash: HashToken(in.Token),
				CreatedAt: created, UpdatedAt: now,
			}
			if err := store.ReplacePushRegistration(ctx, tx, es, reg); err != nil {
				return err
			}
			if err := s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: p.Actor(""), Action: ActionRegistered,
				EntityType: "device", EntityID: d.ID,
				Diff: map[string]any{"platform": d.Platform, "environment": in.Environment, "locale": in.Locale, "topics": in.Topics},
			}); err != nil {
				return err
			}
			out, err = s.status(ctx, tx, p, es)
			return err
		})
	}
	err = register()
	if errors.Is(err, store.ErrConflict) {
		// A concurrent registration of the same device or token won the
		// unique index: the last writer wins, as for a sequential retry.
		err = register()
	}
	if err != nil {
		return nil, fmt.Errorf("push: register: %w", err)
	}
	return out, nil
}

// Unregister deletes the registration of the device of p (idempotent).
// Audit "device.push_unregistered" when one existed.
func (s *Service) Unregister(ctx context.Context, p *auth.Principal) error {
	es, d, err := s.deviceContext(ctx, s.d.DB, p)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		deleted, err := store.DeletePushRegistrationByDevice(ctx, tx, es, d.ID)
		if err != nil || !deleted {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: ActionUnregistered,
			EntityType: "device", EntityID: d.ID, Diff: map[string]any{"reason": "user"},
		})
	})
}

// HashToken is the SHA-256 hex of a native token (uniqueness per
// establishment without decrypting).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// tokenAAD binds a token ciphertext to its device.
func tokenAAD(deviceID string) []byte { return []byte("push_token:" + deviceID) }
