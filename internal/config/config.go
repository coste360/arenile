// Package config parses the Arenile configuration from environment variables
// (specification section 24.4). Parsing is pure apart from the development
// secret key bootstrap, which reads/writes a key file in the data directory.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/crypto"
)

// Environment variable names (spec 24.4). ARENILE_DEV is an Arenile
// extension that relaxes production-only requirements for local development.
const (
	EnvAddr               = "ARENILE_ADDR"
	EnvBaseURL            = "ARENILE_BASE_URL"
	EnvDataDir            = "ARENILE_DATA_DIR"
	EnvDatabaseURL        = "DATABASE_URL"
	EnvSecretKey          = "ARENILE_SECRET_KEY"
	EnvAutoMigrate        = "ARENILE_AUTO_MIGRATE"
	EnvTLSAutocertDomains = "ARENILE_TLS_AUTOCERT_DOMAINS"
	EnvSMTPURL            = "ARENILE_SMTP_URL"
	EnvMailFrom           = "ARENILE_MAIL_FROM"
	// EnvMailDir is an Arenile extension of spec 24.4 (M4): without
	// ARENILE_SMTP_URL, emails are written as .eml files into this
	// directory (local development, end-to-end tests) instead of only
	// being logged.
	EnvMailDir = "ARENILE_MAIL_DIR"
	// EnvStripeAPIURL is an Arenile extension of spec 24.4 (M4): the base
	// URL of the Stripe API, only for test doubles (stripe-mock, the fake
	// used by the end-to-end tests). Empty = https://api.stripe.com.
	EnvStripeAPIURL = "ARENILE_STRIPE_API_URL"
	EnvHoldTTL      = "ARENILE_HOLD_TTL"
	// EnvPublicReadsPerMinute and EnvPublicCreationsPerMinute are an
	// Arenile extension of spec 24.4 (M4): the per-IP limits of the public
	// API (spec 17.2 defaults 60 and 10), for lidos whose customers share
	// one address (hotel or beach Wi-Fi) and for automated runs.
	EnvPublicReadsPerMinute     = "ARENILE_PUBLIC_READS_PER_MINUTE"
	EnvPublicCreationsPerMinute = "ARENILE_PUBLIC_CREATIONS_PER_MINUTE"
	EnvLogLevel                 = "ARENILE_LOG_LEVEL"
	EnvTrustedProxies           = "ARENILE_TRUSTED_PROXIES"
	EnvRequireTOTPForOwners     = "ARENILE_REQUIRE_TOTP_FOR_OWNERS"
	EnvTelemetry                = "ARENILE_TELEMETRY"
	EnvDev                      = "ARENILE_DEV"
	// EnvEdition is set by the desktop launcher (never by an operator):
	// "desktop" is the loopback profile of the core, see Config.Desktop.
	EnvEdition = "ARENILE_EDITION"
	// EnvWebhooksAllowPrivate (M5A, spec 22) lets outgoing webhooks reach
	// loopback and private addresses, for self-hosted LAN receivers.
	// Default: true with ARENILE_DEV, false otherwise.
	EnvWebhooksAllowPrivate = "ARENILE_WEBHOOKS_ALLOW_PRIVATE"
	// EnvPrintersAllowLoopback (ADR 0021) lets a network printer have a
	// loopback address (a fake or virtual printer on this PC): development
	// and tests only. Default: true with ARENILE_DEV, false otherwise.
	EnvPrintersAllowLoopback = "ARENILE_PRINTERS_ALLOW_LOOPBACK"

	// First-owner bootstrap (ADR 0006): when all four are set and the
	// database has no users, `serve` creates the owner once at startup.
	// Needed on distroless images, which have no shell for `admin create`.
	EnvBootstrapOwnerEmail = "ARENILE_BOOTSTRAP_OWNER_EMAIL"
	EnvBootstrapOwnerName  = "ARENILE_BOOTSTRAP_OWNER_NAME"
	EnvBootstrapOwnerOrg   = "ARENILE_BOOTSTRAP_OWNER_ORG"
	EnvAdminPassword       = "ARENILE_ADMIN_PASSWORD" //nolint:gosec // G101: environment variable name, not a credential
)

// Editions (EnvEdition).
const (
	EditionServer  = "server"
	EditionDesktop = "desktop"
)

