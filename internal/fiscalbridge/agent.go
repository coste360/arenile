package fiscalbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/pkg/ext"
)

// Agent tuning.
const (
	// agentPollWait is the long-poll wait asked to the instance.
	agentPollWait = 25
	// agentDriverTimeout bounds one call to the local driver (below the
	// 60 s lease, so a slow printer does not cause a duplicate delivery).
	agentDriverTimeout = 45 * time.Second
	// agentRefresh is how often the agent registers again to report the
	// device status.
	agentRefresh = 2 * time.Minute
	backoffFloor = time.Second
	backoffCeil  = time.Minute
	// submitAttempts bounds the immediate retries of one submission; the
	// result stays in the ResultStore and is submitted again on the next
	// delivery of the job.
	submitAttempts = 5
)

// ErrDeviceRevoked: the instance refused the device token (401): the
// device was revoked or unpaired; the launcher must pair again.
var ErrDeviceRevoked = errors.New("fiscalbridge: device token refused by the instance")

// AgentDeps are the dependencies of the desktop-side Agent.
type AgentDeps struct {
	// BaseURL of the remote instance (from the pairing QR).
	BaseURL string
	// Token is the device token (read from the OS keyring by the launcher;
	// never logged).
	Token string
	// HTTPClient must pin the certificate fingerprint of the QR when the
	// instance uses a self-signed certificate (see desktop.PinnedClientConfig).
	// Its Timeout must be 0 or above 40 s (long-poll).
	HTTPClient *http.Client
	// Driver is the local FiscalDriver (printer / registratore telematico).
	Driver ext.FiscalDriver
	// Results remembers the outcomes already produced (default: in memory;
	// the launcher passes OpenFileResults in the data dir).
	Results ResultStore
	Clock   clock.Clock
	Log     *slog.Logger
	// UserAgent identifies the app version to the instance.
	UserAgent string
	// Sleep waits d or until ctx is done (tests inject a fast one).
	Sleep func(ctx context.Context, d time.Duration) error
	// PollWait is the long-poll wait in seconds (default and max 25).
	PollWait int
}

// AgentState is a snapshot for the UI (tray tooltip, settings page).
type AgentState struct {
	Connected  bool
	LastError  string
	LastPollAt time.Time
	Processed  int
}

// Agent runs on the cash desk PC (desktop app in remote mode): it registers
// as the bridge, long-polls the instance, executes jobs on the local driver
// and submits the outcome, with exponential backoff on network errors.
// Exactly-once issuing on the device: outcomes are remembered by
// idempotency key before being submitted (ResultStore), and the instance
// accepts a duplicate submission of the same outcome.
type Agent struct {
	d   AgentDeps
	api *bridgeClient

	mu    sync.Mutex
	state AgentState
}

