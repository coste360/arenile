package notify

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Admin use cases of the editable customer email texts (ADR 0022): list,
// save, restore default, live preview with sample data and "send test to
// my address". Reading needs bookings.read; saving, restoring and sending
// a test need establishment.configure. Saves and restores write the audit
// log in the same transaction.

// TemplateTemplateTest is the message_log template of a test of edited
// texts (payload: kind and the texts; sample booking data, no customer).
const TemplateTemplateTest = "template_test"

// Errors of the template editor.
var (
	// ErrTemplateNotFound: kind without editable texts or unknown locale.
	ErrTemplateNotFound = fmt.Errorf("%w: email template", configerr.ErrNotFound)
	// ErrNoUserEmail: the current principal has no email address.
	ErrNoUserEmail = fmt.Errorf("%w: il tuo utente non ha un indirizzo email", configerr.ErrConflict)
)

// TemplateView is one editable template of an establishment.
type TemplateView struct {
	Kind        string
	Locale      string
	Texts       Texts
	Defaults    Texts
	Customized  bool
	FixedBlocks []string
	// Version is the ETag: 0 while the built-in texts apply.
	Version   int64
	UpdatedAt *time.Time
}

// Preview is a rendered email of the editor preview.
type Preview struct {
	Subject string
	HTML    string
	Text    string
}

func checkKindLocale(kind, locale string) error {
	if !IsEditableKind(kind) || !slices.Contains(Locales, locale) {
		return ErrTemplateNotFound
	}
	return nil
}

func templateView(kind, locale string, o *store.EmailTemplateOverride) (TemplateView, error) {
	def, err := defaultTexts(kind, locale)
	if err != nil {
		return TemplateView{}, err
	}
	v := TemplateView{Kind: kind, Locale: locale, Texts: def, Defaults: def, FixedBlocks: FixedBlocks(kind, locale)}
	if o != nil {
		// A restored row keeps its version (never reused as an ETag).
		v.Version = o.Version
		at := o.UpdatedAt
		v.UpdatedAt = &at
		if o.Active {
			v.Texts = Texts{Subject: o.Subject, Opening: o.Opening, Closing: o.Closing, Signature: o.Signature}
			v.Customized = true
		}
	}
	return v, nil
}

