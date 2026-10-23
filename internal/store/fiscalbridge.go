package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the fiscal bridge (spec 29.1, 14.2): the registration of
// the device that executes fiscal jobs on its local FiscalDriver and the
// lease-based job queue. Owned by internal/fiscalbridge.
//
// Concurrency model: every state transition of a job is a conditional
// UPDATE whose WHERE clause repeats the expected state, so concurrent
// pollers, duplicate submissions and the lease expiry job can race freely
// on both dialects and a transition happens at most once (the caller looks
// at RowsAffected). Enqueue relies on the unique index
// (establishment_id, kind, idempotency_key) with ON CONFLICT DO NOTHING.

// ---- registrations ----------------------------------------------------------

// GetActiveFiscalBridge returns the active bridge of es with the name of
// its device, or ErrNotFound.
func GetActiveFiscalBridge(ctx context.Context, db bun.IDB, es EstablishmentScope) (*FiscalBridge, string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, "", err
	}
	var row struct {
		FiscalBridge
		DeviceName string `bun:"device_name"`
	}
	err := db.NewSelect().Model(&row.FiscalBridge).
		ColumnExpr("fb.*").
		ColumnExpr("dev.name AS device_name").
		Join("JOIN devices AS dev ON dev.id = fb.device_id").
		Where("fb.establishment_id = ?", es.ID()).
		Where("fb.active = ?", true).
		Limit(1).
		Scan(ctx, &row)
	if err != nil {
		return nil, "", wrapRead("get active fiscal bridge", err)
	}
	return &row.FiscalBridge, row.DeviceName, nil
}

// ActivateFiscalBridge makes deviceID the active bridge of es: it
// deactivates the registration of any other device, then refreshes the
// existing row of deviceID or inserts one. Run it in a transaction; a
// concurrent registration of another device surfaces as ErrConflict (the
// partial unique index on the active row).
func ActivateFiscalBridge(ctx context.Context, db bun.IDB, es EstablishmentScope, deviceID, driver string, deviceStatus JSON, now time.Time) (*FiscalBridge, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if deviceStatus == nil {
		deviceStatus = JSON("{}")
	}
	if _, err := db.NewUpdate().Model((*FiscalBridge)(nil)).
		Set("active = ?", false).Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("active = ?", true).
		Where("device_id <> ?", deviceID).
		Exec(ctx); err != nil {
		return nil, wrapWrite("deactivate fiscal bridges", err)
	}
	b := &FiscalBridge{}
	err := db.NewSelect().Model(b).
		Where("fb.establishment_id = ?", es.ID()).
		Where("fb.device_id = ?", deviceID).
		OrderExpr("fb.created_at DESC").Limit(1).
		Scan(ctx)
	switch {
	case err == nil:
		b.Driver, b.Active, b.DeviceStatus, b.LastSeenAt, b.UpdatedAt = driver, true, deviceStatus, &now, now
		_, err = db.NewUpdate().Model(b).
			Column("driver", "active", "device_status", "last_seen_at", "updated_at").
			WherePK().Exec(ctx)
		return b, wrapWrite("activate fiscal bridge", err)
	case isNoRows(err):
		b = &FiscalBridge{
			ID: NewID(), EstablishmentID: es.ID(), DeviceID: deviceID, Driver: driver, Active: true,
			DeviceStatus: deviceStatus, LastSeenAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		_, err = db.NewInsert().Model(b).Exec(ctx)
		return b, wrapWrite("insert fiscal bridge", err)
	default:
		return nil, wrapRead("get fiscal bridge", err)
	}
}

// TouchFiscalBridge records that the active bridge deviceID of es was seen
// at now. It reports false when deviceID is not the active bridge.
func TouchFiscalBridge(ctx context.Context, db bun.IDB, es EstablishmentScope, deviceID string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*FiscalBridge)(nil)).
		Set("last_seen_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("device_id = ?", deviceID).
		Where("active = ?", true).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("touch fiscal bridge", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeactivateFiscalBridgeOfDevice deactivates the registration of deviceID
// on es (idempotent) and returns the jobs leased to it to the queue. It
// reports whether a registration was active.
func DeactivateFiscalBridgeOfDevice(ctx context.Context, db bun.IDB, es EstablishmentScope, deviceID string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*FiscalBridge)(nil)).
		Set("active = ?", false).Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("device_id = ?", deviceID).
		Where("active = ?", true).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("deactivate fiscal bridge", err)
	}
	n, _ := res.RowsAffected()
	if _, err := db.NewUpdate().Model((*FiscalBridgeJob)(nil)).
		Set("status = ?", FiscalBridgeJobQueued).
		Set("lease_expires_at = NULL").
		Set("next_attempt_at = ?", now).
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("leased_device_id = ?", deviceID).
		Where("status = ?", FiscalBridgeJobLeased).
		Exec(ctx); err != nil {
		return false, wrapWrite("release fiscal bridge jobs", err)
	}
	return n > 0, nil
}

