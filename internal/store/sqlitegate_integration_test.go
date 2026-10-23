//go:build integration

package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"
)

// openGatedSQLite opens a file database through the write gate with a
// busy_timeout far shorter than the burst of writers below, so that only
// the in-process queue can make all of them succeed.
func openGatedSQLite(t *testing.T, busyMS string) *sql.DB {
	t.Helper()
	params := url.Values{}
	params.Add("_pragma", "busy_timeout("+busyMS+")")
	params.Add("_pragma", "journal_mode(WAL)")
	params.Set("_txlock", "immediate")
	dsn := "file:" + t.TempDir() + "/gate.db?" + params.Encode()
	probe, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	drv := probe.Driver()
	_ = probe.Close()
	db := sql.OpenDB(&gatedConnector{drv: drv, dsn: dsn, gate: newWriteGate()})
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("CREATE TABLE counter (id INTEGER PRIMARY KEY, n INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO counter (id, n) VALUES (1, 0)"); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestConcurrency_SQLiteWriteGate: 200 parallel read-modify-write
// transactions and 100 autocommit updates on the same row, each holding the
// write lock longer than busy_timeout allows a waiter to poll, all succeed
// and none is lost. Without the gate the waiters fail with SQLITE_BUSY.
func TestConcurrency_SQLiteWriteGate(t *testing.T) {
	db := openGatedSQLite(t, "20")
	ctx := context.Background()
	const txs, execs = 200, 100
	var wg sync.WaitGroup
	errs := make(chan error, txs+execs)
	start := make(chan struct{})
	for range txs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				errs <- err
				return
			}
			var n int
			if err := tx.QueryRowContext(ctx, "SELECT n FROM counter WHERE id = 1").Scan(&n); err != nil {
				_ = tx.Rollback()
				errs <- err
				return
			}
			time.Sleep(time.Millisecond) // hold the lock
			if _, err := tx.ExecContext(ctx, "UPDATE counter SET n = ? WHERE id = 1", n+1); err != nil {
				_ = tx.Rollback()
				errs <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errs <- err
			}
		}()
	}
	for range execs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := db.ExecContext(ctx, "UPDATE counter SET n = n + 1 WHERE id = 1"); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	var n int
	if err := db.QueryRow("SELECT n FROM counter WHERE id = 1").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != txs+execs {
		t.Fatalf("counter = %d, want %d (lost updates)", n, txs+execs)
	}
}

// TestSQLiteWriteGate_ContextAndRelease: a writer waiting in the gate gives
// up with its context; the gate is released by Commit, Rollback and by a
// read-only transaction never being taken.
func TestSQLiteWriteGate_ContextAndRelease(t *testing.T) {
	db := openGatedSQLite(t, "1000")
	ctx := context.Background()
	holder, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Reads and read-only transactions do not wait for the writer.
	ro, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := ro.QueryRowContext(ctx, "SELECT n FROM counter").Scan(&n); err != nil {
		t.Fatal(err)
	}
	_ = ro.Rollback()

	wctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := db.BeginTx(wctx, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting writer: %v, want deadline exceeded", err)
	}
	if err := holder.Rollback(); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("gate not released: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE counter SET n = 1"); err != nil {
		t.Fatalf("autocommit after release: %v", err)
	}
}
