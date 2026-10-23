package checkout

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Domain types of the public flow. Handlers in internal/http/public map
// them to the api.* types; the field comments name the API schema.

// ---- browse (reads) -------------------------------------------------------------

// CalendarQuery is GET calendar.
type CalendarQuery struct {
	// Month is "YYYY-MM".
	Month  string
	Slots  calendar.Slots
	ZoneID string
}

// Calendar day statuses (api.PublicCalendarDay.status).
const (
	DayAvailable   = "available"
	DayLimited     = "limited"
	DayFull        = "full"
	DayClosed      = "closed"
	DayUnavailable = "unavailable"
)

// CalendarDay is one day of the public calendar.
type CalendarDay struct {
	Date           calendar.Date
	Status         string
	RatePeriodCode string
}

// RatePeriod is a highlighted rate period (api.PublicRatePeriod).
type RatePeriod struct {
	Code     string
	Name     store.I18nText
	Color    string
	StartsOn calendar.Date
	EndsOn   calendar.Date
}

// Calendar is api.PublicCalendar.
type Calendar struct {
	Month       string
	Days        []CalendarDay
	RatePeriods []RatePeriod
}

// MapView is api.PublicMapDocument: active spots of online zones only.
type MapView struct {
	Map         store.Map
	Spots       []store.Spot
	Decorations []store.MapDecoration
	// PriceCategory is the effective category per spot id.
	PriceCategory map[string]string
}

// AvailabilityQuery is GET availability.
type AvailabilityQuery struct {
	From, To calendar.Date
	Slots    calendar.Slots
	Formula  string
	ZoneID   string
}

// Availability indicators (api.AvailabilityIndicator).
const (
	IndicatorPlenty = "plenty"
	IndicatorFew    = "few"
	IndicatorLast   = "last"
	IndicatorNone   = "none"
)

// SpotAvailability is api.PublicSpotAvailability.
type SpotAvailability struct {
	SpotID        string
	Available     bool
	PriceCategory string
}

// ZoneAvailability is api.PublicZoneAvailability.
type ZoneAvailability struct {
	ZoneID    string
	Mode      string
	Available int
	Indicator string
	FromPrice *money.Cents
	PoolID    string
	Spots     []SpotAvailability
}

// LegendEntry is api.PriceLegendEntry.
type LegendEntry struct {
	PriceCategory string
	Label         store.I18nText
	FromPrice     money.Cents
	Tone          int
}

// Availability is api.PublicAvailability.
type Availability struct {
	Query       AvailabilityQuery
	ClosedDates []calendar.Date
	Zones       []ZoneAvailability
	PriceLegend []LegendEntry
}

// CatalogQuery is GET catalog.
type CatalogQuery struct {
	From, To *calendar.Date
	Slots    calendar.Slots
	Formula  string
	ZoneID   string
	SpotID   string
}

// ComponentView is api.PublicPackComponent.
type ComponentView struct {
	Code     string
	Name     store.I18nText
	Quantity int
	Icon     string
}

// PackView is api.PublicPack.
type PackView struct {
	Pack       store.Pack
	Components []ComponentView
	FromPrice  *money.Cents
}

// ExtraView is api.PublicExtra.
type ExtraView struct {
	Extra     store.Extra
	UnitPrice *money.Cents
}

// Catalog is api.PublicCatalog.
type Catalog struct {
	Packs  []PackView
	Extras []ExtraView
}

// Selection is api.PublicSelection.
type Selection struct {
	StartsOn, EndsOn calendar.Date
	Slots            calendar.Slots
	Formula          string
	Lines            []booking.LineInput
	Extras           []booking.ExtraInput
	PromoCode        string
}

// Quote item kinds (api.PublicQuoteItem.kind).
const (
	ItemLine     = "line"
	ItemExtra    = "extra"
	ItemDiscount = "discount"
	ItemRounding = "rounding"
)

// QuoteItem is api.PublicQuoteItem.
type QuoteItem struct {
	Kind     string
	RefID    string
	Label    store.I18nText
	Quantity int
	Amount   money.Cents
}

// Promo statuses (api.PromoStatus.status).
const (
	PromoApplied       = "applied"
	PromoInvalid       = "invalid"
	PromoNotApplicable = "not_applicable"
)

// PromoStatus is api.PromoStatus.
type PromoStatus struct {
	Code   string
	Status string
}

// Payment plans (api.PaymentPlan).
const (
	PlanOnlineFull    = "online_full"
	PlanOnlineDeposit = "online_deposit"
	PlanAtDesk        = "at_desk"
	PlanApproval      = "approval"
)

// Quote is api.PublicQuote.
type Quote struct {
	Available    bool
	Conflicts    []availability.Conflict
	Currency     string
	Total        money.Cents
	DepositDue   money.Cents
	DueNow       money.Cents
	BalanceLater money.Cents
	Items        []QuoteItem
	Violations   []policy.Violation
	Promo        *PromoStatus
	Plan         string
}

// ---- session (writes) -----------------------------------------------------------

