package fiscalbridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/pkg/ext"
)

func DocumentFromRequestKey(k string) ext.FiscalDocumentRequest {
	return ext.FiscalDocumentRequest{EstablishmentID: "e1", IdempotencyKey: k, Currency: "EUR"}
}

// fakeInstance scripts the answers of the device API.
type fakeInstance struct {
	mu        sync.Mutex
	registers int
	polls     int
	batches   [][]api.FiscalBridgeJob // returned by successive polls, then empty
	pollCodes []int                   // status of successive polls (0 = 200)
	submits   []api.FiscalBridgeJobResult
	submitIDs []string
	submitRes []int // status of successive submits (0 = 204)
	cancel    context.CancelFunc
	stopAfter int // cancel the agent after this many polls
}

func (f *fakeInstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer ar_dev_p_s" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodPut:
		f.registers++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"registered":true,"online":true,"pending_jobs":0,"failed_jobs":0}`))
	case http.MethodGet:
		f.polls++
		if f.stopAfter > 0 && f.polls >= f.stopAfter {
			f.cancel()
		}
		if len(f.pollCodes) > 0 {
			code := f.pollCodes[0]
			f.pollCodes = f.pollCodes[1:]
			if code != 0 {
				w.WriteHeader(code)
				return
			}
		}
		batch := []api.FiscalBridgeJob{}
		if len(f.batches) > 0 {
			batch, f.batches = f.batches[0], f.batches[1:]
		}
		_ = json.NewEncoder(w).Encode(api.FiscalBridgeJobBatch{Jobs: batch})
	case http.MethodPost:
		var res api.FiscalBridgeJobResult
		_ = json.NewDecoder(r.Body).Decode(&res)
		f.submits = append(f.submits, res)
		f.submitIDs = append(f.submitIDs, r.PathValue("id"))
		code := http.StatusNoContent
		if len(f.submitRes) > 0 {
			if f.submitRes[0] != 0 {
				code = f.submitRes[0]
			}
			f.submitRes = f.submitRes[1:]
		}
		w.WriteHeader(code)
	}
}

type scriptedDriver struct {
	mu     sync.Mutex
	calls  int
	err    error
	panics bool
}

func (*scriptedDriver) Code() string { return "rt-x" }
func (d *scriptedDriver) Issue(_ context.Context, req ext.FiscalDocumentRequest) (*ext.FiscalDocumentResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.panics {
		panic("driver bug")
	}
	if d.err != nil {
		return nil, d.err
	}
	return &ext.FiscalDocumentResult{Ref: "ref-" + req.IdempotencyKey, Number: "N1", Raw: json.RawMessage(`{"z":1}`)}, nil
}
func (*scriptedDriver) Void(context.Context, string) error { return nil }
func (*scriptedDriver) Status(context.Context) (ext.DeviceStatus, error) {
	return ext.DeviceStatus{Online: true}, nil
}

func issueJob(id, key string) api.FiscalBridgeJob {
	return api.FiscalBridgeJob{Id: id, Kind: api.FiscalBridgeJobKind(KindIssue), Attempt: 1, Document: &api.FiscalBridgeDocument{
		EstablishmentId: "e1", Currency: "EUR", IdempotencyKey: key, Total: 100,
		Lines:    []api.FiscalBridgeLine{{Description: "x", Quantity: 1, UnitGross: 100, Gross: 100, VatRateBp: 2200}},
		Payments: []api.FiscalBridgePayment{{Method: "cash", Amount: 100}},
	}}
}

func runAgent(t *testing.T, f *fakeInstance, drv ext.FiscalDriver, results ResultStore) error {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/api/device/v1/fiscal-bridge", f)
	mux.Handle("/api/device/v1/fiscal-bridge/jobs", f)
	mux.Handle("/api/device/v1/fiscal-bridge/jobs/{id}/result", f)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.cancel = cancel
	a := NewAgent(AgentDeps{
		BaseURL: srv.URL + "/", Token: "ar_dev_p_s", Driver: drv, Results: results, PollWait: 1,
		Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	})
	return a.Run(ctx)
}

func TestAgent_DeduplicatesRedeliveries(t *testing.T) {
	drv := &scriptedDriver{}
	f := &fakeInstance{
		batches:   [][]api.FiscalBridgeJob{{issueJob("j1", "k1")}, {issueJob("j1", "k1")}},
		submitRes: []int{http.StatusBadGateway, http.StatusBadGateway, http.StatusBadGateway, http.StatusBadGateway, http.StatusBadGateway},
		stopAfter: 4,
	}
	if err := runAgent(t, f, drv, NewMemoryResults()); err != nil {
		t.Fatal(err)
	}
	if drv.calls != 1 {
		t.Fatalf("driver called %d times for one job delivered twice", drv.calls)
	}
	if len(f.submits) != submitAttempts+1 || f.submits[len(f.submits)-1].Number == nil || *f.submits[len(f.submits)-1].Number != "N1" {
		t.Fatalf("submits %d %+v", len(f.submits), f.submits)
	}
	if f.submits[0].Raw == nil || (*f.submits[0].Raw)["z"] != float64(1) {
		t.Fatalf("raw not forwarded: %+v", f.submits[0].Raw)
	}
}

func TestAgent_ReRegistersOnConflictAndStopsWhenRevoked(t *testing.T) {
	f := &fakeInstance{pollCodes: []int{http.StatusConflict, http.StatusServiceUnavailable, 0}, stopAfter: 3}
	if err := runAgent(t, f, &scriptedDriver{}, nil); err != nil {
		t.Fatal(err)
	}
	if f.registers != 2 {
		t.Fatalf("registers %d: a 409 on poll must register again", f.registers)
	}
	f2 := &fakeInstance{pollCodes: []int{http.StatusUnauthorized}}
	if err := runAgent(t, f2, &scriptedDriver{}, nil); !errors.Is(err, ErrDeviceRevoked) {
		t.Fatalf("401: %v", err)
	}
}

func TestAgent_FailuresAndPoisonJobs(t *testing.T) {
	// Retryable failure: reported as retryable and not remembered, so the
	// next delivery prints for real.
	drv := &scriptedDriver{err: Retryable(errors.New("carta finita"))}
	results := NewMemoryResults()
	f := &fakeInstance{batches: [][]api.FiscalBridgeJob{{issueJob("j1", "k1")}}, stopAfter: 2}
	if err := runAgent(t, f, drv, results); err != nil {
		t.Fatal(err)
	}
	if len(f.submits) != 1 || f.submits[0].Retryable == nil || !*f.submits[0].Retryable {
		t.Fatalf("retryable failure %+v", f.submits)
	}
	if _, ok := results.Get("issue:k1"); ok {
		t.Fatal("a retryable failure must not be remembered")
	}
	// A panicking driver yields a final failure; a 409 on submit is dropped.
	drv = &scriptedDriver{panics: true}
	f = &fakeInstance{batches: [][]api.FiscalBridgeJob{{issueJob("j2", "k2")}}, submitRes: []int{http.StatusConflict}, stopAfter: 2}
	if err := runAgent(t, f, drv, results); err != nil {
		t.Fatal(err)
	}
	if len(f.submits) != 1 || f.submits[0].Outcome != api.FiscalBridgeJobResultOutcome(OutcomeFailed) || f.submits[0].Retryable != nil {
		t.Fatalf("panic outcome %+v", f.submits)
	}
}

func TestJobKey(t *testing.T) {
	ref := "r9"
	if jobKey(issueJob("j", "k")) != "issue:k" || jobKey(api.FiscalBridgeJob{Id: "j", Kind: "void", Ref: &ref}) != "void:r9" || jobKey(api.FiscalBridgeJob{Id: "j"}) != "job:j" {
		t.Fatal("jobKey")
	}
}
