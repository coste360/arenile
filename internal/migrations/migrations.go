// Package migrations embeds the SQL migrations of both dialects and runs them
// with goose. Every migration exists in sqlite/ and postgres/ with the same
// number; they are forward-only in production.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed sqlite/*.sql postgres/*.sql
var files embed.FS

// Dialect selects the migration set.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// Status is the state of one migration.
type Status struct {
	Version   int64
	Name      string
	Applied   bool
	AppliedAt string
}

// Runner applies and inspects migrations on a database.
type Runner struct {
	provider *goose.Provider
}

// FS returns the embedded migrations of a dialect (used by tests).
func FS(d Dialect) (fs.FS, error) {
	switch d {
	case SQLite, Postgres:
		return fs.Sub(files, string(d))
	}
	return nil, fmt.Errorf("migrations: unknown dialect %q", d)
}

// NewRunner builds a Runner for db.
func NewRunner(db *sql.DB, d Dialect) (*Runner, error) {
	fsys, err := FS(d)
	if err != nil {
		return nil, err
	}
	gd := goose.DialectSQLite3
	if d == Postgres {
		gd = goose.DialectPostgres
	}
	p, err := goose.NewProvider(gd, db, fsys)
	if err != nil {
		return nil, fmt.Errorf("migrations: new provider: %w", err)
	}
	return &Runner{provider: p}, nil
}

// Up applies all pending migrations and returns the versions applied.
func (r *Runner) Up(ctx context.Context) ([]int64, error) {
	res, err := r.provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrations: up: %w", err)
	}
	out := make([]int64, 0, len(res))
	for _, m := range res {
		out = append(out, m.Source.Version)
	}
	return out, nil
}

// Status lists every known migration and whether it is applied.
func (r *Runner) Status(ctx context.Context) ([]Status, error) {
	st, err := r.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrations: status: %w", err)
	}
	out := make([]Status, 0, len(st))
	for _, s := range st {
		item := Status{Version: s.Source.Version, Name: baseName(s.Source.Path), Applied: s.State == goose.StateApplied}
		if item.Applied && !s.AppliedAt.IsZero() {
			item.AppliedAt = s.AppliedAt.UTC().Format("2006-01-02 15:04:05Z")
		}
		out = append(out, item)
	}
	return out, nil
}

// Aligned reports whether the database has no pending migrations.
func (r *Runner) Aligned(ctx context.Context) (bool, error) {
	pending, err := r.provider.HasPending(ctx)
	if err != nil {
		return false, fmt.Errorf("migrations: has pending: %w", err)
	}
	return !pending, nil
}

// Version returns the current database version.
func (r *Runner) Version(ctx context.Context) (int64, error) {
	v, err := r.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrations: db version: %w", err)
	}
	return v, nil
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
