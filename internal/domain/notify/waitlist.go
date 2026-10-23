package notify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

// Waitlist emails (M5B, spec 10.5, 19): TemplateWaitlistJoined (the
// request was registered, personal link) and TemplateWaitlistOffer (a spot
// is reserved until a deadline, link to accept). They are not about a
// booking: the content comes from the payload (ids, dates, the offer
// expiry) and the customer. The personal link carries a raw token, so it is
// stored in the payload only encrypted with the key ring (like the
// recipient) and decrypted at compose time; message_log never holds it in
// clear and logs never see it.

// Payload keys of the waitlist messages.
const (
	waitlistLinkKey      = "link_enc"
	waitlistPackKey      = "pack_id"
	waitlistStartsKey    = "starts_on"
	waitlistEndsKey      = "ends_on"
	waitlistExpiresAtKey = "expires_at"
)

// waitlistTemplates are composed by waitlistContent.
var waitlistTemplates = map[string]bool{TemplateWaitlistJoined: true, TemplateWaitlistOffer: true}

// WaitlistVars are the template variables of the waitlist emails (besides
// establishment.name and customer.first_name).
var WaitlistVars = []string{"waitlist.period", "waitlist.pack", "waitlist_url", "offer.expires"}

// WaitlistMessage is a waitlist email to queue.
type WaitlistMessage struct {
	Template   string // TemplateWaitlistJoined | TemplateWaitlistOffer
	CustomerID string
	Recipient  string
	Locale     string
	DedupeKey  string
	PackID     string
	StartsOn   calendar.Date
	EndsOn     calendar.Date
	// ExpiresAt is the deadline of an offer (zero for the joined email).
	ExpiresAt time.Time
	// URL is the personal waitlist link (raw token inside): encrypted.
	URL string
}

func waitlistAAD(establishmentID, dedupeKey string) []byte {
	return []byte("waitlist_link:" + establishmentID + ":" + dedupeKey)
}

// EnqueueWaitlist queues a waitlist email in tx (the transaction of the
// entry or offer it announces). Implements waitlist.Notifier.
func (s *Service) EnqueueWaitlist(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m WaitlistMessage) (*store.Message, error) {
	if !waitlistTemplates[m.Template] {
		return nil, fmt.Errorf("notify: enqueue waitlist: unknown template %q", m.Template)
	}
	enc, err := s.d.Keys.Encrypt([]byte(m.URL), waitlistAAD(es.ID(), m.DedupeKey))
	if err != nil {
		return nil, fmt.Errorf("notify: encrypt waitlist link: %w", err)
	}
	payload := map[string]any{
		waitlistLinkKey: enc, waitlistPackKey: m.PackID,
		waitlistStartsKey: m.StartsOn.String(), waitlistEndsKey: m.EndsOn.String(),
	}
	if !m.ExpiresAt.IsZero() {
		payload[waitlistExpiresAtKey] = m.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return s.Enqueue(ctx, tx, es, Enqueue{
		Template: m.Template, CustomerID: m.CustomerID, Recipient: m.Recipient, Locale: m.Locale,
		DedupeKey: m.DedupeKey, Payload: payload,
	})
}

// waitlistContent builds the content of a waitlist email (compose).
func (s *Service) waitlistContent(ctx context.Context, es store.EstablishmentScope, m *store.Message, est *store.Establishment, cat catalog) (Content, error) {
	if m.CustomerID == nil {
		return Content{}, fmt.Errorf("%w: no customer", errSkip)
	}
	cu, err := store.GetCustomer(ctx, s.d.DB, es, *m.CustomerID)
	if errors.Is(err, store.ErrNotFound) {
		return Content{}, fmt.Errorf("%w: customer not found", errSkip)
	}
	if err != nil {
		return Content{}, err
	}
	if !canReceive(cu) {
		return Content{}, fmt.Errorf("%w: customer cannot receive messages", errSkip)
	}
	enc, _ := m.Payload[waitlistLinkKey].(string)
	if enc == "" {
		return Content{}, fmt.Errorf("%w: no waitlist link", ErrPermanent)
	}
	raw, err := s.d.Keys.Decrypt(enc, waitlistAAD(es.ID(), m.DedupeKey))
	if err != nil {
		return Content{}, fmt.Errorf("%w: decrypt waitlist link: %w", ErrPermanent, err)
	}
	vars := map[string]string{
		"establishment.name":  est.Name,
		"customer.first_name": firstNonEmpty(cu.FirstName, cu.LastName),
		"customer.last_name":  cu.LastName,
		"waitlist_url":        string(raw),
	}
	from, ferr := calendar.Parse(fmt.Sprint(m.Payload[waitlistStartsKey]))
	to, terr := calendar.Parse(fmt.Sprint(m.Payload[waitlistEndsKey]))
	if ferr == nil && terr == nil {
		vars["waitlist.period"] = formatPeriod(cat, from, to)
	}
	if id, _ := m.Payload[waitlistPackKey].(string); id != "" {
		if pk, err := store.GetPackForInventory(ctx, s.d.DB, es, id); err == nil {
			vars["waitlist.pack"] = i18nText(pk.Name, cat.locale, est.DefaultLocale)
		}
	}
	if v, _ := m.Payload[waitlistExpiresAtKey].(string); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			vars["offer.expires"] = formatDateTime(cat, t.In(estLocation(est)))
		}
	}
	return Content{Locale: cat.locale, Vars: vars, Contacts: establishmentContacts(est)}, nil
}

// formatDateTime is formatDate plus the time of day (24 h).
func formatDateTime(cat catalog, t time.Time) string {
	return formatDate(cat, calendar.In(t, t.Location())) + ", " + t.Format("15:04")
}

// The waitlist templates exist in every locale (templates/<lang>/
// waitlist_*.md): registered here, next to their compose step.
func init() {
	Templates = append(Templates, TemplateWaitlistJoined, TemplateWaitlistOffer)
}
