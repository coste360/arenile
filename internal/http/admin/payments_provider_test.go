package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// TestRefundIntentToAPI_Statuses: provider refunds map to api.Payment with
// the ledger id once confirmed and never expose the placeholder reference.
func TestRefundIntentToAPI_Statuses(t *testing.T) {
	ledger, booking := "pm-9", "bk-1"
	for _, tc := range []struct {
		status, external string
		paymentID        *string
		wantID           string
		wantStatus       api.PaymentStatus
		wantRef          bool
	}{
		{store.IntentCreated, "pending:ri-1", nil, "ri-1", api.PaymentStatusPending, false},
		{store.IntentPending, "re_1", nil, "ri-1", api.PaymentStatusPending, true},
		{store.IntentSucceeded, "re_1", &ledger, "pm-9", api.PaymentStatusSucceeded, true},
		{store.IntentFailed, "re_1", nil, "ri-1", api.PaymentStatusFailed, true},
		{store.IntentCancelled, "re_1", nil, "ri-1", api.PaymentStatusCancelled, true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			ri := &store.PaymentIntent{
				ID: "ri-1", BookingID: &booking, PaymentID: tc.paymentID, Provider: "stripe", Kind: store.IntentKindRefund,
				ExternalID: tc.external, Status: tc.status, Amount: 1250, CreatedAt: time.Unix(0, 0),
			}
			got := RefundIntentToAPI(ri, "pm-1")
			if got.Id != tc.wantID || got.Status != tc.wantStatus || (got.ProviderRef != nil) != tc.wantRef ||
				got.Direction != "refund" || got.Amount != 1250 || *got.RefundOfPaymentId != "pm-1" {
				t.Fatalf("%+v", got)
			}
		})
	}
}

// TestPaymentSettingsToAPI_NoSecrets: the response carries flags, last4
// and the publishable key only.
func TestPaymentSettingsToAPI_NoSecrets(t *testing.T) {
	ps := &online.PaymentSettings{
		Provider: "stripe", Mode: "live", WebhookURL: "https://x/webhooks/payments/stripe/e", Version: 3,
		Stripe: online.StripeView{Configured: true, PublishableKey: "pk_live_1", SecretKeyLast4: "4242", WebhookSecretConfigured: true},
	}
	b, _ := json.Marshal(PaymentSettingsToAPI(ps))
	s := string(b)
	if strings.Contains(s, "secret_key\"") || strings.Contains(s, "webhook_secret\"") || !strings.Contains(s, `"secret_key_last4":"4242"`) || !strings.Contains(s, `"available_providers":[]`) {
		t.Fatalf("payload: %s", s)
	}
}

// TestReconciliationReportToAPI_Maps.
func TestReconciliationReportToAPI_Maps(t *testing.T) {
	d := calendar.MustParse("2027-07-01")
	rep := &gateway.Report{GeneratedAt: time.Unix(10, 0), From: &d, To: &d, Checked: 1, Issues: 1, Items: []gateway.ReportItem{{
		Intent:      store.PaymentIntent{ID: "pi-1", Provider: "stripe", ExternalID: "cs_1", Kind: store.IntentKindPayment, Status: store.IntentPending, Amount: 500},
		BookingCode: "LD-1", ProviderStatus: "succeeded", Issue: gateway.IssueStatusMismatch,
	}}}
	out := ReconciliationReportToAPI(rep)
	it := out.Items[0]
	if out.Checked != 1 || it.Issue != api.ReconciliationIssueStatusMismatch || *it.BookingCode != "LD-1" || *it.ProviderStatus != "succeeded" ||
		it.LocalStatus != api.ReconciliationItemLocalStatusPending || it.Kind != api.ReconciliationItemKindPayment || it.LastCheckedAt != nil {
		t.Fatalf("%+v", out)
	}
}
