// Package store is the persistence layer (uptrace/bun) shared by SQLite and
// PostgreSQL. Repository functions take an explicit bun.IDB so that callers
// decide transaction boundaries (see RunInTx); operational queries require an
// EstablishmentScope obtained from an authenticated Scope (see scope.go).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	_ "modernc.org/sqlite" // pure Go SQLite driver registered as "sqlite"

	"github.com/coste360/arenile/internal/migrations"
)

// Dialect identifies the SQL backend.
type Dialect = migrations.Dialect

const (
	SQLite   = migrations.SQLite
	Postgres = migrations.Postgres
)

// Options selects and configures the database.
type Options struct {
	// PostgresURL, when set (postgres://...), selects PostgreSQL.
	PostgresURL string
	// SQLitePath is the SQLite file path. Ignored when PostgresURL is set.
	SQLitePath string
	// SQLiteMemory opens a private in-memory SQLite database (tests).
	SQLiteMemory bool
}

// DB is an open database with its dialect.
type DB struct {
	*bun.DB
	Kind Dialect
}

// Open opens the database described by opts and verifies the connection.
func Open(ctx context.Context, opts Options) (*DB, error) {
	if opts.PostgresURL != "" {
		return openPostgres(ctx, opts.PostgresURL)
	}
	return openSQLite(ctx, opts)
}

// SQLiteDSN builds the modernc DSN with the pragmas Arenile requires:
// WAL journal, busy timeout, foreign keys and immediate write transactions
// (a transaction takes the write lock when it begins, never upgrades to it
// half-way). Writers of the same process queue on the write gate before
// that (see sqlitegate.go); busy_timeout covers other processes.
func SQLiteDSN(path string, memory bool) string {
	params := url.Values{}
	for _, p := range []string{"busy_timeout(10000)", "foreign_keys(1)", "synchronous(NORMAL)"} {
		params.Add("_pragma", p)
	}
	if !memory {
		params.Add("_pragma", "journal_mode(WAL)")
	}
	params.Set("_txlock", "immediate")
	if memory {
		params.Set("mode", "memory")
		params.Set("cache", "shared")
		return "file:" + path + "?" + params.Encode()
	}
	return "file:" + path + "?" + params.Encode()
}

func openSQLite(ctx context.Context, opts Options) (*DB, error) {
	var dsn string
	if opts.SQLiteMemory {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("store: random name: %w", err)
		}
		dsn = SQLiteDSN("arenile-mem-"+hex.EncodeToString(b), true)
	} else {
		if opts.SQLitePath == "" {
			return nil, errors.New("store: sqlite path is required")
		}
		if err := os.MkdirAll(filepath.Dir(opts.SQLitePath), 0o750); err != nil {
			return nil, fmt.Errorf("store: create data dir: %w", err)
		}
		dsn = SQLiteDSN(opts.SQLitePath, false)
	}
	probe, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	drv := probe.Driver()
	_ = probe.Close()
	// Writers queue in-process on one gate per database (see sqlitegate.go)
	// instead of polling SQLite's lock under busy_timeout.
	sqldb := sql.OpenDB(&gatedConnector{drv: drv, dsn: dsn, gate: newWriteGate()})
	// SQLite allows a single writer. One connection for the in-memory database
	// (each connection would otherwise see a separate database); a small pool
	// for files, where WAL allows concurrent readers.
	if opts.SQLiteMemory {
		sqldb.SetMaxOpenConns(1)
	} else {
		sqldb.SetMaxOpenConns(8)
		sqldb.SetMaxIdleConns(8)
	}
	sqldb.SetConnMaxLifetime(0)
	db := &DB{DB: bun.NewDB(sqldb, sqlitedialect.New()), Kind: SQLite}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}
	return db, nil
}

func openPostgres(ctx context.Context, dsn string) (*DB, error) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return nil, errors.New("store: postgres url must start with postgres://")
	}
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn), withLockTimeout))
	sqldb.SetMaxOpenConns(20)
	sqldb.SetMaxIdleConns(10)
	db := &DB{DB: bun.NewDB(sqldb, pgdialect.New()), Kind: Postgres}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping postgres: %w", err)
	}
	return db, nil
}

// Migrator returns a migration runner bound to this database.
func (db *DB) Migrator() (*migrations.Runner, error) {
	return migrations.NewRunner(db.DB.DB, db.Kind)
}

// withLockTimeout sets the PostgreSQL lock_timeout to half the driver read
// timeout, unless the URL sets it. A transaction queued on a busy row lock
// (many desks changing the same bookings) then fails with lock_not_available
// (55P03), which RunInTx reports as a conflict, before the driver gives up on
// the read and the request ends with a network error and a 500.
func withLockTimeout(conf *pgdriver.Config) {
	if _, ok := conf.ConnParams["lock_timeout"]; ok {
		return
	}
	params := make(map[string]any, len(conf.ConnParams)+1)
	for k, v := range conf.ConnParams {
		params[k] = v
	}
	params["lock_timeout"] = fmt.Sprintf("%dms", (conf.ReadTimeout / 2).Milliseconds())
	conf.ConnParams = params
}

// RunInTx runs fn inside a transaction, committing on nil error and rolling
// back otherwise (including on panic, which is re-raised).
func RunInTx(ctx context.Context, db bun.IDB, fn func(ctx context.Context, tx bun.Tx) error) error {
	if err := db.RunInTx(ctx, nil, fn); err != nil {
		if IsSerializationFailure(err) {
			// The transaction lost a race with another one (deadlock or
			// serialization failure): a conflict, not a server error.
			return fmt.Errorf("%w: transaction aborted by a concurrent one: %w", ErrConflict, err)
		}
		return err
	}
	return nil
}
