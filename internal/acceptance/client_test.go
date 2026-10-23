//go:build integration

package acceptance

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// apiClient is a logged-in panel user talking to the admin API like the
// browser does: session cookie, X-CSRF-Token on writes, Idempotency-Key on
// creations. Its methods are safe for concurrent use and never call
// t.Fatal (bursts run in goroutines); the scenario checks results.
type apiClient struct {
	t    *testing.T
	base string
	http *http.Client

	mutations atomic.Int64 // state-changing calls answered 2xx (the operation count)
	refused   atomic.Int64 // state-changing calls refused by the server (4xx)

	mu      sync.Mutex
	missing map[string]int // operation -> 501 answers
	counts  map[string]int // operation -> calls
}

// result is one HTTP exchange.
type result struct {
	Status  int
	Body    []byte
	ETag    string
	Problem problemBody
}

// problemBody is the application/problem+json shape (spec 17).
type problemBody struct {
	Type          string `json:"type"`
	Status        int    `json:"status"`
	Detail        string `json:"detail"`
	CurrentStatus string `json:"current_status"`
	Conflicts     []any  `json:"conflicts"`
}

func (r result) ok() bool { return r.Status >= 200 && r.Status < 300 }

func (r result) String() string {
	b := r.Body
	if len(b) > 400 {
		b = b[:400]
	}
	return fmt.Sprintf("%d %s", r.Status, b)
}

func newClient(t *testing.T, base string) *apiClient {
	jar, _ := cookiejar.New(nil)
	return &apiClient{
		t: t, base: base, missing: map[string]int{}, counts: map[string]int{},
		http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func (c *apiClient) login(email, password string) {
	c.t.Helper()
	r := c.do("login", http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": email, "password": password}, nil)
	if r.Status != http.StatusOK {
		c.t.Fatalf("login: %s", r)
	}
	c.mutations.Add(-1) // logging in is not a beach operation
}

func (c *apiClient) csrf() string {
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "arenile_csrf" {
			return ck.Value
		}
	}
	return ""
}

// do sends one request. op names the contract operation (for the 501
// report and the counters).
func (c *apiClient) do(op, method, path string, in any, hdr map[string]string) result {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return result{Status: -1, Body: []byte(err.Error())}
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return result{Status: -1, Body: []byte(err.Error())}
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", c.csrf())
	}
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", newKey())
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return result{Status: -1, Body: []byte(err.Error())}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	r := result{Status: resp.StatusCode, Body: raw, ETag: resp.Header.Get("ETag")}
	if strings.Contains(resp.Header.Get("Content-Type"), "problem+json") {
		_ = json.Unmarshal(raw, &r.Problem)
	}
	// Only a mutation the server accepted counts as an operation of the
	// day; refusals are reported apart (INV-43 counts work done, not tried).
	if method != http.MethodGet {
		if r.ok() {
			c.mutations.Add(1)
		} else {
			c.refused.Add(1)
		}
	}
	c.mu.Lock()
	c.counts[op]++
	if r.Status == http.StatusNotImplemented {
		c.missing[op]++
	}
	c.mu.Unlock()
	return r
}

// get is do with GET.
func (c *apiClient) get(op, path string) result { return c.do(op, http.MethodGet, path, nil, nil) }

// post is do with POST.
func (c *apiClient) post(op, path string, in any) result {
	return c.do(op, http.MethodPost, path, in, nil)
}

// notImplemented lists the operations that answered 501.
func (c *apiClient) notImplemented() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.missing))
	for op := range c.missing {
		out = append(out, op)
	}
	sort.Strings(out)
	return out
}

// failIfNotImplemented fails the test when some operation of the contract
// answered 501 Not Implemented: every M3 operation must be served. Main
// goroutine only.
func (c *apiClient) failIfNotImplemented() {
	c.t.Helper()
	if missing := c.notImplemented(); len(missing) > 0 {
		c.t.Fatalf("M3 operations not implemented (501): %s", strings.Join(missing, ", "))
	}
}

// must checks a call that the scenario expects to succeed.
func (c *apiClient) must(r result, what string) result {
	c.t.Helper()
	if r.Status == http.StatusNotImplemented {
		c.failIfNotImplemented()
	}
	if !r.ok() {
		c.t.Fatalf("%s: %s", what, r)
	}
	return r
}

// decode unmarshals a successful response into T.
func decode[T any](t *testing.T, r result) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.Body, &v); err != nil {
		t.Fatalf("decode %T from %s: %v", v, r, err)
	}
	return v
}

// ifMatch returns the If-Match header of a version.
func ifMatch(version int64) map[string]string {
	return map[string]string{"If-Match": fmt.Sprintf("%q", fmt.Sprint(version))}
}

func newKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "acc-" + hex.EncodeToString(b)
}
