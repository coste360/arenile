package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of ESC/POS network printing (migration 00011, ADR
// 0021), owned by internal/domain/printing. Operational functions take an
// EstablishmentScope; the delivery functions of the job runner (DuePrintJobs,
// ClaimPrintJob, Finish*) are system operations across establishments that
// address one job by id.

// ---- printers ---------------------------------------------------------------

// ListPrinters returns the printers of es by name; inactive ones only with
// includeInactive.
func ListPrinters(ctx context.Context, db bun.IDB, es EstablishmentScope, includeInactive bool) ([]Printer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []Printer
	q := db.NewSelect().Model(&out).Where("prn.establishment_id = ?", es.ID()).OrderExpr("prn.name ASC, prn.id ASC")
	if !includeInactive {
		q = q.Where("prn.active = ?", true)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list printers: %w", err)
	}
	return out, nil
}

// GetPrinter loads a printer of es.
func GetPrinter(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Printer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(Printer)
	err := db.NewSelect().Model(p).Where("prn.id = ?", id).Where("prn.establishment_id = ?", es.ID()).Scan(ctx)
	return p, wrapRead("get printer", err)
}

// GetPrinterByID loads a printer by id (job runner, system operation).
func GetPrinterByID(ctx context.Context, db bun.IDB, id string) (*Printer, error) {
	p := new(Printer)
	err := db.NewSelect().Model(p).Where("prn.id = ?", id).Scan(ctx)
	return p, wrapRead("get printer", err)
}

// InsertPrinter inserts a printer of es; a duplicate name is ErrConflict.
func InsertPrinter(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Printer) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert printer", err)
}

// UpdatePrinter saves the configuration columns of p if its stored version
// is still expectedVersion, bumping the version. ErrNotFound when the
// printer is gone or the version changed (the caller re-reads to tell them
// apart); a duplicate name is ErrConflict.
func UpdatePrinter(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Printer, expectedVersion int64) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	p.Version = expectedVersion + 1
	res, err := db.NewUpdate().Model(p).
		Column("name", "host", "port", "paper_width_mm", "chars_per_line", "codepage", "codepage_table", "cut_mode",
			"feed_lines", "print_logo", "logo_raster", "logo_source_sha256", "drawer_pin", "active", "status",
			"last_error", "version", "updated_at").
		Where("id = ?", p.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", expectedVersion).
		Exec(ctx)
	if err != nil {
		p.Version = expectedVersion
		return wrapWrite("update printer", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		p.Version = expectedVersion
		return ErrNotFound
	}
	return nil
}

// DeletePrinter deletes a printer of es if its version is expectedVersion:
// its routes go with it and its queued (or in-flight) jobs become
// cancelled; the job history keeps the rows with printer_id NULL.
func DeletePrinter(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, expectedVersion int64, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if _, err := db.NewUpdate().Model((*PrintJob)(nil)).
		Set("status = ?", PrintJobCancelled).Set("locked_until = NULL").Set("next_attempt_at = NULL").Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Where("printer_id = ?", id).
		Where("status IN (?)", bun.List([]string{PrintJobQueued, PrintJobPrinting})).
		Exec(ctx); err != nil {
		return wrapWrite("cancel printer jobs", err)
	}
	if _, err := db.NewDelete().Model((*PrinterRoute)(nil)).
		Where("establishment_id = ?", es.ID()).Where("printer_id = ?", id).Exec(ctx); err != nil {
		return wrapWrite("delete printer routes", err)
	}
	if _, err := db.NewUpdate().Model((*PrintJob)(nil)).Set("printer_id = NULL").
		Where("establishment_id = ?", es.ID()).Where("printer_id = ?", id).Exec(ctx); err != nil {
		return wrapWrite("detach printer jobs", err)
	}
	res, err := db.NewDelete().Model((*Printer)(nil)).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("version = ?", expectedVersion).Exec(ctx)
	if err != nil {
		return wrapWrite("delete printer", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}

// PrinterHealth is the outcome of a check or a delivery.
type PrinterHealth struct {
	Status    string
	CheckedAt time.Time
	Success   bool    // sets last_success_at = CheckedAt
	LastError *string // nil clears it
}

// SetPrinterHealth records the reachability of a printer without bumping
// its configuration version (system operation by id: checks and jobs).
func SetPrinterHealth(ctx context.Context, db bun.IDB, printerID string, h PrinterHealth) error {
	q := db.NewUpdate().Model((*Printer)(nil)).
		Set("status = ?", h.Status).Set("last_checked_at = ?", h.CheckedAt).Set("last_error = ?", h.LastError).
		Where("id = ?", printerID)
	if h.Success {
		q = q.Set("last_success_at = ?", h.CheckedAt)
	}
	_, err := q.Exec(ctx)
	return wrapWrite("set printer health", err)
}

// SetPrinterLogo stores the cached logo raster of a printer (nil clears it)
// without bumping its configuration version.
func SetPrinterLogo(ctx context.Context, db bun.IDB, printerID string, raster []byte, sourceSHA256 *string) error {
	_, err := db.NewUpdate().Model((*Printer)(nil)).
		Set("logo_raster = ?", raster).Set("logo_source_sha256 = ?", sourceSHA256).
		Where("id = ?", printerID).Exec(ctx)
	return wrapWrite("set printer logo", err)
}

// ---- routes -----------------------------------------------------------------

// ListPrinterRoutes returns the routes of es ordered by document kind,
// default rule first, then register.
func ListPrinterRoutes(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]PrinterRoute, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []PrinterRoute
	err := db.NewSelect().Model(&out).Where("prr.establishment_id = ?", es.ID()).
		OrderExpr("prr.document_kind ASC, prr.register_id IS NOT NULL, prr.register_id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list printer routes: %w", err)
	}
	return out, nil
}

// ReplacePrinterRoutes deletes every route of es and inserts routes (run it
// in a transaction). Duplicate (kind, register) pairs are ErrConflict.
func ReplacePrinterRoutes(ctx context.Context, db bun.IDB, es EstablishmentScope, routes []PrinterRoute) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if _, err := db.NewDelete().Model((*PrinterRoute)(nil)).Where("establishment_id = ?", es.ID()).Exec(ctx); err != nil {
		return wrapWrite("delete printer routes", err)
	}
	if len(routes) == 0 {
		return nil
	}
	for i := range routes {
		routes[i].EstablishmentID = es.ID()
	}
	_, err := db.NewInsert().Model(&routes).Exec(ctx)
	return wrapWrite("insert printer routes", err)
}

