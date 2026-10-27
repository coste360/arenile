package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// M8 reports (spec 30.2.4-30.2.5, ADR 0028): partner statement, report per
// partner and the agreements register. Basis: the booked statuses of the
// reports, test bookings excluded, bookings whose stay overlaps the
// period. Spot-days count the days of the stay inside the period times
// the units of the booking (a half day counts as a day). People are adults
// + children of the booking (0 when not given). The discount is the one of
// the agreement in the price (price_current, else price_snapshot): list
// price - applied price, the same amounts as the cash reports.

// PartnerTotals are the counters of one partner (or of all of them).
type PartnerTotals struct {
	Bookings int
	People   int
	SpotDays int
	Revenue  money.Cents
	Discount money.Cents
	Charged  money.Cents
	Paid     money.Cents
}

// PartnerRow is one partner of the report.
type PartnerRow struct {
	PartnerID   string
	PartnerName string
	Kind        string
	PartnerTotals
}

// PartnersReport is api.PartnersReport.
type PartnersReport struct {
	Period Period
	Rows   []PartnerRow
	Totals PartnerTotals
}

// StatementBooking is one booking of a partner statement.
type StatementBooking struct {
	BookingID           string
	Code                string
	Status              string
	CustomerDisplayName string
	GroupName           string
	AgreementName       string
	StartsOn, EndsOn    calendar.Date
	SpotDays            int
	People              int
	Total               money.Cents
	Discount            money.Cents
	PartnerAmount       money.Cents
}

// Statement is api.PartnerStatement (without the partner, added by the
// handler from internal/domain/partner). INV-104: ClosingBalance =
// OpeningBalance + Charged - Paid.
type Statement struct {
	Period         Period
	Bookings       []StatementBooking
	Payments       []store.PartnerPayment
	Charged        money.Cents
	Paid           money.Cents
	OpeningBalance money.Cents
	ClosingBalance money.Cents
	Totals         StatementTotals
	GeneratedAt    time.Time
}

// StatementTotals are the counters of the bookings of a statement.
type StatementTotals struct {
	Bookings int
	People   int
	SpotDays int
	Total    money.Cents
	Discount money.Cents
}

// AgreementDiscount returns the discount given by the agreement in a price
// breakdown (the sum of the "agreement:" discount sources, positive).
func AgreementDiscount(price []byte) money.Cents {
	if len(price) == 0 {
		return 0
	}
	var bd pricing.PriceBreakdown
	if err := json.Unmarshal(price, &bd); err != nil {
		return 0
	}
	var d money.Cents
	for _, x := range bd.Discounts {
		if strings.HasPrefix(x.Source, "agreement:") {
			d -= x.Amount
		}
	}
	return max(d, 0)
}

// overlapDays is the number of days of [from, to] inside the period.
func overlapDays(from, to calendar.Date, p Period) int {
	if from.Before(p.From) {
		from = p.From
	}
	if to.After(p.To) {
		to = p.To
	}
	if to.Before(from) {
		return 0
	}
	return from.DaysUntil(to) + 1
}

// people of a booking (adults + children, 0 when not given).
func people(b *store.M8ReportBooking) int {
	n := 0
	if b.PartyAdults != nil {
		n += *b.PartyAdults
	}
	if b.PartyChildren != nil {
		n += *b.PartyChildren
	}
	return n
}

func spotDays(b *store.M8ReportBooking, p Period) int {
	return overlapDays(b.StartsOn, b.EndsOn, p) * max(b.Units, 1)
}

