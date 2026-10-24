package report

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// List exports (GET /exports/{list}, ADR 0019 F): customers, bookings,
// season passes and payments as a table in the format of the report CSVs
// (Italian headers, euro with decimal comma, dd/mm/yyyy, free text
// neutralised against formula injection). The rows are read in batches of
// exportBatch and written to a RowSink as they arrive (streaming), so an
// export never holds the whole list in memory.
//
// Personal data: the customer display name only, as in the report CSVs
// (open balances, top customers); never email, phone, notes or addresses.
// Anonymized customers show AnonymizedName. Test bookings and payments are
// always excluded (INV-74); the online placeholder customer is skipped.

// List kinds (api.ExportListKind).
const (
	ListCustomers    = "customers"
	ListBookings     = "bookings"
	ListSeasonPasses = "season-passes"
	ListPayments     = "payments"
)

// exportBatch is the page size of the export queries.
const exportBatch = 500

// maxListRange bounds a dated export (bookings, payments) to about two
// seasons, so that a typo in the dates never scans years of rows.
const maxListRange = 731

// placeholderTag is customer.TagOnlinePlaceholder (not imported to keep
// report independent of the customer service).
const placeholderTag = "_online_placeholder"

// RowSink receives the rows of an export (CSVWriter, or the XLSX adapter
// of the HTTP layer). The first row is the header.
type RowSink interface {
	Row(fields ...string)
}

// ListExport is an authorised and validated export, ready to be written.
type ListExport struct {
	s        *Service
	es       store.EstablishmentScope
	kind     string
	from, to *calendar.Date
	tz       *time.Location
}

// Kind is the list kind.
func (x *ListExport) Kind() string { return x.kind }

// PrepareList authorises auth.PermReportsRead and validates the request
// before anything is written (errors here can still become a problem
// response). from/to apply to bookings and season passes (stay overlap)
// and payments (business date); customers ignore them.
func (s *Service) PrepareList(ctx context.Context, p *auth.Principal, establishmentID, kind string, from, to *calendar.Date) (*ListExport, error) {
	if !slices.Contains([]string{ListCustomers, ListBookings, ListSeasonPasses, ListPayments}, kind) {
		return nil, configerr.Invalid("list", "Elenco sconosciuto.")
	}
	if from != nil && to != nil {
		if to.Before(*from) {
			return nil, configerr.Invalid("to", "La data finale precede quella iniziale.")
		}
		if from.DaysUntil(*to) > maxListRange {
			return nil, configerr.Invalid("to", "Il periodo può durare al massimo due anni.")
		}
	}
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	return &ListExport{s: s, es: e.es, kind: kind, from: from, to: to, tz: e.loc}, nil
}

// Write streams the export into sink.
func (x *ListExport) Write(ctx context.Context, sink RowSink) error {
	switch x.kind {
	case ListCustomers:
		return x.customers(ctx, sink)
	case ListBookings, ListSeasonPasses:
		return x.bookings(ctx, sink, x.kind == ListSeasonPasses)
	default:
		return x.payments(ctx, sink)
	}
}

func (x *ListExport) customers(ctx context.Context, sink RowSink) error {
	sink.Row("Cliente", "Lingua", "Etichette", "Consenso marketing", "Cliente dal")
	after := ""
	for {
		rows, err := store.ExportCustomersBatch(ctx, x.s.d.DB, x.es, after, exportBatch)
		if err != nil {
			return err
		}
		for _, c := range rows {
			if slices.Contains(c.Tags, placeholderTag) {
				continue
			}
			tags := ""
			if c.AnonymizedAt == nil {
				tags = text(strings.Join(c.Tags, ", "))
			}
			sink.Row(text(displayName(c.FirstName, c.LastName, c.AnonymizedAt)), c.Locale, tags,
				yesNo(c.MarketingConsent != nil && c.AnonymizedAt == nil), dayLabel(calendar.In(c.CreatedAt, x.loc())))
		}
		if len(rows) < exportBatch {
			return nil
		}
		after = rows[len(rows)-1].ID
	}
}