// NewAgent returns an Agent.
func NewAgent(d AgentDeps) *Agent {
	if d.HTTPClient == nil {
		d.HTTPClient = &http.Client{}
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Results == nil {
		d.Results = NewMemoryResults()
	}
	if d.Sleep == nil {
		d.Sleep = sleepCtx
	}
	if d.Driver == nil {
		d.Driver = NoopDriver{Clock: d.Clock}
	}
	if d.PollWait <= 0 || d.PollWait > agentPollWait {
		d.PollWait = agentPollWait
	}
	return &Agent{d: d, api: &bridgeClient{base: strings.TrimRight(d.BaseURL, "/"), token: d.Token, hc: d.HTTPClient, ua: d.UserAgent}}
}

// State returns a snapshot of the agent state.
func (a *Agent) State() AgentState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

func (a *Agent) setState(f func(*AgentState)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f(&a.state)
}

// Run blocks until ctx is cancelled (nil) or the device token is refused
// (ErrDeviceRevoked). Network errors never stop it.
func (a *Agent) Run(ctx context.Context) error {
	bo := backoff{floor: backoffFloor, ceil: backoffCeil}
	var registeredAt time.Time
	for ctx.Err() == nil {
		if registeredAt.IsZero() || time.Since(registeredAt) >= agentRefresh {
			if err := a.register(ctx); err != nil {
				if errors.Is(err, ErrDeviceRevoked) {
					return err
				}
				a.fail(ctx, "register", err)
				_ = a.d.Sleep(ctx, bo.next())
				continue
			}
			registeredAt = time.Now()
		}
		jobs, err := a.api.poll(ctx, a.d.PollWait, DefaultMaxJobs)
		switch {
		case ctx.Err() != nil:
			return nil
		case errors.Is(err, ErrDeviceRevoked):
			return err
		case isStatus(err, http.StatusConflict):
			registeredAt = time.Time{} // another device took over or we were unregistered
			a.fail(ctx, "poll", err)
			_ = a.d.Sleep(ctx, bo.next())
			continue
		case err != nil:
			a.fail(ctx, "poll", err)
			_ = a.d.Sleep(ctx, bo.next())
			continue
		}
		bo.reset()
		a.setState(func(s *AgentState) { s.Connected, s.LastError, s.LastPollAt = true, "", a.d.Clock.Now().UTC() })
		for _, j := range jobs {
			if err := a.handle(ctx, j); errors.Is(err, ErrDeviceRevoked) {
				return err
			}
		}
	}
	return nil
}

func (a *Agent) fail(ctx context.Context, op string, err error) {
	a.setState(func(s *AgentState) { s.Connected, s.LastError = false, err.Error() })
	a.d.Log.WarnContext(ctx, "fiscal bridge "+op+" failed", slog.String("error", err.Error()))
}

func (a *Agent) register(ctx context.Context) error {
	reg := api.FiscalBridgeRegistration{Driver: a.d.Driver.Code()}
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	st, err := a.d.Driver.Status(sctx)
	cancel()
	ds := api.FiscalDeviceStatus{Online: err == nil && st.Online}
	msg := st.Message
	if err != nil {
		msg = err.Error()
	}
	if msg != "" {
		ds.Message = &msg
	}
	checked := a.d.Clock.Now().UTC()
	ds.CheckedAt = &checked
	reg.DeviceStatus = &ds
	return a.api.register(ctx, reg)
}

// jobKey is the deduplication key of a job on the device.
func jobKey(j api.FiscalBridgeJob) string {
	if j.Kind == api.FiscalBridgeJobKind(KindIssue) && j.Document != nil && j.Document.IdempotencyKey != "" {
		return "issue:" + j.Document.IdempotencyKey
	}
	if j.Ref != nil {
		return "void:" + *j.Ref
	}
	return "job:" + j.Id
}

// handle executes one job at most once on the local driver and submits
// its outcome.
func (a *Agent) handle(ctx context.Context, j api.FiscalBridgeJob) error {
	key := jobKey(j)
	res, seen := a.d.Results.Get(key)
	if !seen {
		res = a.execute(ctx, j)
		if !res.Retryable {
			// Remember before submitting: if the ack is lost the next
			// delivery re-submits this outcome instead of printing again.
			if err := a.d.Results.Put(key, res); err != nil {
				a.d.Log.ErrorContext(ctx, "fiscal bridge: cannot persist result", slog.String("job_id", j.Id), slog.String("error", err.Error()))
			}
		}
	}
	bo := backoff{floor: backoffFloor, ceil: 10 * time.Second}
	for i := 0; i < submitAttempts; i++ {
		err := a.api.submit(ctx, j.Id, res)
		switch {
		case err == nil:
			a.setState(func(s *AgentState) { s.Processed++ })
			return nil
		case errors.Is(err, ErrDeviceRevoked):
			return err
		case isStatus(err, http.StatusConflict), isStatus(err, http.StatusNotFound), isStatus(err, http.StatusBadRequest):
			// Lease lost or result rejected: the instance owns the job now.
			a.d.Log.WarnContext(ctx, "fiscal bridge result refused", slog.String("job_id", j.Id), slog.String("error", err.Error()))
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		a.d.Log.WarnContext(ctx, "fiscal bridge submit failed, retrying", slog.String("job_id", j.Id), slog.String("error", err.Error()))
		_ = a.d.Sleep(ctx, bo.next())
	}
	return nil // delivered again after the lease expires; the result is remembered
}

// execute runs the job on the local driver; a panic of the driver becomes a
// final failure (a poison job must not crash the agent in a loop).
func (a *Agent) execute(ctx context.Context, j api.FiscalBridgeJob) (res Result) {
	dctx, cancel := context.WithTimeout(ctx, agentDriverTimeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			res = Result{Outcome: OutcomeFailed, Error: fmt.Sprintf("errore interno del driver fiscale: %v", p)}
		}
	}()
	switch JobKind(j.Kind) {
	case KindIssue:
		if j.Document == nil {
			return Result{Outcome: OutcomeFailed, Error: "lavoro senza documento"}
		}
		out, err := a.d.Driver.Issue(dctx, documentFromAPI(*j.Document).Request())
		if err != nil {
			return failure(err)
		}
		r := Result{Outcome: OutcomeSucceeded, Ref: out.Ref, Number: out.Number}
		if !out.IssuedAt.IsZero() {
			t := out.IssuedAt.UTC()
			r.IssuedAt = &t
		}
		if len(out.Raw) > 0 && len(out.Raw) <= MaxRawBytes && json.Valid(out.Raw) && isJSONObject(out.Raw) {
			r.Raw = out.Raw
		}
		return r
	case KindVoid:
		if j.Ref == nil {
			return Result{Outcome: OutcomeFailed, Error: "annullo senza riferimento"}
		}
		if err := a.d.Driver.Void(dctx, *j.Ref); err != nil {
			return failure(err)
		}
		return Result{Outcome: OutcomeSucceeded, Ref: *j.Ref}
	default:
		return Result{Outcome: OutcomeFailed, Error: "tipo di lavoro sconosciuto: " + string(j.Kind)}
	}
}

func failure(err error) Result {
	return Result{Outcome: OutcomeFailed, Error: err.Error(), Retryable: IsRetryable(err)}
}

func isJSONObject(b []byte) bool {
	b = bytes.TrimSpace(b)
	return len(b) > 0 && b[0] == '{'
}

func documentFromAPI(d api.FiscalBridgeDocument) Document {
	out := Document{
		EstablishmentID: d.EstablishmentId, Currency: d.Currency, Total: d.Total, IssuedAt: d.IssuedAt.UTC(),
		IdempotencyKey: d.IdempotencyKey, Lines: make([]Line, len(d.Lines)), Payments: make([]Payment, len(d.Payments)),
	}
	if d.BookingId != nil {
		out.BookingID = *d.BookingId
	}
	if d.PaymentId != nil {
		out.PaymentID = *d.PaymentId
	}
	for i, l := range d.Lines {
		out.Lines[i] = Line{Description: l.Description, Quantity: l.Quantity, UnitGross: l.UnitGross, Gross: l.Gross, VATRateBP: l.VatRateBp}
	}
	for i, p := range d.Payments {
		out.Payments[i] = Payment{Method: p.Method, Amount: p.Amount}
	}
	return out
}

// ---- backoff ----------------------------------------------------------------

// backoff doubles from floor to ceil (deterministic; the agent is a single
// client per establishment, so there is no thundering herd to jitter).
type backoff struct {
	floor, ceil, cur time.Duration
}

func (b *backoff) next() time.Duration {
	if b.cur == 0 {
		b.cur = b.floor
		return b.cur
	}
	b.cur = min(b.cur*2, b.ceil)
	return b.cur
}

func (b *backoff) reset() { b.cur = 0 }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ---- HTTP client of /api/device/v1/fiscal-bridge ---------------------------------

// StatusError is a non-2xx answer of the instance.
type StatusError struct {
	Status int
	Type   string
	Detail string
}

func (e *StatusError) Error() string {
	s := "instance answered " + strconv.Itoa(e.Status)
	if e.Type != "" {
		s += " " + e.Type
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}

func isStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == status
}

type bridgeClient struct {
	base  string
	token string
	hc    *http.Client
	ua    string
}

func (c *bridgeClient) register(ctx context.Context, reg api.FiscalBridgeRegistration) error {
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.do(rctx, http.MethodPut, "/api/device/v1/fiscal-bridge", reg, nil)
}

func (c *bridgeClient) poll(ctx context.Context, wait, maxJobs int) ([]api.FiscalBridgeJob, error) {
	rctx, cancel := context.WithTimeout(ctx, time.Duration(wait+15)*time.Second)
	defer cancel()
	q := url.Values{"wait": {strconv.Itoa(wait)}, "max": {strconv.Itoa(maxJobs)}}
	var out api.FiscalBridgeJobBatch
	if err := c.do(rctx, http.MethodGet, "/api/device/v1/fiscal-bridge/jobs?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out.Jobs, nil
}

func (c *bridgeClient) submit(ctx context.Context, jobID string, r Result) error {
	body := api.FiscalBridgeJobResult{Outcome: api.FiscalBridgeJobResultOutcome(r.Outcome), IssuedAt: r.IssuedAt}
	if r.Ref != "" {
		body.Ref = &r.Ref
	}
	if r.Number != "" {
		body.Number = &r.Number
	}
	if r.Error != "" {
		body.Error = &r.Error
	}
	if r.Retryable {
		body.Retryable = &r.Retryable
	}
	if len(r.Raw) > 0 {
		var raw map[string]any
		if json.Unmarshal(r.Raw, &raw) == nil {
			body.Raw = &raw
		}
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.do(rctx, http.MethodPost, "/api/device/v1/fiscal-bridge/jobs/"+url.PathEscape(jobID)+"/result", body, nil)
}

func (c *bridgeClient) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return ErrDeviceRevoked
	}
	if resp.StatusCode/100 != 2 {
		se := &StatusError{Status: resp.StatusCode}
		var p struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&p) == nil {
			se.Type, se.Detail = p.Type, p.Detail
		}
		return se
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}