// ---- jobs -------------------------------------------------------------------

// EnqueueFiscalBridgeJob inserts j unless a job with the same
// (establishment, kind, idempotency key) exists, and returns the stored job
// (the new one or the existing one). created tells which.
func EnqueueFiscalBridgeJob(ctx context.Context, db bun.IDB, es EstablishmentScope, j *FiscalBridgeJob) (*FiscalBridgeJob, bool, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, false, err
	}
	j.EstablishmentID = es.ID()
	if j.Payload == nil {
		j.Payload = JSON("{}")
	}
	res, err := db.NewInsert().Model(j).
		On("CONFLICT (establishment_id, kind, idempotency_key) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return nil, false, wrapWrite("enqueue fiscal bridge job", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return j, true, nil
	}
	got := &FiscalBridgeJob{}
	err = db.NewSelect().Model(got).
		Where("fbj.establishment_id = ?", es.ID()).
		Where("fbj.kind = ?", j.Kind).
		Where("fbj.idempotency_key = ?", j.IdempotencyKey).
		Scan(ctx)
	if err != nil {
		return nil, false, wrapRead("get fiscal bridge job", err)
	}
	return got, false, nil
}

// GetFiscalBridgeJob returns job id of es.
func GetFiscalBridgeJob(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*FiscalBridgeJob, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	j := &FiscalBridgeJob{}
	err := db.NewSelect().Model(j).
		Where("fbj.id = ?", id).
		Where("fbj.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get fiscal bridge job", err)
	}
	return j, nil
}

// leasableWhere selects the jobs a bridge may take at now: queued and due,
// or leased with an expired lease (the previous holder crashed or lost the
// connection).
const leasableWhere = "((status = ? AND next_attempt_at <= ?) OR (status = ? AND lease_expires_at <= ?))"

// leaseRounds bounds the selections of one LeaseFiscalBridgeJobs call.
const leaseRounds = 5

func leasableArgs(now time.Time) []any {
	return []any{FiscalBridgeJobQueued, now, FiscalBridgeJobLeased, now}
}

