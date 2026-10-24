package printing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// JobDeliver is the name of the delivery job (desktop edition only).
const JobDeliver = "printing.deliver"

// Backoff is the delay before the retry following the n-th failed attempt
// (1-based): 5 s, 15 s, 1 min, then 5 min.
func Backoff(n int) time.Duration {
	switch {
	case n <= 1:
		return 5 * time.Second
	case n == 2:
		return 15 * time.Second
	case n == 3:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

// JobInput is a print request.
type JobInput struct {
	DocumentKind string
	DocumentID   string // required unless test_page
	RegisterID   *string
	PrinterID    *string // explicit printer: routing ignored
	Copies       *int    // nil = route copies (or 1)
	KickDrawer   *bool   // nil = route setting (or false)
}

// Enqueue renders a document and queues it for its printer (cash.operate,
// desktop edition). idemKey is the Idempotency-Key header; when empty the
// key is "<kind>:<document id>:<printer id>". created is false when a job
// with the same key already existed (it is returned unchanged).
func (s *Service) Enqueue(ctx context.Context, p *auth.Principal, establishmentID string, in JobInput, idemKey string) (job *store.PrintJob, created bool, err error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, false, err
	}
	if !s.d.Available {
		return nil, false, ErrUnavailable
	}
	return s.enqueue(ctx, es, p, in, idemKey)
}

// TestPrint queues a test page on a printer, ignoring routing (cash.operate,
// desktop edition).
func (s *Service) TestPrint(ctx context.Context, p *auth.Principal, establishmentID, printerID, idemKey string) (*store.PrintJob, bool, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, false, err
	}
	if !s.d.Available {
		return nil, false, ErrUnavailable
	}
	return s.enqueue(ctx, es, p, JobInput{DocumentKind: store.PrintDocTestPage, PrinterID: &printerID}, idemKey)
}

