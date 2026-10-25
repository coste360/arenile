package config

import (
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/pushrelay"
)

func TestParse_PushRelay(t *testing.T) {
	const key = "lido-a.0123456789abcdef0123456789abcdef"
	tests := []struct {
		name       string
		env        map[string]string
		wantErr    bool
		wantURL    string
		configured bool
	}{
		{"default: Coste360 relay, no key, off", nil, false, pushrelay.DefaultURL, false},
		{"key set", map[string]string{EnvPushRelayKey: key}, false, pushrelay.DefaultURL, true},
		{"own relay", map[string]string{EnvPushRelayKey: key, EnvPushRelayURL: "https://push.example.com/v1/notifications"}, false, "https://push.example.com/v1/notifications", true},
		{"loopback test relay over http", map[string]string{EnvPushRelayKey: key, EnvPushRelayURL: "http://127.0.0.1:9/v1"}, false, "http://127.0.0.1:9/v1", true},
		{"desktop edition never offers push", map[string]string{EnvPushRelayKey: key, EnvEdition: EditionDesktop}, false, pushrelay.DefaultURL, false},
		{"plain http to a remote host", map[string]string{EnvPushRelayURL: "http://push.example.com/v1"}, true, "", false},
		{"not a URL", map[string]string{EnvPushRelayURL: "push.example.com"}, true, "", false},
		{"malformed key", map[string]string{EnvPushRelayKey: "lido-a.short"}, true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(env(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				if strings.Contains(err.Error(), "short") || strings.Contains(err.Error(), "0123456789abcdef") {
					t.Fatalf("error reveals the secret: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PushRelayURL != tt.wantURL || cfg.PushRelayConfigured() != tt.configured {
				t.Fatalf("url %q configured %v", cfg.PushRelayURL, cfg.PushRelayConfigured())
			}
		})
	}
}