// CashRegisterExists reports whether registerID is a cash register of es.
func CashRegisterExists(ctx context.Context, db bun.IDB, es EstablishmentScope, registerID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().Model((*CashRegister)(nil)).
		Where("id = ?", registerID).Where("establishment_id = ?", es.ID()).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: cash register exists: %w", err)
	}
	return n > 0, nil
}

// GetCashRegisterName returns the name of a cash register of es.
func GetCashRegisterName(ctx context.Context, db bun.IDB, es EstablishmentScope, registerID string) (string, error) {
	if err := es.mustBeValid(); err != nil {
		return "", err
	}
	var name string
	err := db.NewSelect().Model((*CashRegister)(nil)).Column("name").
		Where("id = ?", registerID).Where("establishment_id = ?", es.ID()).Scan(ctx, &name)
	return name, wrapRead("get cash register name", err)
}

// ---- jobs -------------------------------------------------------------------

// InsertPrintJob inserts j unless a job with the same idempotency key
// exists in es; it reports whether j was inserted. On a duplicate the
// caller loads the existing job with GetPrintJobByKey. The unique index is
// the guarantee (ON CONFLICT DO NOTHING keeps a PostgreSQL transaction
// usable).
func InsertPrintJob(ctx context.Context, db bun.IDB, es EstablishmentScope, j *PrintJob) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	j.EstablishmentID = es.ID()
	res, err := db.NewInsert().Model(j).On("CONFLICT (establishment_id, idempotency_key) DO NOTHING").Exec(ctx)
	if err != nil {
		return false, wrapWrite("insert print job", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// GetPrintJob loads a job of es.
func GetPrintJob(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*PrintJob, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	j := new(PrintJob)
	err := db.NewSelect().Model(j).Where("pj.id = ?", id).Where("pj.establishment_id = ?", es.ID()).Scan(ctx)
	return j, wrapRead("get print job", err)
}

// GetPrintJobByKey loads the job of es with an idempotency key.
func GetPrintJobByKey(ctx context.Context, db bun.IDB, es EstablishmentScope, key string) (*PrintJob, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	j := new(PrintJob)
	err := db.NewSelect().Model(j).Where("pj.establishment_id = ?", es.ID()).Where("pj.idempotency_key = ?", key).Scan(ctx)
	return j, wrapRead("get print job by key", err)
}

// PrintJobFilter narrows ListPrintJobs.
type PrintJobFilter struct {
	Status    string
	PrinterID string
}

// ListPrintJobs returns a page of the jobs of es, newest first (the
// payload column is not loaded).
func ListPrintJobs(ctx context.Context, db bun.IDB, es EstablishmentScope, f PrintJobFilter, cursor string, limit int) (Page[PrintJob], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[PrintJob]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[PrintJob]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []PrintJob
	q := db.NewSelect().Model(&items).ExcludeColumn("payload").
		Where("pj.establishment_id = ?", es.ID()).OrderExpr("pj.id DESC").Limit(int64(limit + 1))
	if f.Status != "" {
		q = q.Where("pj.status = ?", f.Status)
	}
	if f.PrinterID != "" {
		q = q.Where("pj.printer_id = ?", f.PrinterID)
	}
	if before != "" {
		q = q.Where("pj.id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[PrintJob]{}, fmt.Errorf("store: list print jobs: %w", err)
	}
	page := Page[PrintJob]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(items[limit-1].ID)
	}
	return page, nil
}

// PrinterNames returns the names of the listed printers of es, by id.
func PrinterNames(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Printer
	err := db.NewSelect().Model(&rows).Column("id", "name").
		Where("prn.establishment_id = ?", es.ID()).Where("prn.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: printer names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// RetryPrintJob moves a failed job of es back to queued with attempts reset.
// ErrNotFound when the job does not exist or is not failed.
func RetryPrintJob(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewUpdate().Model((*PrintJob)(nil)).
		Set("status = ?", PrintJobQueued).Set("attempts = 0").Set("next_attempt_at = ?", now).
		Set("locked_until = NULL").Set("last_error = NULL").Set("updated_at = ?", now).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("status = ?", PrintJobFailed).
		Exec(ctx)
	if err != nil {
		return wrapWrite("retry print job", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}

// DuePrintJobs returns the ids of the jobs to deliver at now, oldest due
// first: queued jobs whose next attempt is due and printing jobs whose
// lease expired (a worker crashed mid-send).
func DuePrintJobs(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]string, error) {
	var ids []string
	err := db.NewSelect().Model((*PrintJob)(nil)).Column("id").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("status = ? AND next_attempt_at <= ?", PrintJobQueued, now)
			}).WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("status = ? AND locked_until < ?", PrintJobPrinting, now)
			})
		}).
		OrderExpr("next_attempt_at ASC, id ASC").Limit(int64(limit)).Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: due print jobs: %w", err)
	}
	return ids, nil
}

