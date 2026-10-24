package online

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Errors of the online package (mapped by internal/http/problem through the
// configerr kinds).
var (
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrBookingDisabled: the establishment exists but online booking is off
	// (the public API answers 404 for every flow endpoint except the profile).
	ErrBookingDisabled = fmt.Errorf("%w: online booking disabled", configerr.ErrNotFound)
	// ErrTermsVersionExists: a terms version with the same label exists.
	ErrTermsVersionExists = fmt.Errorf("%w: versione delle condizioni già pubblicata", configerr.ErrConflict)
	// ErrUnknownProvider: the provider code is not registered.
	ErrUnknownProvider = fmt.Errorf("%w: provider di pagamento sconosciuto", configerr.ErrValidation)
)

// Provider codes and credential keys known to the settings (ADR 0013 §7).
const (
	ProviderManual = "manual"
	ProviderStripe = "stripe"

	CredSecretKey      = "secret_key"
	CredWebhookSecret  = "webhook_secret"
	CredPublishableKey = "publishable_key"
)

// Keys of payment_provider_settings.public_config (non-secret hints).
const (
	pubPublishableKey   = "publishable_key"
	pubSecretLast4      = "secret_key_last4"
	pubWebhookSecretSet = "webhook_secret_configured"
	pubEmbeddedMode     = "embedded_mode"
	pubAccountLabel     = "account_label"
)

// Providers lists the registered payment providers. Implemented by
// *app.Registry.
type Providers interface {
	PaymentProviderCodes() []string
	PaymentProvider(code string) (ext.PaymentProvider, bool)
}

// credentialsVerifier is the optional driver capability used by
// TestPayments (same method set as gateway.CredentialsVerifier, declared
// here to avoid an import cycle: gateway consumes this package).
type credentialsVerifier interface {
	VerifyCredentials(ctx context.Context, creds ext.Credentials) (accountLabel string, err error)
}

// Deps are the dependencies of the online service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// Keys encrypts and decrypts payment provider credentials.
	Keys      *crypto.KeyRing
	Providers Providers
	// BaseURL is config.EffectiveBaseURL() (page URL, embed snippets,
	// webhook URL shown to the manager).
	BaseURL string
	// Transport is the email transport chosen by the server configuration.
	Transport Transport
}

// Service implements the online settings use cases and the read ports used
// by checkout, the payment gateway and notify.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	d.BaseURL = strings.TrimRight(d.BaseURL, "/")
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	return p.Authorize(ctx, s.d.DB, establishmentID, perm)
}

// record writes the audit entry and the config.online_changed event of a
// change in tx (INV-18). diff never contains secret values.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, action, entity, entityID string, version int64, diff map[string]any) error {
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(reqctx.From(ctx).ClientIP), Action: action,
		EntityType: entity, EntityID: entityID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("online: %w", err)
	}
	if _, err := s.d.Outbox.Write(ctx, tx, event.ConfigOnlineChanged, es.ID(), 1, event.ConfigChange{
		Entity: entity, EntityID: entityID, Change: event.ChangeUpdated, Version: version,
	}); err != nil {
		return fmt.Errorf("online: %w", err)
	}
	return nil
}

