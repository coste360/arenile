package waitlist

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// JobExpire is the name of the offer/entry expiry job.
const JobExpire = "waitlist.expire"

// SinkName is the name of the event sink.
const SinkName = "waitlist"

// Default settings (migration 00008 defaults) when no row exists.
const (
	DefaultOfferHoldMinutes = 120
)

// Deps are the dependencies of the service.
type Deps struct {
	DB       bun.IDB
	Clock    clock.Clock
	Audit    *audit.Writer
	Outbox   *event.Outbox
	Log      *slog.Logger
	Holds    Holds
	Checkout Checkout
	Notify   Notifier
	// BaseURL builds the waitlist links (<base>/b/<slug>/lista-attesa/<token>).
	BaseURL string
}

// Service implements the waitlist use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil && d.Clock != nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Outbox == nil && d.Clock != nil {
		d.Outbox = event.NewOutbox(d.Clock)
	}
	return &Service{d: d}
}

var _ ext.EventSink = (*Service)(nil)

// Entry is api.WaitlistEntry (row + display fields).
type Entry struct {
	store.WaitlistEntry
	CustomerDisplayName string
	Position            int
	OfferCount          int
	ActiveOffer         *Offer
}

// Offer is api.WaitlistOffer.
type Offer struct {
	store.WaitlistOffer
	SpotLabel string
}

// Filter filters the waitlist.
type Filter struct {
	Status string
	ZoneID string
	PackID string
	From   *calendar.Date
	To     *calendar.Date
	Query  string
}

// Input is api.WaitlistEntryCreate / the public join request.
type Input struct {
	CustomerID string
	StartsOn   calendar.Date
	EndsOn     calendar.Date
	Slots      calendar.Slots
	PackID     string
	ZoneID     string
	SpotID     string
	Quantity   int
	Channel    string
	Note       string
	IP         string
}

// Patch is api.WaitlistEntryUpdate (nil = unchanged).
type Patch struct {
	StartsOn *calendar.Date
	EndsOn   *calendar.Date
	Slots    *calendar.Slots
	PackID   *string
	ZoneID   *string
	SpotID   *string
	Quantity *int
	Priority *int
	Note     *string
	IP       string
}

// Settings is api.WaitlistSettings.
type Settings struct {
	Enabled          bool
	PublicJoin       bool
	AutoOffer        bool
	OfferHoldMinutes int
	Version          int64
}

// PublicJoinInput is api.PublicWaitlistJoin (customer found or created by
// email through the customer service).
type PublicJoinInput struct {
	StartsOn           calendar.Date
	EndsOn             calendar.Date
	Slots              calendar.Slots
	PackID             string
	ZoneID             string
	Quantity           int
	FirstName          string
	LastName           string
	Email              string
	Phone              string
	Locale             string
	AcceptTermsVersion string
	IP                 string
}

// PublicEntry is api.PublicWaitlistEntry.
type PublicEntry struct {
	Entry    store.WaitlistEntry
	PackName string
	ZoneName string
	Offer    *Offer
}

// ---- panel ------------------------------------------------------------------

// List returns the entries in priority order (bookings.read).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f Filter, cursor string, limit int) (store.Page[Entry], error) {
	return store.Page[Entry]{}, configerr.ErrNotImplemented
}

// Create adds an entry from the panel (bookings.write).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in Input) (*Entry, error) {
	return nil, configerr.ErrNotImplemented
}

// Get returns an entry with its offers (bookings.read).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Entry, []Offer, error) {
	return nil, nil, configerr.ErrNotImplemented
}

// Update changes an entry (bookings.write, optimistic concurrency).
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in Patch) (*Entry, error) {
	return nil, configerr.ErrNotImplemented
}

// Remove marks an entry removed and revokes its pending offer.
func (s *Service) Remove(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, ip string) error {
	return configerr.ErrNotImplemented
}

// Reorder assigns priority 10, 20, ... to ids in order.
func (s *Service) Reorder(ctx context.Context, p *auth.Principal, establishmentID string, ids []string, ip string) error {
	return configerr.ErrNotImplemented
}

// Offer creates an offer now (manual cause), optionally on spotID.
func (s *Service) Offer(ctx context.Context, p *auth.Principal, establishmentID, entryID, spotID string, hold time.Duration, ip string) (*Offer, error) {
	return nil, configerr.ErrNotImplemented
}

// RevokeOffer revokes a pending offer and releases its hold.
func (s *Service) RevokeOffer(ctx context.Context, p *auth.Principal, establishmentID, entryID, offerID, ip string) (*Offer, error) {
	return nil, configerr.ErrNotImplemented
}

// GetSettings returns the settings (defaults when no row).
func (s *Service) GetSettings(ctx context.Context, p *auth.Principal, establishmentID string) (*Settings, error) {
	return nil, configerr.ErrNotImplemented
}

// UpdateSettings replaces the settings (establishment.configure).
func (s *Service) UpdateSettings(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in Settings, ip string) (*Settings, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- customer (link) ------------------------------------------------------------

// Join registers a public request (ErrDisabled when public join is off).
func (s *Service) Join(ctx context.Context, slug string, in PublicJoinInput) error {
	return configerr.ErrNotImplemented
}

// GetByToken returns the entry of a waitlist link.
func (s *Service) GetByToken(ctx context.Context, slug, token string) (*PublicEntry, error) {
	return nil, configerr.ErrNotImplemented
}

// LeaveByToken removes the entry of a waitlist link.
func (s *Service) LeaveByToken(ctx context.Context, slug, token string) error {
	return configerr.ErrNotImplemented
}

// AcceptByToken converts the pending offer into a checkout session.
func (s *Service) AcceptByToken(ctx context.Context, slug, token, idempotencyKey string) (*checkout.Session, error) {
	return nil, configerr.ErrNotImplemented
}

// DeclineByToken declines the pending offer (leave = also remove the entry).
func (s *Service) DeclineByToken(ctx context.Context, slug, token string, leave bool) (*PublicEntry, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- event sink and jobs -----------------------------------------------------------

// Name implements ext.EventSink.
func (s *Service) Name() string { return SinkName }

// Handle implements ext.EventSink: offers freed inventory to waiting
// entries (no-op until implemented).
func (s *Service) Handle(ctx context.Context, ev ext.Event) error { return nil }

// ExpireDue expires pending offers past expires_at and past entries.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	return 0, configerr.ErrNotImplemented
}

// Jobs returns the waitlist jobs. Registered by internal/app once implemented.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobExpire, Interval: time.Minute, RunOnStart: true, Timeout: 2 * time.Minute, Run: func(ctx context.Context) error {
		_, err := s.ExpireDue(ctx)
		return err
	}}}
}
