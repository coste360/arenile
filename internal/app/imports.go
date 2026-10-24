package app

import (
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/importer"
	"github.com/coste360/arenile/internal/jobs"
)

// wireImports builds the CSV import service (deferred features F): the
// customer and pass services are bound to the import transaction, so a
// whole file is applied all or nothing (each row in a savepoint).
func (a *App) wireImports(aw *audit.Writer) {
	a.Imports = importer.NewService(importer.Deps{
		DB: a.db, Clock: a.opts.clock, Audit: aw, Log: a.log,
		Customers: func(db bun.IDB) importer.CustomerWriter { return a.Customers.WithDB(db) },
		Passes:    func(db bun.IDB) importer.PassWriter { return a.Passes.WithDB(db) },
	})
}

// importJobs is the retention job of the uploaded files (7 days).
func (a *App) importJobs() []jobs.Job { return a.Imports.Jobs() }
