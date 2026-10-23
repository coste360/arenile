package notify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/store"
)

// Magic links of the customer manage area (spec 12.4): 256-bit random
// tokens (base64url, 43 characters), stored as SHA-256 hex; the raw token
// exists only in the URL returned here and in the email. Valid until the
// end of the booking (midnight after the last day, establishment time)
// plus MagicLinkGrace, unless revoked.

// LinkTokenBytes is the entropy of a magic link token.
const LinkTokenBytes = 32

// newLinkToken returns a random token and its hash.
func newLinkToken() (token, hash string, err error) {
	b := make([]byte, LinkTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("notify: token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashLinkToken(token), nil
}

// HashLinkToken is magic_link_tokens.token_hash for a raw token.
func HashLinkToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// validTokenShape rejects obviously malformed tokens before any query.
func validTokenShape(token string) bool {
	if len(token) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}

// ManageURL is the public URL of the manage area for a raw token.
func ManageURL(baseURL, slug, token string) string {
	return strings.TrimRight(baseURL, "/") + "/b/" + slug + "/m/" + token
}

// IssueMagicLink creates a magic link for a booking in tx (expiry = end of
// the booking + MagicLinkGrace). createdBy is the staff user, nil for the
// system. Implements checkout.Notifier.
func (s *Service) IssueMagicLink(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, createdBy *string) (*MagicLink, error) {
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("notify: issue link: %w", err)
	}
	b, err := store.GetBooking(ctx, tx, es, bookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("notify: issue link: %w", err)
	}
	token, hash, err := newLinkToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	row := &store.MagicLinkToken{
		ID: store.NewID(), BookingID: b.ID, TokenHash: hash,
		ExpiresAt: LinkExpiry(b, est, now), CreatedByUserID: createdBy, CreatedAt: now,
	}
	if err := store.InsertMagicLink(ctx, tx, es, row); err != nil {
		return nil, fmt.Errorf("notify: issue link: %w", err)
	}
	return &MagicLink{ID: row.ID, Token: token, URL: ManageURL(s.d.BaseURL, est.Slug, token), ExpiresAt: row.ExpiresAt}, nil
}

// LinkExpiry is the end of the last booked day in the establishment
// timezone plus MagicLinkGrace, and never earlier than now + grace (links
// sent after the stay, e.g. for a refund, stay usable).
func LinkExpiry(b *store.Booking, est *store.Establishment, now time.Time) time.Time {
	end := b.EndsOn.AddDays(1)
	endOfStay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, estLocation(est))
	exp := endOfStay.Add(MagicLinkGrace).UTC()
	if floor := now.Add(MagicLinkGrace).UTC(); exp.Before(floor) {
		return floor
	}
	return exp
}

// RevokeMagicLinks revokes every active link of a booking in tx.
func (s *Service) RevokeMagicLinks(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	if _, err := store.RevokeMagicLinks(ctx, tx, es, bookingID, s.now()); err != nil {
		return fmt.Errorf("notify: revoke links: %w", err)
	}
	return nil
}

// ResolveMagicLink returns the active link with the token hash in es and
// marks it used; ErrLinkNotFound otherwise (unknown, expired, revoked or of
// another establishment, without telling the cases apart). Implements
// checkout.Notifier.
func (s *Service) ResolveMagicLink(ctx context.Context, db bun.IDB, es store.EstablishmentScope, token string) (*store.MagicLinkToken, error) {
	if !validTokenShape(token) {
		return nil, ErrLinkNotFound
	}
	now := s.now()
	link, err := store.FindActiveMagicLink(ctx, db, es, HashLinkToken(token), now)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("notify: resolve link: %w", err)
	}
	if err := store.TouchMagicLink(ctx, db, es, link.ID, now); err != nil {
		return nil, fmt.Errorf("notify: resolve link: %w", err)
	}
	link.LastUsedAt = &now
	return link, nil
}

// RequestLink sends a new manage link when email and code match a booking
// of es; it returns nil also when they do not, and when the request is
// rate limited (no enumeration, spec 12.4 and 22). The link itself is
// issued by the delivery job, so it exists only in the email. Implements
// checkout.Notifier.
func (s *Service) RequestLink(ctx context.Context, es store.EstablishmentScope, email, code string) error {
	addr, err := normalizeRecipient(email)
	code = strings.ToUpper(strings.TrimSpace(code))
	if err != nil || code == "" || len(code) > 64 {
		return nil
	}
	if ok, _ := s.limiter.Allow(es.ID() + ":" + code); !ok {
		return nil
	}
	if ok, _ := s.limiter.Allow(es.ID() + ":" + RecipientHash(es.ID(), addr)); !ok {
		return nil
	}
	queued := false
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		id, err := store.FindBookingByCodeAndEmail(ctx, tx, es, code, addr)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("notify: request link: %w", err)
		}
		b, cu, err := s.bookingRecipient(ctx, tx, es, id)
		if err != nil || cu == nil {
			return err
		}
		switch b.Status {
		case statusDraft, statusHeld, statusPendingPayment, statusExpired:
			return nil // no manage area before the booking exists for the customer
		}
		// One email per booking per 10 minutes even under retries.
		bucket := s.now().Unix() / 600
		_, err = s.Enqueue(ctx, tx, es, Enqueue{
			Template: TemplateManageLink, BookingID: b.ID, CustomerID: cu.ID, Recipient: *cu.Email,
			Locale: bookingLocale(b, cu), DedupeKey: fmt.Sprintf("manage_link:%s:%d", b.ID, bucket),
		})
		queued = err == nil
		return err
	})
	if queued && err == nil {
		s.kick()
	}
	return err
}