// ClaimPrintJob takes the job for one delivery attempt: the conditional
// update (queued and due, or printing with an expired lease → printing with
// a new lease) succeeds for exactly one of concurrent workers. It returns
// the claimed job, or ErrNotFound when another worker has it.
func ClaimPrintJob(ctx context.Context, db bun.IDB, id string, now, leaseUntil time.Time) (*PrintJob, error) {
	res, err := db.NewUpdate().Model((*PrintJob)(nil)).
		Set("status = ?", PrintJobPrinting).Set("locked_until = ?", leaseUntil).Set("updated_at = ?", now).
		Where("id = ?", id).
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.WhereGroup(" OR ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
				return q.Where("status = ? AND next_attempt_at <= ?", PrintJobQueued, now)
			}).WhereGroup(" OR ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
				return q.Where("status = ? AND locked_until < ?", PrintJobPrinting, now)
			})
		}).Exec(ctx)
	if err != nil {
		return nil, wrapWrite("claim print job", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNotFound
	}
	j := new(PrintJob)
	err = db.NewSelect().Model(j).Where("pj.id = ?", id).Scan(ctx)
	return j, wrapRead("get claimed print job", err)
}

// FinishPrintJobPrinted marks a claimed job printed. It only applies while
// the job is still printing with the lease of this worker (a deleted
// printer may have cancelled it meanwhile): false otherwise.
func FinishPrintJobPrinted(ctx context.Context, db bun.IDB, id string, lease, now time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*PrintJob)(nil)).
		Set("status = ?", PrintJobPrinted).Set("printed_at = ?", now).Set("attempts = attempts + 1").
		Set("locked_until = NULL").Set("next_attempt_at = NULL").Set("last_error = NULL").Set("updated_at = ?", now).
		Where("id = ?", id).Where("status = ?", PrintJobPrinting).Where("locked_until = ?", lease).Exec(ctx)
	if err != nil {
		return false, wrapWrite("finish print job", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishPrintJobFailed records a failed attempt of a claimed job: back to
// queued at nextAttempt, or failed when nextAttempt is nil.
func FinishPrintJobFailed(ctx context.Context, db bun.IDB, id string, lease, now time.Time, nextAttempt *time.Time, lastError string) (bool, error) {
	status := PrintJobQueued
	if nextAttempt == nil {
		status = PrintJobFailed
	}
	res, err := db.NewUpdate().Model((*PrintJob)(nil)).
		Set("status = ?", status).Set("attempts = attempts + 1").Set("next_attempt_at = ?", nextAttempt).
		Set("locked_until = NULL").Set("last_error = ?", lastError).Set("updated_at = ?", now).
		Where("id = ?", id).Where("status = ?", PrintJobPrinting).Where("locked_until = ?", lease).Exec(ctx)
	if err != nil {
		return false, wrapWrite("fail print job", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
