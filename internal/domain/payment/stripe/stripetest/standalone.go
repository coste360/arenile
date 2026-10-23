package stripetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// StandaloneOptions configure a fake started outside a Go test (Start):
// the end-to-end and real-binary acceptance tests run it as a process.
type StandaloneOptions struct {
	// ArenileBaseURL is where the signed webhooks are delivered:
	// <ArenileBaseURL>/webhooks/payments/stripe/<establishment_id>.
	ArenileBaseURL string
	// WebhookSecret signs the deliveries (default WebhookSecret).
	WebhookSecret string
	// Client delivers the webhooks (default: 10 s timeout).
	Client *http.Client
}

// Start runs the fake on addr (for example "127.0.0.1:12111") with, in
// addition to the API, the hosted payment page that Checkout Sessions
// point to: GET /c/pay/{id} shows a "Pay" button, POST completes the
// session (Pay), delivers a signed checkout.session.completed webhook to
// Arenile, waits for its answer and redirects the browser to the session
// success_url, like Stripe Checkout does. Close the server when done.
func Start(addr string, o StandaloneOptions) (*Server, error) {
	if o.WebhookSecret == "" {
		o.WebhookSecret = WebhookSecret
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 10 * time.Second}
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("stripetest: listen %s: %w", addr, err)
	}
	s := &Server{
		sessions: map[string]*Session{}, intents: map[string]*PaymentIntent{}, refunds: map[string]*Refund{},
		idem: map[string]idemEntry{}, RefundStatus: "succeeded",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/c/pay/{id}", func(w http.ResponseWriter, r *http.Request) { s.hostedPage(w, r, o) })
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", s.serve)
	s.Server = httptest.NewUnstartedServer(mux)
	_ = s.Listener.Close()
	s.Listener = ln
	s.Start()
	s.checkoutBase = s.URL
	return s, nil
}

var hostedTmpl = template.Must(template.New("pay").Funcs(template.FuncMap{
	"euro": func(cents int64) string { return fmt.Sprintf("%d,%02d", cents/100, cents%100) },
}).Parse(`<!doctype html>
<html lang="it"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Stripe di prova · pagamento</title>
<style>
body{margin:0;font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:#f6f8fa;color:#1a1f36}
main{max-width:420px;margin:48px auto;padding:0 16px}
.card{background:#fff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,.08);padding:28px}
.badge{display:inline-block;background:#fff4e5;color:#a35200;border-radius:6px;padding:2px 8px;font-size:12px;font-weight:600}
h1{font-size:20px;margin:12px 0 4px}.amount{font-size:32px;font-weight:700;margin:8px 0 20px}
.muted{color:#697386;font-size:14px}button{width:100%;padding:12px;border:0;border-radius:8px;font-size:16px;font-weight:600;cursor:pointer;margin-top:10px}
.pay{background:#635bff;color:#fff}.cancel{background:transparent;color:#697386}
</style></head>
<body><main><div class="card">
<span class="badge">MODALITÀ DI PROVA · finto Stripe dei test</span>
<h1>Pagamento della prenotazione</h1>
<div class="amount">{{euro .Amount}} €</div>
<p class="muted">Sessione {{.ID}}. Nessun addebito reale: il pulsante invia ad Arenile un webhook firmato come farebbe Stripe.</p>
{{if eq .Status "open"}}
<form method="post"><button class="pay" type="submit" name="action" value="pay">Paga</button>
<button class="cancel" type="submit" name="action" value="cancel">Annulla e torna indietro</button></form>
{{else}}<p>Sessione non aperta: {{.Status}}</p>{{end}}
</div></main></body></html>`))

func (s *Server) hostedPage(w http.ResponseWriter, r *http.Request, o StandaloneOptions) {
	id := r.PathValue("id")
	s.mu.Lock()
	cs := s.sessions[id]
	var view Session
	if cs != nil {
		view = *cs
	}
	s.mu.Unlock()
	if cs == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = hostedTmpl.Execute(w, view)
		return
	}
	if view.Status != "open" {
		http.Error(w, "session not open", http.StatusConflict)
		return
	}
	if r.FormValue("action") == "cancel" {
		http.Redirect(w, r, view.CancelURL, http.StatusSeeOther)
		return
	}
	s.Pay(id)
	if err := s.deliver(r.Context(), o, s.SessionEvent("evt_"+id, "checkout.session.completed", id, time.Now()), view.Metadata["establishment_id"]); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// The success URL is the one Arenile gave when creating the session, as
	// with Stripe Checkout (test double, never deployed).
	http.Redirect(w, r, strings.ReplaceAll(view.SuccessURL, "{CHECKOUT_SESSION_ID}", id), http.StatusSeeOther) //nolint:gosec // G710: test double, URL from the session creator
}

// deliver posts a signed webhook to Arenile and requires a 2xx answer.
func (s *Server) deliver(ctx context.Context, o StandaloneOptions, payload []byte, establishmentID string) error {
	if o.ArenileBaseURL == "" || establishmentID == "" {
		return errors.New("stripetest: no webhook target (ArenileBaseURL or establishment_id metadata missing)")
	}
	target := strings.TrimRight(o.ArenileBaseURL, "/") + "/webhooks/payments/stripe/" + establishmentID
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Stripe-Signature", Sign(payload, time.Now(), o.WebhookSecret))
	resp, err := o.Client.Do(req) //nolint:gosec // G704: test double, target is the configured Arenile instance
	if err != nil {
		return fmt.Errorf("stripetest: webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("stripetest: webhook answered %d", resp.StatusCode)
	}
	return nil
}
