package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// JobReconcile is the name of the reconciliation job.
const JobReconcile = "payments.reconcile"

// Jobs returns the background jobs of the gateway: the reconciliation
// every ReconcileInterval (spec 13.3: 15 minutes).
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     JobReconcile,
		Interval: s.d.ReconcileInterval,
		Timeout:  5 * time.Minute,
		Run: func(ctx context.Context) error {
			_, err := s.Reconcile(ctx)
			return err
		},
	}}
}

// Reconcile is the job body: across establishments, payment intents still
// open after PendingAfter are looked up at the provider
// (PaymentStatusChecker) and their outcome applied through the webhook
// code path (apply, synthetic event id), so a missed webhook is fixed
// exactly once and a late real webhook changes nothing; refund intents
// never sent (provider down, crash) are sent again with their idempotency
// key and pending refunds are looked up. Per-intent lookups are exact and
// bounded (no listing or pagination of provider objects): every provider
// payment Arenile cares about has an intent row.
func (s *Service) Reconcile(ctx context.Context) (*ReconcileStats, error) {
	return s.reconcile(ctx, "", false)
}

// ReconcileEstablishment runs the reconciliation now for one establishment
// (auth.PermPaymentsConfigure), without the PendingAfter delay.
func (s *Service) ReconcileEstablishment(ctx context.Context, p *auth.Principal, establishmentID string) (*ReconcileStats, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermPaymentsConfigure)
	if err != nil {
		return nil, err
	}
	return s.reconcile(ctx, es.ID(), true)
}

func (s *Service) reconcile(ctx context.Context, establishmentID string, now bool) (*ReconcileStats, error) {
	st := &ReconcileStats{StartedAt: s.now()}
	cutoff := st.StartedAt.Add(-refundRetryAfter)
	if now {
		cutoff = st.StartedAt.Add(time.Second)
	}
	refs, err := store.ListReconcilableIntents(ctx, s.d.DB, establishmentID, cutoff, reconcileBatch)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	scopes := map[string]store.EstablishmentScope{}
	for _, ref := range refs {
		if ctx.Err() != nil {
			break
		}
		es, ok := scopes[ref.EstablishmentID]
		if !ok {
			if es, err = store.SystemEstablishmentScope(ctx, s.d.DB, ref.EstablishmentID); err != nil {
				st.Failed++
				continue
			}
			scopes[ref.EstablishmentID] = es
		}
		checked, changed, err := s.reconcileIntent(ctx, es, ref.ID, now)
		if checked {
			st.Checked++
		}
		switch {
		case err != nil:
			st.Failed++
			s.d.Log.WarnContext(ctx, "gateway: reconcile intent", "establishment_id", es.ID(), "intent_id", ref.ID, "err", err)
		case changed:
			st.Updated++
		}
	}
	st.FinishedAt = s.now()
	return st, nil
}

// reconcileIntent settles one intent. checked reports whether it was due.
func (s *Service) reconcileIntent(ctx context.Context, es store.EstablishmentScope, id string, force bool) (checked, changed bool, err error) {
	pi, err := store.GetPaymentIntent(ctx, s.d.DB, es, id)
	if err != nil {
		return false, false, fmt.Errorf("gateway: %w", err)
	}
	if !isOpen(pi.Status) {
		return false, false, nil
	}
	if pi.Kind == store.IntentKindRefund && pi.Status == store.IntentCreated {
		if err := s.executeRefund(ctx, es, pi.ID); err != nil {
			return true, false, err
		}
		after, err := store.GetPaymentIntent(ctx, s.d.DB, es, pi.ID)
		if err != nil {
			return true, false, fmt.Errorf("gateway: %w", err)
		}
		return true, after.Status != pi.Status, nil
	}
	if !force && pi.Kind == store.IntentKindPayment && pi.CreatedAt.After(s.now().Add(-s.d.PendingAfter)) {
		return false, false, nil
	}
	ev, cfg, err := s.lookup(ctx, es, pi)
	if err != nil || ev == nil {
		// Checked: rotate to the end of the queue (ListReconcilableIntents
		// orders by last check), so failing intents never starve others.
		now := s.now()
		pi.LastCheckedAt, pi.UpdatedAt = &now, now
		if uerr := store.UpdatePaymentIntent(ctx, s.d.DB, es, pi, "last_checked_at"); uerr != nil && err == nil {
			err = fmt.Errorf("gateway: %w", uerr)
		}
		return true, false, err
	}
	if ev.ProviderRef == "" {
		ev.ProviderRef = pi.ExternalID
	}
	ev.ProviderEventID = syntheticEventID(pi.ExternalID, ev)
	out, err := s.apply(ctx, es, pi.Provider, cfg, ev, sourceReconcile)
	if err != nil {
		return true, false, err
	}
	s.executeRefunds(ctx, es, out.refunds)
	return true, out.changed, nil
}

