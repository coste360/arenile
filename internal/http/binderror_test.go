package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coste360/arenile/internal/http/api"
)

func TestBindErrorHandler_Status(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"missing if-match", &api.RequiredHeaderError{ParamName: "If-Match", Err: errors.New("missing")}, http.StatusPreconditionRequired},
		{"missing other header", &api.RequiredHeaderError{ParamName: "X-Other", Err: errors.New("missing")}, http.StatusBadRequest},
		{"invalid if-match", &api.InvalidParamFormatError{ParamName: "If-Match", Err: errors.New("bad")}, http.StatusBadRequest},
		{"generic", errors.New("boom"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/x", nil)
			bindErrorHandler(rec, req, tc.err)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("content type = %q", ct)
			}
		})
	}
}
