// Package storetest opens migrated databases for integration tests:
// an in-memory SQLite database always, and a fresh PostgreSQL database when
// ARENILE_TEST_POSTGRES_URL is set (skipped otherwise).
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
)

// EnvPostgresURL points to a PostgreSQL server usable by tests. The user must
// be allowed to create databases.
const EnvPostgresURL = "ARENILE_TEST_POSTGRES_URL"

// SQLite returns a migrated private in-memory SQLite database.
func SQLite(t testing.TB) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{SQLiteMemory: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrate(t, db)
	return db
}

// SQLiteFile returns a migrated SQLite database in a temporary file (useful
// for concurrency tests that need several connections).
func SQLiteFile(t testing.TB) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), store.Options{SQLitePath: t.TempDir() + "/arenile.db"})
	if err != nil {
		t.Fatalf("open sqlite file: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrate(t, db)
	return db
}

// Postgres returns a migrated, freshly created PostgreSQL database that is
// dropped at the end of the test. It skips the test when
// ARENILE_TEST_POSTGRES_URL is not set.
func Postgres(t testing.TB) *store.DB {
	t.Helper()
	base := os.Getenv(EnvPostgresURL)
	if base == "" {
		t.Skipf("%s not set: skipping PostgreSQL test", EnvPostgresURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := store.Open(ctx, store.Options{PostgresURL: base})
	if err != nil {
		t.Fatalf("open postgres admin: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "arenile_test_" + hex.EncodeToString(b)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close()
		t.Fatalf("create database: %v", err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	db, err := store.Open(ctx, store.Options{PostgresURL: u.String()})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		_, _ = admin.ExecContext(cctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close()
	})
	migrate(t, db)
	return db
}

// Each runs fn against SQLite and, when configured, PostgreSQL.
func Each(t *testing.T, fn func(t *testing.T, db *store.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { fn(t, SQLite(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, Postgres(t)) })
}

// EachConcurrent is Each with a file-backed SQLite database (several
// connections, WAL), for TestConcurrency_* tests.
func EachConcurrent(t *testing.T, fn func(t *testing.T, db *store.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { fn(t, SQLiteFile(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, Postgres(t)) })
}

func migrate(t testing.TB, db *store.DB) {
	t.Helper()
	r, err := db.Migrator()
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if _, err := r.Up(context.Background()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
}
