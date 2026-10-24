package notify

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// compose turns a claimed message into an Email: it reads the current
// booking data, issues the magic link the email carries (confirmation,
// modification, new link), renders the template and builds attachments.
// onFailure (may be nil) revokes that link when the transport fails, so a
// link exists only if its email left.
func (s *Service) compose(ctx context.Context, es store.EstablishmentScope, m *store.Message, to string) (Email, func(context.Context), error) {
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return Email{}, nil, fmt.Errorf("notify: compose: %w", err)
	}
	cfg, err := s.emailConfig(ctx, s.d.DB, es)
	if err != nil {
		return Email{}, nil, err
	}
	cat, err := catalogFor(m.Locale)
	if err != nil {
		return Email{}, nil, err
	}
	br := BrandingFor(est)
	now := s.now()
	// ARENILE_MAIL_FROM may carry a display name: the sender name of the
	// establishment wins, the address stays the server one (SPF/DKIM).
	fromAddr := s.d.MailFrom
	if a, err := mail.ParseAddress(fromAddr); err == nil {
		fromAddr = a.Address
	}
	email := Email{
		From: (&mail.Address{Name: firstNonEmpty(cfg.SenderName, est.Name), Address: fromAddr}).String(),
		To:   to, DedupeID: m.ID, MessageID: m.ID + "@" + icsHost(s.d.BaseURL), Date: now,
		Headers: map[string]string{headerTemplate: m.Template},
	}
	if rt := firstNonEmpty(cfg.ReplyTo, est.ContactEmail); rt != "" {
		if _, err := mail.ParseAddress(rt); err == nil {
			email.ReplyTo = rt
		}
	}

	var (
		content   Content
		onFailure func(context.Context)
		// test: booking created in test mode (spec 15.4), subject prefixed.
		test bool
	)
	switch {
	case m.Template == TemplateTest:
		content = Content{
			Locale: cat.locale, Manager: true, Contacts: establishmentContacts(est),
			Vars: map[string]string{"establishment.name": est.Name},
		}
	case panelTemplates[m.Template]:
		// Owner emails without a booking (panel.go).
		content = Content{Locale: cat.locale, Manager: true, Contacts: establishmentContacts(est), Vars: panelVars(m, est)}
	case waitlistTemplates[m.Template]:
		// M5B waitlist emails: no booking (waitlist.go).
		if content, err = s.waitlistContent(ctx, es, m, est, cat); err != nil {
			return Email{}, nil, err
		}
	default:
		if m.BookingID == nil {
			return Email{}, nil, fmt.Errorf("%w: no booking", errSkip)
		}
		d, err := loadBooking(ctx, s.d.DB, es, *m.BookingID)
		if errors.Is(err, store.ErrNotFound) {
			return Email{}, nil, fmt.Errorf("%w: booking not found", errSkip)
		}
		if err != nil {
			return Email{}, nil, err
		}
		test = d.Booking.Test
		manager := managerTemplates[m.Template]
		if !manager && !canReceive(d.Customer) {
			return Email{}, nil, fmt.Errorf("%w: customer cannot receive messages", errSkip)
		}
		opts := contentOptions{Manager: manager, Vars: payloadVars(m, est, cat)}
		switch m.Template {
		case TemplateBookingConfirmed, TemplateBookingModified:
			if d.Booking.Status != statusConfirmed && d.Booking.Status != statusCheckedIn && d.Booking.Status != statusCompleted {
				return Email{}, nil, fmt.Errorf("%w: booking is %s", errSkip, d.Booking.Status)
			}
			link, err := s.issueForEmail(ctx, es, d.Booking.ID, nil)
			if err != nil {
				return Email{}, nil, err
			}
			onFailure = s.revokeOnFailure(es, link.ID)
			opts.QR, opts.ICS, opts.Policy, opts.ManageURL, opts.LinkExpires = true, true, m.Template == TemplateBookingConfirmed, link.URL, link.ExpiresAt
			png, _, err := RenderQR(QRPayload(d.Booking.Code), QRPNG)
			if err != nil {
				return Email{}, nil, err
			}
			ics := bookingICS(cat, d, s.d.BaseURL, link.URL, now).Bytes()
			email.Attach = append(email.Attach,
				Attachment{Filename: "qr-" + d.Booking.Code + ".png", ContentType: contentTypePNG, ContentID: QRContentID, Inline: true, Data: png},
				Attachment{Filename: cat.t("ics_filename", map[string]string{"code": d.Booking.Code}), ContentType: ICSContentType + "; method=PUBLISH", Data: ics},
			)
		case TemplateRenewalProposal, TemplateRenewalReminder:
			// M5B renewal emails: the manage link of the proposal booking (renewal.go).
			link, err := s.renewalLink(ctx, es, d)
			if err != nil {
				return Email{}, nil, err
			}
			onFailure = s.revokeOnFailure(es, link.ID)
			opts.ManageURL, opts.LinkExpires, opts.Policy = link.URL, link.ExpiresAt, m.Template == TemplateRenewalProposal
			for k, v := range renewalVars(m, est, cat) {
				opts.Vars[k] = v
			}
		case TemplateManageLink:
			keep, _ := m.Payload["keep_link_id"].(string)
			link, err := s.issueForEmail(ctx, es, d.Booking.ID, []string{keep})
			if err != nil {
				return Email{}, nil, err
			}
			onFailure = s.revokeOnFailure(es, link.ID)
			opts.ManageURL, opts.LinkExpires = link.URL, link.ExpiresAt
		}
		content = bookingContent(cat, d, opts)
	}

	src, err := s.templateSource(ctx, es, m.Template, cat.locale)
	if err != nil {
		return Email{}, nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	r, err := Render(src, cat, br, content)
	if err != nil {
		return Email{}, nil, fmt.Errorf("%w: render: %w", ErrPermanent, err)
	}
	email.Subject, email.Text, email.HTML = r.Subject, r.Text, r.HTML
	if test {
		email.Subject = TestSubjectPrefix + email.Subject
	}
	email.Attach = append(email.Attach, BrandMarkAttachment())
	return email, onFailure, nil
}

