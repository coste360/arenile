package store

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"sync"
)

// SQLite admits a single writer per database file. Two connections of the
// pool that want to write at the same time do not queue inside SQLite: the
// loser polls under busy_timeout (sleeps of up to 100ms) and fails with
// SQLITE_BUSY once the timeout elapses, however short its turn would have
// been. Under a burst of writers (webhooks, sweeper, cash desk) a waiter can
// lose the race for the lock again and again until busy_timeout runs out,
// even though the total work fits in it.
//
// writeGate queues the writers of one *DB (one per process: the server opens
// the database once) in FIFO order (a
// buffered channel of capacity one serves blocked senders in arrival order),
// before they ask SQLite for the lock. busy_timeout is then left to cover
// the contention with other processes (CLI, backup), which is rare and
// short. Waiting in the gate honours the context.
//
// Gated: every read-write transaction (BeginTx without ReadOnly, which the
// DSN opens with BEGIN IMMEDIATE, so it takes the write lock at once), every
// ExecContext outside a transaction (an autocommit write) and every
// QueryContext outside a transaction whose statement has a RETURNING clause
// (an autocommit INSERT/UPDATE/DELETE ... RETURNING, held until its rows are
// closed). Reads and read-only transactions use WAL snapshots and never wait.
// Prepared statements (PrepareContext) are not gated: bun does not use them.
//
// Rules that keep the gate sound:
//   - inside a write transaction, never write through the pool (the *DB)
//     instead of the transaction: the write would wait for the gate that the
//     same goroutine holds, until its context ends;
//   - never run BEGIN/COMMIT as statements: use BeginTx (bun RunInTx).
type writeGate chan struct{}

func newWriteGate() writeGate { return make(writeGate, 1) }

func (g writeGate) acquire(ctx context.Context) error {
	select {
	case g <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g writeGate) release() { <-g }

// gatedConnector opens SQLite connections that share one writeGate.
type gatedConnector struct {
	drv  driver.Driver
	dsn  string
	gate writeGate
}

func (c *gatedConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	inner, ok := conn.(sqliteConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("store: sqlite driver connection lacks the context interfaces")
	}
	return &gatedConn{sqliteConn: inner, gate: c.gate}, nil
}

func (c *gatedConnector) Driver() driver.Driver { return c.drv }

// sqliteConn is the set of interfaces the modernc connection implements and
// database/sql uses; gatedConn forwards all of them.
type sqliteConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
}

// gatedConn is used by one goroutine at a time (database/sql guarantees it
// for a driver.Conn); mu only guards tx against a Rollback that
// database/sql issues from its context watcher.
type gatedConn struct {
	sqliteConn
	gate writeGate
	mu   sync.Mutex
	tx   *gatedTx // the open read-write transaction holding the gate
}

func (c *gatedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.ReadOnly {
		return c.sqliteConn.BeginTx(ctx, opts)
	}
	if err := c.gate.acquire(ctx); err != nil {
		return nil, err
	}
	tx, err := c.sqliteConn.BeginTx(ctx, opts)
	if err != nil {
		c.gate.release()
		return nil, err
	}
	gt := &gatedTx{Tx: tx, conn: c}
	c.mu.Lock()
	c.tx = gt
	c.mu.Unlock()
	return gt, nil
}

// Close releases the gate if the connection is discarded with its
// transaction still open (database/sql does so on a bad connection), after
// closing it so that SQLite has dropped the lock.
func (c *gatedConn) Close() error {
	err := c.sqliteConn.Close()
	c.mu.Lock()
	tx := c.tx
	c.mu.Unlock()
	if tx != nil {
		tx.done()
	}
	return err
}

// Begin is the deprecated entry point; database/sql prefers BeginTx.
func (c *gatedConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *gatedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.mu.Lock()
	inTx := c.tx != nil
	c.mu.Unlock()
	if inTx {
		return c.sqliteConn.ExecContext(ctx, query, args)
	}
	if err := c.gate.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.gate.release()
	return c.sqliteConn.ExecContext(ctx, query, args)
}

// returning matches a RETURNING clause: through QueryContext, the only
// statements that write (bun runs every other write through ExecContext).
var returning = regexp.MustCompile(`(?i)\breturning\b`)

func (c *gatedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.mu.Lock()
	inTx := c.tx != nil
	c.mu.Unlock()
	if inTx || !returning.MatchString(query) {
		return c.sqliteConn.QueryContext(ctx, query, args)
	}
	if err := c.gate.acquire(ctx); err != nil {
		return nil, err
	}
	rows, err := c.sqliteConn.QueryContext(ctx, query, args)
	if err != nil {
		c.gate.release()
		return nil, err
	}
	inner, ok := rows.(sqliteRows)
	if !ok {
		_ = rows.Close()
		c.gate.release()
		return nil, errors.New("store: sqlite driver rows lack the column type interfaces")
	}
	return &gatedRows{sqliteRows: inner, gate: c.gate}, nil
}

// sqliteRows is the set of interfaces the modernc rows implement; gatedRows
// forwards all of them (bun reads column types when scanning into maps).
type sqliteRows interface {
	driver.Rows
	driver.RowsColumnTypeDatabaseTypeName
	driver.RowsColumnTypeLength
	driver.RowsColumnTypeNullable
	driver.RowsColumnTypePrecisionScale
	driver.RowsColumnTypeScanType
}

// gatedRows holds the gate until the statement is finished (rows closed).
type gatedRows struct {
	sqliteRows
	gate writeGate
	once sync.Once
}

func (r *gatedRows) Close() error {
	err := r.sqliteRows.Close()
	r.once.Do(r.gate.release)
	return err
}

type gatedTx struct {
	driver.Tx
	conn *gatedConn
	once sync.Once
}

func (t *gatedTx) done() {
	t.once.Do(func() {
		t.conn.mu.Lock()
		t.conn.tx = nil
		t.conn.mu.Unlock()
		t.conn.gate.release()
	})
}

func (t *gatedTx) Commit() error {
	defer t.done()
	return t.Tx.Commit()
}

func (t *gatedTx) Rollback() error {
	defer t.done()
	return t.Tx.Rollback()
}
