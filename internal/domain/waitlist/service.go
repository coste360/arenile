package waitlist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
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

// Audit actions.
const (
	ActionJoined          = "waitlist.joined"
	ActionUpdated         = "waitlist.updated"
	ActionRemoved         = "waitlist.removed"
	ActionReordered       = "waitlist.reordered"
	ActionOffered         = "waitlist.offered"
	ActionOfferClosed     = "waitlist.offer_closed"
	ActionConverted       = "waitlist.converted"
	ActionSettingsUpdated = "waitlist.settings_updated"
)

// Limits.
const (
	maxEntryDays    = 200
	maxQuantity     = 20
	maxNote         = 1000
	minHoldMinutes  = 5
	maxHoldMinutes  = 10080
	priorityStep    = 10
	queueLimit      = 5000
	maxReorder      = 1000
	maxSearchResult = 200
)

// Errors of the waitlist package besides errors.go.
var (
	// ErrEstablishmentNotFound: unknown establishment or outside the scope (404).
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrNotFound: unknown entry or offer, or a link that does not match (404).
	ErrNotFound = fmt.Errorf("%w: lista d'attesa", configerr.ErrNotFound)
	// ErrClosed: the entry is no longer open (converted, expired, removed) (409).
	ErrClosed = fmt.Errorf("%w: l'iscrizione non è più attiva", configerr.ErrConflict)
	// ErrNoAvailability: no compatible resource could be held for the
	// offer (409).
	ErrNoAvailability = fmt.Errorf("%w: nessuna disponibilità adatta per l'offerta", configerr.ErrConflict)
	// ErrVersionMismatch: If-Match does not match (412).
	ErrVersionMismatch = configerr.ErrVersionConflict
)

// Deps are the dependencies of the service.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Holds     Holds
	Checkout  Checkout
	Notify    Notifier
	Customers Customers
	// Terms is optional (no terms check on the public join when nil).
	Terms Terms
	// BaseURL builds the waitlist links (<base>/b/<slug>/m/w/<token>, under /m/ for the no-referrer and no-store headers).
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
	d.BaseURL = strings.TrimRight(d.BaseURL, "/")
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

