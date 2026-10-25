package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// ResetScope is what Reset deletes.
type ResetScope string

const (
	// ResetConfiguration deletes the configuration only (seasons, maps,
	// zones, catalog, prices, policies): the setup wizard starts over.
	// Refused while the establishment has operational data.
	ResetConfiguration ResetScope = "configuration"
	// ResetEverything also deletes the operational data (bookings, passes,
	// customers, payments, cash sessions, tickets...).
	ResetEverything ResetScope = "everything"
)

// ErrHasOperations refuses ResetConfiguration on an establishment with
// operational data (409 conflict).
var ErrHasOperations = errors.New("setup: establishment has operational data")

// ErrPaymentsInFlight refuses ResetEverything while an online payment or a
// refund is still in progress at the provider (409 conflict): deleting it
// would lose money owed to the customer (INV-51, INV-75).
var ErrPaymentsInFlight = errors.New("setup: online payments or refunds in progress")

// ErrHasFiscalRecords refuses ResetEverything when the establishment has
// receipts issued by a real registratore telematico (409 conflict): fiscal
// records must be kept, a reset would delete them.
var ErrHasFiscalRecords = errors.New("setup: establishment has issued fiscal documents")

// ResetInput is a reset request: the scope and the establishment name typed
// by the owner to confirm.
type ResetInput struct {
	Scope       ResetScope
	ConfirmName string
	IP          string
}

// ResetResult reports the deleted rows per table.
type ResetResult struct {
	Scope   ResetScope
	Deleted map[string]int64
}

// Reset deletes the configuration of the establishment, and with
// ResetEverything its operational data too, in one transaction (store.
// ResetEstablishment), audited as "setup.reset". Only the owner may do it,
// signed in as a person: the caller needs the owner-only permissions
// users.manage and payments.configure besides establishment.configure, and
// API keys and devices are refused. The name must match the establishment's
// (trimmed, case-insensitive). ResetConfiguration with operational data is
// a conflict: the caller must choose ResetEverything. ResetEverything is a
// conflict while payments are in flight or real fiscal documents exist.
func (s *Service) Reset(ctx context.Context, p *auth.Principal, establishmentID string, in ResetInput) (*ResetResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if p.Kind != auth.PrincipalUser || p.DeviceID != "" || !p.Can(es, auth.PermUsersManage) || !p.Can(es, auth.PermPaymentsConfigure) {
		return nil, auth.ErrForbidden
	}
	if in.Scope != ResetConfiguration && in.Scope != ResetEverything {
		return nil, configerr.Invalid("scope", "Valore non valido (configuration, everything).")
	}
	res := &ResetResult{Scope: in.Scope}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		e, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(in.ConfirmName), strings.TrimSpace(e.Name)) {
			return configerr.Invalid("confirm_name", "Scrivi il nome dello stabilimento esattamente come appare.")
		}
		if in.Scope == ResetEverything {
			moving, err := store.HasPaymentsInFlight(ctx, tx, es)
			if err != nil {
				return err
			}
			if moving {
				return ErrPaymentsInFlight
			}
			fiscal, err := store.HasFiscalRecords(ctx, tx, es)
			if err != nil {
				return err
			}
			if fiscal {
				return ErrHasFiscalRecords
			}
		}
		if in.Scope == ResetConfiguration {
			busy, err := store.HasOperations(ctx, tx, es)
			if err != nil {
				return err
			}
			if busy {
				return ErrHasOperations
			}
		}
		res.Deleted, err = store.ResetEstablishment(ctx, tx, es, in.Scope == ResetEverything, s.d.Clock.Now())
		if err != nil {
			return err
		}
		// The map and the price list are gone: one config event of each kind
		// (ADR 0008, INV-39) for the panels and the webhook subscribers.
		change := event.ConfigChange{
			Entity: event.EntitySetup, EntityID: es.ID(), Change: event.ChangeDeleted, Reason: "setup_reset",
		}
		for _, typ := range []string{event.ConfigMapChanged, event.ConfigPricingChanged} {
			if _, err := s.d.Outbox.Write(ctx, tx, typ, es.ID(), 1, change); err != nil {
				return err
			}
		}
		diff := map[string]any{"scope": string(in.Scope), "booking_enabled": false}
		for t, n := range res.Deleted {
			diff[t] = n
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: "setup.reset",
			EntityType: "establishment", EntityID: es.ID(), Diff: diff,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("setup: reset: %w", err)
	}
	return res, nil
}