// settingsRow returns the stored row of es or the defaults (version 1,
// not written: reads never write).
func settingsRow(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*store.OnlineSettings, error) {
	row, err := store.GetOnlineSettings(ctx, db, es)
	if errors.Is(err, store.ErrNotFound) {
		return &store.OnlineSettings{EstablishmentID: es.ID(), Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	return row, nil
}

// lockedSettings ensures the row of es inside tx and checks If-Match.
func (s *Service) lockedSettings(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, expected *int64) (*store.OnlineSettings, error) {
	cfg, _ := json.Marshal(DefaultConfig())
	em, _ := json.Marshal(DefaultEmailConfig())
	row, err := store.EnsureOnlineSettings(ctx, tx, es, cfg, em, s.now())
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	if err := configerr.CheckVersion(expected, row.Version); err != nil {
		return nil, err
	}
	return row, nil
}

// saveSettings writes row with optimistic concurrency.
func (s *Service) saveSettings(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, row *store.OnlineSettings) error {
	old := row.Version
	row.UpdatedAt = s.now()
	ok, err := store.UpdateOnlineSettingsVersioned(ctx, tx, es, row, old)
	if err != nil {
		return fmt.Errorf("online: %w", err)
	}
	if !ok {
		cur, err := store.GetOnlineSettings(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("online: %w", err)
		}
		return &configerr.VersionConflictError{Current: cur.Version}
	}
	return nil
}

// ---- admin use cases -------------------------------------------------------------

// Get returns the online settings (auth.PermBookingsRead).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID string) (*Settings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, s.d.DB, es)
}

func (s *Service) view(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*Settings, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	row, err := settingsRow(ctx, db, es)
	if err != nil {
		return nil, err
	}
	out := &Settings{
		EstablishmentID: est.ID, Slug: est.Slug, BookingEnabled: est.BookingEnabled,
		AllowedEmbedOrigins: slices.Clone(est.AllowedEmbedOrigins), Theme: est.Theme,
		Flow: ParseConfig(row.Config), PageURL: s.PageURL(est.Slug), Version: row.Version,
	}
	if out.AllowedEmbedOrigins == nil {
		out.AllowedEmbedOrigins = []string{}
	}
	if row.CurrentTermsVersionID != nil {
		tv, err := store.GetTermsVersion(ctx, db, es, *row.CurrentTermsVersionID)
		if err == nil {
			out.CurrentTerms = tv
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("online: %w", err)
		}
	}
	return out, nil
}

// PageURL is the public page of slug (<base>/b/<slug>).
func (s *Service) PageURL(slug string) string { return s.d.BaseURL + "/b/" + slug }

// Update replaces the online settings with optimistic concurrency
// (expected = If-Match version, nil = any) (auth.PermEstablishmentConfigure).
// Theme, embedding origins and the booking switch are written on the
// establishment in the same transaction.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in Input) (*Settings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	in, err = ValidateInput(in)
	if err != nil {
		return nil, err
	}
	var out *Settings
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := s.lockedSettings(ctx, tx, es, expected)
		if err != nil {
			return err
		}
		before, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("online: %w", err)
		}
		oldFlow := ParseConfig(row.Config)
		if row.Config, err = store.MarshalJSONValue(in.Flow); err != nil {
			return fmt.Errorf("online: %w", err)
		}
		if err := store.UpdateEstablishmentOnline(ctx, tx, es, in.BookingEnabled, in.AllowedEmbedOrigins, in.Theme, s.now()); err != nil {
			return fmt.Errorf("online: %w", err)
		}
		if err := s.saveSettings(ctx, tx, es, row); err != nil {
			return err
		}
		diff := map[string]any{"changed": changedSettings(before, oldFlow, in), "version": row.Version}
		if err := s.record(ctx, tx, p, es, "online.settings_updated", event.EntityOnlineSettings, es.ID(), row.Version, diff); err != nil {
			return err
		}
		out, err = s.view(ctx, tx, es)
		return err
	})
	return out, err
}

// changedSettings lists the names of the changed fields (audit diff without values).
func changedSettings(before *store.Establishment, oldFlow Config, in Input) []string {
	out := []string{}
	if before.BookingEnabled != in.BookingEnabled {
		out = append(out, "booking_enabled")
	}
	if !slices.Equal(before.AllowedEmbedOrigins, in.AllowedEmbedOrigins) {
		out = append(out, "allowed_embed_origins")
	}
	a, _ := json.Marshal(before.Theme)
	b, _ := json.Marshal(in.Theme)
	if string(a) != string(b) {
		out = append(out, "theme")
	}
	c, _ := json.Marshal(oldFlow)
	d, _ := json.Marshal(in.Flow)
	if string(c) != string(d) {
		out = append(out, "flow")
	}
	return out
}

// ListTerms lists the terms versions, newest first, and the id of the
// current one (auth.PermBookingsRead).
func (s *Service) ListTerms(ctx context.Context, p *auth.Principal, establishmentID string) ([]store.TermsVersion, string, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, "", err
	}
	items, err := store.ListTermsVersions(ctx, s.d.DB, es, 200)
	if err != nil {
		return nil, "", fmt.Errorf("online: %w", err)
	}
	row, err := settingsRow(ctx, s.d.DB, es)
	if err != nil {
		return nil, "", err
	}
	current := ""
	if row.CurrentTermsVersionID != nil {
		current = *row.CurrentTermsVersionID
	}
	return items, current, nil
}