func (x *ListExport) bookings(ctx context.Context, sink RowSink, passes bool) error {
	if passes {
		sink.Row("Codice", "Cliente", "Postazione", "Stato", "Dal", "Al", "Fascia", "Totale", "Pagato", "Saldo", "Creato il")
	} else {
		sink.Row("Codice", "Cliente", "Stato", "Canale", "Dal", "Al", "Fascia", "Formula", "Abbonamento", "Totale", "Pagato", "Saldo", "Creata il")
	}
	after := ""
	for {
		rows, err := store.ExportBookingsBatch(ctx, x.s.d.DB, x.es, x.from, x.to, passes, after, exportBatch)
		if err != nil {
			return err
		}
		var spots map[string]string
		if passes {
			ids := make([]string, len(rows))
			for i := range rows {
				ids[i] = rows[i].ID
			}
			if spots, err = store.ExportPassSpots(ctx, x.s.d.DB, x.es, ids); err != nil {
				return err
			}
		}
		for i := range rows {
			b := &rows[i]
			name := text(displayName(b.FirstName, b.LastName, b.AnonymizedAt))
			balance := max(b.TotalAmount-b.PaidAmount, 0)
			created := dayLabel(calendar.In(b.CreatedAt, x.loc()))
			if passes {
				sink.Row(text(b.Code), name, text(spots[b.ID]), labelOr(bookingStatusLabels, b.Status), dateCell(b.StartsOn), dateCell(b.EndsOn),
					labelOr(slotsLabels, string(b.Slots)), eur(b.TotalAmount), eur(b.PaidAmount), eur(balance), created)
				continue
			}
			sink.Row(text(b.Code), name, labelOr(bookingStatusLabels, b.Status), labelOr(channelLabels, b.Channel),
				dateCell(b.StartsOn), dateCell(b.EndsOn), labelOr(slotsLabels, string(b.Slots)), labelOr(formulaLabels, b.Formula), yesNo(b.IsSeasonPass),
				eur(b.TotalAmount), eur(b.PaidAmount), eur(balance), created)
		}
		if len(rows) < exportBatch {
			return nil
		}
		after = rows[len(rows)-1].ID
	}
}

func (x *ListExport) payments(ctx context.Context, sink RowSink) error {
	sink.Row("Data", "Prenotazione", "Movimento", "Metodo", "Provider", "Stato", "Importo")
	after := ""
	for {
		rows, err := store.ExportPaymentsBatch(ctx, x.s.d.DB, x.es, x.from, x.to, after, exportBatch)
		if err != nil {
			return err
		}
		for i := range rows {
			pm := &rows[i]
			code := ""
			if pm.BookingCode != nil {
				code = text(*pm.BookingCode)
			}
			amount := pm.Amount
			if pm.Direction == store.PaymentDirectionRefund {
				amount = -amount
			}
			sink.Row(dateCell(pm.BusinessDate), code, labelOr(directionLabels, pm.Direction), labelOr(methodLabels, pm.Method),
				text(pm.Provider), labelOr(paymentStatusLabels, pm.Status), eur(amount))
		}
		if len(rows) < exportBatch {
			return nil
		}
		after = rows[len(rows)-1].ID
	}
}

// loc is the establishment timezone (UTC when unknown).
func (x *ListExport) loc() *time.Location {
	if x.tz != nil {
		return x.tz
	}
	return time.UTC
}

func yesNo(b bool) string {
	if b {
		return "Sì"
	}
	return "No"
}

var (
	bookingStatusLabels = map[string]string{
		"draft": "Bozza", "held": "In attesa", "pending_payment": "In pagamento", "awaiting_approval": "Da approvare",
		"confirmed": "Confermata", "checked_in": "Arrivato", "completed": "Completata", "cancelled": "Annullata", //nolint:misspell // Italian
		"no_show": "Non presentato", "expired": "Scaduta", "rejected": "Rifiutata",
	}
	slotsLabels         = map[string]string{"AM": "Mattina", "PM": "Pomeriggio", "FULL": "Giornata intera"}
	directionLabels     = map[string]string{store.PaymentDirectionIn: "Incasso", store.PaymentDirectionRefund: "Rimborso"}
	paymentStatusLabels = map[string]string{
		store.PaymentPending: "In attesa", store.PaymentSucceeded: "Riuscito", store.PaymentFailed: "Non riuscito", store.PaymentCancelled: "Annullato",
	}
)