// errNoStatusCheck: the driver cannot look payments up (report:
// provider_unreachable).
var errNoStatusCheck = errors.New("gateway: provider cannot look up payments")

// lookup asks the provider for the current outcome of an intent.
func (s *Service) lookup(ctx context.Context, es store.EstablishmentScope, pi *store.PaymentIntent) (*ext.PaymentEvent, *online.ProviderConfig, error) {
	cfg, pp, err := s.provider(ctx, s.d.DB, es, pi.Provider)
	if err != nil {
		return nil, nil, err
	}
	checker, ok := pp.(PaymentStatusChecker)
	if !ok {
		return nil, cfg, errNoStatusCheck
	}
	if isPlaceholder(pi.ExternalID) {
		return nil, cfg, nil
	}
	ev, err := checker.PaymentStatus(ctx, pi.ExternalID, cfg.Credentials)
	if err != nil {
		return nil, cfg, providerError("payment status", err)
	}
	if ev != nil && ev.Metadata == nil {
		ev.Metadata = map[string]string{}
	}
	return ev, cfg, nil
}

// Report builds the reconciliation report of es (auth.PermBookingsRead):
// the intents created in [from, to] (dates in the establishment timezone,
// default the last 30 days), each with the first issue found:
//
//	missing_ledger_row   succeeded at the provider, no ledger row
//	amount_mismatch      ledger amount differs from the provider amount
//	pending_too_long     still open after PendingAfter
//	status_mismatch      the provider reports an outcome not applied yet
//	provider_unreachable the provider could not be asked
//
// Open intents past PendingAfter are looked up live (at most
// reportLiveChecks per report); fixing is RunReconciliation's job.
func (s *Service) Report(ctx context.Context, p *auth.Principal, establishmentID string, q ReportQuery) (*Report, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := s.now()
	today := calendar.In(now, loc)
	from, to := today.AddDays(-30), today
	if q.From != nil {
		from = *q.From
	}
	if q.To != nil {
		to = *q.To
	}
	if to.Before(from) {
		return nil, configerr.Invalid("to", "La data finale non può precedere quella iniziale.")
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	rows, err := store.ListPaymentIntents(ctx, s.d.DB, es, start, end, reportLimit)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	rep := &Report{GeneratedAt: now, From: &from, To: &to, Checked: len(rows), Items: []ReportItem{}}
	live := 0
	for i := range rows {
		r := &rows[i]
		item := ReportItem{Intent: r.PaymentIntent, BookingCode: deref(r.BookingCode), Issue: IssueNone}
		switch {
		case r.Status == store.IntentSucceeded && r.LedgerPayment == nil:
			item.Issue = IssueMissingLedgerRow
		case r.Status == store.IntentSucceeded && r.Kind == store.IntentKindPayment && r.LedgerAmount != nil && *r.LedgerAmount != r.Amount:
			item.Issue = IssueAmountMismatch
		case isOpen(r.Status) && r.CreatedAt.Before(now.Add(-s.d.PendingAfter)):
			item.Issue = IssuePendingTooLong
			if live < reportLiveChecks {
				live++
				ev, _, err := s.lookup(ctx, es, &r.PaymentIntent)
				switch {
				case err != nil:
					item.Issue, item.ProviderStatus = IssueProviderUnreachable, "unknown"
				case ev == nil:
					item.ProviderStatus = store.IntentPending
				default:
					item.Issue, item.ProviderStatus = IssueStatusMismatch, providerStatus(ev)
				}
			}
		}
		if item.Issue != IssueNone {
			rep.Issues++
		} else if q.IssuesOnly {
			continue
		}
		rep.Items = append(rep.Items, item)
	}
	return rep, nil
}

// providerStatus names the outcome of a provider event as an intent status.
func providerStatus(ev *ext.PaymentEvent) string {
	switch ev.Type {
	case ext.PaymentSucceeded, ext.PaymentRefunded:
		return store.IntentSucceeded
	case ext.PaymentFailed:
		if ev.Metadata[MetaOutcome] == OutcomeExpired {
			return store.IntentExpired
		}
		return store.IntentFailed
	default:
		return store.IntentPending
	}
}
