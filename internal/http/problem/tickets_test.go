package problem

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/api"
)

func TestError_TicketingMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		typ    api.ProblemType
	}{
		{fmt.Errorf("sale: %w", ticketing.ErrZoneFull), http.StatusConflict, ZoneFull},
		{ticketing.ErrAssignmentConflict, http.StatusConflict, AssignmentConflict},
		{ticketing.ErrNumberConflict, http.StatusConflict, TicketNumberConflict},
		{ticketing.ErrVoided, http.StatusConflict, TicketVoided},
		{ticketing.ErrAlreadyEntered, http.StatusConflict, TicketAlreadyEntered},
		{ticketing.ErrPriceChanged, http.StatusConflict, PriceChanged},
		{ticketing.ErrNotFound, http.StatusNotFound, NotFound},
	}
	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			rec := httptest.NewRecorder()
			Error(rec, httptest.NewRequest(http.MethodPost, "/x", nil), slog.New(slog.NewTextHandler(io.Discard, nil)), tt.err)
			var p api.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tt.status || p.Type != tt.typ {
				t.Fatalf("got %d %q, want %d %q", rec.Code, p.Type, tt.status, tt.typ)
			}
		})
	}
}
