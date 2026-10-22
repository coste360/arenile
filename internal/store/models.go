package store

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var (
	// ErrNotFound is returned when a row does not exist or is outside the
	// caller's scope (the two cases are deliberately indistinguishable).
	ErrNotFound = errors.New("store: not found")
	// ErrConflict is returned on unique constraint violations.
	ErrConflict = errors.New("store: conflict")
)

// NewID returns a new UUIDv7 as a string.
func NewID() string { return uuid.Must(uuid.NewV7()).String() }

// JSON is a raw JSON document stored as TEXT (SQLite) or jsonb (PostgreSQL).
// It always binds as a string literal so both dialects accept it.
type JSON []byte

// MarshalJSONValue encodes v into a JSON column value.
func MarshalJSONValue(v any) (JSON, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("store: marshal json: %w", err)
	}
	return JSON(b), nil
}

// Value implements driver.Valuer.
func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return string(j), nil
}

// Scan implements sql.Scanner.
func (j *JSON) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case string:
		*j = JSON(v)
	case []byte:
		*j = append(JSON(nil), v...)
	default:
		return fmt.Errorf("store: cannot scan %T into JSON", src)
	}
	return nil
}

// MarshalJSON emits the raw document.
func (j JSON) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

// Unmarshal decodes the document into v; an empty document leaves v unchanged.
func (j JSON) Unmarshal(v any) error {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	if err := json.Unmarshal(j, v); err != nil {
		return fmt.Errorf("store: unmarshal json: %w", err)
	}
	return nil
}

// IsUniqueViolation reports whether err is a unique constraint violation on
// either dialect.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr interface{ Field(byte) string }
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "SQLITE_CONSTRAINT_UNIQUE") ||
		strings.Contains(msg, "SQLITE_CONSTRAINT_PRIMARYKEY")
}

// Organization is a legal entity owning establishments.
type Organization struct {
	bun.BaseModel `bun:"table:organizations"`

	ID             string    `bun:"id,pk"`
	Name           string    `bun:"name,notnull"`
	VATNumber      *string   `bun:"vat_number"`
	FiscalCode     *string   `bun:"fiscal_code"`
	BillingAddress JSON      `bun:"billing_address"`
	CreatedAt      time.Time `bun:"created_at,notnull"`
	UpdatedAt      time.Time `bun:"updated_at,notnull"`
}

