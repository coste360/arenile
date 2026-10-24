package config

import (
	"errors"
	"testing"
)

func TestParse_PrintersAllowLoopback(t *testing.T) {
	key := validKey(1)
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"production default", map[string]string{}, false},
		{"dev default", map[string]string{EnvDev: "true", EnvSecretKey: key}, true},
		{"explicit", map[string]string{EnvPrintersAllowLoopback: "true"}, true},
		{"dev strict", map[string]string{EnvDev: "true", EnvSecretKey: key, EnvPrintersAllowLoopback: "false"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(env(tt.env))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PrintersAllowLoopback != tt.want {
				t.Fatalf("PrintersAllowLoopback = %v, want %v", cfg.PrintersAllowLoopback, tt.want)
			}
		})
	}
	if _, err := Parse(env(map[string]string{EnvPrintersAllowLoopback: "maybe"})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid bool: %v", err)
	}
}
