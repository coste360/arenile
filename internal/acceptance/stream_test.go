//go:build integration

package acceptance

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// streamRecorder keeps the SSE stream of the establishment open during the
// scenario (as the panel does) and records every frame. Like EventSource,
// it reconnects with Last-Event-ID when the server closes the connection
// (a client that falls behind is disconnected on purpose, INV-45) and
// resumes from the last id received, so no event is lost under load.
type streamRecorder struct {
	mu         sync.Mutex
	raw        strings.Builder
	topics     map[string]int
	ids        map[string]int
	lastID     string
	reconnects int
	done       chan struct{}
}

// listen opens GET /api/admin/v1/stream with the client's session cookie.
func listen(t *testing.T, c *apiClient, establishmentID string) *streamRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	url := c.base + "/api/admin/v1/stream?establishment=" + establishmentID
	open := func(lastID string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("stream: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		return resp, nil
	}
	resp, err := open("")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	s := &streamRecorder{topics: map[string]int{}, ids: map[string]int{}, done: make(chan struct{})}
	t.Cleanup(func() {
		cancel()
		<-s.done
	})
	go func() {
		defer close(s.done)
		for {
			s.consume(resp.Body)
			_ = resp.Body.Close()
			if ctx.Err() != nil {
				return
			}
			// Dropped by the server (lagging) or closed: resume like the browser does.
			s.mu.Lock()
			s.reconnects++
			last := s.lastID
			s.mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			if resp, err = open(last); err != nil {
				if ctx.Err() == nil {
					t.Errorf("stream reconnect: %v", err)
				}
				return
			}
		}
	}()
	return s
}

// consume records the frames of one connection until it ends.
func (s *streamRecorder) consume(body io.Reader) {
	sc := bufio.NewScanner(body)
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
			id := strings.TrimPrefix(line, "id: ")
			s.ids[id]++
			s.lastID = id
		}
		s.mu.Unlock()
	}
}

// waitTopics waits until every topic was received at least once. The outbox
// was drained before this is called: only the delivery through the bus and
// the connection is pending, which under load (parallel packages, -race)
// may take a few seconds and a reconnection.
func (s *streamRecorder) waitTopics(t *testing.T, topics ...string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
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
			s.mu.Lock()
			rc := s.reconnects
			s.mu.Unlock()
			t.Fatalf("SSE topics never received: %v (got %v, %d reconnections)", missing, s.snapshotTopics(), rc)
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
