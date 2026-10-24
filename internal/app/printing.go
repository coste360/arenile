package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/printing"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/jobs"
)

// wirePrinting builds the ESC/POS network printing service (ADR 0021).
// Printing is available on the desktop edition only (the launcher sets
// ARENILE_EDITION=desktop and passes the control): there the server is in
// the establishment's LAN and reaches the printers. On the server edition
// printers cannot be added and nothing is sent (409 printing_unavailable)
// and the delivery and retention jobs are not registered.
//
// Daily-sale tickets (ADR 0020) print through Tickets: a read-only
// ticketing service (PrintableTicket only reads the ticket and its view).
//
// Extension point: a future bridge driver (desktop client for a VPS,
// mobile Bluetooth) is added to Drivers here; see the printing package doc.
func (a *App) wirePrinting(aw *audit.Writer) {
	a.Printing = printing.NewService(printing.Deps{
		DB: a.db, Clock: a.opts.clock, Audit: aw, Log: a.log,
		Available:     a.cfg.Desktop() || a.opts.desktop != nil,
		AllowLoopback: a.cfg.PrintersAllowLoopback,
		Logos:         printing.HTTPLogoLoader{BaseURL: a.cfg.EffectiveBaseURL()},
		Tickets:       ticketing.NewService(ticketing.Deps{DB: a.db, Clock: a.opts.clock, Audit: aw, Log: a.log}),
	})
}

// printingJobs is the delivery job on the desktop edition (none otherwise).
func (a *App) printingJobs() []jobs.Job { return a.Printing.Jobs() }
