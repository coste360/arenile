package report

import (
	"io"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// CSV exports of the reports (format=csv or Accept: text/csv), one writer
// per report, in the shared format of CSVWriter: UTF-8 BOM, ';', CRLF,
// Italian headers, amounts in euro with decimal comma (FormatEuro), ratios
// as percentages with two decimals (FormatBP), dates as dd/mm/yyyy. Every
// table ends with a "Totale" row where totals make sense. Free text coming
// from the database (zone, pack and customer names) is neutralised against
// spreadsheet formula injection (text).

// text neutralises a free-text cell that a spreadsheet would read as a
// formula (leading =, +, -, @, tab or CR) by prefixing an apostrophe.
func text(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func eur(c money.Cents) string { return FormatEuro(c) }

func dateCell(d calendar.Date) string {
	if d.IsZero() {
		return ""
	}
	return dayLabel(d)
}

// WriteSummaryCSV writes the KPI summary as indicator/value pairs.
func WriteSummaryCSV(w io.Writer, s *Summary) error {
	c := NewCSVWriter(w)
	c.Row("Indicatore", "Valore") //nolint:misspell // Italian
	c.Row("Dal", dateCell(s.Period.From))
	c.Row("Al", dateCell(s.Period.To))
	c.Row("Occupazione %", FormatBP(s.OccupancyBP))
	c.Row("Unità occupate", FormatCount(s.OccupiedUnits))
	c.Row("Unità disponibili", FormatCount(s.CapacityUnits))
	c.Row("Incassato", eur(s.RevenueCollected))
	c.Row("Venduto", eur(s.RevenueBooked))
	c.Row("Prenotazioni", FormatCount(s.Bookings))
	c.Row("Valore medio prenotazione", eur(s.AverageBookingValue))
	c.Row("Cancellazioni", FormatCount(s.Cancellations))
	c.Row("Mancate presentazioni", FormatCount(s.NoShows))
	c.Row("Abbonamenti venduti", FormatCount(s.SeasonPassesSold))
	c.Row("Saldi aperti", eur(s.OpenBalanceTotal))
	c.Row("Prenotazioni con saldo aperto", FormatCount(s.OpenBalanceBookings))
	return c.Flush()
}

// WriteOccupancyCSV writes the occupancy report.
func WriteOccupancyCSV(w io.Writer, o *Occupancy) error {
	c := NewCSVWriter(w)
	c.Row("Chiave", "Gruppo", "Unità disponibili", "Unità occupate", "Di cui abbonati", "Rilasciate libere", "Bloccate", "Occupazione %")
	row := func(r OccupancyRow) {
		c.Row(text(r.Key), text(r.Label), FormatCount(r.CapacityUnits), FormatCount(r.OccupiedUnits), FormatCount(r.SeasonUnits),
			FormatCount(r.ReleasedUnits), FormatCount(r.BlockedUnits), FormatBP(r.OccupancyBP))
	}
	for _, r := range o.Rows {
		row(r)
	}
	row(o.Totals)
	return c.Flush()
}

// WriteRevenueCSV writes the revenue report.
func WriteRevenueCSV(w io.Writer, r *Revenue) error {
	c := NewCSVWriter(w)
	count := "Movimenti"
	if r.Basis == BasisBooked {
		switch r.GroupBy {
		case RevenueByPack:
			count = "Righe"
		case RevenueByExtra:
			count = "Quantità"
		default:
			count = "Prenotazioni"
		}
	}
	c.Row("Chiave", "Gruppo", count, "Lordo", "Imponibile", "IVA", "Rimborsi")
	row := func(x RevenueRow) {
		c.Row(text(x.Key), text(x.Label), FormatCount(x.Count), eur(x.Gross), eur(x.Taxable), eur(x.Tax), eur(x.Refunds))
	}
	for _, x := range r.Rows {
		row(x)
	}
	row(r.Totals)
	return c.Flush()
}

// WriteBookingsCSV writes the bookings report.
func WriteBookingsCSV(w io.Writer, b *Bookings) error {
	c := NewCSVWriter(w)
	c.Row("Chiave", "Gruppo", "Prenotazioni", "Valore", "Valore medio", "Quota %", "Anticipo medio (giorni)")
	row := func(x BookingsRow) {
		c.Row(text(x.Key), text(x.Label), FormatCount(x.Bookings), eur(x.Value), eur(x.AverageValue), FormatBP(x.ShareBP), FormatCount(x.AverageLeadDays))
	}
	for _, x := range b.Rows {
		row(x)
	}
	row(b.Totals)
	return c.Flush()
}

// WriteCancellationsCSV writes the cancellations and no-shows report.
func WriteCancellationsCSV(w io.Writer, r *Cancellations) error {
	c := NewCSVWriter(w)
	c.Row("Chiave", "Gruppo", "Prenotazioni", "Cancellate", "Mancate presentazioni", "Cancellate %", "Mancate presentazioni %", "Valore perso", "Rimborsato")
	row := func(x CancellationsRow) {
		c.Row(text(x.Key), text(x.Label), FormatCount(x.Bookings), FormatCount(x.Cancelled), FormatCount(x.NoShows),
			FormatBP(x.CancelledBP), FormatBP(x.NoShowBP), eur(x.LostValue), eur(x.Refunded))
	}
	for _, x := range r.Rows {
		row(x)
	}
	row(r.Totals)
	return c.Flush()
}

// WriteSeasonPassesCSV writes the releases table followed by the passes
// sold in the period.
func WriteSeasonPassesCSV(w io.Writer, r *SeasonPasses) error {
	c := NewCSVWriter(w)
	c.Row("Chiave", "Gruppo", "Unità rilasciate", "Rivendute", "Rivendita %", "Ricavo rivendite", "Crediti riconosciuti")
	row := func(x SeasonPassesRow) {
		c.Row(text(x.Key), text(x.Label), FormatCount(x.ReleasedUnits), FormatCount(x.ResoldUnits), FormatBP(x.ResaleBP),
			eur(x.ResaleRevenue), eur(x.CreditsIssued))
	}
	for _, x := range r.Rows {
		row(x)
	}
	row(r.Totals)
	c.Row()
	c.Row("Abbonamenti venduti", FormatCount(r.PassesSold))
	c.Row("Valore abbonamenti", eur(r.PassesValue))
	c.Row("Di cui rinnovi", FormatCount(r.Renewals))
	return c.Flush()
}

// WriteTopCustomersCSV writes the customer ranking (display names only).
func WriteTopCustomersCSV(w io.Writer, r *TopCustomers) error {
	c := NewCSVWriter(w)
	c.Row("Posizione", "Cliente", "Prenotazioni", "Giorni-postazione", "Incassato", "Ultima prenotazione")
	for _, x := range r.Rows {
		last := ""
		if x.LastBookingOn != nil {
			last = dateCell(*x.LastBookingOn)
		}
		c.Row(FormatCount(x.Rank), text(x.DisplayName), FormatCount(x.Bookings), FormatCount(x.SpotDays), eur(x.Revenue), last)
	}
	return c.Flush()
}

// WriteOpenBalancesCSV writes the open balances.
func WriteOpenBalancesCSV(w io.Writer, r *OpenBalances) error {
	c := NewCSVWriter(w)
	c.Row("Codice", "Cliente", "Stato", "Dal", "Al", "Totale", "Pagato", "Saldo")
	var total, paid money.Cents
	for _, x := range r.Rows {
		c.Row(text(x.Code), text(x.DisplayName), labelOr(statusLabels, x.Status), dateCell(x.StartsOn), dateCell(x.EndsOn),
			eur(x.Total), eur(x.Paid), eur(x.Balance))
		total += x.Total
		paid += x.Paid
	}
	c.Row("Totale", "", "", "", "", eur(total), eur(paid), eur(r.TotalBalance))
	return c.Flush()
}

var metricLabels = map[string]string{ //nolint:gosec // Italian labels, not credentials
	MetricOccupancyBP:      "Occupazione %",
	MetricRevenueCollected: "Incassato",
	MetricRevenueBooked:    "Venduto",
	MetricBookings:         "Prenotazioni",
	MetricAverageValue:     "Valore medio prenotazione",
	MetricCancellations:    "Cancellazioni",
	MetricNoShows:          "Mancate presentazioni",
	MetricSeasonPassesSold: "Abbonamenti venduti",
	MetricReleasedUnits:    "Unità rilasciate",
	MetricResoldUnits:      "Unità rivendute",
}

// formatMetric formats a comparison value by unit.
func formatMetric(unit string, v int64) string {
	switch unit {
	case UnitCents:
		return eur(money.Cents(v))
	case UnitBasisPoints:
		return FormatBP(v)
	default:
		return FormatCount(v)
	}
}

// WriteComparisonCSV writes the metrics table, an empty line and the
// aligned series.
func WriteComparisonCSV(w io.Writer, r *Comparison) error {
	c := NewCSVWriter(w)
	c.Row("Indicatore", "Periodo corrente", "Periodo di confronto", "Differenza", "Variazione %") //nolint:misspell // Italian
	c.Row("Periodo", dateCell(r.Current.From)+" - "+dateCell(r.Current.To), dateCell(r.Previous.From)+" - "+dateCell(r.Previous.To), "", "")
	for _, m := range r.Metrics {
		delta := ""
		if m.DeltaBP != nil {
			delta = FormatBP(*m.DeltaBP)
		}
		c.Row(labelOr(metricLabels, m.Metric), formatMetric(m.Unit, m.Current), formatMetric(m.Unit, m.Previous), formatMetric(m.Unit, m.Delta), delta)
	}
	c.Row()
	c.Row("Periodo", "Dal", "Dal (confronto)", "Incassato", "Incassato (confronto)", "Occupazione %", "Occupazione % (confronto)",
		"Prenotazioni", "Prenotazioni (confronto)")
	for _, p := range r.Series {
		c.Row(p.Label, dateCell(p.CurrentFrom), dateCell(p.PreviousFrom), eur(p.CurrentRevenue), eur(p.PreviousRevenue),
			FormatBP(p.CurrentOccupancyBP), FormatBP(p.PreviousOccupancyBP), FormatCount(p.CurrentBookings), FormatCount(p.PreviousBookings))
	}
	return c.Flush()
}