// TestSubjectPrefix marks the emails of test-mode bookings (spec 15.4).
const TestSubjectPrefix = "[PROVA] "

// issueForEmail issues a link in its own transaction. For the manage_link
// and renewal templates (revokePrevious != nil) the previous links are
// revoked, except the new one and the ids in revokePrevious (the link just
// shown at the desk by RegenerateLink).
func (s *Service) issueForEmail(ctx context.Context, es store.EstablishmentScope, bookingID string, revokePrevious []string) (*MagicLink, error) {
	var link *MagicLink
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		if link, err = s.IssueMagicLink(ctx, tx, es, bookingID, nil); err != nil {
			return err
		}
		if revokePrevious != nil {
			keep := append([]string{link.ID}, revokePrevious...)
			_, err = store.RevokeMagicLinks(ctx, tx, es, bookingID, s.now(), keep...)
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("notify: issue link: %w", err)
	}
	return link, nil
}

func (s *Service) revokeOnFailure(es store.EstablishmentScope, linkID string) func(context.Context) {
	return func(ctx context.Context) {
		_ = store.RevokeMagicLink(context.WithoutCancel(ctx), s.d.DB, es, linkID, s.now())
	}
}

// templateSource returns the establishment override of a template in
// locale, else the built-in one.
func (s *Service) templateSource(ctx context.Context, es store.EstablishmentScope, code, locale string) (templateSource, error) {
	t, err := store.GetNotificationTemplate(ctx, s.d.DB, es, code, locale)
	switch {
	case err == nil:
		return templateSource{Subject: t.Subject, Body: t.BodyMarkdown}, nil
	case !errors.Is(err, store.ErrNotFound):
		return templateSource{}, err
	}
	return builtinTemplate(code, locale)
}

// payloadVars formats the non-personal payload of a message as variables.
func payloadVars(m *store.Message, est *store.Establishment, cat catalog) map[string]string {
	out := map[string]string{}
	if v, ok := payloadInt(m.Payload["amount"]); ok {
		amount := formatMoney(money.Cents(v), est.Currency, cat.locale)
		out["refund.amount"], out["payment.amount"] = amount, amount
	}
	if v, ok := m.Payload["failed_template"].(string); ok {
		out["message.template"] = v
	}
	if v, ok := payloadInt(m.Payload["attempts"]); ok {
		out["message.attempts"] = strconv.FormatInt(v, 10)
	}
	return out
}

// payloadInt reads an integer from a JSON-decoded payload value.
func payloadInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i, err == nil
	}
	return 0, false
}

// ICS renders the .ics file of a booking of es; manageURL is included when
// not empty. Implements checkout.Notifier.
func (s *Service) ICS(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID, manageURL string) ([]byte, error) {
	d, err := loadBooking(ctx, db, es, bookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	cat, err := catalogFor(d.messageLocale())
	if err != nil {
		return nil, err
	}
	stamp := d.Booking.UpdatedAt
	if stamp.IsZero() {
		stamp = s.now()
	}
	return bookingICS(cat, d, s.d.BaseURL, manageURL, stamp.Truncate(time.Second)).Bytes(), nil
}