// BuildStatement is the pure builder of a statement.
func BuildStatement(p Period, rows []store.M8ReportBooking, agreementNames map[string]string, before store.PartnerBalance, payments []store.PartnerPayment) *Statement {
	st := &Statement{Period: p, Bookings: []StatementBooking{}, Payments: []store.PartnerPayment{}}
	for i := range rows {
		b := &rows[i]
		sb := StatementBooking{
			BookingID: b.ID, Code: b.Code, Status: b.Status, GroupName: b.GroupName, StartsOn: b.StartsOn, EndsOn: b.EndsOn,
			CustomerDisplayName: strings.TrimSpace(b.LastName + " " + b.FirstName), SpotDays: spotDays(b, p), People: people(b),
			Total: b.TotalAmount, Discount: AgreementDiscount(b.Price()), PartnerAmount: b.PartnerAmount,
		}
		if b.AgreementID != nil {
			sb.AgreementName = agreementNames[*b.AgreementID]
		}
		st.Bookings = append(st.Bookings, sb)
		st.Charged += b.PartnerAmount
		st.Totals.Bookings++
		st.Totals.People += sb.People
		st.Totals.SpotDays += sb.SpotDays
		st.Totals.Total += sb.Total
		st.Totals.Discount += sb.Discount
	}
	for _, pay := range payments {
		if !p.Contains(pay.ReceivedOn) {
			continue
		}
		st.Payments = append(st.Payments, pay)
		if pay.VoidedAt == nil {
			st.Paid += pay.Amount
		}
	}
	st.OpeningBalance = before.Balance()
	st.ClosingBalance = st.OpeningBalance + st.Charged - st.Paid
	return st
}

// BuildPartnersReport is the pure builder of the report per partner: one
// row per partner with bookings or payments in the period (or the partner
// asked for), by name.
func BuildPartnersReport(p Period, partners []store.Partner, rows []store.M8ReportBooking, payments []store.PartnerPayment, only string) *PartnersReport {
	byID := map[string]*PartnerRow{}
	get := func(id string) *PartnerRow {
		if r, ok := byID[id]; ok {
			return r
		}
		r := &PartnerRow{PartnerID: id}
		byID[id] = r
		return r
	}
	for i := range rows {
		b := &rows[i]
		if b.PartnerID == nil {
			continue
		}
		r := get(*b.PartnerID)
		r.Bookings++
		r.People += people(b)
		r.SpotDays += spotDays(b, p)
		r.Revenue += b.TotalAmount
		r.Discount += AgreementDiscount(b.Price())
		r.Charged += b.PartnerAmount
	}
	for _, pay := range payments {
		if pay.VoidedAt != nil || !p.Contains(pay.ReceivedOn) {
			continue
		}
		get(pay.PartnerID).Paid += pay.Amount
	}
	if only != "" {
		get(only)
	}
	out := &PartnersReport{Period: p, Rows: []PartnerRow{}}
	for _, pa := range partners {
		r, ok := byID[pa.ID]
		if !ok || (only != "" && pa.ID != only) {
			continue
		}
		r.PartnerName, r.Kind = pa.Name, pa.Kind
		out.Rows = append(out.Rows, *r)
		t := &out.Totals
		t.Bookings += r.Bookings
		t.People += r.People
		t.SpotDays += r.SpotDays
		t.Revenue += r.Revenue
		t.Discount += r.Discount
		t.Charged += r.Charged
		t.Paid += r.Paid
	}
	sort.SliceStable(out.Rows, func(i, j int) bool { return out.Rows[i].PartnerName < out.Rows[j].PartnerName })
	return out
}

