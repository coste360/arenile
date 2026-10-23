package fiscal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// DefaultDriver is the fiscal driver used when the establishment settings
// name none (settings.fiscal.driver): builtin.NoneFiscal.
const DefaultDriver = "none"

// MaxRangeDays bounds the date range of one corrispettivi export.
const MaxRangeDays = 366

// Errors of the fiscal package.
var (
	// ErrEstablishmentNotFound: unknown or out-of-scope establishment (404).
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrUnknownDriver: settings.fiscal.driver names a driver that is not
	// registered; payments cannot be recorded until it is fixed.
	ErrUnknownDriver = fmt.Errorf("%w: driver fiscale sconosciuto", configerr.ErrConflict)
)

// Drivers resolves fiscal drivers by code. Implemented by *app.Registry.
type Drivers interface {
	FiscalDriver(code string) (ext.FiscalDriver, bool)
}

// Deps are the dependencies of the fiscal service. Every field is required
// except Log.
type Deps struct {
	DB      bun.IDB
	Clock   clock.Clock
	Audit   *audit.Writer
	Outbox  *event.Outbox
	Log     *slog.Logger
	Drivers Drivers
}

// Service implements the fiscal use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// DocumentChange is the payload (version 1) of fiscal_document.recorded:
// ids, status and amounts only, no personal data.
type DocumentChange struct {
	DocumentID     string `json:"document_id"`
	PaymentID      string `json:"payment_id"`
	BookingID      string `json:"booking_id,omitempty"`
	Driver         string `json:"driver"`
	Status         string `json:"status"`
	Sequence       int    `json:"sequence"`
	DocumentNumber string `json:"document_number,omitempty"`
	BusinessDate   string `json:"business_date"`
	Total          int64  `json:"total"`
}

// documentPayload is stored in fiscal_documents.payload: the request sent
// to the driver plus the internal daily sequence number.
type documentPayload struct {
	Sequence int                       `json:"sequence"`
	Request  ext.FiscalDocumentRequest `json:"request"`
}

// settingsDoc is the part of establishments.settings the fiscal module
// reads: settings.fiscal.driver (absent = DefaultDriver) and the default VAT
// rate used for payments recorded without a VAT split.
type settingsDoc struct {
	Fiscal struct {
		Driver string `json:"driver"`
	} `json:"fiscal"`
	Pricing struct {
		DefaultVATRateBP int64 `json:"default_vat_rate_bp"`
	} `json:"pricing"`
}

func parseSettings(e *store.Establishment) settingsDoc {
	var sd settingsDoc
	_ = e.Settings.Unmarshal(&sd) // a malformed document falls back to the defaults
	if sd.Fiscal.Driver == "" {
		sd.Fiscal.Driver = DefaultDriver
	}
	return sd
}

// RecordPayment implements payment.Fiscal. In the payment transaction it
// assigns the next daily sequence number of the establishment (serialized by
// store.LockFiscalSequence) and writes a fiscal_documents row plus the
// fiscal_document.recorded event. The "none" driver performs no I/O, so it
// is called inline and the document is stored as issued (tracking only: the
// operator issues the receipt with their own registered cash register);
// any other driver leaves the document pending and the job of Jobs issues
// it after commit, so that device I/O never runs inside the transaction.
func (s *Service) RecordPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, pm *store.Payment) error {
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return fmt.Errorf("fiscal: load establishment: %w", err)
	}
	code := parseSettings(est).Fiscal.Driver
	drv, ok := s.d.Drivers.FiscalDriver(code)
	if !ok {
		return ErrUnknownDriver
	}
	if err := store.LockFiscalSequence(ctx, tx, es, pm.BusinessDate); err != nil {
		return fmt.Errorf("fiscal: %w", err)
	}
	n, err := store.CountFiscalDocumentsOn(ctx, tx, es, pm.BusinessDate)
	if err != nil {
		return fmt.Errorf("fiscal: %w", err)
	}
	seq := n + 1
	now := s.d.Clock.Now().UTC()
	req := documentRequest(est, pm, now)
	payload, err := store.MarshalJSONValue(documentPayload{Sequence: seq, Request: req})
	if err != nil {
		return fmt.Errorf("fiscal: %w", err)
	}
	doc := &store.FiscalDocument{
		ID: store.NewID(), PaymentID: pm.ID, Driver: drv.Code(), Status: store.FiscalPending,
		BusinessDate: pm.BusinessDate, Payload: payload, CreatedAt: now, UpdatedAt: now,
	}
	if drv.Code() == DefaultDriver {
		// No device: the call is pure and cannot fail on I/O.
		res, ierr := drv.Issue(ctx, req)
		applyOutcome(doc, sequenceNumber(pm.BusinessDate, seq), res, ierr)
	}
	if err := store.InsertFiscalDocument(ctx, tx, es, doc); err != nil {
		return fmt.Errorf("fiscal: %w", err)
	}
	return s.publish(ctx, tx, es, doc, req.BookingID, req.Total, seq)
}

