// Package inventory is the DB-backed availability engine (spec 7): spot
// allocations, holds, manual blocks, pool usage, online quotas, season
// releases, automatic assignment and the operational queries. It wraps the
// pure engine internal/domain/availability (assignment, quotas,
// aggregation) with the database constraints that make double booking
// impossible (ADR 0003, ADR 0010, docs/invariants.md INV-01..INV-10).
//
// Ownership (M3): the "inventory" builder owns this package, the handlers in
// internal/http/admin/availability.go (holds, availability, operations/day,
// planning, spot blocks) and the repository files internal/store/
// allocation.go, pool_usage.go, hold.go, inventory*.go. Models:
// internal/store/models_booking.go (Hold, SpotBlock, SpotAllocation,
// PoolUsage, PoolAllocation).
//
// Architecture (functional core / imperative shell):
//   - plan.go is pure: buildPlan turns an AllocateRequest and a snapshot of
//     the configuration (zones, spots, pools, tracked components, closures)
//     into spot rows and pool demands, sorted for a stable lock order;
//   - allocate.go is the shell: it loads the snapshot, writes the plan and
//     lets the database decide; holds.go, blocks.go and assign.go are the
//     use cases with a principal; queries.go and operations.go are the read
//     side (availability, operations day, planning).
//
// Correctness rules (never replace them with application checks):
//   - a spot is occupied iff a spot_allocations row exists for (spot, date,
//     slot). Rows are inserted with INSERT ... ON CONFLICT (spot_id, date,
//     slot) DO NOTHING: the unique index refuses a taken half-day without
//     aborting the PostgreSQL transaction, the shortfall in the affected row
//     count is detected and the occupying rows are read back to build
//     *availability.ConflictError{Err: ErrSpotUnavailable} listing every
//     refused half-day; the caller rolls back;
//   - expired holds (kind hold, expires_at <= now) on the spots and pools
//     being allocated are deleted (DELETE ... RETURNING, counters
//     decremented by exactly what was deleted) inside the allocating
//     transaction before the insert (spec 7.2, INV-05), so a late sweeper
//     never blocks sales; reads ignore them (INV-04);
//   - pools: INSERT ... ON CONFLICT DO NOTHING the missing pool_usage rows,
//     then one UPDATE pool_usage SET held|used = ... + n WHERE used + held +
//     n <= effective_capacity per half-day (capacity_overrides applied);
//     a refused UPDATE is a conflict (pool_exhausted or quota_exceeded).
//     Every counter change has its pool_allocations detail row (INV-03);
//   - zone-mode zones consume their implicit system pool (ADR 0008 §3) and
//     get a spot later (assignment_status pending, AutoAssign); quantity
//     zones consume their zone pool; tracked components consume the pool of
//     the component (a component bound to the zone pool counts once);
//   - online quota (spec 7.6) binds every channel but desk (online and api,
//     as availability.OnlineAvailable): pools add online_used under the
//     quota limit in the same conditional UPDATE; seat zones lock the zone
//     rows (no-op UPDATE, as store.LockZones) after inserting and count the
//     valid online rows against the limit;
//   - full day = AM + PM rows; closure half-days (closure_days) are skipped
//     (INV-07). Out-of-season dates are reported as closed by the queries
//     but not refused here (pricing refuses them);
//   - releasing (cancel, expiry, no-show from a date, move, season release,
//     unblock, hold release) deletes the rows and decrements the counters
//     in the caller's transaction;
//   - ConvertHold closes the hold with a conditional update (one winner),
//     releases its rows and allocates the booking lines in the same
//     transaction, so the freed rows stay locked until commit.
//
// Transactions: the methods that take tx (Allocate, Check, Release,
// Confirm, ExtendHold, ConvertHold, RemoveSeasonDays, RestoreSeasonDays,
// AssignPendingInTx, SpotsWithFutureAllocations) never open their own
// transaction: the booking service (or pass, mapping) opens one with
// store.RunInTx and passes it, so allocation, booking rows, audit log and
// outbox event commit together (INV-18). Use cases with a principal
// (CreateHold, ReleaseHold, blocks, AutoAssign, availability, operations
// day, planning) authorize and open their own transaction.
//
// Permissions (spec 16.1, checked here): availability, operations day,
// planning, hold and block reads require auth.PermBookingsRead; holds and
// auto-assignment auth.PermBookingsWrite; spot blocks auth.PermOperationsRun.
//
// Events (outbox, same transaction, published to the SSE hub by the
// dispatcher after commit): hold.created / hold.released / hold.converted,
// spot_block.created / spot_block.deleted, allocation.assigned, with payload
// event.AllocationChange carrying spot_ids, pool_ids and dates (no PII).
// Allocate/Release write no event: the booking service writes booking.*
// with the hints of the returned Allocation.
//
// Jobs (Jobs()): inventory.sweep_expired_holds every 30 s releases expired
// panel holds (idempotent: conditional close; booking holds are expired by
// the booking service, which owns the status change).
//
// M4 hooks: holds with expiry (KindHold + ExtendHold + Confirm), the channel
// on every allocation (quota), Check for quotes without writes.
package inventory
