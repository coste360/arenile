package store

import (
	"context"
	"database/sql/driver"
	"errors"
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
// writeGate queues the writers of one *DB in-process, in FIFO order (a
// buffered channel of capacity one serves blocked senders in arrival order),
// before they ask SQLite for the lock. busy_timeout is then left to cover
// the contention with other processes (CLI, backup), which is rare and
// short. Waiting in the gate honours the context.
//
// Gated: every read-write transaction (BeginTx without ReadOnly, which the
// DSN opens with BEGIN IMMEDIATE, so it takes the write lock at once) and
// every ExecContext outside a transaction (an autocommit write). Reads and
// read-only transactions use WAL snapshots and never wait.
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
// transaction still open (database/sql does so on a bad connection).
func (c *gatedConn) Close() error {
	c.mu.Lock()
	tx := c.tx
	c.mu.Unlock()
	if tx != nil {
		tx.done()
	}
	return c.sqliteConn.Close()
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
