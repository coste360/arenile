package storetest

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/store"
)

// EnvPerfBudgets enables the wall-clock budgets of the performance tests.
// Elapsed times depend on the machine (shared CI runners, race detector), so
// by default the tests only log them and enforce what does not depend on
// speed: the number of SQL statements each operation issues, which stays
// constant as the data grows unless the code regresses to a per-row query
// pattern (N+1). Set ARENILE_PERF_BUDGETS=1 on a known machine to also fail
// on the time budgets.
const EnvPerfBudgets = "ARENILE_PERF_BUDGETS"

// QueryCounter counts the SQL statements a database runs.
type QueryCounter struct{ n atomic.Int64 }

// CountQueries returns a view of db that counts the statements run through
// it, and the counter. Build the services under test on the returned DB.
func CountQueries(db *store.DB) (*store.DB, *QueryCounter) {
	c := &QueryCounter{}
	return &store.DB{DB: db.WithQueryHook(c), Kind: db.Kind}, c
}

// BeforeQuery implements bun.QueryHook.
func (c *QueryCounter) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	c.n.Add(1)
	return ctx
}

// AfterQuery implements bun.QueryHook.
func (c *QueryCounter) AfterQuery(context.Context, *bun.QueryEvent) {}

// Load returns the statements counted so far.
func (c *QueryCounter) Load() int { return int(c.n.Load()) }

// Perf measures fn: it fails if fn returns an error or issues more than
// maxQueries statements, logs the elapsed time and, only with
// ARENILE_PERF_BUDGETS=1, fails when it exceeds budget.
func Perf(t testing.TB, qc *QueryCounter, name string, maxQueries int, budget time.Duration, fn func() error) {
	t.Helper()
	before := qc.Load()
	start := time.Now()
	err := fn()
	el := time.Since(start)
	queries := qc.Load() - before
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	t.Logf("%s: %s, %d queries", name, el, queries)
	if queries > maxQueries {
		t.Fatalf("%s issued %d queries (at most %d): per-row query pattern?", name, queries, maxQueries)
	}
	if os.Getenv(EnvPerfBudgets) == "1" && el > budget {
		t.Fatalf("%s took %s (budget %s, %s=1)", name, el, budget, EnvPerfBudgets)
	}
}
