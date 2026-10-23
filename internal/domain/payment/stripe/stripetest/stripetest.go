// Package stripetest is an in-memory fake of the Stripe REST API endpoints
// used by the core Stripe driver, served by an httptest.Server, plus
// builders of signed webhook deliveries. Tests of the driver, the payment
// gateway, the webhook endpoint and the public flow use it: no network, and
// webhook signatures are real (HMAC-SHA256 with a test secret, verified by
// the driver through the official SDK).
//
// Supported: Checkout Sessions (create, retrieve, expire), PaymentIntents
// (create, retrieve, cancel), Refunds (create, retrieve), Account
// (retrieve). Idempotency keys behave like Stripe: the same key with the
// same parameters replays the first answer, with different parameters it
// is a 400 idempotency_error.
package stripetest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	stripego "github.com/stripe/stripe-go/v87"
)

// Test credentials accepted by the fake (not real keys).
const (
	SecretKey      = "sk_test_arenile_fake_0000" //nolint:gosec // fake test credential
	WebhookSecret  = "whsec_arenile_test_secret" //nolint:gosec // fake test credential
	PublishableKey = "pk_test_arenile_fake_0000"
)

// Session is a fake Checkout Session.
type Session struct {
	ID, PaymentIntent, Status, PaymentStatus, Currency, Locale, CustomerEmail string
	ClientReferenceID, SuccessURL, CancelURL                                  string
	Amount, ExpiresAt, Created                                                int64
	Metadata                                                                  map[string]string
}

// PaymentIntent is a fake PaymentIntent.
type PaymentIntent struct {
	ID, Status, Currency string
	Amount, Received     int64
	Refunded             int64
	Metadata             map[string]string
}

// Refund is a fake Refund.
type Refund struct {
	ID, PaymentIntent, Status string
	Amount                    int64
	Metadata                  map[string]string
}

// Request is a request received by the fake (form already parsed).
type Request struct {
	Method, Path, IdempotencyKey, Authorization, StripeVersion string
	Form                                                       url.Values
}

// Server is the fake Stripe API.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	seq      int
	sessions map[string]*Session
	intents  map[string]*PaymentIntent
	refunds  map[string]*Refund
	idem     map[string]idemEntry
	requests []Request
	failNext int
	// RefundStatus is the status of new refunds ("succeeded" by default,
	// "pending" to exercise the asynchronous confirmation).
	RefundStatus string
	// Now is the clock of created/expires defaults (time.Now when nil).
	Now func() time.Time
	// checkoutBase is the base of the session URLs: the fake's own hosted
	// page in standalone mode (Start), a non-resolvable host otherwise.
	checkoutBase string
}

type idemEntry struct {
	params string
	status int
	body   []byte
}

