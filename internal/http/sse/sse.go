package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/pkg/ext"
)

const (
	// DefaultHeartbeat is the interval between keep-alive comments.
	DefaultHeartbeat = 25 * time.Second
	// DefaultWriteTimeout bounds every write to a client: a client that does
	// not drain its socket within it is disconnected.
	DefaultWriteTimeout = 10 * time.Second
	// RetryMillis is the reconnection delay suggested to EventSource.
	RetryMillis = 5000
)

// Disconnect reasons (log attribute "reason").
const (
	reasonLagging    = "lagging"
	reasonWriteError = "write_error"
)

// Server streams events of one establishment.
type Server struct {
	Bus       ext.EventBus
	Heartbeat time.Duration
	Log       *slog.Logger
	// WriteTimeout bounds each write (DefaultWriteTimeout when zero).
	WriteTimeout time.Duration
	// Replay answers Last-Event-ID resumes. When nil, Bus is used if it
	// implements Replayer (ReplayBus); otherwise resumes start live.
	Replay Replayer

	disconnected atomic.Int64
}

// Disconnected returns how many slow clients were disconnected.
func (s *Server) Disconnected() int64 { return s.disconnected.Load() }

// Serve streams the events of establishmentID until the client disconnects,
// falls behind, or ctx is cancelled. Authorization must be checked by the
// caller (see the admin StreamEvents handler).
func (s *Server) Serve(w http.ResponseWriter, r *http.Request, establishmentID string) error {
	rc := http.NewResponseController(w)
	// Streams are long-lived: lift the server write deadline; each write
	// gets its own deadline below.
	_ = rc.SetWriteDeadline(time.Time{})

	ctx := r.Context()
	// Subscribe before reading the history: an event published in between
	// is in both and deduplicated by id, never lost.
	ch, cancel, err := s.Bus.Subscribe(ctx, establishmentID)
	if err != nil {
		return fmt.Errorf("sse: subscribe: %w", err)
	}
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	st := &stream{s: s, w: w, rc: rc, est: establishmentID, done: ctx.Done}
	if err := st.write(fmt.Sprintf("retry: %d\n: connected\n\n", RetryMillis)); err != nil {
		return nil
	}
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("sse: streaming not supported: %w", err)
	}

	seen := st.replay(r.Header.Get("Last-Event-ID"))

	hb := s.Heartbeat
	if hb <= 0 {
		hb = DefaultHeartbeat
	}
	ticker := time.NewTicker(hb)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := st.write(": ping\n\n"); err != nil {
				return nil
			}
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			// A buffer that was full before this receive means the broker may have dropped
			// messages for this client: end the stream so that it
			// reconnects (and resumes) instead of showing stale data.
			if c := cap(ch); c > 1 && len(ch) >= c-1 {
				st.drop(reasonLagging)
				return nil
			}
			if _, dup := seen[ev.ID]; dup {
				delete(seen, ev.ID)
				continue
			}
			if err := st.event(ev); err != nil {
				return nil
			}
		}
	}
}

// stream is one client connection.
type stream struct {
	s    *Server
	w    http.ResponseWriter
	rc   *http.ResponseController
	est  string
	done func() <-chan struct{}
}

// replay sends the events after lastID, when a replayer knows it, and
// returns their ids for deduplication against the live channel.
func (st *stream) replay(lastID string) map[string]struct{} {
	lastID = sanitize(lastID)
	if lastID == "" {
		return nil
	}
	rp := st.s.Replay
	if rp == nil {
		rp, _ = st.s.Bus.(Replayer)
	}
	if rp == nil {
		return nil
	}
	evs, ok := rp.Since(st.est, lastID)
	if !ok {
		_ = st.write(": resume unavailable\n\n")
		return nil
	}
	seen := make(map[string]struct{}, len(evs))
	for _, ev := range evs {
		if err := st.event(ev); err != nil {
			return seen
		}
		seen[ev.ID] = struct{}{}
	}
	return seen
}

// event writes one message: the SSE event name is the stream topic
// (booking.changed, allocation.changed, ...), the domain type is in data,
// and data never carries personal data (event.ToStreamMessage).
func (st *stream) event(ev ext.Event) error {
	msg := event.ToStreamMessage(ev)
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return st.write(fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", sanitize(ev.ID), sanitize(msg.Topic), b))
}

// write sends s and flushes it within the write timeout; a failure ends
// the stream.
func (st *stream) write(s string) error {
	timeout := st.s.WriteTimeout
	if timeout <= 0 {
		timeout = DefaultWriteTimeout
	}
	_ = st.rc.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := fmt.Fprint(st.w, s); err != nil {
		st.drop(reasonWriteError)
		return err
	}
	if err := st.rc.Flush(); err != nil {
		st.drop(reasonWriteError)
		return err
	}
	return nil
}

// drop records the disconnection of a client that fell behind. A write
// error after the client went away is a normal end, not a drop.
func (st *stream) drop(reason string) {
	select {
	case <-st.done():
		return
	default:
	}
	st.s.disconnected.Add(1)
	if st.s.Log != nil {
		st.s.Log.Debug("sse client disconnected", slog.String("establishment_id", st.est), slog.String("reason", reason))
	}
}

func sanitize(s string) string {
	return strings.NewReplacer("\n", "", "\r", "").Replace(s)
}
