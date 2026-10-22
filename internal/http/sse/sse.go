// Package sse serves Server-Sent Events streams of domain events to the
// admin panel (spec 21.1).
package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/pkg/ext"
)

// DefaultHeartbeat is the interval between keep-alive comments.
const DefaultHeartbeat = 25 * time.Second

// Server streams events of one establishment.
type Server struct {
	Bus       ext.EventBus
	Heartbeat time.Duration
	Log       *slog.Logger
}

// Serve streams the events of establishmentID until the client disconnects
// or ctx is cancelled. Authorization must be checked by the caller.
func (s *Server) Serve(w http.ResponseWriter, r *http.Request, establishmentID string) error {
	rc := http.NewResponseController(w)
	// Streams are long-lived: lift the server write deadline.
	_ = rc.SetWriteDeadline(time.Time{})

	ctx := r.Context()
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
	if _, err := fmt.Fprint(w, "retry: 5000\n: connected\n\n"); err != nil {
		return nil
	}
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("sse: streaming not supported: %w", err)
	}

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
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
			if err := rc.Flush(); err != nil {
				return nil
			}
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := writeEvent(w, ev); err != nil {
				return nil
			}
			if err := rc.Flush(); err != nil {
				return nil
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, ev ext.Event) error {
	b, err := json.Marshal(event.ToStreamMessage(ev))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", sanitize(ev.ID), sanitize(ev.Type), b)
	return err
}

func sanitize(s string) string {
	return strings.NewReplacer("\n", "", "\r", "").Replace(s)
}
