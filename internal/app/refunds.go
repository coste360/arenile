package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/refund"
	"github.com/coste360/arenile/internal/jobs"
)

// Compile-time checks of the guaranteed refund ports (ADR 0019 feature B).
var (
	_ booking.CancelRefunds          = (*refund.Service)(nil)
	_ checkout.RefundSender          = (*refund.Service)(nil)
	_ refund.Sender                  = (*gateway.Service)(nil)
	_ interface{ Jobs() []jobs.Job } = (*refund.Service)(nil)
)

// wireRefunds builds the refund service on top of the gateway (sender)
// and plugs it into the cancellations of the booking service and the
// manage area of the checkout. Runs after wireOnline.
func (a *App) wireRefunds(aw *audit.Writer) {
	a.Refunds = refund.NewService(refund.Deps{
		DB: a.db, Clock: a.opts.clock, Audit: aw, Log: a.log, Sender: a.Gateway, MaxAttempts: a.cfg.RefundMaxAttempts,
	})
	a.Bookings.SetCancelRefunds(a.Refunds)
	a.Checkout.SetRefunds(a.Refunds)
}

// refundJobs are the background jobs of the refund service.
func (a *App) refundJobs() []jobs.Job { return a.Refunds.Jobs() }