// New starts a fake server closed at the end of the test.
func New(t testing.TB) *Server {
	s := &Server{
		sessions: map[string]*Session{}, intents: map[string]*PaymentIntent{}, refunds: map[string]*Refund{},
		idem: map[string]idemEntry{}, RefundStatus: "succeeded",
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// FailNext makes the next n requests answer 500 api_error.
func (s *Server) FailNext(n int) {
	s.mu.Lock()
	s.failNext = n
	s.mu.Unlock()
}

// Requests returns the requests received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Session returns a copy of a session (nil when unknown).
func (s *Server) Session(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cs, ok := s.sessions[id]; ok {
		c := *cs
		return &c
	}
	return nil
}

// RefundsOf returns the refunds of a payment intent.
func (s *Server) RefundsOf(pi string) []Refund {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Refund
	for _, r := range s.refunds {
		if r.PaymentIntent == pi {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Pay completes a Checkout Session (customer paid): status complete,
// payment_status paid, its PaymentIntent succeeded.
func (s *Server) Pay(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.sessions[sessionID]
	if cs == nil {
		panic("stripetest: unknown session " + sessionID)
	}
	cs.Status, cs.PaymentStatus = "complete", "paid"
	if pi := s.intents[cs.PaymentIntent]; pi != nil {
		pi.Status, pi.Received = "succeeded", pi.Amount
	}
}

// ExpireSession marks a session expired (Stripe's own expiry).
func (s *Server) ExpireSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cs := s.sessions[sessionID]; cs != nil && cs.Status == "open" {
		cs.Status = "expired"
	}
}

// SetRefundStatus changes the status of an existing refund (asynchronous
// confirmation or failure).
func (s *Server) SetRefundStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.refunds[id]; r != nil {
		if status == "succeeded" && r.Status != "succeeded" {
			s.intents[r.PaymentIntent].Refunded += r.Amount
		}
		r.Status = status
	}
}

// RefundDashboard simulates a refund made in the Stripe dashboard.
func (s *Server) RefundDashboard(pi string, amount int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("re")
	s.refunds[id] = &Refund{ID: id, PaymentIntent: pi, Status: "succeeded", Amount: amount, Metadata: map[string]string{}}
	s.intents[pi].Refunded += amount
	return id
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) id(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_test_%06d", prefix, s.seq)
}

// ---- HTTP -------------------------------------------------------------------

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	req := Request{
		Method: r.Method, Path: r.URL.Path, IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Authorization: r.Header.Get("Authorization"), StripeVersion: r.Header.Get("Stripe-Version"), Form: r.PostForm,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if s.failNext > 0 {
		s.failNext--
		writeErr(w, http.StatusInternalServerError, "api_error", "", "fake outage")
		return
	}
	if req.Authorization != "Bearer "+SecretKey {
		writeErr(w, http.StatusUnauthorized, "invalid_request_error", "", "Invalid API Key provided")
		return
	}
	if r.Method == http.MethodPost && req.IdempotencyKey != "" {
		params := r.PostForm.Encode()
		if e, ok := s.idem[req.IdempotencyKey]; ok {
			if e.params != params {
				writeErr(w, http.StatusBadRequest, "idempotency_error", "", "Keys for idempotent requests can only be used with the same parameters they were first used with.")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(e.status)
			_, _ = w.Write(e.body)
			return
		}
		rec := httptest.NewRecorder()
		s.route(rec, r, req)
		s.idem[req.IdempotencyKey] = idemEntry{params: params, status: rec.Code, body: rec.Body.Bytes()}
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	s.route(w, r, req)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request, req Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
		s.createSession(w, req.Form)
	case r.Method == http.MethodGet && len(parts) == 4 && parts[1] == "checkout" && parts[2] == "sessions":
		s.withSession(w, parts[3], func(cs *Session) { writeJSON(w, s.sessionJSON(cs)) })
	case r.Method == http.MethodPost && len(parts) == 5 && parts[1] == "checkout" && parts[4] == "expire":
		s.withSession(w, parts[3], func(cs *Session) {
			if cs.Status != "open" {
				writeErr(w, http.StatusBadRequest, "invalid_request_error", "checkout_session_not_open", "not open")
				return
			}
			cs.Status = "expired"
			writeJSON(w, s.sessionJSON(cs))
		})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
		s.createIntent(w, req.Form)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "payment_intents":
		s.withIntent(w, parts[2], func(pi *PaymentIntent) { writeJSON(w, intentJSON(pi)) })
	case r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "payment_intents" && parts[3] == "cancel":
		s.withIntent(w, parts[2], func(pi *PaymentIntent) {
			if pi.Status == "succeeded" {
				writeErr(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state", "succeeded")
				return
			}
			pi.Status = "canceled"
			writeJSON(w, intentJSON(pi))
		})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
		s.createRefund(w, req.Form)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "refunds":
		rf := s.refunds[parts[2]]
		if rf == nil {
			writeErr(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "no such refund")
			return
		}
		writeJSON(w, refundJSON(rf))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/account":
		writeJSON(w, map[string]any{"id": "acct_test_lido", "object": "account", "settings": map[string]any{"dashboard": map[string]any{"display_name": "Lido Test"}}})
	default:
		writeErr(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "unknown endpoint "+r.URL.Path)
	}
}

func metadataOf(f url.Values, prefix string) map[string]string {
	md := map[string]string{}
	for k, v := range f {
		if strings.HasPrefix(k, prefix+"[") && strings.HasSuffix(k, "]") && len(v) > 0 {
			md[strings.TrimSuffix(strings.TrimPrefix(k, prefix+"["), "]")] = v[0]
		}
	}
	return md
}

func (s *Server) createSession(w http.ResponseWriter, f url.Values) {
	amount, _ := strconv.ParseInt(f.Get("line_items[0][price_data][unit_amount]"), 10, 64)
	if amount <= 0 || f.Get("mode") != "payment" {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "amount or mode")
		return
	}
	expires, _ := strconv.ParseInt(f.Get("expires_at"), 10, 64)
	now := s.now()
	if expires != 0 && (expires < now.Add(30*time.Minute).Unix() || expires > now.Add(24*time.Hour).Unix()) {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid_integer", "expires_at out of range")
		return
	}
	if expires == 0 {
		expires = now.Add(24 * time.Hour).Unix()
	}
	pi := &PaymentIntent{ID: s.id("pi"), Status: "requires_payment_method", Currency: f.Get("line_items[0][price_data][currency]"), Amount: amount, Metadata: metadataOf(f, "payment_intent_data[metadata]")}
	s.intents[pi.ID] = pi
	cs := &Session{
		ID: s.id("cs"), PaymentIntent: pi.ID, Status: "open", PaymentStatus: "unpaid", Currency: pi.Currency,
		Locale: f.Get("locale"), CustomerEmail: f.Get("customer_email"), ClientReferenceID: f.Get("client_reference_id"),
		SuccessURL: f.Get("success_url"), CancelURL: f.Get("cancel_url"), Amount: amount, ExpiresAt: expires,
		Created: now.Unix(), Metadata: metadataOf(f, "metadata"),
	}
	s.sessions[cs.ID] = cs
	writeJSON(w, s.sessionJSON(cs))
}

func (s *Server) createIntent(w http.ResponseWriter, f url.Values) {
	amount, _ := strconv.ParseInt(f.Get("amount"), 10, 64)
	if amount <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "amount")
		return
	}
	pi := &PaymentIntent{ID: s.id("pi"), Status: "requires_payment_method", Currency: f.Get("currency"), Amount: amount, Metadata: metadataOf(f, "metadata")}
	s.intents[pi.ID] = pi
	writeJSON(w, intentJSON(pi))
}

func (s *Server) createRefund(w http.ResponseWriter, f url.Values) {
	pi := s.intents[f.Get("payment_intent")]
	if pi == nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "no such payment_intent")
		return
	}
	if pi.Status != "succeeded" {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "charge_not_refundable", "not captured")
		return
	}
	amount, _ := strconv.ParseInt(f.Get("amount"), 10, 64)
	var reserved int64
	for _, r := range s.refunds {
		if r.PaymentIntent == pi.ID && (r.Status == "succeeded" || r.Status == "pending") {
			reserved += r.Amount
		}
	}
	if amount <= 0 || reserved+amount > pi.Received {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "amount_too_large", "refund exceeds charge")
		return
	}
	rf := &Refund{ID: s.id("re"), PaymentIntent: pi.ID, Status: s.RefundStatus, Amount: amount, Metadata: metadataOf(f, "metadata")}
	if rf.Status == "succeeded" {
		pi.Refunded += amount
	}
	s.refunds[rf.ID] = rf
	writeJSON(w, refundJSON(rf))
}

