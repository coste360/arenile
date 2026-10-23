//go:build integration

package acceptance

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// streamRecorder keeps the SSE stream of the establishment open during the
// scenario (as the panel does) and records every frame.
type streamRecorder struct {
	mu     sync.Mutex
	raw    strings.Builder
	topics map[string]int
	ids    map[string]int
	done   chan struct{}
}

// listen opens GET /api/admin/v1/stream with the client's session cookie.
func listen(t *testing.T, c *apiClient, establishmentID string) *streamRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/admin/v1/stream?establishment="+establishmentID, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		cancel()
		t.Fatalf("stream: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	s := &streamRecorder{topics: map[string]int{}, ids: map[string]int{}, done: make(chan struct{})}
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
		<-s.done
	})
	go func() {
		defer close(s.done)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			s.mu.Lock()
			s.raw.WriteString(line)
			s.raw.WriteByte('\n')
			switch {
			case strings.HasPrefix(line, "event: "):
				s.topics[strings.TrimPrefix(line, "event: ")]++
			case strings.HasPrefix(line, "id: "):
				s.ids[strings.TrimPrefix(line, "id: ")]++
			}
			s.mu.Unlock()
		}
	}()
	return s
}

// waitTopics waits until every topic was received at least once.
func (s *streamRecorder) waitTopics(t *testing.T, topics ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		missing := []string{}
		for _, tp := range topics {
			if s.topics[tp] == 0 {
				missing = append(missing, tp)
			}
		}
		s.mu.Unlock()
		if len(missing) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("SSE topics never received: %v (got %v)", missing, s.snapshotTopics())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *streamRecorder) snapshotTopics() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.topics))
	for k, v := range s.topics {
		out[k] = v
	}
	return out
}

// text returns everything received so far.
func (s *streamRecorder) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw.String()
}

// duplicates returns the event ids received more than once.
func (s *streamRecorder) duplicates() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for id, n := range s.ids {
		if n > 1 {
			out = append(out, id)
		}
	}
	return out
}
