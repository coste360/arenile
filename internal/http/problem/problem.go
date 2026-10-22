// Package problem writes RFC 9457 application/problem+json responses with a
// stable "type" slug and maps domain errors to them.
package problem

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/httpctx"
	"github.com/coste360/arenile/internal/store"
)

// ContentType is the media type of problem responses.
const ContentType = "application/problem+json"

// Type slugs (mirrors the ProblemType enum of api/openapi.yaml).
const (
	BadRequest            = api.ProblemType("bad_request")
	ValidationFailed      = api.ProblemType("validation_failed")
	Unauthorized          = api.ProblemType("unauthorized")
	InvalidCredentials    = api.ProblemType("invalid_credentials")
	TOTPRequired          = api.ProblemType("totp_required")
	InvalidTOTP           = api.ProblemType("invalid_totp")
	TOTPEnrolmentRequired = api.ProblemType("totp_enrolment_required")
	TOTPAlreadyEnabled    = api.ProblemType("totp_already_enabled")
	Forbidden             = api.ProblemType("forbidden")
	CSRFFailed            = api.ProblemType("csrf_failed")
	NotFound              = api.ProblemType("not_found")
	Conflict              = api.ProblemType("conflict")
	MethodNotAllowed      = api.ProblemType("method_not_allowed")
	UnsupportedMediaType  = api.ProblemType("unsupported_media_type")
	RateLimited           = api.ProblemType("rate_limited")
	IdempotencyKeyReused  = api.ProblemType("idempotency_key_reused")
	IdempotencyInProgress = api.ProblemType("idempotency_in_progress")
	InternalError         = api.ProblemType("internal_error")
)

type meta struct {
	status int
	title  string
}

// catalog holds the HTTP status and the Italian (source language) title of
// each type. Clients localise using the type slug.
var catalog = map[api.ProblemType]meta{
	BadRequest:            {http.StatusBadRequest, "Richiesta non valida"},
	ValidationFailed:      {http.StatusBadRequest, "Dati non validi"},
	Unauthorized:          {http.StatusUnauthorized, "Autenticazione richiesta"},
	InvalidCredentials:    {http.StatusUnauthorized, "Email o password non corretti"},
	TOTPRequired:          {http.StatusUnauthorized, "Codice di verifica richiesto"},
	InvalidTOTP:           {http.StatusUnauthorized, "Codice di verifica non valido"},
	TOTPEnrolmentRequired: {http.StatusForbidden, "Attiva la verifica in due passaggi per continuare"},
	TOTPAlreadyEnabled:    {http.StatusConflict, "Verifica in due passaggi già attiva"},
	Forbidden:             {http.StatusForbidden, "Permesso negato"},
	CSRFFailed:            {http.StatusForbidden, "Token CSRF mancante o non valido"},
	NotFound:              {http.StatusNotFound, "Risorsa non trovata"},
	Conflict:              {http.StatusConflict, "Conflitto con lo stato attuale"},
	MethodNotAllowed:      {http.StatusMethodNotAllowed, "Metodo non consentito"},
	UnsupportedMediaType:  {http.StatusUnsupportedMediaType, "Formato del contenuto non supportato"},
	RateLimited:           {http.StatusTooManyRequests, "Troppe richieste, riprova più tardi"},
	IdempotencyKeyReused:  {http.StatusUnprocessableEntity, "Chiave di idempotenza già usata con un'altra richiesta"},
	IdempotencyInProgress: {http.StatusConflict, "Richiesta con la stessa chiave di idempotenza in corso"},
	InternalError:         {http.StatusInternalServerError, "Errore interno"},
}

// Option customises a problem.
type Option func(*api.Problem)

// Detail sets the human readable detail.
func Detail(d string) Option { return func(p *api.Problem) { p.Detail = &d } }

// Fields attaches field validation errors.
func Fields(errs ...api.FieldError) Option { return func(p *api.Problem) { p.Errors = &errs } }

// RetryAfter sets retry_after (seconds) and the Retry-After header.
func RetryAfter(d time.Duration) Option {
	return func(p *api.Problem) {
		s := int(math.Ceil(d.Seconds()))
		if s < 1 {
			s = 1
		}
		p.RetryAfter = &s
	}
}

// New builds a problem of type t.
func New(r *http.Request, t api.ProblemType, opts ...Option) api.Problem {
	m, ok := catalog[t]
	if !ok {
		m = catalog[InternalError]
		t = InternalError
	}
	p := api.Problem{Type: t, Title: m.title, Status: m.status}
	if r != nil {
		inst := r.URL.Path
		p.Instance = &inst
		if id := httpctx.From(r.Context()).RequestID; id != "" {
			p.RequestId = &id
		}
	}
	for _, o := range opts {
		o(&p)
	}
	return p
}

// Write sends a problem of type t.
func Write(w http.ResponseWriter, r *http.Request, t api.ProblemType, opts ...Option) {
	WriteProblem(w, New(r, t, opts...))
}

// WriteProblem sends p.
func WriteProblem(w http.ResponseWriter, p api.Problem) {
	h := w.Header()
	h.Set("Content-Type", ContentType)
	h.Set("Cache-Control", "no-store")
	if p.RetryAfter != nil {
		h.Set("Retry-After", strconv.Itoa(*p.RetryAfter))
	}
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// Error maps a domain error to a problem response. Unknown errors are logged
// and reported as internal_error without details.
func Error(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var rl *auth.RateLimitError
	switch {
	case errors.As(err, &rl):
		Write(w, r, RateLimited, RetryAfter(rl.RetryAfter))
	case errors.Is(err, auth.ErrUnauthenticated):
		Write(w, r, Unauthorized)
	case errors.Is(err, auth.ErrInvalidCredentials):
		Write(w, r, InvalidCredentials)
	case errors.Is(err, auth.ErrTOTPRequired):
		Write(w, r, TOTPRequired)
	case errors.Is(err, auth.ErrInvalidTOTP):
		Write(w, r, InvalidTOTP)
	case errors.Is(err, auth.ErrTOTPEnrolmentNeeded):
		Write(w, r, TOTPEnrolmentRequired)
	case errors.Is(err, auth.ErrTOTPAlreadyEnabled):
		Write(w, r, TOTPAlreadyEnabled)
	case errors.Is(err, auth.ErrTOTPNotEnrolled):
		Write(w, r, Conflict, Detail("Avvia prima l'attivazione della verifica in due passaggi."))
	case errors.Is(err, auth.ErrForbidden):
		Write(w, r, Forbidden)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, establishment.ErrNotFound):
		Write(w, r, NotFound)
	case errors.Is(err, store.ErrInvalidCursor), errors.Is(err, establishment.ErrInvalidCursor):
		Write(w, r, BadRequest, Detail("Cursore di paginazione non valido."))
	case errors.Is(err, auth.ErrInvalidInput), errors.Is(err, auth.ErrWeakPassword), errors.Is(err, establishment.ErrInvalidSlug):
		Write(w, r, ValidationFailed, Detail(err.Error()))
	case errors.Is(err, store.ErrConflict):
		Write(w, r, Conflict)
	default:
		if log != nil {
			log.ErrorContext(r.Context(), "request failed", slog.String("error", err.Error()),
				slog.String("request_id", httpctx.From(r.Context()).RequestID))
		}
		Write(w, r, InternalError)
	}
}