// PublishTerms publishes an immutable terms version, optionally making it
// current (auth.PermEstablishmentConfigure). Bookings keep the label of
// the version their customer accepted.
func (s *Service) PublishTerms(ctx context.Context, p *auth.Principal, establishmentID string, in TermsInput) (*store.TermsVersion, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	in, err = ValidateTerms(in)
	if err != nil {
		return nil, err
	}
	now := s.now()
	tv := &store.TermsVersion{ID: store.NewID(), Version: in.Version, Texts: in.Texts, PublishedAt: now, CreatedAt: now}
	if p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		tv.CreatedByUserID = &u
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertTermsVersion(ctx, tx, es, tv); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrTermsVersionExists
			}
			return fmt.Errorf("online: %w", err)
		}
		version := int64(0)
		if in.MakeCurrent {
			row, err := s.lockedSettings(ctx, tx, es, nil)
			if err != nil {
				return err
			}
			id := tv.ID
			row.CurrentTermsVersionID = &id
			if err := s.saveSettings(ctx, tx, es, row); err != nil {
				return err
			}
			version = row.Version
		}
		locales := make([]string, 0, len(in.Texts))
		for l := range in.Texts {
			locales = append(locales, l)
		}
		slices.Sort(locales)
		return s.record(ctx, tx, p, es, "online.terms_published", event.EntityTermsVersion, tv.ID, version, map[string]any{
			"version": tv.Version, "locales": locales, "current": in.MakeCurrent,
		})
	})
	if err != nil {
		return nil, err
	}
	return tv, nil
}

// ---- payment settings --------------------------------------------------------------

// GetPayments returns the payment settings without secrets (auth.PermPaymentsConfigure).
func (s *Service) GetPayments(ctx context.Context, p *auth.Principal, establishmentID string) (*PaymentSettings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermPaymentsConfigure)
	if err != nil {
		return nil, err
	}
	return s.paymentsView(ctx, s.d.DB, es)
}

func (s *Service) paymentsView(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*PaymentSettings, error) {
	row, err := settingsRow(ctx, db, es)
	if err != nil {
		return nil, err
	}
	out := &PaymentSettings{Provider: ProviderManual, Mode: store.ProviderModeTest, AvailableProviders: s.providerCodes(), Version: row.Version}
	active, err := store.GetActivePaymentProvider(ctx, db, es)
	canPay := false
	switch {
	case err == nil:
		out.Provider, out.Mode = active.Provider, active.Mode
		canPay = active.Credentials != "" && s.providerOnline(active.Provider)
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("online: %w", err)
	}
	stripeRow, err := store.GetPaymentProviderSettings(ctx, db, es, ProviderStripe, out.Mode)
	if errors.Is(err, store.ErrNotFound) && out.Provider != ProviderStripe {
		stripeRow, err = store.LatestPaymentProviderSettings(ctx, db, es, ProviderStripe)
	}
	switch {
	case err == nil:
		out.Stripe = stripeView(stripeRow)
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("online: %w", err)
	}
	on, err := store.TestModeEnabled(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	out.LivePaymentsBlocked = canPay && LivePaymentsBlocked(on, out.Provider, out.Mode)
	out.WebhookURL = s.d.BaseURL + "/webhooks/payments/" + out.Provider + "/" + es.ID()
	return out, nil
}

func (s *Service) providerCodes() []string {
	if s.d.Providers == nil {
		return []string{ProviderManual}
	}
	codes := s.d.Providers.PaymentProviderCodes()
	if !slices.Contains(codes, ProviderManual) {
		codes = append([]string{ProviderManual}, codes...)
	}
	return codes
}

func stripeView(r *store.PaymentProviderSettings) StripeView {
	str := func(k string) string { v, _ := r.PublicConfig[k].(string); return v }
	flag := func(k string) bool { v, _ := r.PublicConfig[k].(bool); return v }
	v := StripeView{
		PublishableKey: str(pubPublishableKey), SecretKeyLast4: str(pubSecretLast4),
		WebhookSecretConfigured: flag(pubWebhookSecretSet), EmbeddedMode: flag(pubEmbeddedMode), VerifiedAt: r.VerifiedAt,
	}
	v.Configured = v.SecretKeyLast4 != "" && v.WebhookSecretConfigured
	return v
}

// UpdatePayments stores the provider choice and the encrypted credentials
// (auth.PermPaymentsConfigure). Secret fields absent from in.Stripe keep
// the stored value, an empty string clears them. Activating Stripe needs a
// secret key and a webhook secret.
func (s *Service) UpdatePayments(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in PaymentSettingsInput) (*PaymentSettings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermPaymentsConfigure)
	if err != nil {
		return nil, err
	}
	in.Provider = strings.TrimSpace(in.Provider)
	if in.Mode == "" {
		in.Mode = store.ProviderModeTest
	}
	if !slices.Contains(s.providerCodes(), in.Provider) {
		return nil, ErrUnknownProvider
	}
	if in.Mode != store.ProviderModeTest && in.Mode != store.ProviderModeLive {
		return nil, configerr.Invalid("mode", "Usa test o live.")
	}
	var out *PaymentSettings
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := s.lockedSettings(ctx, tx, es, expected)
		if err != nil {
			return err
		}
		now := s.now()
		changed := []string{}
		if in.Provider == ProviderManual {
			if err := store.DeactivatePaymentProviders(ctx, tx, es, now); err != nil {
				return fmt.Errorf("online: %w", err)
			}
		} else {
			if changed, err = s.saveProvider(ctx, tx, es, in, now); err != nil {
				return err
			}
		}
		if err := s.saveSettings(ctx, tx, es, row); err != nil {
			return err
		}
		if err := s.record(ctx, tx, p, es, "payments.settings_updated", event.EntityPaymentSettings, es.ID(), row.Version, map[string]any{
			"provider": in.Provider, "mode": in.Mode, "changed": changed,
		}); err != nil {
			return err
		}
		out, err = s.paymentsView(ctx, tx, es)
		return err
	})
	return out, err
}