func (s *Service) enqueue(ctx context.Context, es store.EstablishmentScope, p *auth.Principal, in JobInput, idemKey string) (*store.PrintJob, bool, error) {
	var ve configerr.ValidationError
	if !documentKinds[in.DocumentKind] {
		ve.Add("document_kind", "Tipo di documento non valido.")
	}
	in.DocumentID = strings.TrimSpace(in.DocumentID)
	if in.DocumentKind == store.PrintDocTestPage {
		in.DocumentID = ""
	} else if in.DocumentID == "" {
		ve.Add("document_id", "Obbligatorio per questo documento.")
	} else if !validID(in.DocumentID) {
		ve.Add("document_id", "Identificativo non valido.")
	}
	if in.Copies != nil && (*in.Copies < 1 || *in.Copies > 3) {
		ve.Add("copies", "Da 1 a 3 copie.")
	}
	if len(idemKey) > 255 {
		ve.Add("Idempotency-Key", "Al massimo 255 caratteri.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, false, err
	}
	if in.RegisterID != nil {
		if !validID(*in.RegisterID) {
			return nil, false, ErrNotFound
		}
		ok, err := store.CashRegisterExists(ctx, s.d.DB, es, *in.RegisterID)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, ErrNotFound
		}
	}

	copies, kick := 1, false
	var pr *store.Printer
	var err error
	if in.PrinterID != nil {
		if pr, err = s.printer(ctx, es, *in.PrinterID); err != nil {
			return nil, false, err
		}
	} else {
		route, err := s.findRoute(ctx, es, in.DocumentKind, in.RegisterID)
		if err != nil {
			return nil, false, err
		}
		if pr, err = s.printer(ctx, es, route.PrinterID); err != nil {
			return nil, false, err
		}
		copies, kick = route.Copies, route.KickDrawer
	}
	if in.Copies != nil {
		copies = *in.Copies
	}
	if in.KickDrawer != nil {
		kick = *in.KickDrawer
	}
	if !pr.Active {
		return nil, false, ErrPrinterInactive
	}

	key := idemKey
	if key == "" {
		if in.DocumentKind == store.PrintDocTestPage {
			key = in.DocumentKind + ":" + pr.ID + ":" + store.NewID()
		} else {
			key = in.DocumentKind + ":" + in.DocumentID + ":" + pr.ID
		}
	}
	if j, err := store.GetPrintJobByKey(ctx, s.d.DB, es, key); err == nil {
		return j, false, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, false, err
	}

	payload, err := s.renderDocument(ctx, es, pr, in.DocumentKind, in.DocumentID, copies, kick)
	if err != nil {
		return nil, false, err
	}
	now := s.now()
	j := &store.PrintJob{
		ID: store.NewID(), PrinterID: &pr.ID, DocumentKind: in.DocumentKind, RegisterID: in.RegisterID,
		Copies: copies, Status: store.PrintJobQueued, IdempotencyKey: key, Payload: payload,
		MaxAttempts: store.DefaultPrintJobMaxAttempts, NextAttemptAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if in.DocumentID != "" {
		j.DocumentID = &in.DocumentID
	}
	if p != nil && p.UserID != "" {
		uid := p.UserID
		j.CreatedByUserID = &uid
	}
	inserted, err := store.InsertPrintJob(ctx, s.d.DB, es, j)
	if err != nil {
		return nil, false, fmt.Errorf("printing: enqueue: %w", err)
	}
	if !inserted {
		// A concurrent request with the same key won the unique index.
		existing, err := store.GetPrintJobByKey(ctx, s.d.DB, es, key)
		return existing, false, err
	}
	s.d.Log.InfoContext(ctx, "print job queued", slog.String("job_id", j.ID), slog.String("printer_id", pr.ID),
		slog.String("kind", j.DocumentKind), slog.Int("copies", copies))
	s.Wake()
	return j, true, nil
}

// Wake asks the delivery job to run now (non-blocking, coalesced).
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// GetJob returns a print job (cash.operate).
func (s *Service) GetJob(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.PrintJob, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	if !validID(id) {
		return nil, ErrNotFound
	}
	j, err := store.GetPrintJob(ctx, s.d.DB, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	return j, err
}

// JobPage is a page of jobs with the names of their printers.
type JobPage struct {
	Items        []store.PrintJob
	NextCursor   string
	PrinterNames map[string]string
}

// ListJobs returns the jobs newest first (cash.operate).
func (s *Service) ListJobs(ctx context.Context, p *auth.Principal, establishmentID string, f store.PrintJobFilter, cursor string, limit int) (JobPage, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return JobPage{}, err
	}
	page, err := store.ListPrintJobs(ctx, s.d.DB, es, f, cursor, limit)
	if err != nil {
		return JobPage{}, err
	}
	names, err := s.PrinterNames(ctx, es, page.Items)
	if err != nil {
		return JobPage{}, err
	}
	return JobPage{Items: page.Items, NextCursor: page.NextCursor, PrinterNames: names}, nil
}

// PrinterNames returns the names of the printers of jobs, by id.
func (s *Service) PrinterNames(ctx context.Context, es store.EstablishmentScope, js []store.PrintJob) (map[string]string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, j := range js {
		if j.PrinterID != nil && !seen[*j.PrinterID] {
			seen[*j.PrinterID] = true
			ids = append(ids, *j.PrinterID)
		}
	}
	return store.PrinterNames(ctx, s.d.DB, es, ids)
}

// JobPrinterName returns the name of the printer of j ("" when deleted).
func (s *Service) JobPrinterName(ctx context.Context, p *auth.Principal, establishmentID string, j *store.PrintJob) string {
	if j == nil || j.PrinterID == nil {
		return ""
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return ""
	}
	names, err := s.PrinterNames(ctx, es, []store.PrintJob{*j})
	if err != nil {
		return ""
	}
	return names[*j.PrinterID]
}

// RetryJob puts a failed job back in the queue with attempts reset
// (cash.operate, desktop edition). Other statuses: ErrInvalidTransition.
func (s *Service) RetryJob(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.PrintJob, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	if !s.d.Available {
		return nil, ErrUnavailable
	}
	if !validID(id) {
		return nil, ErrNotFound
	}
	j, err := store.GetPrintJob(ctx, s.d.DB, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if j.Status != store.PrintJobFailed || j.PrinterID == nil || len(j.Payload) == 0 {
		return nil, ErrInvalidTransition
	}
	if err := store.RetryPrintJob(ctx, s.d.DB, es, id, s.now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidTransition
		}
		return nil, err
	}
	s.d.Log.InfoContext(ctx, "print job retried", slog.String("job_id", id))
	s.Wake()
	return store.GetPrintJob(ctx, s.d.DB, es, id)
}

// Jobs returns the delivery job on the desktop edition (none otherwise).
func (s *Service) Jobs() []jobs.Job {
	if !s.d.Available {
		return nil
	}
	return []jobs.Job{{
		Name: JobDeliver, Interval: 2 * time.Second, RunOnStart: true, Timeout: 5 * time.Minute, Wake: s.wake,
		Run: func(ctx context.Context) error {
			_, err := s.DeliverDue(ctx, 20)
			return err
		},
	}}
}

// DeliverDue sends up to limit due jobs and returns how many were printed.
// Each job is claimed with a conditional update and a lease, so concurrent
// runners (or a crash mid-send) never print a job twice in normal
// operation; a job whose lease expired is retried.
func (s *Service) DeliverDue(ctx context.Context, limit int) (int, error) {
	ids, err := store.DuePrintJobs(ctx, s.d.DB, s.now(), limit)
	if err != nil {
		return 0, err
	}
	printed := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return printed, ctx.Err()
		}
		ok, err := s.deliver(ctx, id)
		if err != nil {
			return printed, err
		}
		if ok {
			printed++
		}
	}
	return printed, nil
}

