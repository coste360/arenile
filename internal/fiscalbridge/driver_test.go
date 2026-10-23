package fiscalbridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/pkg/ext"
)

type fakeQueue struct {
	res  *Result
	done bool
	kind JobKind
	key  string
	est  string
}

func (f *fakeQueue) Enqueue(_ context.Context, est string, kind JobKind, key string, _ any) (string, *Result, bool, error) {
	f.est, f.kind, f.key = est, kind, key
	return "job-1", f.res, f.done, nil
}

func (f *fakeQueue) BridgeStatus(_ context.Context, est string) (DeviceStatus, error) {
	f.est = est
	return DeviceStatus{Online: true, Message: "ready"}, nil
}

// TestDriver_VoidAndStatus: Void and Status take the establishment from the
// context (fiscalbridge.WithEstablishment, wired by the app); without it
// they fail closed instead of touching another establishment's queue.
func TestDriver_VoidAndStatus(t *testing.T) {
	q := &fakeQueue{}
	d := &Driver{Queue: q, EstablishmentFromContext: EstablishmentFromContext}
	bg := context.Background()
	if err := d.Void(bg, "r1"); !errors.Is(err, ErrNoEstablishment) {
		t.Fatalf("void without establishment: %v", err)
	}
	if _, err := d.Status(bg); !errors.Is(err, ErrNoEstablishment) {
		t.Fatalf("status without establishment: %v", err)
	}
	ctx := WithEstablishment(bg, "e1")
	var de interface{ Deferred() bool }
	if err := d.Void(ctx, "r1"); !errors.As(err, &de) || q.est != "e1" || q.kind != KindVoid || q.key != "void:r1" {
		t.Fatalf("void must enqueue a deferred void job: %v %+v", err, q)
	}
	if err := d.Void(ctx, ""); err == nil || errors.As(err, &de) {
		t.Fatalf("void without ref: %v", err)
	}
	q.res, q.done = &Result{Outcome: OutcomeSucceeded, Ref: "r1"}, true
	if err := d.Void(ctx, "r1"); err != nil {
		t.Fatalf("void done: %v", err)
	}
	st, err := d.Status(ctx)
	if err != nil || !st.Online || st.Message != "ready" || q.est != "e1" {
		t.Fatalf("status %+v %v", st, err)
	}
	if _, err := (&Driver{Queue: q}).Status(ctx); !errors.Is(err, ErrNoEstablishment) {
		t.Fatalf("driver without resolver must fail closed: %v", err)
	}
}

func TestDriver_Issue(t *testing.T) {
	req := ext.FiscalDocumentRequest{
		EstablishmentID: "e1", PaymentID: "p1", Currency: "EUR", Total: 1220, IdempotencyKey: "k1",
		Lines:    []ext.FiscalLine{{Description: "Ombrellone", Quantity: 1, UnitGross: 1220, Gross: 1220, VATRateBP: 2200}},
		Payments: []ext.FiscalPayment{{Method: "cash", Amount: 1220}},
	}
	q := &fakeQueue{}
	d := &Driver{Queue: q}
	_, err := d.Issue(context.Background(), req)
	var de interface{ Deferred() bool }
	if !errors.As(err, &de) || !de.Deferred() || q.kind != KindIssue || q.key != "k1" {
		t.Fatalf("pending job must defer, err = %v", err)
	}
	now := time.Date(2027, 7, 1, 10, 0, 0, 0, time.UTC)
	q.res, q.done = &Result{Outcome: OutcomeSucceeded, Ref: "r1", Number: "0001-0001", IssuedAt: &now}, true
	res, err := d.Issue(context.Background(), req)
	if err != nil || res.Number != "0001-0001" || res.Ref != "r1" {
		t.Fatalf("res %+v err %v", res, err)
	}
	q.res = &Result{Outcome: OutcomeFailed, Error: "carta finita"}
	if _, err := d.Issue(context.Background(), req); err == nil || errors.As(err, &de) {
		t.Fatalf("failed outcome must be a plain error, got %v", err)
	}
	if _, err := d.Issue(context.Background(), ext.FiscalDocumentRequest{EstablishmentID: "e1"}); err == nil {
		t.Fatal("missing idempotency key must fail")
	}
}

func TestDocument_RoundTrip(t *testing.T) {
	req := ext.FiscalDocumentRequest{
		EstablishmentID: "e1", BookingID: "b1", Currency: "EUR", Total: -610, IdempotencyKey: "k",
		IssuedAt: time.Date(2027, 7, 1, 12, 0, 0, 0, time.UTC),
		Lines:    []ext.FiscalLine{{Description: "Reso", Quantity: 1, UnitGross: -610, Gross: -610, VATRateBP: 2200}},
		Payments: []ext.FiscalPayment{{Method: "cash", Amount: -610}},
	}
	got := DocumentFromRequest(req).Request()
	if got.Total != req.Total || got.Lines[0] != req.Lines[0] || got.Payments[0] != req.Payments[0] || !got.IssuedAt.Equal(req.IssuedAt) {
		t.Fatalf("roundtrip %+v", got)
	}
}
