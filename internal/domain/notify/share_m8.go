package notify

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

// Booking summary for the customer (spec 30.9): a text ready to send from
// the operator's own WhatsApp (Arenile sends nothing to third parties) and
// the data of the printable summary with the booking QR. Texts are in the
// customer's language and reuse the formatting of the booking emails.

// ShareSummary is the text of the summary and its links. ManageURL is a
// new manage link (previous links are revoked, as with RegenerateLink);
// WhatsAppURL is set only when the customer has a phone.
type ShareSummary struct {
	Text        string
	Locale      string
	ManageURL   string
	WhatsAppURL string
}

// SummaryPrint is the content of the printable summary (no manage link).
type SummaryPrint struct {
	Locale        string
	Establishment string
	Title         string
	Hint          string
	BookingCode   string
	QRPayload     string
	QRNote        string
	Summary       []Row
	Totals        []Row
	Contacts      []string
}

// ShareSummary builds the summary text of a booking with a new manage link
// (auth.PermBookingsWrite, audited by RegenerateLink). The text and the
// link are returned once and never logged.
func (s *Service) ShareSummary(ctx context.Context, p *auth.Principal, establishmentID, bookingID string) (*ShareSummary, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	d, err := s.summaryBooking(ctx, es, bookingID)
	if err != nil {
		return nil, err
	}
	link, _, err := s.RegenerateLink(ctx, p, establishmentID, bookingID, false)
	if err != nil {
		return nil, err
	}
	cat, err := catalogFor(d.messageLocale())
	if err != nil {
		return nil, err
	}
	out := &ShareSummary{Locale: cat.locale, ManageURL: link.URL}
	out.Text = shareText(cat, d, bookingContent(cat, d, contentOptions{ManageURL: link.URL}))
	if d.Customer.AnonymizedAt == nil && d.Customer.Phone != nil {
		if n := waNumber(*d.Customer.Phone); n != "" {
			out.WhatsAppURL = "https://wa.me/" + n + "?text=" + url.QueryEscape(out.Text)
		}
	}
	return out, nil
}

// SummaryPrint returns the content of the printable summary
// (auth.PermBookingsRead), in the customer's language.
func (s *Service) SummaryPrint(ctx context.Context, p *auth.Principal, establishmentID, bookingID string) (*SummaryPrint, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	d, err := s.summaryBooking(ctx, es, bookingID)
	if err != nil {
		return nil, err
	}
	cat, err := catalogFor(d.messageLocale())
	if err != nil {
		return nil, err
	}
	c := bookingContent(cat, d, contentOptions{})
	return &SummaryPrint{
		Locale: cat.locale, Establishment: d.Est.Name, Title: cat.t("summary_title", nil),
		Hint: cat.t("summary_print_hint", nil), BookingCode: d.Booking.Code, QRPayload: QRPayload(d.Booking.Code),
		QRNote: cat.t("summary_qr", nil), Summary: c.Summary, Totals: c.Totals, Contacts: c.Contacts,
	}, nil
}

func (s *Service) summaryBooking(ctx context.Context, es store.EstablishmentScope, bookingID string) (*bookingData, error) {
	d, err := loadBooking(ctx, s.d.DB, es, bookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrBookingNotFound
	}
	return d, err
}

// shareText is the plain text of the summary (pure).
func shareText(cat catalog, d *bookingData, c Content) string {
	var b strings.Builder
	name := ""
	if d.Customer.AnonymizedAt == nil {
		name = firstNonEmpty(d.Customer.FirstName, d.Customer.LastName)
	}
	if name != "" {
		b.WriteString(cat.t("share_greeting", map[string]string{"name": name, "establishment": d.Est.Name}))
	} else {
		b.WriteString(cat.t("share_greeting_anonymous", map[string]string{"establishment": d.Est.Name}))
	}
	b.WriteString("\n")
	for _, r := range c.Summary {
		if r.Label == cat.t("label_guest", nil) {
			continue
		}
		b.WriteString("\n" + r.Label + ": " + r.Value)
	}
	if len(c.Totals) > 0 {
		b.WriteString("\n")
		for _, r := range c.Totals {
			b.WriteString("\n" + r.Label + ": " + r.Value)
		}
	}
	if c.ManageURL != "" {
		b.WriteString("\n\n" + cat.t("share_manage", nil) + "\n" + c.ManageURL)
	}
	return b.String()
}

// waNumber is the wa.me number of a stored phone (E.164, "+39..."): digits
// only, without "+" or "00"; "" when too short to be a number.
func waNumber(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	n := b.String()
	if !strings.HasPrefix(strings.TrimSpace(phone), "+") {
		n = strings.TrimPrefix(n, "00")
	}
	if len(n) < 8 {
		return ""
	}
	return n
}
