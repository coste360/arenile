package checkout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// MaxPaymentHold bounds the hold extension while a payment is in progress
// (spec 10.2: pending_payment, max 30 minutes).
const MaxPaymentHold = 30 * time.Minute

// Default rate limits of the public flow (spec 17.2, 22). The per-IP
// limits of every public call live in the HTTP middleware; these protect
// one establishment from a distributed burst and the "send me a new link"
// form from enumeration and mail bombing.
const (
	DefaultCreatesPerEstablishment = 300 // per minute
	DefaultLinkRequestsPerIP       = 5   // per 15 minutes
	DefaultLinkRequestsPerBooking  = 3   // per hour (email + code)
	linkRequestWorkers             = 4
	linkRequestTimeout             = 30 * time.Second
)

// Errors of the checkout package (mapped by internal/http/problem).
var (
	// ErrSessionNotFound: missing or wrong X-Arenile-Checkout token, or a
	// token of another booking or establishment (404 checkout_not_found).
	ErrSessionNotFound = errors.New("checkout: session not found")
	// ErrDetailsMissing: pay called before customer details and terms (422).
	ErrDetailsMissing = fmt.Errorf("%w: inserisci i tuoi dati e accetta le condizioni", configerr.ErrValidation)
	// ErrTermsOutdated: the accepted terms version is not the current one (409).
	ErrTermsOutdated = fmt.Errorf("%w: le condizioni sono state aggiornate, rileggile e accettale di nuovo", configerr.ErrConflict)
	// ErrNothingToPay: no balance due (409).
	ErrNothingToPay = fmt.Errorf("%w: nessun importo da pagare", configerr.ErrConflict)
	// ErrOnlinePaymentUnavailable: the establishment takes no online
	// payments (409); the page offers payment at the desk.
	ErrOnlinePaymentUnavailable = fmt.Errorf("%w: il pagamento online non è disponibile", configerr.ErrConflict)
	// ErrSettlementNotAllowed: at_desk asked while the policy wants an
	// online payment (422).
	ErrSettlementNotAllowed = fmt.Errorf("%w: il pagamento in cassa non è previsto per questa prenotazione", configerr.ErrValidation)
	// ErrBookingChanged: the booking changed between two steps of the page
	// (409); the page reloads its state and continues from there.
	ErrBookingChanged = fmt.Errorf("%w: la prenotazione è cambiata, ricarica la pagina", configerr.ErrConflict)
	// ErrDocumentNotReady: QR and .ics exist only for confirmed bookings (404).
	ErrDocumentNotReady = fmt.Errorf("%w: document", configerr.ErrNotFound)
)

// Deps are the dependencies of the checkout service. Every field is
// required except Log, HoldTTL, Passes and the limits.
type Deps struct {
	DB       bun.IDB
	Clock    clock.Clock
	Log      *slog.Logger
	Settings Settings
	// Inventory, Pricer: reads of the online channel.
	Inventory Inventory
	Pricer    Pricer
	Bookings  Bookings
	Payments  Payments
	Notifier  Notifier
	Passes    SeasonPasses
	// HoldTTL is ARENILE_HOLD_TTL (countdown of the page).
	HoldTTL time.Duration
	// BaseURL is config.EffectiveBaseURL() (return URLs, manage links).
	BaseURL string
	// CreatesPerEstablishment limits checkout creations per establishment
	// per minute (0 = DefaultCreatesPerEstablishment, negative = off).
	CreatesPerEstablishment int
	// LinkRequestsPerIP / LinkRequestsPerBooking limit "send me a new link"
	// (0 = defaults, negative = off).
	LinkRequestsPerIP      int
	LinkRequestsPerBooking int
	// PriceCacheTTL caches the "from" prices of availability and catalog
	// (0 = one minute, negative = off).
	PriceCacheTTL time.Duration
}

