package problem

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/http/api"
)

func TestError_M8Mapping(t *testing.T) {
	quota := &availability.ConflictError{Err: availability.ErrAgreementQuotaExceeded, Conflicts: []availability.Conflict{
		{ZoneID: "z1", Date: calendar.MustParse("2027-08-15"), Slot: calendar.AM, Reason: availability.ReasonAgreementReserved},
	}}
	tests := []struct {
		err       error
		status    int
		typ       api.ProblemType
		conflicts int
	}{
		{fmt.Errorf("allocate: %w", quota), http.StatusConflict, AgreementQuotaExceeded, 1},
		{booking.ErrAgreementNotApplicable, http.StatusUnprocessableEntity, AgreementNotApplicable, 0},
		{fmt.Errorf("swap: %w", booking.ErrSwapConflict), http.StatusConflict, SwapConflict, 0},
		{booking.ErrMoveUndoExpired, http.StatusGone, MoveUndoExpired, 0},
		{booking.ErrMoveUndoConflict, http.StatusConflict, MoveUndoConflict, 0},
		{partner.ErrPartnerInUse, http.StatusConflict, PartnerInUse, 0},
		{partner.ErrCategoryInUse, http.StatusConflict, CustomerCategoryInUse, 0},
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
			got := 0
			if p.Conflicts != nil {
				got = len(*p.Conflicts)
			}
			if got != tt.conflicts {
				t.Fatalf("conflicts %d, want %d", got, tt.conflicts)
			}
		})
	}
}