// saveProvider merges the input into the stored credentials, encrypts them
// and activates the provider row. It returns the names of the changed
// fields (never their values).
func (s *Service) saveProvider(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in PaymentSettingsInput, now time.Time) ([]string, error) {
	creds := ext.Credentials{}
	pub := map[string]any{}
	var verifiedAt *time.Time
	cur, err := store.GetPaymentProviderSettings(ctx, tx, es, in.Provider, in.Mode)
	switch {
	case err == nil:
		if creds, err = s.decrypt(es.ID(), cur); err != nil {
			return nil, err
		}
		for k, v := range cur.PublicConfig {
			pub[k] = v
		}
		verifiedAt = cur.VerifiedAt
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("online: %w", err)
	}
	changed := []string{}
	if in.Provider == ProviderStripe {
		if changed, err = applyStripe(creds, pub, in.Stripe, in.Mode); err != nil {
			return nil, err
		}
		if creds[CredSecretKey] == "" || creds[CredWebhookSecret] == "" {
			return nil, configerr.Invalid("stripe", "Per attivare Stripe servono la chiave segreta e il segreto del webhook.")
		}
	}
	if len(changed) > 0 {
		verifiedAt = nil // new credentials must be verified again
	}
	enc := ""
	if len(creds) > 0 {
		raw, err := json.Marshal(creds)
		if err != nil {
			return nil, fmt.Errorf("online: credentials: %w", err)
		}
		if s.d.Keys == nil {
			return nil, errors.New("online: key ring not configured")
		}
		if enc, err = s.d.Keys.Encrypt(raw, AAD(es.ID(), in.Provider, in.Mode)); err != nil {
			return nil, fmt.Errorf("online: encrypt credentials: %w", err)
		}
	}
	if err := store.DeactivatePaymentProviders(ctx, tx, es, now); err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	row := &store.PaymentProviderSettings{
		Provider: in.Provider, Mode: in.Mode, Active: true, Credentials: enc, PublicConfig: pub,
		VerifiedAt: verifiedAt, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SavePaymentProviderSettings(ctx, tx, es, row); err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	return changed, nil
}

// applyStripe applies a Stripe input to the credentials and the public
// hints: nil = unchanged, "" = clear. Keys are checked against the mode
// (sk_test_/pk_test_ in test, *_live_ in live; rk_ restricted keys allowed).
func applyStripe(creds ext.Credentials, pub map[string]any, in *StripeInput, mode string) ([]string, error) {
	changed := []string{}
	if in == nil {
		return changed, nil
	}
	ve := &configerr.ValidationError{}
	set := func(field, key string, v *string, check func(string) bool, msg string) {
		if v == nil {
			return
		}
		val := strings.TrimSpace(*v)
		if val != "" && !check(val) {
			ve.Add("stripe."+field, msg)
			return
		}
		if creds[key] != val {
			changed = append(changed, field)
		}
		if val == "" {
			delete(creds, key)
		} else {
			creds[key] = val
		}
	}
	env := "_" + mode + "_"
	set("secret_key", CredSecretKey, in.SecretKey, func(v string) bool {
		return (strings.HasPrefix(v, "sk"+env) || strings.HasPrefix(v, "rk"+env)) && len(v) > 12
	}, "Chiave segreta non valida per la modalità scelta (sk_"+mode+"_…).")
	set("webhook_secret", CredWebhookSecret, in.WebhookSecret, func(v string) bool {
		return strings.HasPrefix(v, "whsec_") && len(v) > 10
	}, "Segreto del webhook non valido (whsec_…).")
	set("publishable_key", CredPublishableKey, in.PublishableKey, func(v string) bool {
		return strings.HasPrefix(v, "pk"+env) && len(v) > 12
	}, "Chiave pubblicabile non valida per la modalità scelta (pk_"+mode+"_…).")
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	if in.EmbeddedMode != nil {
		if old, _ := pub[pubEmbeddedMode].(bool); old != *in.EmbeddedMode {
			changed = append(changed, "embedded_mode")
		}
		pub[pubEmbeddedMode] = *in.EmbeddedMode
	}
	pub[pubPublishableKey] = creds[CredPublishableKey]
	pub[pubSecretLast4] = Last4(creds[CredSecretKey])
	pub[pubWebhookSecretSet] = creds[CredWebhookSecret] != ""
	return changed, nil
}

func (s *Service) decrypt(establishmentID string, r *store.PaymentProviderSettings) (ext.Credentials, error) {
	creds := ext.Credentials{}
	if r.Credentials == "" {
		return creds, nil
	}
	if s.d.Keys == nil {
		return nil, errors.New("online: key ring not configured")
	}
	raw, err := s.d.Keys.Decrypt(r.Credentials, AAD(establishmentID, r.Provider, r.Mode))
	if err != nil {
		return nil, fmt.Errorf("online: decrypt credentials of %s: %w", r.Provider, err)
	}
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, fmt.Errorf("online: credentials of %s: %w", r.Provider, err)
	}
	return creds, nil
}

