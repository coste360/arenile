package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func env(m map[string]string) Lookup {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func validKey(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
}

func TestParse_Defaults(t *testing.T) {
	cfg, err := Parse(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.DataDir != "/data" || !cfg.AutoMigrate || cfg.HoldTTL != 10*time.Minute ||
		cfg.LogLevel != slog.LevelInfo || cfg.Telemetry || cfg.RequireTOTPForOwners || cfg.Dev {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.UsesPostgres() {
		t.Fatal("default must be SQLite")
	}
	if cfg.SQLitePath() != filepath.FromSlash("/data/arenile.db") {
		t.Fatal(cfg.SQLitePath())
	}
	if err := cfg.ValidateServe(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("serve without base url and key must fail, got %v", err)
	}
}

func TestParse_AllValues(t *testing.T) {
	cfg, err := Parse(env(map[string]string{
		EnvAddr:                 "127.0.0.1:9000",
		EnvBaseURL:              "https://prenota.lido.it/",
		EnvDataDir:              "/var/lib/arenile",
		EnvDatabaseURL:          "postgres://u:p@db:5432/arenile?sslmode=disable",
		EnvSecretKey:            validKey(1) + "," + validKey(2),
		EnvAutoMigrate:          "false",
		EnvTLSAutocertDomains:   "Prenota.Lido.it, www.lido.it",
		EnvSMTPURL:              "smtp://user:pass@mail:587",
		EnvMailFrom:             "Lido <noreply@lido.it>",
		EnvHoldTTL:              "15m",
		EnvLogLevel:             "debug",
		EnvTrustedProxies:       "10.0.0.0/8, 192.168.1.1",
		EnvRequireTOTPForOwners: "true",
		EnvTelemetry:            "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://prenota.lido.it" {
		t.Fatalf("base url not normalised: %q", cfg.BaseURL)
	}
	if !cfg.UsesPostgres() || cfg.AutoMigrate || cfg.HoldTTL != 15*time.Minute || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected: %+v", cfg)
	}
	if len(cfg.SecretKeys) != 2 || len(cfg.TLSAutocertDomains) != 2 || cfg.TLSAutocertDomains[0] != "prenota.lido.it" {
		t.Fatalf("lists: %+v", cfg)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[1].Bits() != 32 {
		t.Fatalf("proxies: %v", cfg.TrustedProxies)
	}
	if !cfg.RequireTOTPForOwners || !cfg.SecureCookies() {
		t.Fatal("flags")
	}
	if err := cfg.ValidateServe(); err != nil {
		t.Fatal(err)
	}
}

func TestParse_Invalid(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"bad bool", map[string]string{EnvAutoMigrate: "maybe"}},
		{"bad ttl", map[string]string{EnvHoldTTL: "ten"}},
		{"negative ttl", map[string]string{EnvHoldTTL: "-1m"}},
		{"bad level", map[string]string{EnvLogLevel: "verbose"}},
		{"bad cidr", map[string]string{EnvTrustedProxies: "10.0.0.0/99"}},
		{"bad base url", map[string]string{EnvBaseURL: "prenota.lido.it"}},
		{"mysql url", map[string]string{EnvDatabaseURL: "mysql://x"}},
		{"bad smtp", map[string]string{EnvSMTPURL: "http://x"}},
		{"short key", map[string]string{EnvSecretKey: base64.StdEncoding.EncodeToString([]byte("too short"))}},
		{"non base64 key", map[string]string{EnvSecretKey: "%%%"}},
		{"bootstrap owner without password", map[string]string{EnvBootstrapOwnerEmail: "a@b.it", EnvBootstrapOwnerName: "A", EnvBootstrapOwnerOrg: "Lido"}},
		{"bootstrap owner without org", map[string]string{EnvBootstrapOwnerEmail: "a@b.it", EnvBootstrapOwnerName: "A", EnvAdminPassword: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(env(tt.env)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestParse_DevGeneratesKeyOnce(t *testing.T) {
	dir := t.TempDir()
	e := env(map[string]string{EnvDev: "true", EnvDataDir: dir})
	cfg, err := Parse(e)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GeneratedSecretKey == "" || len(cfg.SecretKeys) != 1 {
		t.Fatal("dev must generate a key")
	}
	st, err := os.Stat(filepath.Join(dir, DevKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("key file perms %v", st.Mode().Perm())
	}
	if err := cfg.ValidateServe(); err != nil {
		t.Fatalf("dev serve must be valid without base url: %v", err)
	}
	if cfg.SecureCookies() {
		t.Fatal("dev over http must not force Secure cookies")
	}
	again, err := Parse(e)
	if err != nil {
		t.Fatal(err)
	}
	if again.GeneratedSecretKey != "" {
		t.Fatal("key must be printed only the first time")
	}
	if !bytes.Equal(again.SecretKeys[0], cfg.SecretKeys[0]) {
		t.Fatal("persisted key must be reused")
	}
}

func TestParse_ProdDoesNotGenerateKey(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Parse(env(map[string]string{EnvDataDir: dir, EnvBaseURL: "https://x.it"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SecretKeys) != 0 || cfg.GeneratedSecretKey != "" {
		t.Fatal("production must never generate a key")
	}
	if err := cfg.ValidateServe(); err == nil {
		t.Fatal("missing key must fail in production")
	}
}
