package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// TestDriverContract_SharedKeys: the metadata keys and outcomes the
// gateway reads are the ones the core Stripe driver writes.
func TestDriverContract_SharedKeys(t *testing.T) {
	for _, tc := range [][2]string{
		{MetaLocale, stripe.HintLocale},
		{MetaExpiresAt, stripe.HintExpiresAt},
		{MetaOutcome, stripe.MetaOutcome},
		{MetaPaymentIntent, stripe.MetaPaymentIntent},
		{MetaAmountCaptured, stripe.MetaAmountCaptured},
		{OutcomeExpired, stripe.OutcomeExpired},
		{OutcomeAttemptFailed, stripe.OutcomeAttemptFailed},
		{OutcomeRefundFailed, stripe.OutcomeRefundFailed},
		{OutcomeRefundTotal, stripe.OutcomeRefundTotal},
		{OutcomeRefundSingle, stripe.OutcomeRefundSingle},
	} {
		if tc[0] != tc[1] {
			t.Errorf("gateway %q != stripe %q", tc[0], tc[1])
		}
	}
	var _ PaymentCanceller = (*stripe.Driver)(nil)
	var _ PaymentStatusChecker = (*stripe.Driver)(nil)
	var _ CredentialsVerifier = (*stripe.Driver)(nil)
}

// TestProviderError_Classification: temporary driver errors are 503,
// refusals 409 with the provider code only.
func TestProviderError_Classification(t *testing.T) {
	if err := providerError("x", fmt.Errorf("wrap: %w", stripe.ErrUnavailable)); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("temporary: %v", err)
	}
	if err := providerError("x", context.DeadlineExceeded); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("timeout: %v", err)
	}
	err := providerError("x", &stripe.APIError{Status: 402, Type: "card_error", Code: "card_declined"})
	if !errors.Is(err, ErrProviderRefused) || !errors.Is(err, configerr.ErrConflict) || err.Error() != ErrProviderRefused.Error()+" (card_declined)" {
		t.Fatalf("refused: %v", err)
	}
	if !isIdempotencyConflict(fmt.Errorf("w: %w", stripe.ErrIdempotencyConflict)) || isIdempotencyConflict(errors.New("x")) {
		t.Fatal("idempotency conflict detection")
	}
}

// TestSyntheticEventID_PerOutcome: one id per reference and outcome; a
// cumulative refund id changes with the amount.
func TestSyntheticEventID_PerOutcome(t *testing.T) {
	ok := &ext.PaymentEvent{Type: ext.PaymentSucceeded, Metadata: map[string]string{}}
	exp := &ext.PaymentEvent{Type: ext.PaymentFailed, Metadata: map[string]string{MetaOutcome: OutcomeExpired}}
	r1 := &ext.PaymentEvent{Type: ext.PaymentRefunded, Amount: 100, Metadata: map[string]string{MetaOutcome: OutcomeRefundTotal}}
	r2 := &ext.PaymentEvent{Type: ext.PaymentRefunded, Amount: 200, Metadata: map[string]string{MetaOutcome: OutcomeRefundTotal}}
	ids := map[string]bool{}
	for _, ev := range []*ext.PaymentEvent{ok, exp, r1, r2} {
		ids[syntheticEventID("cs_1", ev)] = true
	}
	if len(ids) != 4 || syntheticEventID("cs_1", ok) != "reconcile:cs_1:payment.succeeded" {
		t.Fatalf("ids: %v", ids)
	}
}

// TestAppendEvent_BoundedAndUnique.
func TestAppendEvent_BoundedAndUnique(t *testing.T) {
	pi := &store.PaymentIntent{}
	for i := 0; i < maxRawEventIDs+10; i++ {
		appendEvent(pi, fmt.Sprintf("evt_%d", i))
		appendEvent(pi, fmt.Sprintf("evt_%d", i))
	}
	if len(pi.RawEventIDs) != maxRawEventIDs || pi.RawEventIDs[0] != "evt_10" {
		t.Fatalf("raw ids: %d %s", len(pi.RawEventIDs), pi.RawEventIDs[0])
	}
}
