package checkout

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the checkout service (ports & adapters). Implementations
// live in the provider packages and are asserted in internal/app/m4.go.
// Methods taking tx run in the caller's transaction and never open their own.

// Settings resolves a public slug and the online configuration.
// Implemented by *online.Service.
type Settings interface {
	// ResolvePublic returns scope, establishment, flow, current terms and
	// payment configuration; online.ErrEstablishmentNotFound /
	// online.ErrBookingDisabled (both 404).
	ResolvePublic(ctx context.Context, slug string) (*online.PublicEstablishment, error)
	// CurrentTerms is the version the customer must accept (nil = none).
	CurrentTerms(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*store.TermsVersion, error)
}

// Inventory reads availability for the online channel. Implemented by
// *inventory.Service.
type Inventory interface {
	PublicAvailability(ctx context.Context, es store.EstablishmentScope, q inventory.AvailabilityQuery) (*inventory.AvailabilityResult, error)
}

// Pricer prices a selection with the online policy. Implemented by
// *pricingconfig.Service (Quote: Simulate without the permission check).
type Pricer interface {
	Quote(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in pricingconfig.SimulateInput) (*pricingconfig.SimulateResult, error)
}

// Bookings performs the booking transitions of the public channel.
// Implemented by *booking.Service (internal/domain/booking/online.go).
type Bookings interface {
	CreateOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
	AttachCustomer(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, in booking.OnlineCustomerInput) (*booking.Detail, error)
	StartPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, holdUntil time.Time) (*booking.Detail, error)
	ConfirmWithoutPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string) (*booking.Detail, error)
	Abandon(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string) error
	ModifyAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, bookingID string, expected *int64, in booking.PatchInput) (*booking.ChangeResult, error)
	CancelAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, bookingID string, in booking.CancelInput) (*booking.CancelResult, error)
	DetailIn(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*booking.Detail, error)
}

// Payments starts provider payments and reports their state. Implemented
// by *gateway.Service.
type Payments interface {
	Start(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req gateway.StartRequest) (*gateway.StartResult, error)
	LatestForBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*store.PaymentIntent, error)
	RefundViaProvider(ctx context.Context, es store.EstablishmentScope, req gateway.RefundRequest) (*store.PaymentIntent, error)
}

// Notifier issues and resolves magic links, renders QR and .ics, and
// handles "send me a new link". Implemented by *notify.Service.
type Notifier interface {
	IssueMagicLink(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, createdBy *string) (*notify.MagicLink, error)
	ResolveMagicLink(ctx context.Context, db bun.IDB, es store.EstablishmentScope, token string) (*store.MagicLinkToken, error)
	RequestLink(ctx context.Context, es store.EstablishmentScope, email, code string) error
	QR(payload, format string) ([]byte, string, error)
	ICS(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID, manageURL string) ([]byte, error)
}

// SeasonPasses releases season pass days from the manage area.
// Implemented by *pass.Service (internal/domain/pass/online.go).
type SeasonPasses interface {
	ReleaseAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, passID string, in pass.ReleaseInput) ([]store.SeasonRelease, error)
	// PreviewReleaseAsCustomer runs the release and rolls it back.
	PreviewReleaseAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, passID string, in pass.ReleaseInput) ([]store.SeasonRelease, error)
}