// DevKeyFile is the file, inside the data directory, where a generated
// development secret key is persisted so that it survives restarts.
const DevKeyFile = "dev-secret.key"

// ErrInvalid wraps every validation error.
var ErrInvalid = errors.New("config: invalid configuration")

// Config is the fully parsed configuration.
type Config struct {
	Addr               string
	BaseURL            string
	DataDir            string
	DatabaseURL        string
	SecretKeys         [][]byte
	AutoMigrate        bool
	TLSAutocertDomains []string
	SMTPURL            string
	MailFrom           string
	MailDir            string
	StripeAPIURL       string
	HoldTTL            time.Duration
	// PublicReadsPerMinute / PublicCreationsPerMinute: per-IP limits of the
	// public API (spec 17.2).
	PublicReadsPerMinute     int
	PublicCreationsPerMinute int
	LogLevel                 slog.Level
	TrustedProxies           []netip.Prefix
	RequireTOTPForOwners     bool
	Telemetry                bool
	Dev                      bool
	// Edition is EditionServer (default) or EditionDesktop: the desktop
	// launcher serves the panel on http://127.0.0.1 and needs no public
	// base URL; cookies get Secure per request (TLS LAN listener). It is
	// not a development switch: Dev stays off on shipped desktop installs.
	Edition string
	// WebhooksAllowPrivate disables the private-address part of the SSRF
	// protection of outgoing webhooks (event.EgressPolicy.AllowPrivate).
	WebhooksAllowPrivate bool
	// PrintersAllowLoopback admits loopback printer addresses
	// (printing.AddressPolicy.AllowLoopback).
	PrintersAllowLoopback bool

	// BootstrapOwner is the optional first owner created by `serve` when the
	// database has no users. Zero value means disabled.
	BootstrapOwner BootstrapOwner

	// GeneratedSecretKey is set (base64) only when a development key was just
	// generated. The caller must show it once on stderr, never through slog.
	GeneratedSecretKey string
}

// BootstrapOwner describes the first owner account created at startup.
type BootstrapOwner struct {
	Email        string
	Name         string
	Organization string
	Password     string
}

// Enabled reports whether a bootstrap owner is configured.
func (b BootstrapOwner) Enabled() bool { return b.Email != "" }

// LogValue never exposes the password.
func (b BootstrapOwner) LogValue() slog.Value {
	return slog.GroupValue(slog.String("email", b.Email), slog.String("organization", b.Organization))
}

// Lookup returns an environment value and whether it was set.
type Lookup func(key string) (string, bool)

// FromEnv parses the configuration from the process environment.
func FromEnv() (Config, error) { return Parse(os.LookupEnv) }

