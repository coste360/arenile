package app

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/domain/printing"
)

// TestWirePrinting_Editions: ESC/POS printing and its delivery job exist
// only on the desktop edition (ARENILE_EDITION=desktop, as the launcher and
// the Playwright desktop simulation set it).
func TestWirePrinting_Editions(t *testing.T) {
	for _, tt := range []struct {
		edition string
		want    bool
	}{{config.EditionServer, false}, {config.EditionDesktop, true}} {
		t.Run(tt.edition, func(t *testing.T) {
			cfg := devConfig(t)
			cfg.Edition = tt.edition
			a, err := New(cfg, WithoutSignalHandling(), WithLogger(slog.New(slog.DiscardHandler)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.Close() })
			if a.Printing == nil || a.Printing.Available() != tt.want {
				t.Fatalf("printing available = %v, want %v", a.Printing != nil && a.Printing.Available(), tt.want)
			}
			if got := slices.Contains(a.Jobs().Jobs(), printing.JobDeliver); got != tt.want {
				t.Fatalf("delivery job registered = %v, want %v", got, tt.want)
			}
		})
	}
}
