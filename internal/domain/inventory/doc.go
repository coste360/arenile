// Package inventory is the DB-backed availability engine (spec 7): spot
// allocations, holds, manual blocks, pool usage and online quotas. It wraps
// the pure engine internal/domain/availability (assignment, quotas,
// aggregation) with the database constraints that make double booking
// impossible (ADR 0003, ADR 0010, docs/invariants.md INV-01..INV-10).
//
// Ownership (M3): the "inventory" builder owns this package, the handlers in
// internal/http/admin/availability.go (holds, availability, operations/day,
// planning, spot blocks) and the repository functions in
// internal/store/inventory.go. Models: internal/store/models_booking.go
// (Hold, SpotBlock, SpotAllocation, PoolUsage, PoolAllocation).
//
// Correctness rules (never replace them with application checks):
//   - a spot is occupied iff a spot_allocations row exists for (spot, date,
//     slot); inserting a conflicting row violates the unique index, which is
//     translated into *availability.ConflictError{Err: ErrSpotUnavailable}
//     listing every refused half-day (collect them with a read of the
//     conflicting rows after the failed insert, or insert row by row in a
//     savepoint);
//   - expired holds (kind hold, expires_at <= now) are deleted inside the
//     allocating transaction before the insert (spec 7.2, INV-05), so a late
//     cleanup job never blocks sales;
//   - pools: INSERT ... ON CONFLICT DO NOTHING the missing pool_usage rows,
//     then UPDATE pool_usage SET held|used = ... + :n WHERE used + held + :n
//     <= :effective_capacity (capacity_overrides applied); fewer rows than
//     expected => *availability.ConflictError{Err: ErrPoolExhausted}. Every
//     change of pool_usage has its pool_allocations detail row (INV-03);
//   - zone-mode zones consume their implicit system pool (mapping, ADR 0008
//     §3) and get a spot later (assignment_status pending, AutoAssign);
//   - online quota (spec 7.6): channel online consumes pool_usage.online_used
//     with a conditional UPDATE against the quota limit
//     (availability.OnlineQuota.Limit); desk and api ignore it;
//   - full day = AM + PM rows; closure days (closure_days) are skipped
//     (INV-07);
//   - releasing (cancel, expiry, no-show from a date, move, season release,
//     unblock) deletes the rows and decrements the counters in the caller's
//     transaction.
//
// Transactions: the methods that take tx (Allocate, Check, Release,
// Confirm, ExtendHold, ConvertHold, RemoveSeasonDays, RestoreSeasonDays,
// SpotsWithFutureAllocations) never open their own transaction: the booking
// service (or pass, mapping) opens one with store.RunInTx and passes it, so
// allocation, booking rows, audit log and outbox event commit together
// (INV-18). Use cases with a principal (CreateHold, ReleaseHold, blocks,
// availability, operations day, planning) authorize and open their own
// transaction.
//
// Permissions: availability, operations day and planning require
// auth.PermBookingsRead; holds auth.PermBookingsWrite; spot blocks
// auth.PermOperationsRun.
//
// Events (outbox, same transaction): hold.created / hold.released /
// hold.converted, spot_block.created / spot_block.deleted with payload
// event.AllocationChange carrying spot_ids and dates (SSE hints, no PII).
//
// Jobs (Jobs()): release expired panel holds every 30 s (booking holds are
// expired by the booking service, which owns the status change).
package inventory