// Service implements the public flow and the manage area.
type Service struct {
	d Deps

	createLimit      *auth.RateLimiter
	linkLimitIP      *auth.RateLimiter
	linkLimitBooking *auth.RateLimiter

	prices *priceCache

	// link requests run in the background (constant response time).
	linkSem chan struct{}
	linkWG  sync.WaitGroup
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.HoldTTL <= 0 {
		d.HoldTTL = 10 * time.Minute
	}
	d.BaseURL = strings.TrimRight(d.BaseURL, "/")
	s := &Service{d: d, linkSem: make(chan struct{}, linkRequestWorkers)}
	if d.Clock != nil {
		s.createLimit = limiter(d.Clock, d.CreatesPerEstablishment, DefaultCreatesPerEstablishment, time.Minute)
		s.linkLimitIP = limiter(d.Clock, d.LinkRequestsPerIP, DefaultLinkRequestsPerIP, 15*time.Minute)
		s.linkLimitBooking = limiter(d.Clock, d.LinkRequestsPerBooking, DefaultLinkRequestsPerBooking, time.Hour)
		ttl := d.PriceCacheTTL
		if ttl == 0 {
			ttl = time.Minute
		}
		if ttl > 0 {
			s.prices = newPriceCache(d.Clock, ttl)
		}
	}
	return s
}

func limiter(c clock.Clock, n, def int, window time.Duration) *auth.RateLimiter {
	switch {
	case n < 0:
		return nil
	case n == 0:
		n = def
	}
	return auth.NewRateLimiter(c, n, window)
}

// allow consumes one unit of l for key (nil limiter = unlimited).
func allow(l *auth.RateLimiter, key string) error {
	if l == nil {
		return nil
	}
	if ok, retry := l.Allow(key); !ok {
		return &auth.RateLimitError{RetryAfter: retry}
	}
	return nil
}

// Wait blocks until the background link requests finished (shutdown, tests).
func (s *Service) Wait() { s.linkWG.Wait() }

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// tokenRe is the shape of session and magic link tokens (43 base64url
// characters): anything else is rejected without a database lookup.
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// ValidToken reports whether t has the shape of a 256-bit token.
func ValidToken(t string) bool { return tokenRe.MatchString(t) }

// establishment resolves a slug. requireBooking = true for the selling
// flow (online booking switched off -> 404); false for the session reads
// and the manage area, which must keep working for bookings already made.
func (s *Service) establishment(ctx context.Context, slug string, requireBooking bool) (*online.PublicEstablishment, error) {
	pe, err := s.d.Settings.ResolvePublic(ctx, slug)
	if err == nil || requireBooking || !errors.Is(err, online.ErrBookingDisabled) {
		return pe, err
	}
	est, err := store.GetEstablishmentBySlug(ctx, s.d.DB, strings.ToLower(slug))
	if err != nil {
		return nil, online.ErrEstablishmentNotFound
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, est.ID)
	if err != nil {
		return nil, fmt.Errorf("checkout: %w", err)
	}
	terms, err := s.d.Settings.CurrentTerms(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	return &online.PublicEstablishment{Scope: es, Establishment: est, Flow: online.DefaultConfig(), Terms: terms, Provider: online.ProviderManual}, nil
}

// session authenticates a checkout token for bookingID in the
// establishment of slug.
func (s *Service) session(ctx context.Context, slug, bookingID, token string) (*online.PublicEstablishment, *store.CheckoutSession, error) {
	if !ValidToken(token) || bookingID == "" {
		return nil, nil, ErrSessionNotFound
	}
	pe, err := s.establishment(ctx, slug, false)
	if err != nil {
		if errors.Is(err, configerr.ErrNotFound) {
			return nil, nil, ErrSessionNotFound
		}
		return nil, nil, err
	}
	cs, err := store.GetCheckoutSessionByToken(ctx, s.d.DB, pe.Scope, bookingID, HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("checkout: %w", err)
	}
	return pe, cs, nil
}

// actor is the audit author of a public request: the customer, identified
// by the checkout session or magic link id (never the token), with the IP.
func actor(ctx context.Context, id string) audit.Actor {
	return audit.Actor{Type: audit.ActorCustomer, ID: id, IP: reqctx.From(ctx).ClientIP}
}

// location is the timezone of an establishment (UTC when unknown).
func location(est *store.Establishment) *time.Location {
	if loc, err := time.LoadLocation(est.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// today is the current date of the establishment.
func (s *Service) today(est *store.Establishment) calendar.Date {
	return calendar.In(s.now(), location(est))
}

// pageURL is the public page of slug.
func (s *Service) pageURL(slug string) string { return s.d.BaseURL + "/b/" + slug }
