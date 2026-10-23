package config

import (
	"errors"
	"testing"
)

func TestParse_WebhooksAllowPrivate(t *testing.T) {
	key := validKey(1)
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"production default", map[string]string{}, false},
		{"dev default", map[string]string{EnvDev: "true", EnvSecretKey: key}, true},
		{"production LAN", map[string]string{EnvWebhooksAllowPrivate: "true"}, true},
		{"dev strict", map[string]string{EnvDev: "true", EnvSecretKey: key, EnvWebhooksAllowPrivate: "false"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(env(tt.env))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.WebhooksAllowPrivate != tt.want {
				t.Fatalf("WebhooksAllowPrivate = %v, want %v", cfg.WebhooksAllowPrivate, tt.want)
			}
		})
	}
	if _, err := Parse(env(map[string]string{EnvWebhooksAllowPrivate: "maybe"})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid bool: %v", err)
	}
}