// TestPayments checks the stored credentials with the provider
// (auth.PermPaymentsConfigure). Drivers that cannot check credentials
// answer OK=false with a message.
func (s *Service) TestPayments(ctx context.Context, p *auth.Principal, establishmentID string) (*TestResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermPaymentsConfigure)
	if err != nil {
		return nil, err
	}
	cfg, err := s.ActiveProvider(ctx, s.d.DB, es, false)
	if err != nil {
		return nil, err
	}
	res := &TestResult{Provider: cfg.Code, Mode: cfg.Mode}
	if cfg.Code == ProviderManual {
		res.OK, res.Message = true, "Pagamento alla cassa: nessuna credenziale da verificare."
		return res, nil
	}
	var drv ext.PaymentProvider
	if s.d.Providers != nil {
		drv, _ = s.d.Providers.PaymentProvider(cfg.Code)
	}
	v, ok := drv.(credentialsVerifier)
	if drv == nil || !ok {
		res.Message = "Il provider non consente la verifica delle credenziali."
		return res, nil
	}
	label, err := v.VerifyCredentials(ctx, cfg.Credentials)
	if err != nil {
		// The driver error may describe the request: log the class only.
		s.d.Log.WarnContext(ctx, "payment credentials check failed", slog.String("provider", cfg.Code), slog.String("establishment_id", es.ID()))
		res.Message = "Le credenziali sono state rifiutate dal provider o il provider non è raggiungibile."
		return res, nil
	}
	row, err := store.GetActivePaymentProvider(ctx, s.d.DB, es)
	if err == nil {
		if err := store.MarkPaymentProviderVerified(ctx, s.d.DB, es, row.ID, s.now(), label); err != nil {
			return nil, fmt.Errorf("online: %w", err)
		}
	}
	res.OK, res.AccountLabel, res.Message = true, label, "Credenziali verificate."
	return res, nil
}

// ---- email settings -------------------------------------------------------------------

