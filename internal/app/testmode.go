package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/testmode"
)

// wireTestMode builds the test-mode service (deferred features A, spec
// 15.4): switch, preview link of the public page and bulk purge. Purges
// release the pool counters through the inventory.
func (a *App) wireTestMode(aw *audit.Writer) {
	a.TestMode = testmode.NewService(testmode.Deps{
		DB: a.db, Clock: a.opts.clock, Audit: aw, Outbox: a.Outbox, Keys: a.keys,
		Inventory: a.Inventory, BaseURL: a.cfg.EffectiveBaseURL(), Log: a.log,
	})
}
