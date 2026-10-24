package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/printing"
	"github.com/coste360/arenile/internal/jobs"
)

// wirePrinting builds the ESC/POS network printing service (ADR 0021).
// Printing is available on the desktop edition only (the launcher sets
// ARENILE_EDITION=desktop and passes the control): there the server is in
// the establishment's LAN and reaches the printers. On the server edition
// the configuration still works but nothing is sent (409
// printing_unavailable) and the delivery job is not registered.
//
// Extension point: a future bridge driver (desktop client for a VPS,
// mobile Bluetooth) is added to Drivers here; see the printing package doc.
func (a *App) wirePrinting(aw *audit.Writer) {
	a.Printing = printing.NewService(printing.Deps{
		DB: a.db, Clock: a.opts.clock, Audit: aw, Log: a.log,
		Available: a.cfg.Desktop() || a.opts.desktop != nil,
		Logos:     printing.HTTPLogoLoader{BaseURL: a.cfg.EffectiveBaseURL()},
	})
}

// printingJobs is the delivery job on the desktop edition (none otherwise).
func (a *App) printingJobs() []jobs.Job { return a.Printing.Jobs() }