// GetEmail returns the email settings (auth.PermBookingsRead).
func (s *Service) GetEmail(ctx context.Context, p *auth.Principal, establishmentID string) (*EmailSettings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := settingsRow(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	return &EmailSettings{Transport: s.d.Transport, Config: ParseEmailConfig(row.EmailConfig), Version: row.Version}, nil
}

// UpdateEmail replaces the email settings (auth.PermEstablishmentConfigure).
func (s *Service) UpdateEmail(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in EmailConfig) (*EmailSettings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	in, err = ValidateEmailConfig(in)
	if err != nil {
		return nil, err
	}
	var out *EmailSettings
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		row, err := s.lockedSettings(ctx, tx, es, expected)
		if err != nil {
			return err
		}
		if in.Reminders == nil {
			// Clients that do not send the schedule keep the stored one.
			in.Reminders = ParseEmailConfig(row.EmailConfig).Reminders
		}
		if row.EmailConfig, err = store.MarshalJSONValue(in); err != nil {
			return fmt.Errorf("online: %w", err)
		}
		if err := s.saveSettings(ctx, tx, es, row); err != nil {
			return err
		}
		if err := s.record(ctx, tx, p, es, "online.email_settings_updated", event.EntityEmailSettings, es.ID(), row.Version, map[string]any{
			"reply_to_set": in.ReplyTo != "", "manager_email_set": in.Manager != "",
			"balance_reminder": in.ToCustomer.BalanceReminder, "day_before_reminder": in.ToCustomer.DayBeforeReminder,
			"reminders": in.Schedule(),
		}); err != nil {
			return err
		}
		out = &EmailSettings{Transport: s.d.Transport, Config: in, Version: row.Version}
		return nil
	})
	return out, err
}

// ---- read ports (no principal: public flows and jobs) --------------------

// ResolvePublic resolves a slug for the public flow: ErrEstablishmentNotFound
// for unknown or archived establishments, ErrBookingDisabled when online
// booking is off. Implements checkout.Settings.
func (s *Service) ResolvePublic(ctx context.Context, slug string) (*PublicEstablishment, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || len(slug) > 64 {
		return nil, ErrEstablishmentNotFound
	}
	est, err := store.GetEstablishmentBySlug(ctx, s.d.DB, slug)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrEstablishmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("online: resolve: %w", err)
	}
	if est.ArchivedAt != nil {
		return nil, ErrEstablishmentNotFound
	}
	if !est.BookingEnabled {
		return nil, ErrBookingDisabled
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, est.ID)
	if err != nil {
		return nil, fmt.Errorf("online: resolve: %w", err)
	}
	row, err := settingsRow(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	out := &PublicEstablishment{Scope: es, Establishment: est, Flow: ParseConfig(row.Config), Provider: ProviderManual}
	if out.Terms, err = s.CurrentTerms(ctx, s.d.DB, es); err != nil {
		return nil, err
	}
	on, err := store.TestModeEnabled(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("online: resolve: %w", err)
	}
	active, err := providerForBooking(ctx, s.d.DB, es, on)
	switch {
	case err == nil:
		out.Provider = active.Provider
		out.TestMode = active.Mode == store.ProviderModeTest
		out.Online = s.providerOnline(active.Provider) && active.Credentials != ""
		if active.Provider == ProviderStripe {
			v := stripeView(active)
			out.Online = out.Online && v.Configured
			out.Embedded = v.EmbeddedMode && v.PublishableKey != ""
			if out.Embedded {
				out.PublishableKey = v.PublishableKey
			}
		}
		// Test keys never take the money of a real customer (INV-88): the
		// bookings of the page are real while test mode is off. Only a
		// setup that could pay is reported as blocked.
		if out.Online && LivePaymentsBlocked(on, active.Provider, active.Mode) {
			out.Online, out.LivePaymentsBlocked = false, true
		}
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("online: resolve: %w", err)
	}
	return out, nil
}

func (s *Service) providerOnline(code string) bool {
	if s.d.Providers == nil {
		return false
	}
	drv, ok := s.d.Providers.PaymentProvider(code)
	return ok && drv.Capabilities().Online
}

