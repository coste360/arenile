package ext

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ---- fiscal (spec 14.2) ---------------------------------------------------------

// FiscalDriver issues commercial documents through a fiscal device or service.
type FiscalDriver interface {
	// Code is the stable identifier (e.g. "none", "export").
	Code() string
	// Issue emits a fiscal document.
	Issue(ctx context.Context, doc FiscalDocumentRequest) (*FiscalDocumentResult, error)
	// Void cancels a previously issued document.
	Void(ctx context.Context, ref string) error
	// Status reports the state of the device or service.
	Status(ctx context.Context) (DeviceStatus, error)
}

// FiscalLine is one line of a fiscal document, derived from the price
// snapshot. Amounts are gross cents; VATRateBP is in basis points (2200 = 22%).
type FiscalLine struct {
	Description string
	Quantity    int64
	UnitGross   int64
	Gross       int64
	VATRateBP   int64
}

// FiscalPayment is one payment method share of a fiscal document.
type FiscalPayment struct {
	Method string // "cash", "card", "online", ...
	Amount int64
}

// FiscalDocumentRequest is the document to issue.
type FiscalDocumentRequest struct {
	EstablishmentID string
	BookingID       string
	PaymentID       string
	Currency        string
	Lines           []FiscalLine
	Payments        []FiscalPayment
	Total           int64
	IssuedAt        time.Time
	IdempotencyKey  string
}

// FiscalDocumentResult identifies an issued document.
type FiscalDocumentResult struct {
	Ref      string
	Number   string
	IssuedAt time.Time
	// Raw is the driver-specific response, stored for audit.
	Raw json.RawMessage
}

// DeviceStatus describes a fiscal device.
type DeviceStatus struct {
	Online    bool
	Message   string
	CheckedAt time.Time
}

// ---- events (spec 18) --------------------------------------------------------------

// Event is the envelope of a domain event (spec 18.1). ID has the form
// "evt_<uuidv7>"; Data is the versioned JSON payload.
type Event struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	EstablishmentID string          `json:"establishment_id"`
	OccurredAt      time.Time       `json:"occurred_at"`
	Data            json.RawMessage `json:"data"`
	Version         int             `json:"version"`
}

// EventSink consumes domain events in-process after they leave the outbox.
// Delivery is at-least-once: implementations must be idempotent on Event.ID
// and handle their own retries; returned errors are logged.
type EventSink interface {
	Name() string
	Handle(ctx context.Context, ev Event) error
}

// EventBus fans out lightweight real-time messages to connected panels
// (spec 21.1). The core ships an in-memory bus; multi-instance deployments
// can provide one backed by PostgreSQL LISTEN/NOTIFY.
type EventBus interface {
	// Publish delivers ev to the current subscribers of its establishment.
	Publish(ctx context.Context, ev Event) error
	// Subscribe returns a channel of events for one establishment and a
	// cancel function that must be called to release resources. The channel
	// is closed after cancel or when ctx is done.
	Subscribe(ctx context.Context, establishmentID string) (<-chan Event, func(), error)
}

// ---- concession data -----------------------------------------------------------------

// ConcessionInfo describes a state-owned maritime concession.
type ConcessionInfo struct {
	Ref          string
	Holder       string
	Municipality string
	ExpiresOn    string // YYYY-MM-DD, empty if unknown
	AreaSqm      float64
	// Perimeter is a GeoJSON geometry (Polygon or MultiPolygon).
	Perimeter json.RawMessage
	// SatelliteImageURL is an optional background image for the map editor.
	SatelliteImageURL string
	Extra             map[string]string
}

// ConcessionDataProvider looks up concession data by the establishment's
// concession_ref.
type ConcessionDataProvider interface {
	Lookup(ctx context.Context, concessionRef string) (*ConcessionInfo, error)
}

// ---- tenancy ------------------------------------------------------------------------

// TenantResolver resolves the establishment slug of a public request from
// host or headers (custom domains). ok=false falls back to the URL slug.
type TenantResolver interface {
	ResolveSlug(r *http.Request) (slug string, ok bool, err error)
}

// ---- authentication -----------------------------------------------------------------

// ExternalIdentity is a user identity asserted by an AuthProvider.
type ExternalIdentity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// AuthProvider adds an external sign-in method (e.g. OIDC). The core maps
// the identity to an existing user by verified email; it never creates
// memberships on its own.
type AuthProvider interface {
	// Code is the stable identifier used in URLs (/auth/ext/{code}/...).
	Code() string
	// DisplayName is shown on the login page.
	DisplayName() string
	// BeginLogin redirects the browser to the identity provider. state must
	// be round-tripped and is verified by the core.
	BeginLogin(w http.ResponseWriter, r *http.Request, state, callbackURL string) error
	// CompleteLogin validates the callback and returns the identity.
	CompleteLogin(ctx context.Context, r *http.Request, callbackURL string) (*ExternalIdentity, error)
}

// ---- notifications ------------------------------------------------------------------

// Attachment is a file attached to a message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
	// Inline attachments are referenced from the body (e.g. a QR code).
	Inline    bool
	ContentID string
}

// Message is a rendered notification ready to send.
type Message struct {
	EstablishmentID string
	Channel         string
	To              string
	Locale          string
	TemplateCode    string
	Subject         string
	// BodyText and BodyHTML are already rendered from the template.
	BodyText    string
	BodyHTML    string
	Attachments []Attachment
	// IdempotencyKey lets channels deduplicate retries.
	IdempotencyKey string
}

// SendResult identifies a sent message on the channel.
type SendResult struct {
	ProviderRef string
}

// Notifier is an outgoing message channel (email is core; WhatsApp, SMS...
// are extensions).
type Notifier interface {
	// Channel is the stable channel code ("email", "sms", "whatsapp").
	Channel() string
	Send(ctx context.Context, msg Message) (*SendResult, error)
}

// ---- bot protection -----------------------------------------------------------------

// BotProtectionClientConfig is what the public page needs to render the challenge.
type BotProtectionClientConfig struct {
	Provider  string `json:"provider"`
	SiteKey   string `json:"site_key"`
	ScriptURL string `json:"script_url"`
}

// BotProtection verifies an invisible challenge token on public creations.
type BotProtection interface {
	ClientConfig() BotProtectionClientConfig
	// Verify reports whether token is valid for remoteIP.
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

// ---- admin UI plugins ---------------------------------------------------------------

// UIMenuItem is an entry added to the admin navigation.
type UIMenuItem struct {
	// Label by locale, Italian ("it") required.
	Label map[string]string `json:"label"`
	// Path inside the plugin page (appended to PageURL).
	Path string `json:"path"`
	// Icon is an optional icon name of the panel icon set.
	Icon string `json:"icon,omitempty"`
	// RequiredPermission hides the entry from users lacking it (e.g. "reports.read").
	RequiredPermission string `json:"required_permission,omitempty"`
}

// UIPluginManifest declares extra pages of the admin panel. Pages are
// loaded in a sandboxed iframe with a short-lived session token.
type UIPluginManifest struct {
	ID        string            `json:"id"`
	Title     map[string]string `json:"title"`
	PageURL   string            `json:"page_url"`
	MenuItems []UIMenuItem      `json:"menu_items"`
}
