package cash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Sentinel errors of the cash domain (mapped by internal/http/problem).
var (
	// ErrSessionClosed -> 409 cash_session_closed.
	ErrSessionClosed = errors.New("cash: session closed")
	// ErrSessionAlreadyOpen -> 409 cash_session_already_open.
	ErrSessionAlreadyOpen = errors.New("cash: a session is already open on this register")
	// ErrNotFound: unknown session (404).
	ErrNotFound = fmt.Errorf("%w: cash session", configerr.ErrNotFound)
)

// Deps are the dependencies of the cash service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements the cash desk use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// SessionForPayment implements payment.CashDesk.
func (s *Service) SessionForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, sessionID *string, method string) (*string, error) {
	return nil, fmt.Errorf("cash: session for payment: %w", configerr.ErrNotImplemented)
}