func (s *Service) publish(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, doc *store.FiscalDocument, bookingID string, total int64, seq int) error {
	ch := DocumentChange{
		DocumentID: doc.ID, PaymentID: doc.PaymentID, BookingID: bookingID, Driver: doc.Driver, Status: doc.Status,
		Sequence: seq, BusinessDate: doc.BusinessDate.String(), Total: total,
	}
	if doc.DocumentNumber != nil {
		ch.DocumentNumber = *doc.DocumentNumber
	}
	if _, err := s.d.Outbox.Write(ctx, tx, event.FiscalDocumentRecorded, es.ID(), 1, ch); err != nil {
		return fmt.Errorf("fiscal: %w", err)
	}
	return nil
}

// sequenceNumber is the internal progressive of a document: YYYYMMDD-NNNN,
// restarting every business day.
func sequenceNumber(d calendar.Date, seq int) string {
	return fmt.Sprintf("%04d%02d%02d-%04d", d.Year(), int(d.Month()), d.Day(), seq)
}

// isDeferred reports whether a driver error means "accepted, outcome not
// known yet" (capability interface implemented by fiscalbridge.DeferredError).
func isDeferred(err error) bool {
	var d interface{ Deferred() bool }
	return errors.As(err, &d) && d.Deferred()
}

// applyOutcome stores the driver answer on doc: issued with the driver
// number (or the internal sequence number when the driver has none), or
// failed with the error text.
func applyOutcome(doc *store.FiscalDocument, fallbackNumber string, res *ext.FiscalDocumentResult, err error) {
	if err != nil {
		msg := err.Error()
		doc.Status = store.FiscalFailed
		doc.Error = &msg
		return
	}
	doc.Status = store.FiscalIssued
	number := fallbackNumber
	if res != nil && res.Number != "" {
		number = res.Number
	}
	doc.DocumentNumber = &number
	if res != nil {
		raw, _ := json.Marshal(struct {
			Ref      string          `json:"ref"`
			Number   string          `json:"number,omitempty"`
			IssuedAt time.Time       `json:"issued_at"`
			Raw      json.RawMessage `json:"raw,omitempty"`
		}{res.Ref, res.Number, res.IssuedAt.UTC(), res.Raw})
		doc.Response = raw
	}
}

// documentRequest builds the driver request of a payment: one line per VAT
// rate of the frozen split (refunds negative), one payment share.
func documentRequest(est *store.Establishment, pm *store.Payment, now time.Time) ext.FiscalDocumentRequest {
	sign := int64(1)
	desc := "Corrispettivo"
	if pm.Direction == store.PaymentDirectionRefund {
		sign = -1
		desc = "Reso"
	}
	req := ext.FiscalDocumentRequest{
		EstablishmentID: est.ID, PaymentID: pm.ID, Currency: est.Currency,
		Total: sign * int64(pm.Amount), IssuedAt: now, IdempotencyKey: "fiscal_" + pm.ID,
		Payments: []ext.FiscalPayment{{Method: pm.Method, Amount: sign * int64(pm.Amount)}},
	}
	if pm.BookingID != nil {
		req.BookingID = *pm.BookingID
	}
	if pm.Description != "" {
		desc = pm.Description
	}
	for _, sh := range sharesOf(pm, parseSettings(est).Pricing.DefaultVATRateBP) {
		g := sign * int64(sh.Gross)
		req.Lines = append(req.Lines, ext.FiscalLine{
			Description: fmt.Sprintf("%s IVA %s%%", desc, FormatRate(sh.RateBP)),
			Quantity:    1, UnitGross: g, Gross: g, VATRateBP: sh.RateBP,
		})
	}
	return req
}

// ---- reads ------------------------------------------------------------------

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

// CorrispettiviQuery selects the export range (inclusive) and grouping.
type CorrispettiviQuery struct {
	From    calendar.Date
	To      calendar.Date
	GroupBy GroupBy // default GroupByDay
}