// Partners returns the report per partner (partnerID empty = all).
// Requires auth.PermReportsRead.
func (s *Service) Partners(ctx context.Context, p *auth.Principal, establishmentID string, period Period, partnerID string) (*PartnersReport, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	f := store.M8BookingFilter{From: period.From, To: period.To, WithPartner: true}
	var ids []string
	if partnerID != "" {
		if _, err := store.GetPartner(ctx, s.d.DB, e.es, partnerID); err != nil {
			return nil, notFoundOr(err)
		}
		f.PartnerIDs, ids = []string{partnerID}, []string{partnerID}
	}
	rows, err := store.ReportM8Bookings(ctx, s.d.DB, e.es, f)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	payments, err := store.PartnerPaymentsBetween(ctx, s.d.DB, e.es, ids, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	partners, err := store.ListAllPartners(ctx, s.d.DB, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return BuildPartnersReport(period, partners, rows, payments, partnerID), nil
}

// PartnerStatement returns the statement of a partner for the period.
// Requires auth.PermReportsRead.
func (s *Service) PartnerStatement(ctx context.Context, p *auth.Principal, establishmentID, partnerID string, period Period) (*Statement, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	if _, err := store.GetPartner(ctx, s.d.DB, e.es, partnerID); err != nil {
		return nil, notFoundOr(err)
	}
	rows, err := store.ReportM8Bookings(ctx, s.d.DB, e.es, store.M8BookingFilter{From: period.From, To: period.To, PartnerIDs: []string{partnerID}})
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	before, err := store.PartnerAccountBefore(ctx, s.d.DB, e.es, partnerID, period.From)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	payments, err := store.PartnerPaymentsBetween(ctx, s.d.DB, e.es, []string{partnerID}, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	names, err := s.agreementNames(ctx, e)
	if err != nil {
		return nil, err
	}
	st := BuildStatement(period, rows, names, before, payments)
	st.GeneratedAt = s.d.Clock.Now().UTC()
	return st, nil
}

func (s *Service) agreementNames(ctx context.Context, e *env) (map[string]string, error) {
	ags, err := store.ListAllAgreements(ctx, s.d.DB, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	out := make(map[string]string, len(ags))
	for _, a := range ags {
		out[a.ID] = a.Name
	}
	return out, nil
}

// partnerKindLabels are the Italian labels of the partner kinds.
var partnerKindLabels = map[string]string{
	store.PartnerKindAccommodation: "Struttura ricettiva", store.PartnerKindPublicBody: "Ente",
	store.PartnerKindCompany: "Azienda", store.PartnerKindAssociation: "Associazione", store.PartnerKindOther: "Altro",
}

// WritePartnersCSV writes the report per partner.
func WritePartnersCSV(w io.Writer, r *PartnersReport) error {
	c := NewCSVWriter(w)
	c.Row("Partner", "Tipo", "Prenotazioni", "Presenze", "Giorni-postazione", "Incasso", "Sconto concesso", "Addebitato", "Pagato") //nolint:misspell // Italian
	for _, x := range r.Rows {
		c.Row(text(x.PartnerName), labelOr(partnerKindLabels, x.Kind), itoa(x.Bookings), itoa(x.People), itoa(x.SpotDays),
			eur(x.Revenue), eur(x.Discount), eur(x.Charged), eur(x.Paid))
	}
	t := r.Totals
	c.Row("Totale", "", itoa(t.Bookings), itoa(t.People), itoa(t.SpotDays), eur(t.Revenue), eur(t.Discount), eur(t.Charged), eur(t.Paid))
	return c.Flush()
}

// WriteStatementCSV writes a partner statement: the bookings, the payments
// and the balance.
func WriteStatementCSV(w io.Writer, st *Statement) error {
	c := NewCSVWriter(w)
	c.Row("Codice", "Cliente", "Gruppo", "Convenzione", "Stato", "Dal", "Al", "Giorni-postazione", "Persone", "Totale", "Sconto", "A carico del partner")
	for _, b := range st.Bookings {
		c.Row(text(b.Code), text(b.CustomerDisplayName), text(b.GroupName), text(b.AgreementName), labelOr(statusLabels, b.Status),
			dateCell(b.StartsOn), dateCell(b.EndsOn), itoa(b.SpotDays), itoa(b.People), eur(b.Total), eur(b.Discount), eur(b.PartnerAmount))
	}
	t := st.Totals
	c.Row("Totale", "", "", "", "", "", "", itoa(t.SpotDays), itoa(t.People), eur(t.Total), eur(t.Discount), eur(st.Charged))
	c.Row("")
	c.Row("Pagamento ricevuto il", "Metodo", "Riferimento", "Importo", "Annullato")
	for _, pay := range st.Payments {
		voided := ""
		if pay.VoidedAt != nil {
			voided = "sì"
		}
		c.Row(dateCell(pay.ReceivedOn), labelOr(paymentMethodLabels, pay.Method), text(pay.Reference), eur(pay.Amount), voided)
	}
	c.Row("")
	c.Row("Saldo iniziale", eur(st.OpeningBalance))
	c.Row("Addebitato", eur(st.Charged))
	c.Row("Pagato", eur(st.Paid))
	c.Row("Saldo finale", eur(st.ClosingBalance))
	return c.Flush()
}

var paymentMethodLabels = map[string]string{
	"bank_transfer": "Bonifico", "cash": "Contanti", "card": "Carta", "other": "Altro",
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// ErrNotFoundM8 is an unknown or out-of-scope partner, agreement or season
// of the M8 reports (404).
var ErrNotFoundM8 = fmt.Errorf("%w: report subject", configerr.ErrNotFound)

func notFoundOr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFoundM8
	}
	return fmt.Errorf("report: %w", err)
}