// ListEmailTemplates returns every editable kind in every locale
// (auth.PermBookingsRead).
func (s *Service) ListEmailTemplates(ctx context.Context, p *auth.Principal, establishmentID string) ([]TemplateView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListEmailTemplateOverrides(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("notify: list templates: %w", err)
	}
	byKey := make(map[string]*store.EmailTemplateOverride, len(rows))
	for i := range rows {
		byKey[rows[i].Kind+"/"+rows[i].Locale] = &rows[i]
	}
	out := make([]TemplateView, 0, len(EditableKinds)*len(Locales))
	for _, k := range EditableKinds {
		for _, l := range Locales {
			v, err := templateView(k, l, byKey[k+"/"+l])
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// GetEmailTemplate returns one editable template (auth.PermBookingsRead).
func (s *Service) GetEmailTemplate(ctx context.Context, p *auth.Principal, establishmentID, kind, locale string) (*TemplateView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if err := checkKindLocale(kind, locale); err != nil {
		return nil, err
	}
	return s.templateViewOf(ctx, s.d.DB, es, kind, locale)
}

func (s *Service) templateViewOf(ctx context.Context, db bun.IDB, es store.EstablishmentScope, kind, locale string) (*TemplateView, error) {
	o, err := store.GetEmailTemplateOverride(ctx, db, es, kind, locale)
	switch {
	case errors.Is(err, store.ErrNotFound):
		o = nil
	case err != nil:
		return nil, fmt.Errorf("notify: template: %w", err)
	}
	v, err := templateView(kind, locale, o)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateEmailTemplate saves the texts of kind in locale
// (auth.PermEstablishmentConfigure). expected is the If-Match version (0 =
// not customized yet, nil = no check). Invalid texts are a *TemplateError.
func (s *Service) UpdateEmailTemplate(ctx context.Context, p *auth.Principal, establishmentID, kind, locale string, expected *int64, in Texts) (*TemplateView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if err := checkKindLocale(kind, locale); err != nil {
		return nil, err
	}
	in = NormalizeTexts(in)
	if err := ValidateTexts(in); err != nil {
		return nil, err
	}
	var userID *string
	if p.Kind == auth.PrincipalUser && p.UserID != "" {
		uid := p.UserID
		userID = &uid
	}
	var out *TemplateView
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		cur, err := store.GetEmailTemplateOverride(ctx, tx, es, kind, locale)
		var changed []string
		switch {
		case errors.Is(err, store.ErrNotFound):
			if expected != nil && *expected != 0 {
				return &configerr.VersionConflictError{Current: 0}
			}
			def, err := defaultTexts(kind, locale)
			if err != nil {
				return err
			}
			changed = changedTexts(def, in)
			cur = &store.EmailTemplateOverride{
				ID: store.NewID(), Kind: kind, Locale: locale, Subject: in.Subject, Opening: in.Opening,
				Closing: in.Closing, Signature: in.Signature, Active: true, Version: 1, UpdatedByUserID: userID,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := store.InsertEmailTemplateOverride(ctx, tx, es, cur); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return &configerr.VersionConflictError{Current: 1}
				}
				return fmt.Errorf("notify: save template: %w", err)
			}
		case err != nil:
			return fmt.Errorf("notify: save template: %w", err)
		default:
			if err := configerr.CheckVersion(expected, cur.Version); err != nil {
				return err
			}
			was, err := templateView(kind, locale, cur)
			if err != nil {
				return err
			}
			changed = changedTexts(was.Texts, in)
			prev := cur.Version
			cur.Subject, cur.Opening, cur.Closing, cur.Signature, cur.Active = in.Subject, in.Opening, in.Closing, in.Signature, true
			cur.Version, cur.UpdatedByUserID, cur.UpdatedAt = prev+1, userID, now
			ok, err := store.UpdateEmailTemplateOverride(ctx, tx, es, cur, prev)
			if err != nil {
				return fmt.Errorf("notify: save template: %w", err)
			}
			if !ok {
				return &configerr.VersionConflictError{Current: prev}
			}
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "notify.email_template_updated",
			EntityType: "email_template", EntityID: cur.ID,
			Diff: map[string]any{"kind": kind, "locale": locale, "fields": changed, "version": cur.Version},
		}); err != nil {
			return err
		}
		v, err := templateView(kind, locale, cur)
		if err != nil {
			return err
		}
		out = &v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ResetEmailTemplate restores the built-in texts of kind in locale
// (auth.PermEstablishmentConfigure): the row is deactivated and its version
// bumped, so an ETag read before the restore no longer matches. Restoring a
// template that is not customized is a no-op.
func (s *Service) ResetEmailTemplate(ctx context.Context, p *auth.Principal, establishmentID, kind, locale string, expected *int64) (*TemplateView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if err := checkKindLocale(kind, locale); err != nil {
		return nil, err
	}
	var row *store.EmailTemplateOverride
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetEmailTemplateOverride(ctx, tx, es, kind, locale)
		if errors.Is(err, store.ErrNotFound) {
			if expected != nil && *expected != 0 {
				return &configerr.VersionConflictError{Current: 0}
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("notify: reset template: %w", err)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		row = cur
		if !cur.Active {
			return nil
		}
		var userID *string
		if p.Kind == auth.PrincipalUser && p.UserID != "" {
			uid := p.UserID
			userID = &uid
		}
		prev := cur.Version
		cur.Active, cur.Version, cur.UpdatedByUserID, cur.UpdatedAt = false, prev+1, userID, s.now()
		ok, err := store.UpdateEmailTemplateOverride(ctx, tx, es, cur, prev)
		if err != nil {
			return fmt.Errorf("notify: reset template: %w", err)
		}
		if !ok {
			return &configerr.VersionConflictError{Current: prev}
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "notify.email_template_reset",
			EntityType: "email_template", EntityID: cur.ID,
			Diff: map[string]any{"kind": kind, "locale": locale, "version": cur.Version},
		})
	})
	if err != nil {
		return nil, err
	}
	v, err := templateView(kind, locale, row)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// PreviewEmailTemplate renders unsaved texts with sample booking data and
// the establishment theme (auth.PermBookingsRead). Inline images become
// data: URIs so the page can show the HTML in a sandboxed iframe.
func (s *Service) PreviewEmailTemplate(ctx context.Context, p *auth.Principal, establishmentID, kind, locale string, in Texts) (*Preview, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if err := checkKindLocale(kind, locale); err != nil {
		return nil, err
	}
	in = NormalizeTexts(in)
	if err := ValidateTexts(in); err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("notify: preview: %w", err)
	}
	r, code, err := s.renderSample(ctx, es, est, kind, locale, in)
	if err != nil {
		return nil, err
	}
	h := r.HTML
	if png, _, err := RenderQR(QRPayload(code), QRPNG); err == nil {
		h = strings.ReplaceAll(h, "cid:"+QRContentID, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png))
	}
	h = strings.ReplaceAll(h, "cid:"+BrandMarkContentID, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(brandMarkPNG))
	return &Preview{Subject: r.Subject, HTML: h, Text: r.Text}, nil
}

// SendEmailTemplateTest queues unsaved texts with sample data to the
// address of the current user (auth.PermEstablishmentConfigure).
func (s *Service) SendEmailTemplateTest(ctx context.Context, p *auth.Principal, establishmentID, kind, locale string, in Texts) (*store.Message, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if err := checkKindLocale(kind, locale); err != nil {
		return nil, err
	}
	in = NormalizeTexts(in)
	if err := ValidateTexts(in); err != nil {
		return nil, err
	}
	if p.Kind != auth.PrincipalUser || strings.TrimSpace(p.Email) == "" {
		return nil, ErrNoUserEmail
	}
	var msg *store.Message
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		msg, err = s.Enqueue(ctx, tx, es, Enqueue{
			Template: TemplateTemplateTest, Recipient: p.Email, Locale: locale, DedupeKey: "template-test:" + store.NewID(),
			Payload: map[string]any{
				"kind": kind, "subject": in.Subject, "opening": in.Opening, "closing": in.Closing, "signature": in.Signature,
			},
		})
		if err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "notify.email_template_test_queued",
			EntityType: "message", EntityID: msg.ID, Diff: map[string]any{"kind": kind, "locale": locale},
		})
	})
	if err != nil {
		return nil, fmt.Errorf("notify: template test: %w", err)
	}
	s.kick()
	return msg, nil
}