// Corrispettivi returns the takings of [From, To] by period, VAT rate and
// method (spec 14.2, driver "export"). Requires auth.PermReportsRead.
func (s *Service) Corrispettivi(ctx context.Context, p *auth.Principal, establishmentID string, q CorrispettiviQuery) (*Report, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermReportsRead)
	if err != nil {
		return nil, err
	}
	if q.GroupBy == "" {
		q.GroupBy = GroupByDay
	}
	var ve configerr.ValidationError
	if !q.GroupBy.Valid() {
		ve.Add("group_by", "Valori ammessi: day, month.")
	}
	switch {
	case q.From.IsZero() || q.To.IsZero():
		ve.Add("from", "Indica il primo e l'ultimo giorno.")
	case q.To.Before(q.From):
		ve.Add("to", "L'ultimo giorno non può precedere il primo.")
	case q.From.DaysUntil(q.To) >= MaxRangeDays:
		ve.Add("to", fmt.Sprintf("L'intervallo massimo è di %d giorni.", MaxRangeDays))
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("fiscal: load establishment: %w", err)
	}
	payments, err := store.ListSucceededPayments(ctx, s.d.DB, es, q.From, q.To)
	if err != nil {
		return nil, fmt.Errorf("fiscal: %w", err)
	}
	return BuildReport(payments, q.From, q.To, q.GroupBy, est.Currency, parseSettings(est).Pricing.DefaultVATRateBP), nil
}

// ListDocuments lists the fiscal documents of the establishment, newest
// first. Requires auth.PermReportsRead.
func (s *Service) ListDocuments(ctx context.Context, p *auth.Principal, establishmentID string, f store.FiscalDocumentFilter, cursor string, limit int) (store.Page[store.FiscalDocument], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermReportsRead)
	if err != nil {
		return store.Page[store.FiscalDocument]{}, err
	}
	switch f.Status {
	case "", store.FiscalPending, store.FiscalIssued, store.FiscalFailed, store.FiscalVoid:
	default:
		return store.Page[store.FiscalDocument]{}, configerr.Invalid("status", "Stato non valido.")
	}
	page, err := store.ListFiscalDocuments(ctx, s.d.DB, es, f, cursor, limit)
	if err != nil {
		return page, fmt.Errorf("fiscal: %w", err)
	}
	return page, nil
}

// ---- issuing job ------------------------------------------------------------

// Jobs returns the background job that issues pending documents through
// their driver after commit (drivers other than "none"). Registered by the
// app with the other M3 jobs.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: "fiscal.issue_pending", Interval: time.Minute, Run: s.IssuePending}}
}

// IssuePending sends up to 100 pending documents to their driver and stores
// the outcome. A document whose driver is missing stays pending (logged).
// Device I/O happens outside any transaction; the outcome is written with a
// conditional UPDATE (status still pending), so concurrent runs issue a
// document at most once from Arenile's side (drivers receive the
// IdempotencyKey of the request for their own deduplication).
func (s *Service) IssuePending(ctx context.Context) error {
	docs, err := store.ListPendingFiscalDocuments(ctx, s.d.DB, 100)
	if err != nil {
		return fmt.Errorf("fiscal: %w", err)
	}
	for i := range docs {
		doc := &docs[i]
		drv, ok := s.d.Drivers.FiscalDriver(doc.Driver)
		if !ok {
			s.d.Log.WarnContext(ctx, "fiscal driver not registered", slog.String("driver", doc.Driver), slog.String("document_id", doc.ID))
			continue
		}
		var pl documentPayload
		if err := doc.Payload.Unmarshal(&pl); err != nil {
			s.d.Log.ErrorContext(ctx, "fiscal document payload", slog.String("document_id", doc.ID), slog.String("error", err.Error()))
			continue
		}
		res, ierr := drv.Issue(ctx, pl.Request)
		if isDeferred(ierr) {
			// Accepted by an asynchronous driver (fiscalbridge): the outcome
			// arrives later, the document stays pending until then.
			continue
		}
		applyOutcome(doc, sequenceNumber(doc.BusinessDate, pl.Sequence), res, ierr)
		doc.UpdatedAt = s.d.Clock.Now().UTC()
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, doc.EstablishmentID)
		if err != nil {
			return fmt.Errorf("fiscal: %w", err)
		}
		err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			ok, err := store.UpdateFiscalDocumentOutcome(ctx, tx, es, doc)
			if err != nil || !ok {
				return err
			}
			if err := s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: "fiscal_document." + doc.Status,
				EntityType: "fiscal_document", EntityID: doc.ID,
				Diff: map[string]any{"driver": doc.Driver, "status": doc.Status, "document_number": doc.DocumentNumber},
			}); err != nil {
				return err
			}
			return s.publish(ctx, tx, es, doc, pl.Request.BookingID, pl.Request.Total, pl.Sequence)
		})
		if err != nil {
			return fmt.Errorf("fiscal: store outcome: %w", err)
		}
	}
	return nil
}