// ActiveProvider returns the decrypted configuration the payments of a
// booking of es go through; the "manual" configuration when none applies.
// The mode follows the booking, not the test-mode switch (spec 15.4, ADR
// 0019): a real booking (test = false) pays through the active provider
// row whatever the switch says, a test booking through the mode = test row
// of the active provider when that holds credentials, otherwise through the
// manual driver, so that a test booking never charges a real card and a
// real booking is never paid with test keys. Implements
// gateway.ProviderSettings.
func (s *Service) ActiveProvider(ctx context.Context, db bun.IDB, es store.EstablishmentScope, test bool) (*ProviderConfig, error) {
	row, err := providerForBooking(ctx, db, es, test)
	if errors.Is(err, store.ErrNotFound) {
		return manualConfig(es), nil
	}
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	return s.providerConfig(es, row)
}

// ProviderConfig returns the decrypted configuration of provider and mode
// for es, active or not (webhooks and refunds of a provider just switched
// off must still use the credentials that created the payment). An empty
// mode returns the most recent configuration of the provider in any mode.
// store.ErrNotFound when the provider was never configured in that mode.
// Implements gateway.ProviderSettings.
func (s *Service) ProviderConfig(ctx context.Context, db bun.IDB, es store.EstablishmentScope, provider, mode string) (*ProviderConfig, error) {
	if provider == ProviderManual {
		return manualConfig(es), nil
	}
	var (
		row *store.PaymentProviderSettings
		err error
	)
	if mode == "" {
		row, err = store.LatestPaymentProviderSettings(ctx, db, es, provider)
	} else {
		row, err = store.GetPaymentProviderSettings(ctx, db, es, provider, mode)
	}
	if err != nil {
		return nil, err
	}
	if mode != "" && row.Credentials == "" {
		return nil, store.ErrNotFound
	}
	return s.providerConfig(es, row)
}

// providerForBooking is the provider row a booking with the given test flag
// pays through: the active row for a real booking; for a test booking the
// mode = test row of the active provider when that has credentials (the
// active row itself when it is already in test mode), otherwise none
// (ErrNotFound: the manual driver).
func providerForBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, test bool) (*store.PaymentProviderSettings, error) {
	active, err := store.GetActivePaymentProvider(ctx, db, es)
	if err != nil {
		return nil, err
	}
	if !test || active.Mode == store.ProviderModeTest {
		return active, nil
	}
	row, err := store.GetPaymentProviderSettings(ctx, db, es, active.Provider, store.ProviderModeTest)
	if errors.Is(err, store.ErrNotFound) || (err == nil && row.Credentials == "") {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	return row, nil
}

func manualConfig(es store.EstablishmentScope) *ProviderConfig {
	return &ProviderConfig{EstablishmentID: es.ID(), Code: ProviderManual, Mode: store.ProviderModeLive, Credentials: ext.Credentials{}}
}

func (s *Service) providerConfig(es store.EstablishmentScope, row *store.PaymentProviderSettings) (*ProviderConfig, error) {
	creds, err := s.decrypt(es.ID(), row)
	if err != nil {
		return nil, err
	}
	out := &ProviderConfig{
		EstablishmentID: es.ID(), Code: row.Provider, Mode: row.Mode, Credentials: creds,
		Online: s.providerOnline(row.Provider),
	}
	if row.Provider == ProviderStripe {
		v := stripeView(row)
		out.Embedded = v.EmbeddedMode && v.PublishableKey != ""
		out.PublishableKey = v.PublishableKey
	}
	return out, nil
}

// CurrentTerms returns the current terms version of es (nil, nil when the
// establishment has none). Implements checkout.Settings.
func (s *Service) CurrentTerms(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*store.TermsVersion, error) {
	row, err := settingsRow(ctx, db, es)
	if err != nil {
		return nil, err
	}
	if row.CurrentTermsVersionID == nil {
		return nil, nil
	}
	tv, err := store.GetTermsVersion(ctx, db, es, *row.CurrentTermsVersionID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("online: %w", err)
	}
	return tv, nil
}

// EmailConfigFor returns the email settings of es with defaults applied
// (sender name = establishment name when empty). Implements notify.Settings.
func (s *Service) EmailConfigFor(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*EmailConfig, error) {
	row, err := settingsRow(ctx, db, es)
	if err != nil {
		return nil, err
	}
	c := ParseEmailConfig(row.EmailConfig)
	if c.SenderName == "" {
		if est, err := store.GetEstablishment(ctx, db, es); err == nil {
			c.SenderName = est.Name
		}
	}
	return &c, nil
}