func (s *Server) withSession(w http.ResponseWriter, id string, fn func(*Session)) {
	cs := s.sessions[id]
	if cs == nil {
		writeErr(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "no such checkout session")
		return
	}
	fn(cs)
}

func (s *Server) withIntent(w http.ResponseWriter, id string, fn func(*PaymentIntent)) {
	pi := s.intents[id]
	if pi == nil {
		writeErr(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "no such payment intent")
		return
	}
	fn(pi)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, typ, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	e := map[string]any{"type": typ, "message": msg}
	if code != "" {
		e["code"] = code
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": e})
}

func (s *Server) sessionJSON(cs *Session) map[string]any {
	out := sessionJSON(cs)
	if s.checkoutBase != "" {
		out["url"] = s.checkoutBase + "/c/pay/" + cs.ID
	}
	return out
}

func sessionJSON(cs *Session) map[string]any {
	return map[string]any{
		"id": cs.ID, "object": "checkout.session", "amount_total": cs.Amount, "currency": cs.Currency,
		"payment_intent": cs.PaymentIntent, "payment_status": cs.PaymentStatus, "status": cs.Status,
		"client_reference_id": cs.ClientReferenceID, "metadata": cs.Metadata, "expires_at": cs.ExpiresAt,
		"created": cs.Created, "locale": cs.Locale, "mode": "payment",
		"url": "https://checkout.stripe.test/c/pay/" + cs.ID,
	}
}

func intentJSON(pi *PaymentIntent) map[string]any {
	return map[string]any{
		"id": pi.ID, "object": "payment_intent", "amount": pi.Amount, "amount_received": pi.Received,
		"currency": pi.Currency, "status": pi.Status, "metadata": pi.Metadata, "client_secret": pi.ID + "_secret_test",
	}
}

func refundJSON(rf *Refund) map[string]any {
	return map[string]any{
		"id": rf.ID, "object": "refund", "amount": rf.Amount, "currency": "eur", "payment_intent": rf.PaymentIntent,
		"status": rf.Status, "metadata": rf.Metadata,
	}
}

// ---- events -------------------------------------------------------------------

// Event returns the JSON of a Stripe event of type typ about object,
// with id evtID, created at t.
func Event(evtID, typ string, t time.Time, object map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{
		"id": evtID, "object": "event", "api_version": stripego.APIVersion, "created": t.Unix(),
		"type": typ, "livemode": false, "pending_webhooks": 1, "data": map[string]any{"object": object},
	})
	return b
}