// deliver claims and sends one job.
func (s *Service) deliver(ctx context.Context, id string) (bool, error) {
	now := s.now()
	lease := now.Add(s.d.Lease).Truncate(time.Millisecond)
	j, err := store.ClaimPrintJob(ctx, s.d.DB, id, now, lease)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil // another worker has it
	}
	if err != nil {
		return false, err
	}
	log := s.d.Log.With(slog.String("job_id", j.ID), slog.String("kind", j.DocumentKind))
	fail := func(msg string, retry bool) error {
		var next *time.Time
		if retry && j.Attempts+1 < j.MaxAttempts {
			t := s.now().Add(Backoff(j.Attempts + 1))
			next = &t
		}
		_, err := store.FinishPrintJobFailed(ctx, s.d.DB, j.ID, lease, s.now(), next, msg)
		log.WarnContext(ctx, "print job attempt failed", slog.Int("attempt", j.Attempts+1),
			slog.Bool("will_retry", next != nil), slog.String("error", msg))
		return err
	}
	if j.PrinterID == nil || len(j.Payload) == 0 {
		return false, fail("printer deleted", false)
	}
	pr, err := store.GetPrinterByID(ctx, s.d.DB, *j.PrinterID)
	if errors.Is(err, store.ErrNotFound) {
		return false, fail("printer deleted", false)
	}
	if err != nil {
		return false, err
	}
	log = log.With(slog.String("printer_id", pr.ID))
	if !pr.Active {
		return false, fail("printer inactive", false)
	}
	drv, ok := s.d.Drivers[pr.Driver]
	if !ok {
		return false, fail("no driver "+pr.Driver, false)
	}
	sendCtx, cancel := context.WithTimeout(ctx, min(s.d.Lease, DefaultDialTimeout+DefaultWriteTimeout+2*time.Second))
	sendErr := drv.Send(sendCtx, Target{Host: pr.Host, Port: pr.Port}, j.Payload)
	cancel()
	at := s.now()
	if sendErr != nil {
		msg := technical(sendErr)
		_ = store.SetPrinterHealth(ctx, s.d.DB, pr.ID, store.PrinterHealth{Status: store.PrinterStatusUnreachable, CheckedAt: at, LastError: &msg})
		return false, fail(msg, !errors.Is(sendErr, ErrAddressNotAllowed))
	}
	done, err := store.FinishPrintJobPrinted(ctx, s.d.DB, j.ID, lease, at)
	if err != nil {
		return false, err
	}
	_ = store.SetPrinterHealth(ctx, s.d.DB, pr.ID, store.PrinterHealth{Status: store.PrinterStatusReachable, CheckedAt: at, Success: true})
	if done {
		log.InfoContext(ctx, "print job printed", slog.Int("attempt", j.Attempts+1))
	}
	return done, nil
}