// renderSample renders kind with texts and the sample booking; it returns
// the booking code of the sample (QR).
func (s *Service) renderSample(ctx context.Context, es store.EstablishmentScope, est *store.Establishment, kind, locale string, texts Texts) (Rendered, string, error) {
	cat, err := catalogFor(locale)
	if err != nil {
		return Rendered{}, "", err
	}
	d := sampleBooking(est, cat, s.now())
	c := sampleContent(kind, cat, d, s.d.BaseURL)
	src, builtin, err := s.templateSource(ctx, es, kind, cat.locale)
	if err != nil {
		return Rendered{}, "", err
	}
	if builtin {
		resolved := ResolveTexts(texts, placeholderValues(kind, c.Vars))
		c.Texts = &resolved
	}
	r, err := Render(src, cat, BrandingFor(est), c)
	if err != nil {
		return Rendered{}, "", fmt.Errorf("notify: render sample: %w", err)
	}
	return r, d.Booking.Code, nil
}

// sampleBooking is the fictitious booking of previews and tests: two weeks
// from today in the establishment timezone, three days, one spot, a
// deposit paid and the balance due three days before the start.
func sampleBooking(est *store.Establishment, cat catalog, now time.Time) *bookingData {
	start := calendar.In(now, estLocation(est)).AddDays(14)
	loc := cat.locale
	zoneID, packID := "sample-zone", "sample-pack"
	return &bookingData{
		Est: est,
		Booking: &store.Booking{
			ID: "00000000-0000-7000-8000-000000000000", Code: "PROVA-" + start.String()[:4] + "-000123", Status: statusConfirmed,
			StartsOn: start, EndsOn: start.AddDays(2), Slots: calendar.SlotsFull,
			TotalAmount: money.Cents(18000), PaidAmount: money.Cents(6000), DepositDue: money.Cents(6000), Locale: &loc, Version: 1,
			PolicySnapshot: store.JSON(`{"version":1,"rules":{"payment":{"balance_due":"days_before:3"},"cancellation":[{"until_days_before":7,"refund_percent":100}],"modification":{"allowed_until_days_before":3}},"resolved_at":"2026-01-01T00:00:00Z"}`),
		},
		Customer: &store.Customer{ID: "sample-customer", FirstName: cat.t("preview_first_name", nil), LastName: cat.t("preview_last_name", nil), Locale: loc},
		Lines:    []store.BookingLine{{PackID: packID, ZoneID: zoneID, Quantity: 1, AssignmentStatus: store.AssignmentAssigned}},
		Zones:    map[string]store.Zone{zoneID: {ID: zoneID, Name: store.I18nText{loc: cat.t("preview_zone", nil)}}},
		Packs:    map[string]store.Pack{packID: {ID: packID, Name: store.I18nText{loc: cat.t("preview_pack", nil)}}},
		ExtraDef: map[string]store.Extra{},
		Spots:    []string{"A12"},
	}
}

