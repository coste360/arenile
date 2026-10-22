// Package config parses the Arenile configuration from environment variables
// (specification section 24.4). Parsing is pure apart from the development
// secret key bootstrap, which reads/writes a key file in the data directory.
package config

import (
	"errors"
	"fmt"
	"log/slog"
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
	EnvAddr                 = "ARENILE_ADDR"
	EnvBaseURL              = "ARENILE_BASE_URL"
	EnvDataDir              = "ARENILE_DATA_DIR"
	EnvDatabaseURL          = "DATABASE_URL"
	EnvSecretKey            = "ARENILE_SECRET_KEY"
	EnvAutoMigrate          = "ARENILE_AUTO_MIGRATE"
	EnvTLSAutocertDomains   = "ARENILE_TLS_AUTOCERT_DOMAINS"
	EnvSMTPURL              = "ARENILE_SMTP_URL"
	EnvMailFrom             = "ARENILE_MAIL_FROM"
	EnvHoldTTL              = "ARENILE_HOLD_TTL"
	EnvLogLevel             = "ARENILE_LOG_LEVEL"
	EnvTrustedProxies       = "ARENILE_TRUSTED_PROXIES"
	EnvRequireTOTPForOwners = "ARENILE_REQUIRE_TOTP_FOR_OWNERS"
	EnvTelemetry            = "ARENILE_TELEMETRY"
	EnvDev                  = "ARENILE_DEV"

	// First-owner bootstrap (ADR 0006): when all four are set and the
	// database has no users, `serve` creates the owner once at startup.
	// Needed on distroless images, which have no shell for `admin create`.
	EnvBootstrapOwnerEmail = "ARENILE_BOOTSTRAP_OWNER_EMAIL"
	EnvBootstrapOwnerName  = "ARENILE_BOOTSTRAP_OWNER_NAME"
	EnvBootstrapOwnerOrg   = "ARENILE_BOOTSTRAP_OWNER_ORG"
	EnvAdminPassword       = "ARENILE_ADMIN_PASSWORD" //nolint:gosec // G101: environment variable name, not a credential
)

// DevKeyFile is the file, inside the data directory, where a generated
// development secret key is persisted so that it survives restarts.
const DevKeyFile = "dev-secret.key"

// ErrInvalid wraps every validation error.
var ErrInvalid = errors.New("config: invalid configuration")

// Config is the fully parsed configuration.
type Config struct {
	Addr                 string
	BaseURL              string
	DataDir              string
	DatabaseURL          string
	SecretKeys           [][]byte
	AutoMigrate          bool
	TLSAutocertDomains   []string
	SMTPURL              string
	MailFrom             string
	HoldTTL              time.Duration
	LogLevel             slog.Level
	TrustedProxies       []netip.Prefix
	RequireTOTPForOwners bool
	Telemetry            bool
	Dev                  bool

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
		Addr:        get(EnvAddr, ":8080"),
		BaseURL:     strings.TrimRight(get(EnvBaseURL, ""), "/"),
		DataDir:     get(EnvDataDir, "/data"),
		DatabaseURL: get(EnvDatabaseURL, ""),
		SMTPURL:     get(EnvSMTPURL, ""),
		MailFrom:    get(EnvMailFrom, ""),
	}

	var err error
	if cfg.Dev, err = parseBool(get(EnvDev, "false")); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvDev, err))
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
	if cfg.HoldTTL, err = time.ParseDuration(get(EnvHoldTTL, "10m")); err != nil || cfg.HoldTTL <= 0 {
		errs = append(errs, fmt.Errorf("%s: must be a positive duration like 10m", EnvHoldTTL))
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

// ValidateServe enforces the requirements that only apply to a running
// server in production: a public base URL and a secret key.
func (c Config) ValidateServe() error {
	var errs []error
	if c.BaseURL == "" && !c.Dev {
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

// SecureCookies reports whether cookies must carry the Secure attribute.
// It is false only in development with a non-https (or empty) base URL.
func (c Config) SecureCookies() bool {
	if !c.Dev {
		return true
	}
	return strings.HasPrefix(c.BaseURL, "https://")
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
