// Package builtin contains the extension implementations shipped with the
// core: the "manual" payment provider and the "none" fiscal driver.
package builtin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/pkg/ext"
)

// ManualCode is the code of the manual payment provider.
const ManualCode = "manual"

// ErrInvalidAmount is returned for non-positive payment amounts.
var ErrInvalidAmount = errors.New("builtin: amount must be positive")

// Manual is the payment provider for establishments that collect payments
// at the desk (spec 13.2): no online step, the booking is confirmed (or sent
// for approval) and the balance is collected at the cash desk.
type Manual struct{}

var _ ext.PaymentProvider = Manual{}

// Code implements ext.PaymentProvider.
func (Manual) Code() string { return ManualCode }

// Capabilities implements ext.PaymentProvider.
func (Manual) Capabilities() ext.PaymentCapabilities { return ext.PaymentCapabilities{} }

// CreatePayment returns a "none" mode result: nothing to do online.
func (Manual) CreatePayment(_ context.Context, req ext.CreatePaymentRequest) (*ext.CreatePaymentResult, error) {
	if req.Amount <= 0 {
		return nil, ErrInvalidAmount
	}
	return &ext.CreatePaymentResult{ProviderRef: "manual_" + uuid.Must(uuid.NewV7()).String(), Mode: ext.PaymentModeNone}, nil
}

// ParseWebhook implements ext.PaymentProvider: manual payments have no webhooks.
func (Manual) ParseWebhook(context.Context, *http.Request, ext.Credentials) (*ext.PaymentEvent, error) {
	return nil, ext.ErrNotSupported
}

// Refund implements ext.PaymentProvider: refunds are recorded as manual
// refunds to be completed by the operator.
func (Manual) Refund(context.Context, ext.RefundRequest) (*ext.RefundResult, error) {
	return nil, ext.ErrNotSupported
}

// NoneCode is the code of the no-op fiscal driver.
const NoneCode = "none"

// NoneFiscal is the default fiscal driver (spec 14.2): Arenile does not
// issue fiscal documents; the operator uses their own registered cash
// register. Issue returns a tracking reference without a document number.
type NoneFiscal struct {
	Clock clock.Clock
}

var _ ext.FiscalDriver = NoneFiscal{}

// Code implements ext.FiscalDriver.
func (NoneFiscal) Code() string { return NoneCode }

// Issue implements ext.FiscalDriver.
func (n NoneFiscal) Issue(_ context.Context, _ ext.FiscalDocumentRequest) (*ext.FiscalDocumentResult, error) {
	return &ext.FiscalDocumentResult{Ref: "none_" + uuid.Must(uuid.NewV7()).String(), IssuedAt: n.now()}, nil
}

// Void implements ext.FiscalDriver.
func (NoneFiscal) Void(context.Context, string) error { return nil }

// Status implements ext.FiscalDriver.
func (n NoneFiscal) Status(context.Context) (ext.DeviceStatus, error) {
	return ext.DeviceStatus{Online: true, Message: "Nessun dispositivo fiscale collegato: l'emissione è a cura del gestore.", CheckedAt: n.now()}, nil
}

func (n NoneFiscal) now() time.Time {
	if n.Clock == nil {
		return time.Now().UTC()
	}
	return n.Clock.Now()
}