// sampleContent mirrors the options compose uses for kind.
func sampleContent(kind string, cat catalog, d *bookingData, baseURL string) Content {
	o := contentOptions{Vars: map[string]string{"refund.amount": formatMoney(6000, d.Est.Currency, cat.locale)}}
	manage := strings.TrimRight(baseURL, "/") + "/b/" + d.Est.Slug + "/m/" + strings.Repeat("x", 43)
	expires := d.Booking.EndsOn.AddDays(30).Time()
	switch kind {
	case TemplateBookingConfirmed, TemplateBookingModified:
		o.QR, o.ICS, o.Policy, o.ManageURL, o.LinkExpires = true, true, kind == TemplateBookingConfirmed, manage, expires
	case TemplateManageLink:
		o.ManageURL, o.LinkExpires = manage, expires
	case TemplateDayBeforeReminder:
		o.QR, o.ManageURL, o.LinkExpires = true, manage, expires
	case TemplateBalanceReminder:
		o.BalanceNotice, o.ManageURL, o.LinkExpires = true, manage, expires
	}
	return bookingContent(cat, d, o)
}

// textsFor returns the texts of an editable kind for es: the override, or
// the built-in defaults.
func (s *Service) textsFor(ctx context.Context, db bun.IDB, es store.EstablishmentScope, kind, locale string) (Texts, error) {
	o, err := store.GetEmailTemplateOverride(ctx, db, es, kind, locale)
	switch {
	case err == nil && o.Active:
		return Texts{Subject: o.Subject, Opening: o.Opening, Closing: o.Closing, Signature: o.Signature}, nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return Texts{}, fmt.Errorf("notify: texts: %w", err)
	}
	return defaultTexts(kind, locale)
}

// changedTexts names the texts that differ between a and b (audit diff:
// field names only, never the texts).
func changedTexts(a, b Texts) []string {
	out := []string{}
	if a.Subject != b.Subject {
		out = append(out, "subject")
	}
	if a.Opening != b.Opening {
		out = append(out, "opening")
	}
	if a.Closing != b.Closing {
		out = append(out, "closing")
	}
	if a.Signature != b.Signature {
		out = append(out, "signature")
	}
	return out
}
