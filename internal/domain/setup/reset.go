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
// a conflict: the caller must choose ResetEverything.
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
		if in.Scope == ResetConfiguration {
			busy, err := store.HasOperations(ctx, tx, es)
			if err != nil {
				return err
			}
			if busy {
				return ErrHasOperations
			}
		}
		res.Deleted, err = store.ResetEstablishment(ctx, tx, es, in.Scope == ResetEverything)
		if err != nil {
			return err
		}
		diff := map[string]any{"scope": string(in.Scope)}
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