// SessionEvent is a checkout.session.* event of a fake session, as it is now.
func (s *Server) SessionEvent(evtID, typ, sessionID string, t time.Time) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Event(evtID, typ, t, sessionJSON(s.sessions[sessionID]))
}

// IntentEvent is a payment_intent.* event of a fake payment intent.
func (s *Server) IntentEvent(evtID, typ, piID string, t time.Time) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Event(evtID, typ, t, intentJSON(s.intents[piID]))
}

// ChargeRefundedEvent is a charge.refunded event of the charge of a fake
// payment intent, with its cumulative refunded amount.
func (s *Server) ChargeRefundedEvent(evtID, piID string, t time.Time) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	pi := s.intents[piID]
	return Event(evtID, "charge.refunded", t, map[string]any{
		"id": "ch_" + piID, "object": "charge", "amount": pi.Amount, "amount_captured": pi.Received,
		"amount_refunded": pi.Refunded, "currency": pi.Currency, "payment_intent": pi.ID, "refunded": pi.Refunded >= pi.Received,
		"paid": true, "status": "succeeded", "metadata": map[string]string{},
	})
}

// RefundEvent is a refund.* event of a fake refund.
func (s *Server) RefundEvent(evtID, typ, refundID string, t time.Time) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Event(evtID, typ, t, refundJSON(s.refunds[refundID]))
}

// Sign returns the Stripe-Signature header of payload signed with secret
// at t (extra secrets add more v1 values, as during a rotation).
func Sign(payload []byte, t time.Time, secret string, extra ...string) string {
	h := "t=" + strconv.FormatInt(t.Unix(), 10)
	for _, sec := range append([]string{secret}, extra...) {
		h += ",v1=" + hex.EncodeToString(stripego.ComputeSignature(t, payload, sec))
	}
	return h
}

// WebhookRequest builds the POST of a webhook delivery signed at t.
func WebhookRequest(target string, payload []byte, t time.Time, secret string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	r.Header.Set("Stripe-Signature", Sign(payload, t, secret))
	return r
}
