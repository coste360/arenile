package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/pkg/ext"
)

// Compile-time checks of the M4 ports (ports & adapters): each consumer
// package declares the interface it needs, the provider implements it, and
// this composition root is the only place that knows both.
var (
	_ checkout.Settings     = (*online.Service)(nil)
	_ checkout.Inventory    = (*inventory.Service)(nil)
	_ checkout.Pricer       = (*pricingconfig.Service)(nil)
	_ checkout.Bookings     = (*booking.Service)(nil)
	_ checkout.Payments     = (*gateway.Service)(nil)
	_ checkout.Notifier     = (*notify.Service)(nil)
	_ checkout.SeasonPasses = (*pass.Service)(nil)

	_ gateway.ProviderSettings = (*online.Service)(nil)
	_ gateway.Ledger           = (*payment.Service)(nil)
	_ gateway.Providers        = (*Registry)(nil)

	// Optional capabilities of the core Stripe driver (see gateway doc).
	_ ext.PaymentProvider            = (*stripe.Driver)(nil)
	_ gateway.PaymentStatusChecker   = (*stripe.Driver)(nil)
	_ gateway.CredentialsVerifier    = (*stripe.Driver)(nil)
	_ gateway.PaymentCanceller       = (*stripe.Driver)(nil)
	_ notify.Settings                = (*online.Service)(nil)
	_ online.Providers               = (*Registry)(nil)
	_ ext.EventSink                  = (*notify.Service)(nil).Sink()
	_ gateway.HoldExpirer            = (*booking.Service)(nil)
	_ booking.ExpiredRefunder        = (*gateway.Service)(nil)
	_ interface{ Jobs() []jobs.Job } = (*gateway.Service)(nil)
	_ interface{ Jobs() []jobs.Job } = (*notify.Service)(nil)
)

// wireOnline builds the M4 services (public page, online payments,
// notifications) on top of the M3 services. Construction order: online
// (settings, no dependencies on the others) -> notify -> gateway (ledger =
// payment.Service, confirmation through its BookingConfirmer) -> checkout.
func (a *App) wireOnline(aw *audit.Writer) {
	c := a.opts.clock
	base := a.cfg.EffectiveBaseURL()
	transport := notify.NewTransport(a.cfg.SMTPURL, a.cfg.MailDir, c, a.log)
	a.Online = online.NewService(online.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Keys: a.keys, Providers: a.registry, BaseURL: base,
		Transport: online.Transport{Kind: transport.Kind(), SMTPConfigured: a.cfg.SMTPURL != "", MailFrom: a.cfg.EffectiveMailFrom()},
	})
	a.Notify = notify.NewService(notify.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Keys: a.keys, Settings: a.Online, Transport: transport,
		MailFrom: a.cfg.EffectiveMailFrom(), BaseURL: base, Notifiers: a.registry.Notifiers,
	})
	a.Gateway = gateway.NewService(gateway.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Providers: a.registry, Settings: a.Online, Ledger: a.Payments, BaseURL: base,
	})
	// Hold expiry and payments meet in both directions: a payment arriving
	// after the hold elapsed expires the booking first (late-payment path),
	// an expired hold with money on it schedules the refund (INV-51).
	a.Gateway.SetHoldExpirer(a.Bookings)
	a.Bookings.SetExpiredRefunder(a.Gateway)
	a.Checkout = checkout.NewService(checkout.Deps{
		DB: a.db, Clock: c, Log: a.log,
		Settings: a.Online, Inventory: a.Inventory, Pricer: a.Pricing,
		Bookings: a.Bookings, Payments: a.Gateway, Notifier: a.Notify, Passes: a.Passes,
		HoldTTL: a.cfg.HoldTTL, BaseURL: base,
	})
}

// onlineJobs are the background jobs contributed by the M4 services.
func (a *App) onlineJobs() []jobs.Job {
	var out []jobs.Job
	out = append(out, a.Gateway.Jobs()...)
	out = append(out, a.Notify.Jobs()...)
	return out
}

// onlineSinks are the in-process event sinks of the M4 services (notify
// turns outbox events into messages).
func (a *App) onlineSinks() []ext.EventSink {
	return []ext.EventSink{a.Notify.Sink()}
}