// Parse parses the configuration using lookup. It applies defaults and
// validates every value; it does not enforce serve-only requirements (see
// ValidateServe), so that CLI maintenance commands work with a partial env.
func Parse(lookup Lookup) (Config, error) {
	get := func(k, def string) string {
		if v, ok := lookup(k); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	var errs []error
	cfg := Config{
		Addr:         get(EnvAddr, ":8080"),
		BaseURL:      strings.TrimRight(get(EnvBaseURL, ""), "/"),
		DataDir:      get(EnvDataDir, "/data"),
		DatabaseURL:  get(EnvDatabaseURL, ""),
		SMTPURL:      get(EnvSMTPURL, ""),
		MailFrom:     get(EnvMailFrom, ""),
		MailDir:      get(EnvMailDir, ""),
		StripeAPIURL: strings.TrimRight(get(EnvStripeAPIURL, ""), "/"),
	}

	var err error
	if cfg.Dev, err = parseBool(get(EnvDev, "false")); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvDev, err))
	}
	switch cfg.Edition = strings.ToLower(get(EnvEdition, EditionServer)); cfg.Edition {
	case EditionServer, EditionDesktop:
	default:
		errs = append(errs, fmt.Errorf("%s: must be %q or %q", EnvEdition, EditionServer, EditionDesktop))
	}
	if cfg.AutoMigrate, err = parseBool(get(EnvAutoMigrate, "true")); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvAutoMigrate, err))
	}
	if cfg.RequireTOTPForOwners, err = parseBool(get(EnvRequireTOTPForOwners, "false")); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvRequireTOTPForOwners, err))
	}
	if cfg.Telemetry, err = parseBool(get(EnvTelemetry, "false")); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvTelemetry, err))
	}
	if cfg.WebhooksAllowPrivate, err = parseBool(get(EnvWebhooksAllowPrivate, strconv.FormatBool(cfg.Dev))); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvWebhooksAllowPrivate, err))
	}
	if cfg.PrintersAllowLoopback, err = parseBool(get(EnvPrintersAllowLoopback, strconv.FormatBool(cfg.Dev))); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvPrintersAllowLoopback, err))
	}
	if cfg.HoldTTL, err = time.ParseDuration(get(EnvHoldTTL, "10m")); err != nil || cfg.HoldTTL <= 0 {
		errs = append(errs, fmt.Errorf("%s: must be a positive duration like 10m", EnvHoldTTL))
	}
	for _, l := range []struct {
		env string
		def string
		dst *int
	}{
		{EnvPublicReadsPerMinute, "60", &cfg.PublicReadsPerMinute},
		{EnvPublicCreationsPerMinute, "10", &cfg.PublicCreationsPerMinute},
	} {
		n, perr := strconv.Atoi(get(l.env, l.def))
		if perr != nil || n < 1 || n > 1_000_000 {
			errs = append(errs, fmt.Errorf("%s: must be a whole number of requests per minute between 1 and 1000000", l.env))
			continue
		}
		*l.dst = n
	}
	if cfg.LogLevel, err = parseLevel(get(EnvLogLevel, "info")); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvLogLevel, err))
	}
	if v := get(EnvTLSAutocertDomains, ""); v != "" {
		for _, d := range strings.Split(v, ",") {
			if d = strings.TrimSpace(d); d != "" {
				cfg.TLSAutocertDomains = append(cfg.TLSAutocertDomains, strings.ToLower(d))
			}
		}
	}
	if v := get(EnvTrustedProxies, ""); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			pref, perr := parsePrefix(p)
			if perr != nil {
				errs = append(errs, fmt.Errorf("%s: %q is not a valid CIDR", EnvTrustedProxies, p))
				continue
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, pref)
		}
	}
	if cfg.BaseURL != "" {
		u, uerr := url.Parse(cfg.BaseURL)
		if uerr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s: must be an absolute http(s) URL", EnvBaseURL))
		}
	}
	if cfg.DatabaseURL != "" && !cfg.UsesPostgres() {
		errs = append(errs, fmt.Errorf("%s: only postgres:// or postgresql:// URLs are supported", EnvDatabaseURL))
	}
	if cfg.SMTPURL != "" {
		u, uerr := url.Parse(cfg.SMTPURL)
		if uerr != nil || (u.Scheme != "smtp" && u.Scheme != "smtps") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s: must look like smtp://user:pass@host:587", EnvSMTPURL))
		}
	}
	if cfg.StripeAPIURL != "" {
		u, uerr := url.Parse(cfg.StripeAPIURL)
		switch {
		case uerr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
			errs = append(errs, fmt.Errorf("%s: must be an absolute http(s) URL", EnvStripeAPIURL))
		case !isLoopbackHost(u.Hostname()) && !cfg.Dev:
			// The secret keys of the establishments would be sent to that
			// host: test doubles only (loopback), or an explicit dev run.
			errs = append(errs, fmt.Errorf("%s: a Stripe API double is accepted only on a loopback host or with %s=true", EnvStripeAPIURL, EnvDev))
		case !isLoopbackHost(u.Hostname()) && u.Scheme != "https":
			errs = append(errs, fmt.Errorf("%s: must use https unless the host is loopback", EnvStripeAPIURL))
		}
	}
	if cfg.MailFrom != "" {
		if _, aerr := mail.ParseAddress(cfg.MailFrom); aerr != nil {
			errs = append(errs, fmt.Errorf("%s: must be an email address like prenotazioni@mio-lido.it", EnvMailFrom))
		}
	}
	if v := get(EnvSecretKey, ""); v != "" {
		keys, kerr := crypto.ParseKeys(v)
		if kerr != nil {
			errs = append(errs, fmt.Errorf("%s: must be one or more comma-separated base64 32-byte keys: %w", EnvSecretKey, kerr))
		}
		cfg.SecretKeys = keys
	}

	if v := get(EnvBootstrapOwnerEmail, ""); v != "" {
		cfg.BootstrapOwner = BootstrapOwner{
			Email:        v,
			Name:         get(EnvBootstrapOwnerName, ""),
			Organization: get(EnvBootstrapOwnerOrg, ""),
		}
		// The password is taken verbatim: leading/trailing spaces are legal.
		if pw, ok := lookup(EnvAdminPassword); ok {
			cfg.BootstrapOwner.Password = pw
		}
		if cfg.BootstrapOwner.Name == "" || cfg.BootstrapOwner.Organization == "" || cfg.BootstrapOwner.Password == "" {
			errs = append(errs, fmt.Errorf("%s requires %s, %s and %s", EnvBootstrapOwnerEmail, EnvBootstrapOwnerName, EnvBootstrapOwnerOrg, EnvAdminPassword))
		}
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, errors.Join(errs...))
	}

	if len(cfg.SecretKeys) == 0 && cfg.Dev {
		if err := cfg.bootstrapDevKey(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// Desktop reports whether this process is the desktop edition (loopback
// profile: internal/desktop.DesktopConfig).
func (c Config) Desktop() bool { return c.Edition == EditionDesktop }

// ValidateServe enforces the requirements that only apply to a running
// server in production: a public base URL and a secret key. The desktop
// edition always has a loopback base URL, so only Dev relaxes it here.
func (c Config) ValidateServe() error {
	var errs []error
	if c.BaseURL == "" && !c.Dev && !c.Desktop() {
		errs = append(errs, fmt.Errorf("%s is required in production (set %s=true for local development)", EnvBaseURL, EnvDev))
	}
	if len(c.SecretKeys) == 0 {
		errs = append(errs, fmt.Errorf("%s is required: generate one with `openssl rand -base64 32`", EnvSecretKey))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalid, errors.Join(errs...))
	}
	return nil
}

// UsesPostgres reports whether DATABASE_URL selects PostgreSQL.
func (c Config) UsesPostgres() bool {
	return strings.HasPrefix(c.DatabaseURL, "postgres://") || strings.HasPrefix(c.DatabaseURL, "postgresql://")
}

// SQLitePath is the SQLite database file inside the data directory.
func (c Config) SQLitePath() string { return filepath.Join(c.DataDir, "arenile.db") }

// SecureCookies reports whether cookies must carry the Secure attribute by
// default. It is false only in development or on the desktop edition with
// a non-https (or empty) base URL; a request over TLS still gets Secure
// cookies (middleware.CookieSecure).
func (c Config) SecureCookies() bool {
	if !c.Dev && !c.Desktop() {
		return true
	}
	return strings.HasPrefix(c.BaseURL, "https://")
}

// EffectiveMailFrom returns ARENILE_MAIL_FROM, or "noreply@<host of the
// base URL>" when it is not set (M4 transactional emails).
func (c Config) EffectiveMailFrom() string {
	if c.MailFrom != "" {
		return c.MailFrom
	}
	host := "localhost"
	if u, err := url.Parse(c.EffectiveBaseURL()); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	return "noreply@" + host
}

// EffectiveBaseURL returns BaseURL, or a localhost URL derived from Addr in dev.
func (c Config) EffectiveBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	addr := c.Addr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	return "http://" + addr
}

func (c *Config) bootstrapDevKey() error {
	path := filepath.Join(c.DataDir, DevKeyFile)
	if b, err := os.ReadFile(path); err == nil {
		keys, kerr := crypto.ParseKeys(strings.TrimSpace(string(b)))
		if kerr != nil {
			return fmt.Errorf("%w: development key file %s is corrupt: %w", ErrInvalid, path, kerr)
		}
		c.SecretKeys = keys
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: read dev key: %w", err)
	}
	k, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.DataDir, 0o750); err != nil {
		return fmt.Errorf("config: create data dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(k+"\n"), 0o600); err != nil {
		return fmt.Errorf("config: write dev key: %w", err)
	}
	keys, err := crypto.ParseKeys(k)
	if err != nil {
		return err
	}
	c.SecretKeys = keys
	c.GeneratedSecretKey = k
	return nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b, nil
	}
	return false, fmt.Errorf("invalid boolean %q", s)
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid level %q (debug, info, warn, error)", s)
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		return p.Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// isLoopbackHost reports whether h is localhost, *.localhost or a loopback
// address.
func isLoopbackHost(h string) bool {
	h = strings.ToLower(strings.Trim(h, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