// LeaseFiscalBridgeJobs leases up to limit due jobs of es to deviceID until
// leaseUntil, oldest first, incrementing their attempts. Each job is taken
// with a conditional UPDATE, so concurrent pollers never share a lease.
// Jobs that already reached maxAttempts deliveries are not leased again
// (FailExhaustedFiscalBridgeJobs closes them).
func LeaseFiscalBridgeJobs(ctx context.Context, db bun.IDB, es EstablishmentScope, deviceID string, now, leaseUntil time.Time, limit, maxAttempts int) ([]FiscalBridgeJob, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := make([]FiscalBridgeJob, 0, limit)
	// A poller that loses every candidate to concurrent pollers selects
	// again (bounded), so jobs are not left waiting for the next poll.
	for round := 0; round < leaseRounds && len(out) < limit; round++ {
		var ids []string
		sel := db.NewSelect().Model((*FiscalBridgeJob)(nil)).Column("fbj.id").
			Where("fbj.establishment_id = ?", es.ID()).
			Where("fbj.attempts < ?", maxAttempts).
			Where(leasableWhere, leasableArgs(now)...)
		if err := sel.OrderExpr("fbj.created_at ASC, fbj.id ASC").Limit(int64(limit-len(out))).Scan(ctx, &ids); err != nil {
			return nil, wrapRead("list leasable fiscal bridge jobs", err)
		}
		if len(ids) == 0 {
			break
		}
		lost := false
		for _, id := range ids {
			upd := db.NewUpdate().Model((*FiscalBridgeJob)(nil)).
				Set("status = ?", FiscalBridgeJobLeased).
				Set("leased_device_id = ?", deviceID).
				Set("lease_expires_at = ?", leaseUntil).
				Set("attempts = attempts + 1").
				Set("updated_at = ?", now).
				Where("id = ?", id).
				Where("establishment_id = ?", es.ID()).
				Where("attempts < ?", maxAttempts).
				Where(leasableWhere, leasableArgs(now)...)
			res, err := upd.Exec(ctx)
			if err != nil {
				return nil, wrapWrite("lease fiscal bridge job", err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				lost = true // taken by a concurrent poller
				continue
			}
			j, err := GetFiscalBridgeJob(ctx, db, es, id)
			if err != nil {
				return nil, err
			}
			out = append(out, *j)
		}
		if !lost {
			break
		}
	}
	return out, nil
}

// FiscalBridgeJobOutcome is the transition applied by
// CompleteFiscalBridgeJob.
type FiscalBridgeJobOutcome struct {
	// Status is FiscalBridgeJobSucceeded, FiscalBridgeJobFailed or, for a
	// retryable failure, FiscalBridgeJobQueued with NextAttemptAt.
	Status        string
	Result        JSON
	NextAttemptAt time.Time
}

// CompleteFiscalBridgeJob stores the outcome submitted by deviceID for job
// id. The job must still be open (queued or leased) and its last lease must
// belong to deviceID: a late result of a lease that expired is still
// accepted from the same device, because the receipt was printed. It
// reports false when the transition did not happen (already completed,
// requeued after a retryable failure, or leased by another device).
func CompleteFiscalBridgeJob(ctx context.Context, db bun.IDB, es EstablishmentScope, id, deviceID string, o FiscalBridgeJobOutcome, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	q := db.NewUpdate().Model((*FiscalBridgeJob)(nil)).
		Set("status = ?", o.Status).
		Set("result = ?", o.Result).
		Set("lease_expires_at = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("leased_device_id = ?", deviceID)
	if o.Status == FiscalBridgeJobQueued {
		// Retryable failure: only the current lease can requeue.
		q = q.Set("next_attempt_at = ?", o.NextAttemptAt).Where("status = ?", FiscalBridgeJobLeased)
	} else {
		q = q.Set("completed_at = ?", now).Where("status IN (?)", bun.List([]string{FiscalBridgeJobQueued, FiscalBridgeJobLeased}))
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return false, wrapWrite("complete fiscal bridge job", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountFiscalBridgeJobs returns the open (queued or leased) and failed jobs of es.
func CountFiscalBridgeJobs(ctx context.Context, db bun.IDB, es EstablishmentScope) (pending, failed int, err error) {
	if err := es.mustBeValid(); err != nil {
		return 0, 0, err
	}
	var rows []struct {
		Status string `bun:"status"`
		N      int    `bun:"n"`
	}
	err = db.NewSelect().Model((*FiscalBridgeJob)(nil)).
		ColumnExpr("fbj.status AS status").ColumnExpr("COUNT(*) AS n").
		Where("fbj.establishment_id = ?", es.ID()).
		GroupExpr("fbj.status").
		Scan(ctx, &rows)
	if err != nil {
		return 0, 0, wrapRead("count fiscal bridge jobs", err)
	}
	for _, r := range rows {
		switch r.Status {
		case FiscalBridgeJobQueued, FiscalBridgeJobLeased:
			pending += r.N
		case FiscalBridgeJobFailed:
			failed += r.N
		}
	}
	return pending, failed, nil
}

// FiscalDocumentIDForPayment returns the id of the fiscal document of
// paymentID in es, or "" when there is none (best effort link of a job to
// its document).
func FiscalDocumentIDForPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, paymentID string) (string, error) {
	if err := es.mustBeValid(); err != nil {
		return "", err
	}
	var ids []string
	err := db.NewSelect().TableExpr("fiscal_documents AS fd").Column("fd.id").
		Where("fd.establishment_id = ?", es.ID()).
		Where("fd.payment_id = ?", paymentID).
		OrderExpr("fd.created_at DESC").Limit(1).
		Scan(ctx, &ids)
	if err != nil {
		return "", wrapRead("find fiscal document of payment", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}

// ---- system jobs (no establishment scope) -------------------------------------

// RequeueExpiredFiscalBridgeLeases returns to the queue the jobs whose lease
// expired before now (system job across establishments) and returns how
// many. Jobs that exhausted maxAttempts deliveries are failed with
// exhaustedResult instead.
func RequeueExpiredFiscalBridgeLeases(ctx context.Context, db bun.IDB, now time.Time, maxAttempts int, exhaustedResult JSON) (requeued, failed int64, err error) {
	res, err := db.NewUpdate().Model((*FiscalBridgeJob)(nil)).
		Set("status = ?", FiscalBridgeJobFailed).
		Set("result = ?", exhaustedResult).
		Set("lease_expires_at = NULL").
		Set("completed_at = ?", now).
		Set("updated_at = ?", now).
		Where("status = ?", FiscalBridgeJobLeased).
		Where("lease_expires_at <= ?", now).
		Where("attempts >= ?", maxAttempts).
		Exec(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("store: fail exhausted fiscal bridge jobs: %w", err)
	}
	failed, _ = res.RowsAffected()
	res, err = db.NewUpdate().Model((*FiscalBridgeJob)(nil)).
		Set("status = ?", FiscalBridgeJobQueued).
		Set("lease_expires_at = NULL").
		Set("next_attempt_at = ?", now).
		Set("updated_at = ?", now).
		Where("status = ?", FiscalBridgeJobLeased).
		Where("lease_expires_at <= ?", now).
		Exec(ctx)
	if err != nil {
		return 0, failed, fmt.Errorf("store: requeue fiscal bridge leases: %w", err)
	}
	requeued, _ = res.RowsAffected()
	return requeued, failed, nil
}