// Address is a postal address.
type Address struct {
	Street     string `json:"street,omitempty"`
	City       string `json:"city,omitempty"`
	Province   string `json:"province,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
}

// Geo is a WGS84 point.
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Theme is the public page theme (spec 12.5).
type Theme struct {
	LogoURL           string                       `json:"logo_url,omitempty"`
	PrimaryColor      string                       `json:"primary_color,omitempty"`
	AccentColor       string                       `json:"accent_color,omitempty"`
	FontFamily        string                       `json:"font_family,omitempty"`
	Radius            string                       `json:"radius,omitempty"`
	ShowPricesOnMap   *bool                        `json:"show_prices_on_map,omitempty"`
	HideHeaderInEmbed *bool                        `json:"hide_header_in_embed,omitempty"`
	CustomTexts       map[string]map[string]string `json:"custom_texts,omitempty"`
}

// Establishment is a beach establishment (lido).
type Establishment struct {
	bun.BaseModel `bun:"table:establishments,alias:e"`

	ID                  string     `bun:"id,pk"`
	OrganizationID      string     `bun:"organization_id,notnull"`
	Slug                string     `bun:"slug,notnull"`
	Name                string     `bun:"name,notnull"`
	Timezone            string     `bun:"timezone,notnull"`
	Currency            string     `bun:"currency,notnull"`
	DefaultLocale       string     `bun:"default_locale,notnull"`
	EnabledLocales      []string   `bun:"enabled_locales,type:json,notnull"`
	ContactEmail        string     `bun:"contact_email,notnull"`
	ContactPhone        string     `bun:"contact_phone,notnull"`
	Address             Address    `bun:"address,type:json,notnull"`
	Geo                 *Geo       `bun:"geo,type:json"`
	ConcessionRef       *string    `bun:"concession_ref"`
	BookingEnabled      bool       `bun:"booking_enabled,notnull"`
	AllowedEmbedOrigins []string   `bun:"allowed_embed_origins,type:json,notnull"`
	Theme               Theme      `bun:"theme,type:json,notnull"`
	Settings            JSON       `bun:"settings,notnull"`
	ArchivedAt          *time.Time `bun:"archived_at"`
	CreatedAt           time.Time  `bun:"created_at,notnull"`
	UpdatedAt           time.Time  `bun:"updated_at,notnull"`
}

// User is a staff account of the admin panel.
type User struct {
	bun.BaseModel `bun:"table:users"`

	ID              string     `bun:"id,pk"`
	Email           string     `bun:"email,notnull"`
	PasswordHash    string     `bun:"password_hash,notnull"`
	Name            string     `bun:"name,notnull"`
	Locale          string     `bun:"locale,notnull"`
	TOTPSecret      *string    `bun:"totp_secret"` // encrypted with the key ring
	TOTPConfirmedAt *time.Time `bun:"totp_confirmed_at"`
	TOTPLastStep    int64      `bun:"totp_last_step,notnull"`
	Active          bool       `bun:"active,notnull"`
	LastLoginAt     *time.Time `bun:"last_login_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// TOTPEnabled reports whether the user completed TOTP enrolment.
func (u *User) TOTPEnabled() bool { return u.TOTPConfirmedAt != nil && u.TOTPSecret != nil }

// Membership grants a role on an organization (EstablishmentID nil = every
// establishment of the organization) or on a single establishment.
type Membership struct {
	bun.BaseModel `bun:"table:memberships"`

	ID              string    `bun:"id,pk"`
	UserID          string    `bun:"user_id,notnull"`
	OrganizationID  string    `bun:"organization_id,notnull"`
	EstablishmentID *string   `bun:"establishment_id"`
	Role            string    `bun:"role,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// Session is a panel login session. Only the SHA-256 of the token is stored.
type Session struct {
	bun.BaseModel `bun:"table:sessions"`

	ID         string    `bun:"id,pk"`
	TokenHash  string    `bun:"token_hash,notnull"`
	UserID     string    `bun:"user_id,notnull"`
	ExpiresAt  time.Time `bun:"expires_at,notnull"`
	LastSeenAt time.Time `bun:"last_seen_at,notnull"`
	IP         string    `bun:"ip,notnull"`
	UserAgent  string    `bun:"user_agent,notnull"`
	CreatedAt  time.Time `bun:"created_at,notnull"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"`
}

// APIKey is a server-to-server credential bound to one establishment.
type APIKey struct {
	bun.BaseModel `bun:"table:api_keys"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	Name            string     `bun:"name,notnull"`
	Prefix          string     `bun:"prefix,notnull"`
	KeyHash         string     `bun:"key_hash,notnull"`
	Scopes          []string   `bun:"scopes,type:json,notnull"`
	CreatedByUserID *string    `bun:"created_by_user_id"`
	LastUsedAt      *time.Time `bun:"last_used_at"`
	RevokedAt       *time.Time `bun:"revoked_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// AuditEntry is an insert-only audit log row.
type AuditEntry struct {
	bun.BaseModel `bun:"table:audit_log"`

	ID              string    `bun:"id,pk"`
	EstablishmentID *string   `bun:"establishment_id"`
	ActorType       string    `bun:"actor_type,notnull"`
	ActorID         *string   `bun:"actor_id"`
	Action          string    `bun:"action,notnull"`
	EntityType      string    `bun:"entity_type,notnull"`
	EntityID        *string   `bun:"entity_id"`
	Diff            JSON      `bun:"diff"`
	IP              string    `bun:"ip,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
}

// OutboxEvent is a domain event awaiting dispatch.
type OutboxEvent struct {
	bun.BaseModel `bun:"table:outbox_events"`

	ID              string     `bun:"id,pk"`
	EstablishmentID *string    `bun:"establishment_id"`
	Type            string     `bun:"type,notnull"`
	Payload         JSON       `bun:"payload,notnull"`
	Version         int        `bun:"version,notnull"`
	OccurredAt      time.Time  `bun:"occurred_at,notnull"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	DispatchedAt    *time.Time `bun:"dispatched_at"`
}

// WebhookEndpoint is an outgoing webhook target of an establishment.
type WebhookEndpoint struct {
	bun.BaseModel `bun:"table:webhook_endpoints"`

	ID                  string     `bun:"id,pk"`
	EstablishmentID     string     `bun:"establishment_id,notnull"`
	URL                 string     `bun:"url,notnull"`
	Secret              string     `bun:"secret,notnull"` // encrypted with the key ring
	Events              []string   `bun:"events,type:json,notnull"`
	Active              bool       `bun:"active,notnull"`
	ConsecutiveFailures int        `bun:"consecutive_failures,notnull"`
	DisabledAt          *time.Time `bun:"disabled_at"`
	DisabledReason      *string    `bun:"disabled_reason"`
	CreatedAt           time.Time  `bun:"created_at,notnull"`
	UpdatedAt           time.Time  `bun:"updated_at,notnull"`
}

// Matches reports whether the endpoint subscribes to eventType. An empty list
// or "*" subscribes to everything; "booking.*" matches a prefix.
func (e *WebhookEndpoint) Matches(eventType string) bool {
	if len(e.Events) == 0 {
		return true
	}
	for _, f := range e.Events {
		if f == "*" || f == eventType {
			return true
		}
		if strings.HasSuffix(f, ".*") && strings.HasPrefix(eventType, strings.TrimSuffix(f, "*")) {
			return true
		}
	}
	return false
}

// WebhookDelivery tracks the delivery of one event to one endpoint.
type WebhookDelivery struct {
	bun.BaseModel `bun:"table:webhook_deliveries"`

	ID              string     `bun:"id,pk"`
	EndpointID      string     `bun:"endpoint_id,notnull"`
	EventID         string     `bun:"event_id,notnull"`
	Attempt         int        `bun:"attempt,notnull"`
	StatusCode      *int       `bun:"status_code"`
	ResponseExcerpt *string    `bun:"response_excerpt"`
	LastError       *string    `bun:"last_error"`
	NextAttemptAt   *time.Time `bun:"next_attempt_at"`
	DeliveredAt     *time.Time `bun:"delivered_at"`
	FailedAt        *time.Time `bun:"failed_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// IdempotencyKey stores the outcome of a POST keyed by (scope, key).
type IdempotencyKey struct {
	bun.BaseModel `bun:"table:idempotency_keys"`

	Scope               string    `bun:"scope,pk"`
	Key                 string    `bun:"key,pk"`
	RequestHash         string    `bun:"request_hash,notnull"`
	ResponseStatus      *int      `bun:"response_status"`
	ResponseContentType *string   `bun:"response_content_type"`
	ResponseBody        []byte    `bun:"response_body"`
	CreatedAt           time.Time `bun:"created_at,notnull"`
	ExpiresAt           time.Time `bun:"expires_at,notnull"`
}
