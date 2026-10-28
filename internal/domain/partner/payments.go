package partner

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Partner payment methods (partner_payments.method).
var paymentMethods = map[string]bool{"bank_transfer": true, "cash": true, "card": true, "other": true}

// MaxPartnerPayment bounds one recorded payment (sanity check, 1 million euro).
const MaxPartnerPayment = money.Cents(100_000_000)

// PaymentInput is api.PartnerPaymentInput.
type PaymentInput struct {
	Amount     money.Cents
	ReceivedOn calendar.Date
	Method     string
	Reference  string
	Note       string
}

func validatePayment(in *PaymentInput) error {
	v := &configerr.ValidationError{}
	if in.Amount <= 0 || in.Amount > MaxPartnerPayment {
		v.Add("amount", "Importo maggiore di zero (in centesimi).")
	}
	if in.ReceivedOn.IsZero() {
		v.Add("received_on", "Campo obbligatorio.")
	}
	if !paymentMethods[in.Method] {
		v.Add("method", "Metodo sconosciuto.")
	}
	in.Reference, in.Note = strings.TrimSpace(in.Reference), strings.TrimSpace(in.Note)
	if len([]rune(in.Reference)) > maxShortLen {
		v.Add("reference", fmt.Sprintf("Massimo %d caratteri.", maxShortLen))
	}
	if len([]rune(in.Note)) > maxNotesLen {
		v.Add("note", fmt.Sprintf("Massimo %d caratteri.", maxNotesLen))
	}
	return v.OrNil()
}

// ListPayments pages the payments of a partner (reports.read).
func (s *Service) ListPayments(ctx context.Context, p *auth.Principal, establishmentID, partnerID, cursor string, limit int) (store.Page[store.PartnerPayment], error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermReportsRead)
	if err != nil {
		return store.Page[store.PartnerPayment]{}, err
	}
	if _, err := store.GetPartner(ctx, s.d.DB, es, partnerID); err != nil {
		return store.Page[store.PartnerPayment]{}, notFound(err)
	}
	page, err := store.ListPartnerPayments(ctx, s.d.DB, es, partnerID, cursor, limit)
	if err != nil {
		return page, fmt.Errorf("partner: %w", err)
	}
	return page, nil
}

// PaymentsBetween lists the payments of a partner received in [from, to]
// (voided included) for the statement (reports.read is checked by the
// report service; this is an in-scope read).
func (s *Service) PaymentsBetween(ctx context.Context, p *auth.Principal, establishmentID, partnerID string, from, to calendar.Date) ([]store.PartnerPayment, error) {
	es, err := s.authorize(ctx, s.d.DB, p, establishmentID, auth.PermReportsRead)
	if err != nil {
		return nil, err
	}
	rows, err := store.PartnerPaymentsBetween(ctx, s.d.DB, es, []string{partnerID}, from, to)
	if err != nil {
		return nil, fmt.Errorf("partner: %w", err)
	}
	return rows, nil
}

// RecordPayment records a payment received from a partner (cash.operate).
// The invoice stays outside Arenile (spec 14).
func (s *Service) RecordPayment(ctx context.Context, p *auth.Principal, establishmentID, partnerID string, in PaymentInput) (*store.PartnerPayment, error) {
	var out *store.PartnerPayment
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermCashOperate)
		if err != nil {
			return err
		}
		if _, err := store.GetPartner(ctx, tx, es, partnerID); err != nil {
			return notFound(err)
		}
		if err := validatePayment(&in); err != nil {
			return err
		}
		now := s.now()
		row := &store.PartnerPayment{
			ID: store.NewID(), PartnerID: partnerID, Amount: in.Amount, ReceivedOn: in.ReceivedOn, Method: in.Method,
			Reference: in.Reference, Note: in.Note, RecordedByUserID: store.ExistingUser(ctx, tx, userOf(p)), CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertPartnerPayment(ctx, tx, es, row); err != nil {
			return fmt.Errorf("partner: record payment: %w", err)
		}
		out = row
		return s.record(ctx, tx, p, es, EntityPartnerPayment, row.ID, "created", map[string]any{
			"partner_id": partnerID, "amount": int64(row.Amount), "received_on": row.ReceivedOn.String(), "method": row.Method,
		}, false)
	})
	return out, err
}

// VoidPayment voids a payment recorded by mistake (cash.operate): the row
// stays with reason and operator and leaves the balance.
func (s *Service) VoidPayment(ctx context.Context, p *auth.Principal, establishmentID, partnerID, paymentID, reason string) (*store.PartnerPayment, error) {
	var out *store.PartnerPayment
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := s.authorize(ctx, tx, p, establishmentID, auth.PermCashOperate)
		if err != nil {
			return err
		}
		reason = strings.TrimSpace(reason)
		if reason == "" || len([]rune(reason)) > maxShortLen {
			return configerr.Invalid("reason", fmt.Sprintf("Motivo obbligatorio (massimo %d caratteri).", maxShortLen))
		}
		cur, err := store.GetPartnerPayment(ctx, tx, es, partnerID, paymentID)
		if err != nil {
			return notFound(err)
		}
		if cur.VoidedAt != nil {
			return ErrPaymentVoided
		}
		now := s.now()
		ok, err := store.VoidPartnerPayment(ctx, tx, es, paymentID, reason, store.ExistingUser(ctx, tx, userOf(p)), now)
		if err != nil {
			return fmt.Errorf("partner: void payment: %w", err)
		}
		if !ok {
			return ErrPaymentVoided
		}
		if out, err = store.GetPartnerPayment(ctx, tx, es, partnerID, paymentID); err != nil {
			return notFound(err)
		}
		return s.record(ctx, tx, p, es, EntityPartnerPayment, paymentID, "voided", map[string]any{
			"partner_id": partnerID, "amount": int64(cur.Amount), "reason": reason,
		}, false)
	})
	return out, err
}