// CreateInput is POST bookings (api.PublicBookingCreate).
type CreateInput struct {
	Selection Selection
	Locale    string
	// EmbedOrigin is the verified origin of the embedding page, if any.
	EmbedOrigin string
}

// Session is api.CheckoutSession: Token is returned only here.
type Session struct {
	BookingID     string
	Token         string
	Status        string
	HoldExpiresAt time.Time
	Quote         *Quote
}

// Next actions (api.CheckoutNextAction).
const (
	NextDetails  = "details"
	NextPay      = "pay"
	NextWait     = "wait"
	NextDone     = "done"
	NextExpired  = "expired"
	NextRejected = "rejected"
)

// PaymentState is api.PublicPaymentState.
type PaymentState struct {
	// Status is none, pending, succeeded or failed.
	Status        string
	Amount        money.Cents
	FailureReason string
}

// Confirmation is api.PublicConfirmation.
type Confirmation struct {
	Code           string
	QRPayload      string
	QRSVG          string
	ManageLinkSent bool
	EmailMasked    string
}

// Labels name the packs, extras and zones of a booking summary.
type Labels struct {
	Packs, Extras, Zones map[string]store.I18nText
}

// State is api.CheckoutState.
type State struct {
	Labels       Labels
	Booking      *booking.Detail
	Session      store.CheckoutSession
	NextAction   string
	Quote        *Quote
	Payment      *PaymentState
	Confirmation *Confirmation
}

// CustomerInput is PATCH bookings/{id} (api.CheckoutCustomerInput).
type CustomerInput struct {
	FirstName, LastName string
	Email, Phone        string
	Locale              string
	AcceptTermsVersion  string
	MarketingConsent    bool
	Notes               string
	PromoCode           *string
	Extras              []booking.ExtraInput
	ExtrasSet           bool
}

// Settlements of POST pay (api.CheckoutPayRequest.settlement).
const (
	SettleOnline = "online"
	SettleAtDesk = "at_desk"
)

// PayInput is POST pay.
type PayInput struct {
	Settlement     string
	IdempotencyKey string
	// ReturnTo is the host page of the embedded widget (optional): the
	// provider returns there with arenile_checkout=<booking id>. Its origin
	// must be one of the establishment's allowed embed origins.
	ReturnTo string
}

// Pay modes (api.PayResult.mode).
const (
	PayRedirect = "redirect"
	PayEmbedded = "embedded"
	PayNone     = "none"
)

// PayOutcome is api.PayResult.
type PayOutcome struct {
	Mode           string
	Status         string
	RedirectURL    string
	ClientSecret   string
	PublishableKey string
	Amount         money.Cents
	HoldExpiresAt  *time.Time
}

// Document kinds served by the QR and .ics endpoints.
const (
	DocQRSVG = "qr_svg"
	DocQRPNG = "qr_png"
	DocICS   = "ics"
)

// Document is a binary response (QR image or .ics file).
type Document struct {
	ContentType string
	Filename    string
	Body        []byte
}

// ---- manage area ----------------------------------------------------------------

// Actions is api.ManageActions.
type Actions struct {
	PayBalance, Modify, Cancel, ReleaseDays bool
	ModifyUntil                             *calendar.Date
	CancelUntilFree                         *calendar.Date
}

// Managed is api.ManageBooking.
type Managed struct {
	Labels        Labels
	Quote         *Quote
	Booking       *booking.Detail
	Link          store.MagicLinkToken
	Allowed       Actions
	CancelPreview *booking.RefundPreview
	Releases      []store.SeasonRelease
	QRPayload     string
	QRSVG         string
}

// ModifyInput is POST manage/{token}/modify.
type ModifyInput struct {
	Selection Selection
	DryRun    bool
}

// ModifyOutcome is api.ManageModifyResult.
type ModifyOutcome struct {
	Result     *booking.ChangeResult
	Fee        money.Cents
	Quote      *Quote
	Violations []policy.Violation
	Managed    *Managed
}

// CancelInput is POST manage/{token}/cancel.
type CancelInput struct {
	DryRun     bool
	Reason     string
	Settlement string
}

// Refund statuses after a customer cancellation (api.ManageCancelResult.refund_status).
const (
	RefundNone              = "none"
	RefundProviderPending   = "provider_pending"
	RefundProviderSucceeded = "provider_succeeded"
	RefundManualDue         = "manual_due"
	RefundCredited          = "credited"
)

// CancelOutcome is api.ManageCancelResult.
type CancelOutcome struct {
	Result       *booking.CancelResult
	RefundStatus string
	Managed      *Managed
}

// ReleaseDay is api.ManageReleaseDay.
type ReleaseDay struct {
	Date  calendar.Date
	Slots calendar.Slots
}

// ReleaseInput is POST manage/{token}/releases.
type ReleaseInput struct {
	Days   []ReleaseDay
	DryRun bool
}

// ReleaseOutcome is api.ManageReleaseResult.
type ReleaseOutcome struct {
	Applied  bool
	Released []store.SeasonRelease
	Credit   money.Cents
	Managed  *Managed
}
