package customer

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// CreditLedger is the M5B credit ledger (implemented by *credit.Service):
// every movement updates customer_credit_balances with a conditional
// UPDATE, expiry is materialised and the balance can never go negative.
// When set with UseCreditLedger, GrantCredit and CreditBalance of this
// service delegate to it, so the payment and pass services (which use this
// service as their Credits port) go through the ledger without rewiring.
type CreditLedger interface {
	GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error
	CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error)
}

// UseCreditLedger makes GrantCredit and CreditBalance delegate to l. Call
// it while wiring, before the service is used.
func (s *Service) UseCreditLedger(l CreditLedger) { s.ledger = l }