// EntryChange is the payload (version 1) of the waitlist.* events: ids,
// dates and statuses only, never personal data.
type EntryChange struct {
	EntryID    string `json:"entry_id"`
	OfferID    string `json:"offer_id,omitempty"`
	HoldID     string `json:"hold_id,omitempty"`
	BookingID  string `json:"booking_id,omitempty"`
	Channel    string `json:"channel,omitempty"`
	StartsOn   string `json:"starts_on,omitempty"`
	EndsOn     string `json:"ends_on,omitempty"`
	PackID     string `json:"pack_id,omitempty"`
	Cause      string `json:"cause,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	CustomerID string `json:"customer_id,omitempty"`
}

// ---- panel ------------------------------------------------------------------

// List returns the entries in priority order (bookings.read).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f Filter, cursor string, limit int) (store.Page[Entry], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[Entry]{}, err
	}
	sf := store.WaitlistFilter{Status: f.Status, ZoneID: f.ZoneID, PackID: f.PackID, From: f.From, To: f.To}
	if q := strings.TrimSpace(f.Query); q != "" {
		cs, err := store.ListCustomers(ctx, s.d.DB, es, store.CustomerFilter{Terms: customer.SearchTerms(q)}, "", maxSearchResult)
		if err != nil {
			return store.Page[Entry]{}, fmt.Errorf("waitlist: search: %w", err)
		}
		sf.Customer = make([]string, 0, len(cs.Items))
		for _, c := range cs.Items {
			sf.Customer = append(sf.Customer, c.ID)
		}
	}
	page, err := store.ListWaitlistEntries(ctx, s.d.DB, es, sf, cursor, limit)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return store.Page[Entry]{}, configerr.Invalid("cursor", "Cursore non valido.")
		}
		return store.Page[Entry]{}, fmt.Errorf("waitlist: list: %w", err)
	}
	items, err := s.decorate(ctx, s.d.DB, es, page.Items)
	if err != nil {
		return store.Page[Entry]{}, err
	}
	return store.Page[Entry]{Items: items, NextCursor: page.NextCursor}, nil
}

// decorate adds names, positions, offer counts and the active offer.
func (s *Service) decorate(ctx context.Context, db bun.IDB, es store.EstablishmentScope, rows []store.WaitlistEntry) ([]Entry, error) {
	out := make([]Entry, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids, custIDs := make([]string, len(rows)), make([]string, len(rows))
	for i, r := range rows {
		ids[i], custIDs[i] = r.ID, r.CustomerID
	}
	names, err := store.CustomerDisplayNames(ctx, db, es, custIDs)
	if err != nil {
		return nil, fmt.Errorf("waitlist: names: %w", err)
	}
	queue, err := store.WaitingQueue(ctx, db, es, queueLimit)
	if err != nil {
		return nil, fmt.Errorf("waitlist: queue: %w", err)
	}
	pos := make(map[string]int, len(queue))
	for i, id := range queue {
		pos[id] = i + 1
	}
	counts, pending, err := store.WaitlistOfferStats(ctx, db, es, ids)
	if err != nil {
		return nil, fmt.Errorf("waitlist: offers: %w", err)
	}
	offers := make([]store.WaitlistOffer, 0, len(pending))
	for _, o := range pending {
		offers = append(offers, o)
	}
	labels, err := s.spotLabels(ctx, db, es, offers)
	if err != nil {
		return nil, err
	}
	for i, r := range rows {
		e := Entry{WaitlistEntry: r, CustomerDisplayName: names[r.CustomerID], Position: pos[r.ID], OfferCount: counts[r.ID]}
		if o, ok := pending[r.ID]; ok {
			e.ActiveOffer = &Offer{WaitlistOffer: o, SpotLabel: labelOf(labels, o.SpotID)}
		}
		out[i] = e
	}
	return out, nil
}

func (s *Service) spotLabels(ctx context.Context, db bun.IDB, es store.EstablishmentScope, offers []store.WaitlistOffer) (map[string]string, error) {
	var ids []string
	for _, o := range offers {
		if o.SpotID != nil {
			ids = append(ids, *o.SpotID)
		}
	}
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	spots, err := store.BookingSpotsByIDs(ctx, db, es, ids)
	if err != nil {
		return nil, fmt.Errorf("waitlist: spots: %w", err)
	}
	for _, sp := range spots {
		out[sp.ID] = sp.Label
	}
	return out, nil
}

func labelOf(labels map[string]string, id *string) string {
	if id == nil {
		return ""
	}
	return labels[*id]
}

// Create adds an entry from the panel (bookings.write).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in Input) (*Entry, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	set, err := s.settings(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	if !set.Enabled {
		return nil, ErrDisabled
	}
	if in.Channel == "" {
		in.Channel = store.ChannelDesk
	}
	in.Channel = auth.ChannelFor(p, in.Channel)
	in.Note = strings.TrimSpace(in.Note)
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	if err := s.validate(ctx, s.d.DB, es, &in, today); err != nil {
		return nil, err
	}
	var row *store.WaitlistEntry
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		c, err := store.GetCustomer(ctx, tx, es, in.CustomerID)
		if err != nil || c.AnonymizedAt != nil {
			if err == nil || errors.Is(err, store.ErrNotFound) {
				return configerr.Invalid("customer_id", "Cliente inesistente.")
			}
			return fmt.Errorf("waitlist: customer: %w", err)
		}
		row, err = s.insertEntry(ctx, tx, es, in, c.Locale, nil, p.Actor(in.IP))
		return err
	})
	if err != nil {
		return nil, err
	}
	items, err := s.decorate(ctx, s.d.DB, es, []store.WaitlistEntry{*row})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// insertEntry writes a new entry at the end of the queue with audit and
// waitlist.joined.
func (s *Service) insertEntry(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in Input, locale string, tokenHash *string, actor audit.Actor) (*store.WaitlistEntry, error) {
	top, err := store.MaxWaitlistPriority(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("waitlist: %w", err)
	}
	now := s.now()
	row := &store.WaitlistEntry{
		ID: store.NewID(), CustomerID: in.CustomerID, Channel: in.Channel, StartsOn: in.StartsOn, EndsOn: in.EndsOn,
		Slots: in.Slots, PackID: in.PackID, ZoneID: optional(in.ZoneID), SpotID: optional(in.SpotID),
		Quantity: max(in.Quantity, 1), Priority: top + priorityStep, Status: store.WaitlistWaiting, Note: in.Note,
		Locale: pickLocale(locale), AccessTokenHash: tokenHash, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertWaitlistEntry(ctx, tx, es, row); err != nil {
		return nil, fmt.Errorf("waitlist: insert: %w", err)
	}
	ch := entryChange(row)
	if err := s.record(ctx, tx, es, actor, ActionJoined, row.ID, map[string]any{
		"channel": row.Channel, "starts_on": row.StartsOn.String(), "ends_on": row.EndsOn.String(), "pack_id": row.PackID,
		"quantity": row.Quantity, "priority": row.Priority,
	}, event.WaitlistJoined, ch); err != nil {
		return nil, err
	}
	return row, nil
}

// Get returns an entry with its offers (bookings.read).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Entry, []Offer, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, nil, err
	}
	row, err := s.entry(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, nil, err
	}
	items, err := s.decorate(ctx, s.d.DB, es, []store.WaitlistEntry{*row})
	if err != nil {
		return nil, nil, err
	}
	rows, err := store.ListWaitlistOffers(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, nil, fmt.Errorf("waitlist: offers: %w", err)
	}
	labels, err := s.spotLabels(ctx, s.d.DB, es, rows)
	if err != nil {
		return nil, nil, err
	}
	offers := make([]Offer, len(rows))
	for i, o := range rows {
		offers[i] = Offer{WaitlistOffer: o, SpotLabel: labelOf(labels, o.SpotID)}
	}
	return &items[0], offers, nil
}

// Update changes an entry (bookings.write, optimistic concurrency). The
// request (period, slots, pack, zone, spot, quantity) can change only while
// the entry is waiting; priority and note at any time while open.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in Patch) (*Entry, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	var row store.WaitlistEntry
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := s.entry(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if expected != nil && *expected != cur.Version {
			return ErrVersionMismatch
		}
		if !open(cur.Status) {
			return ErrClosed
		}
		next := *cur
		request := in.StartsOn != nil || in.EndsOn != nil || in.Slots != nil || in.PackID != nil || in.ZoneID != nil || in.SpotID != nil || in.Quantity != nil
		if request && cur.Status != store.WaitlistWaiting {
			return fmt.Errorf("%w: revoca prima l'offerta attiva per modificare la richiesta", configerr.ErrConflict)
		}
		applyPatch(&next, in)
		if request {
			vin := Input{
				CustomerID: next.CustomerID, StartsOn: next.StartsOn, EndsOn: next.EndsOn, Slots: next.Slots, PackID: next.PackID,
				ZoneID: deref(next.ZoneID), SpotID: deref(next.SpotID), Quantity: next.Quantity, Channel: next.Channel, Note: next.Note,
			}
			if err := s.validate(ctx, tx, es, &vin, today); err != nil {
				return err
			}
		}
		if in.Note != nil {
			next.Note = strings.TrimSpace(*in.Note)
			if len([]rune(next.Note)) > maxNote {
				return configerr.Invalid("note", fmt.Sprintf("Massimo %d caratteri.", maxNote))
			}
		}
		next.Version, next.UpdatedAt = cur.Version+1, s.now()
		ok, err := store.UpdateWaitlistEntry(ctx, tx, es, &next, cur.Version)
		if err != nil {
			return fmt.Errorf("waitlist: update: %w", err)
		}
		if !ok {
			return ErrVersionMismatch
		}
		row = next
		return s.record(ctx, tx, es, p.Actor(in.IP), ActionUpdated, id, map[string]any{"fields": patchFields(in)}, "", nil)
	})
	if err != nil {
		return nil, err
	}
	items, err := s.decorate(ctx, s.d.DB, es, []store.WaitlistEntry{row})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func applyPatch(e *store.WaitlistEntry, in Patch) {
	if in.StartsOn != nil {
		e.StartsOn = *in.StartsOn
	}
	if in.EndsOn != nil {
		e.EndsOn = *in.EndsOn
	}
	if in.Slots != nil {
		e.Slots = *in.Slots
	}
	if in.PackID != nil {
		e.PackID = *in.PackID
	}
	if in.ZoneID != nil {
		e.ZoneID = optional(*in.ZoneID)
	}
	if in.SpotID != nil {
		e.SpotID = optional(*in.SpotID)
	}
	if in.Quantity != nil {
		e.Quantity = *in.Quantity
	}
	if in.Priority != nil {
		e.Priority = *in.Priority
	}
}

func patchFields(in Patch) []string {
	var out []string
	add := func(set bool, name string) {
		if set {
			out = append(out, name)
		}
	}
	add(in.StartsOn != nil, "starts_on")
	add(in.EndsOn != nil, "ends_on")
	add(in.Slots != nil, "slots")
	add(in.PackID != nil, "pack_id")
	add(in.ZoneID != nil, "zone_id")
	add(in.SpotID != nil, "spot_id")
	add(in.Quantity != nil, "quantity")
	add(in.Priority != nil, "priority")
	add(in.Note != nil, "note")
	return out
}

// Remove marks an entry removed and revokes its pending offer.
func (s *Service) Remove(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := s.entry(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if expected != nil && *expected != cur.Version {
			return ErrVersionMismatch
		}
		return s.remove(ctx, tx, es, cur, p.Actor(ip), "staff")
	})
}

// remove closes an open entry (removed, token cleared) and revokes its
// pending offer in tx. A closed entry is a no-op.
func (s *Service) remove(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, cur *store.WaitlistEntry, actor audit.Actor, by string) error {
	if !open(cur.Status) {
		return nil
	}
	if cur.Status == store.WaitlistOffered {
		if o, err := store.PendingWaitlistOffer(ctx, tx, es, cur.ID); err == nil {
			if err := s.closeOffer(ctx, tx, es, o, store.WaitlistOfferRevoked, actor); err != nil {
				return err
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("waitlist: %w", err)
		}
	}
	now := s.now()
	next := *cur
	next.Status, next.RemovedAt, next.AccessTokenHash = store.WaitlistRemoved, &now, nil
	next.Version, next.UpdatedAt = cur.Version+1, now
	ok, err := store.UpdateWaitlistEntry(ctx, tx, es, &next, cur.Version)
	if err != nil {
		return fmt.Errorf("waitlist: remove: %w", err)
	}
	if !ok {
		return ErrVersionMismatch
	}
	return s.record(ctx, tx, es, actor, ActionRemoved, cur.ID, map[string]any{"by": by}, "", nil)
}

// Reorder assigns priority 10, 20, ... to ids in order.
func (s *Service) Reorder(ctx context.Context, p *auth.Principal, establishmentID string, ids []string, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return err
	}
	if len(ids) == 0 || len(ids) > maxReorder {
		return configerr.Invalid("entry_ids", fmt.Sprintf("Da 1 a %d iscrizioni.", maxReorder))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return configerr.Invalid("entry_ids", "Iscrizione ripetuta.")
		}
		seen[id] = true
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		for i, id := range ids {
			ok, err := store.SetWaitlistPriority(ctx, tx, es, id, (i+1)*priorityStep, now)
			if err != nil {
				return fmt.Errorf("waitlist: reorder: %w", err)
			}
			if !ok {
				return ErrNotFound
			}
		}
		return s.record(ctx, tx, es, p.Actor(ip), ActionReordered, ids[0], map[string]any{"entry_ids": ids}, "", nil)
	})
}

// GetSettings returns the settings (defaults when no row).
func (s *Service) GetSettings(ctx context.Context, p *auth.Principal, establishmentID string) (*Settings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	return s.settings(ctx, s.d.DB, es)
}

// UpdateSettings replaces the settings (establishment.configure).
func (s *Service) UpdateSettings(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in Settings, ip string) (*Settings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if in.OfferHoldMinutes < minHoldMinutes || in.OfferHoldMinutes > maxHoldMinutes {
		return nil, configerr.Invalid("offer_hold_minutes", fmt.Sprintf("Tra %d e %d minuti.", minHoldMinutes, maxHoldMinutes))
	}
	var out *Settings
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		row := &store.WaitlistSettings{
			Enabled: in.Enabled, PublicJoin: in.PublicJoin, AutoOffer: in.AutoOffer, OfferHoldMinutes: in.OfferHoldMinutes,
			UpdatedAt: now,
		}
		cur, err := store.GetWaitlistSettings(ctx, tx, es)
		switch {
		case errors.Is(err, store.ErrNotFound):
			if expected != nil && *expected != 1 {
				return ErrVersionMismatch
			}
			row.Version, row.CreatedAt = 2, now
			if err := store.InsertWaitlistSettings(ctx, tx, es, row); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return ErrVersionMismatch
				}
				return fmt.Errorf("waitlist: settings: %w", err)
			}
		case err != nil:
			return fmt.Errorf("waitlist: settings: %w", err)
		default:
			if expected != nil && *expected != cur.Version {
				return ErrVersionMismatch
			}
			row.Version, row.CreatedAt = cur.Version+1, cur.CreatedAt
			ok, err := store.UpdateWaitlistSettings(ctx, tx, es, row, cur.Version)
			if err != nil {
				return fmt.Errorf("waitlist: settings: %w", err)
			}
			if !ok {
				return ErrVersionMismatch
			}
		}
		out = settingsOf(row)
		return s.record(ctx, tx, es, p.Actor(ip), ActionSettingsUpdated, es.ID(), map[string]any{
			"enabled": row.Enabled, "public_join": row.PublicJoin, "auto_offer": row.AutoOffer, "offer_hold_minutes": row.OfferHoldMinutes,
		}, "", nil)
	})
	return out, err
}

// settings returns the settings of es (defaults when no row: version 1).
func (s *Service) settings(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*Settings, error) {
	row, err := store.GetWaitlistSettings(ctx, db, es)
	if errors.Is(err, store.ErrNotFound) {
		return &Settings{Enabled: true, PublicJoin: false, AutoOffer: true, OfferHoldMinutes: DefaultOfferHoldMinutes, Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("waitlist: settings: %w", err)
	}
	return settingsOf(row), nil
}

func settingsOf(r *store.WaitlistSettings) *Settings {
	return &Settings{Enabled: r.Enabled, PublicJoin: r.PublicJoin, AutoOffer: r.AutoOffer, OfferHoldMinutes: r.OfferHoldMinutes, Version: r.Version}
}

// ---- validation -------------------------------------------------------------------

// validate checks a request against the catalog of es.
func (s *Service) validate(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in *Input, today calendar.Date) error {
	var ve configerr.ValidationError
	if in.StartsOn.IsZero() || in.EndsOn.IsZero() || in.EndsOn.Before(in.StartsOn) {
		ve.Add("ends_on", "La data finale non può precedere quella iniziale.")
	} else {
		if in.EndsOn.Before(today) {
			ve.Add("ends_on", "Il periodo richiesto è già passato.")
		}
		if in.StartsOn.DaysUntil(in.EndsOn) >= maxEntryDays {
			ve.Add("ends_on", fmt.Sprintf("Al massimo %d giorni.", maxEntryDays))
		}
	}
	if in.Slots == "" {
		in.Slots = calendar.SlotsFull
	}
	if !in.Slots.Valid() {
		ve.Add("slots", "Fasce non valide.")
	}
	if in.Quantity == 0 {
		in.Quantity = 1
	}
	if in.Quantity < 1 || in.Quantity > maxQuantity {
		ve.Add("quantity", fmt.Sprintf("Quantità tra 1 e %d.", maxQuantity))
	}
	switch in.Channel {
	case store.ChannelDesk, store.ChannelOnline, store.ChannelAPI:
	default:
		ve.Add("channel", "Canale non valido.")
	}
	if len([]rune(in.Note)) > maxNote {
		ve.Add("note", fmt.Sprintf("Massimo %d caratteri.", maxNote))
	}
	if in.PackID == "" {
		ve.Add("pack_id", "Campo obbligatorio.")
	} else if pk, err := store.GetPackForInventory(ctx, db, es, in.PackID); err != nil || !pk.Active {
		ve.Add("pack_id", "Pack inesistente o non attivo.")
	}
	if in.ZoneID != "" {
		if zs, err := store.InventoryZones(ctx, db, es, []string{in.ZoneID}); err != nil || len(zs) == 0 {
			ve.Add("zone_id", "Zona inesistente.")
		}
	}
	if in.SpotID != "" {
		sp, err := store.BookingSpotsByIDs(ctx, db, es, []string{in.SpotID})
		switch {
		case err != nil || len(sp) == 0:
			ve.Add("spot_id", "Postazione inesistente.")
		case in.ZoneID != "" && sp[0].ZoneID != in.ZoneID:
			ve.Add("spot_id", "La postazione non appartiene alla zona indicata.")
		case in.Quantity > 1:
			ve.Add("quantity", "Con una postazione indicata la quantità è 1.")
		}
	}
	return ve.OrNil()
}

// ---- helpers ----------------------------------------------------------------------

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

func (s *Service) entry(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*store.WaitlistEntry, error) {
	e, err := store.GetWaitlistEntry(ctx, db, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("waitlist: %w", err)
	}
	return e, nil
}

// today is the current date in the establishment timezone.
func (s *Service) today(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (calendar.Date, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("waitlist: establishment: %w", err)
	}
	return calendar.In(s.d.Clock.Now(), location(est)), nil
}

func location(est *store.Establishment) *time.Location {
	if loc, err := time.LoadLocation(est.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// record writes the audit entry and, when eventType is set, the outbox
// event, in tx.
func (s *Service) record(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, actor audit.Actor, action, entityID string, diff any, eventType string, data any) error {
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "waitlist_entry", EntityID: entityID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("waitlist: audit: %w", err)
	}
	if eventType == "" {
		return nil
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, data); err != nil {
		return fmt.Errorf("waitlist: outbox: %w", err)
	}
	return nil
}

func entryChange(e *store.WaitlistEntry) EntryChange {
	return EntryChange{
		EntryID: e.ID, Channel: e.Channel, StartsOn: e.StartsOn.String(), EndsOn: e.EndsOn.String(), PackID: e.PackID,
		CustomerID: e.CustomerID,
	}
}

// open reports whether an entry still takes part in the queue.
func open(status string) bool {
	return status == store.WaitlistWaiting || status == store.WaitlistOffered
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var locales = map[string]bool{"it": true, "en": true, "de": true, "fr": true}

func pickLocale(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if locales[l] {
		return l
	}
	return "it"
}
