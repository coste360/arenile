package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// PrintEstablishment is the header of the receipt.
type PrintEstablishment struct {
	Name      string
	LegalName string
	VATNumber string
	Address   string
	LogoURL   string
}

// PrintData is what the browser needs to print a ticket (receipt 80/58 mm,
// A4, A5): no money is computed client side.
type PrintData struct {
	Format        string
	View          View
	Establishment PrintEstablishment
	QRPayload     string
	Reprint       bool
	FooterText    string
	IssuedAt      time.Time
}

// validFormat reports whether f is a print format (empty: the default).
func validFormat(f string) bool {
	switch f {
	case "", store.PrintThermal80, store.PrintThermal58, store.PrintA4, store.PrintA5:
		return true
	}
	return false
}

// Print returns the print data of a ticket in format (empty: the
// establishment default), auth.PermBookingsRead. No write: the first print
// after the sale is not a reprint.
func (s *Service) Print(ctx context.Context, p *auth.Principal, establishmentID, id, format string) (*PrintData, error) {
	if !validFormat(format) {
		return nil, configerr.Invalid("format", "Usa thermal_80, thermal_58, a4 o a5.")
	}
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	t, err := store.GetTicket(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	return s.printData(ctx, s.d.DB, es, t, format, t.PrintCount > 0)
}

// Reprint counts a new print of the ticket, writes the audit entry and
// returns the print data marked as a reprint (auth.PermBookingsWrite).
func (s *Service) Reprint(ctx context.Context, p *auth.Principal, establishmentID, id, format string) (*PrintData, error) {
	if !validFormat(format) {
		return nil, configerr.Invalid("format", "Usa thermal_80, thermal_58, a4 o a5.")
	}
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *PrintData
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		t, err := store.LockTicket(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		n, err := store.IncrementTicketPrints(ctx, tx, es, id, s.now())
		if err != nil {
			return err
		}
		t.PrintCount, t.Version = n, t.Version+1
		if err := s.record(ctx, tx, p, es, "ticket.reprinted", t, map[string]any{"print_count": n, "format": format}); err != nil {
			return err
		}
		out, err = s.printData(ctx, tx, es, t, format, true)
		return err
	})
	return out, err
}

func (s *Service) printData(ctx context.Context, db bun.IDB, es store.EstablishmentScope, t *store.Ticket, format string, reprint bool) (*PrintData, error) {
	v, err := view(ctx, db, es, t)
	if err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, err
	}
	out := &PrintData{
		Format: format, View: *v, QRPayload: v.BookingCode, Reprint: reprint, IssuedAt: t.CreatedAt,
		Establishment: PrintEstablishment{Name: est.Name, Address: formatAddress(est.Address), LogoURL: est.Theme.LogoURL},
	}
	if org, err := store.GetOrganization(ctx, db, est.OrganizationID); err == nil {
		out.Establishment.LegalName = org.Name
		if org.VATNumber != nil {
			out.Establishment.VATNumber = *org.VATNumber
		}
	}
	st, err := store.GetDailySaleSettings(ctx, db, es)
	switch {
	case err == nil:
		if out.Format == "" {
			out.Format = st.PrintFormat
		}
		out.FooterText = localized(st.FooterText, est.DefaultLocale)
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	if out.Format == "" {
		out.Format = store.PrintThermal80
	}
	return out, nil
}

// formatAddress is the one-line address of the receipt header.
func formatAddress(a store.Address) string {
	city := strings.TrimSpace(strings.Join(nonEmpty(a.PostalCode, a.City), " "))
	if a.Province != "" {
		city = strings.TrimSpace(city + " (" + a.Province + ")")
	}
	return strings.Join(nonEmpty(a.Street, city), ", ")
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// localized picks the text of locale, then Italian (the source language),
// then any.
func localized(t store.I18nText, locale string) string {
	if v := t[locale]; v != "" {
		return v
	}
	if v := t["it"]; v != "" {
		return v
	}
	for _, v := range t {
		if v != "" {
			return v
		}
	}
	return ""
}
