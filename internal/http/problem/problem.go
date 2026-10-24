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

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/domain/rollover"
	"github.com/coste360/arenile/internal/domain/waitlist"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
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
	SpotUnavailable       = api.ProblemType("spot_unavailable")
	NoPrice               = api.ProblemType("no_price")
	PolicyViolation       = api.ProblemType("policy_violation")
	HoldExpired           = api.ProblemType("hold_expired")
	VersionConflict       = api.ProblemType("version_conflict")
	PreconditionRequired  = api.ProblemType("precondition_required")
	SpotHasFutureAllocs   = api.ProblemType("spot_has_future_allocations")
	NotImplemented        = api.ProblemType("not_implemented")
	PoolExhausted         = api.ProblemType("pool_exhausted")
	QuotaExceeded         = api.ProblemType("quota_exceeded")
	InvalidTransition     = api.ProblemType("invalid_transition")
	CashSessionClosed     = api.ProblemType("cash_session_closed")
	CashSessionOpen       = api.ProblemType("cash_session_already_open")
	BookingOutsidePeriod  = api.ProblemType("booking_outside_period")
	CheckoutNotFound      = api.ProblemType("checkout_not_found")
	ProviderUnavailable   = api.ProblemType("payment_provider_unavailable")
	SignatureInvalid      = api.ProblemType("webhook_signature_invalid")
	PairingCodeInvalid    = api.ProblemType("pairing_code_invalid")
	LANModeRequired       = api.ProblemType("lan_mode_required")
	LoopbackRequired      = api.ProblemType("loopback_required")
	SetupTokenInvalid     = api.ProblemType("setup_token_invalid")
	SetupCompleted        = api.ProblemType("setup_completed")
)

// M5B type slugs (renewals, waitlist, credits, season rollover).
const (
	RenewalExpired         = api.ProblemType("renewal_expired")
	WaitlistOfferExpired   = api.ProblemType("waitlist_offer_expired")
	WaitlistDisabled       = api.ProblemType("waitlist_disabled")
	CreditInsufficient     = api.ProblemType("credit_insufficient")
	SeasonRolloverConflict = api.ProblemType("season_rollover_conflict")
)

// Deferred-features type slugs (test mode, imports, guaranteed refunds; ADR 0019).
const (
	TestModePreviewRequired = api.ProblemType("test_mode_preview_required")
	ImportInvalid           = api.ProblemType("import_invalid")
	ImportAlreadyApplied    = api.ProblemType("import_already_applied")
	RefundInProgress        = api.ProblemType("refund_in_progress")
)

// ESC/POS printing type slugs (ADR 0021).
const (
	PrintingUnavailable      = api.ProblemType("printing_unavailable")
	PrinterAddressNotAllowed = api.ProblemType("printer_address_not_allowed")
	PrintRouteMissing        = api.ProblemType("print_route_missing")
)

// Email template type slugs (ADR 0022).
const (
	EmailTemplateInvalid = api.ProblemType("email_template_invalid")
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
	SpotUnavailable:       {http.StatusConflict, "Postazione non disponibile"},
	NoPrice:               {http.StatusUnprocessableEntity, "Prezzo mancante per le date richieste"},
	PolicyViolation:       {http.StatusUnprocessableEntity, "La richiesta non rispetta le regole di prenotazione"},
	HoldExpired:           {http.StatusGone, "Prenotazione provvisoria scaduta"},
	VersionConflict:       {http.StatusPreconditionFailed, "La risorsa è stata modificata da un altro utente"},
	PreconditionRequired:  {http.StatusPreconditionRequired, "Header If-Match obbligatorio"},
	SpotHasFutureAllocs:   {http.StatusConflict, "Postazione con prenotazioni future: puoi solo disattivarla"},
	NotImplemented:        {http.StatusNotImplemented, "Operazione non ancora disponibile"},
	PoolExhausted:         {http.StatusConflict, "Quantità non disponibile per le date richieste"},
	QuotaExceeded:         {http.StatusConflict, "Quota online esaurita per le date richieste"},
	InvalidTransition:     {http.StatusConflict, "Operazione non ammessa nello stato attuale della prenotazione"},
	CashSessionClosed:     {http.StatusConflict, "La sessione di cassa è chiusa"},
	CashSessionOpen:       {http.StatusConflict, "C'è già una sessione di cassa aperta"},
	BookingOutsidePeriod:  {http.StatusUnprocessableEntity, "Operazione ammessa solo nei giorni del soggiorno"},
	CheckoutNotFound:      {http.StatusNotFound, "Sessione di prenotazione non trovata o scaduta"},
	ProviderUnavailable:   {http.StatusServiceUnavailable, "Servizio di pagamento non raggiungibile, riprova tra poco"},
	SignatureInvalid:      {http.StatusBadRequest, "Firma del webhook non valida"},
	PairingCodeInvalid:    {http.StatusBadRequest, "Codice di abbinamento non valido o scaduto"},
	LANModeRequired:       {http.StatusConflict, "Attiva la modalità rete locale per abbinare altri dispositivi"},
	LoopbackRequired:      {http.StatusForbidden, "Operazione disponibile solo dal computer della cassa"},
	SetupTokenInvalid:     {http.StatusForbidden, "Collegamento di configurazione non valido: riapri Arenile dal menu"},
	SetupCompleted:        {http.StatusConflict, "Configurazione iniziale già completata"},
	// M5B.
	RenewalExpired:          {http.StatusGone, "Il termine per rinnovare l'abbonamento è scaduto"},
	WaitlistOfferExpired:    {http.StatusGone, "L'offerta dalla lista d'attesa è scaduta"},
	WaitlistDisabled:        {http.StatusConflict, "La lista d'attesa non è attiva"},
	CreditInsufficient:      {http.StatusUnprocessableEntity, "Credito del cliente insufficiente"},
	SeasonRolloverConflict:  {http.StatusConflict, "Esiste già una stagione per l'anno scelto"},
	TestModePreviewRequired: {http.StatusForbidden, "Prenotazioni in prova solo dal link di anteprima"},
	ImportInvalid:           {http.StatusUnprocessableEntity, "Il file da importare contiene errori"},
	ImportAlreadyApplied:    {http.StatusConflict, "Questo file è già stato importato"},
	RefundInProgress:        {http.StatusConflict, "C'è già un rimborso in corso per questa prenotazione"},
	// ESC/POS printing (ADR 0021).
	PrintingUnavailable:      {http.StatusConflict, "La stampa ESC/POS è disponibile solo con l'app desktop"},
	PrinterAddressNotAllowed: {http.StatusUnprocessableEntity, "L'indirizzo della stampante deve essere della rete locale"},
	EmailTemplateInvalid:     {http.StatusUnprocessableEntity, "Il testo dell'email non è valido"},
	PrintRouteMissing:        {http.StatusConflict, "Nessuna stampante configurata per questo documento"},
}

// Option customises a problem.
type Option func(*api.Problem)

// Detail sets the human readable detail.
func Detail(d string) Option { return func(p *api.Problem) { p.Detail = &d } }

// Fields attaches field validation errors.
func Fields(errs ...api.FieldError) Option { return func(p *api.Problem) { p.Errors = &errs } }

// CurrentVersion sets current_version (version_conflict).
func CurrentVersion(v int64) Option { return func(p *api.Problem) { p.CurrentVersion = &v } }

// SpotIDs sets spot_ids (spot_has_future_allocations).
func SpotIDs(ids []string) Option { return func(p *api.Problem) { p.SpotIds = &ids } }

// Violations sets violations (policy_violation).
func Violations(vs []api.PolicyViolation) Option { return func(p *api.Problem) { p.Violations = &vs } }

// Conflicts sets conflicts (spot_unavailable, pool_exhausted, quota_exceeded).
func Conflicts(cs []availability.Conflict) Option {
	return func(p *api.Problem) {
		out := make([]api.AllocationConflict, len(cs))
		for i, c := range cs {
			out[i] = api.AllocationConflict{Date: c.Date, Slot: api.Slot(c.Slot), Reason: api.AllocationConflictReason(c.Reason)}
			if c.SpotID != "" {
				out[i].SpotId = &cs[i].SpotID
			}
			if c.PoolID != "" {
				out[i].PoolId = &cs[i].PoolID
			}
			if c.ZoneID != "" {
				out[i].ZoneId = &cs[i].ZoneID
			}
		}
		p.Conflicts = &out
	}
}

// CurrentStatus sets current_status (invalid_transition).
func CurrentStatus(s string) Option {
	return func(p *api.Problem) {
		st := api.BookingStatus(s)
		p.CurrentStatus = &st
	}
}

// Period sets period (booking_outside_period): today in the establishment
// timezone and the stay.
func Period(today, startsOn, endsOn calendar.Date) Option {
	return func(p *api.Problem) {
		p.Period = &struct {
			EndsOn   api.Date `json:"ends_on"`
			StartsOn api.Date `json:"starts_on"`
			Today    api.Date `json:"today"`
		}{EndsOn: endsOn, StartsOn: startsOn, Today: today}
	}
}

// ExistingCustomerID sets existing_customer_id (conflict on createCustomer).
func ExistingCustomerID(id string) Option {
	return func(p *api.Problem) { p.ExistingCustomerId = &id }
}

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
		if id := reqctx.From(r.Context()).RequestID; id != "" {
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
	var (
		rl  *auth.RateLimitError
		ve  *configerr.ValidationError
		vc  *configerr.VersionConflictError
		sa  *configerr.SpotsAllocatedError
		pve *policy.ViolationError
		ce  *availability.ConflictError
		sue *availability.SpotUnavailableError
		te  *booking.TransitionError
		ope *booking.OutsidePeriodError
		dup *customer.DuplicateError
		tpl *notify.TemplateError
	)
	if writeTicketing(w, r, err) {
		return
	}
	switch {
	case errors.As(err, &tpl):
		fields := make([]api.FieldError, len(tpl.Fields))
		for i, f := range tpl.Fields {
			fields[i] = api.FieldError{Field: f.Field, Message: f.Message}
		}
		Write(w, r, EmailTemplateInvalid, Fields(fields...))
	case errors.As(err, &rl):
		Write(w, r, RateLimited, RetryAfter(rl.RetryAfter))
	case errors.Is(err, checkout.ErrSessionNotFound):
		Write(w, r, CheckoutNotFound)
	case errors.Is(err, renewal.ErrExpired):
		Write(w, r, RenewalExpired)
	case errors.Is(err, waitlist.ErrOfferExpired):
		Write(w, r, WaitlistOfferExpired)
	case errors.Is(err, waitlist.ErrDisabled):
		Write(w, r, WaitlistDisabled)
	case errors.Is(err, credit.ErrInsufficient):
		Write(w, r, CreditInsufficient)
	case errors.Is(err, rollover.ErrTargetExists):
		Write(w, r, SeasonRolloverConflict)
	case errors.Is(err, gateway.ErrProviderUnavailable):
		Write(w, r, ProviderUnavailable, RetryAfter(30*time.Second))
	case errors.Is(err, ext.ErrInvalidSignature):
		Write(w, r, SignatureInvalid)
	case errors.Is(err, device.ErrPairingCodeInvalid):
		Write(w, r, PairingCodeInvalid)
	case errors.Is(err, device.ErrLANModeRequired):
		Write(w, r, LANModeRequired)
	case errors.Is(err, control.ErrLoopbackRequired):
		Write(w, r, LoopbackRequired)
	case errors.Is(err, control.ErrSetupTokenInvalid):
		Write(w, r, SetupTokenInvalid)
	case errors.Is(err, control.ErrSetupCompleted):
		Write(w, r, SetupCompleted)
	case errors.Is(err, control.ErrBackupNotFound):
		Write(w, r, NotFound)
	case errors.Is(err, control.ErrBackupInProgress):
		Write(w, r, Conflict, Detail("Un backup o un ripristino è già in corso."))
	case errors.Is(err, control.ErrInvalidSettings):
		Write(w, r, ValidationFailed, Detail(err.Error()))
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
	case errors.As(err, &ve):
		fields := make([]api.FieldError, len(ve.Fields))
		for i, f := range ve.Fields {
			fields[i] = api.FieldError{Field: f.Field, Message: f.Message}
		}
		Write(w, r, ValidationFailed, Fields(fields...))
	case errors.Is(err, configerr.ErrValidation):
		Write(w, r, ValidationFailed, Detail(err.Error()))
	case errors.As(err, &vc):
		Write(w, r, VersionConflict, CurrentVersion(vc.Current))
	case errors.Is(err, configerr.ErrVersionConflict):
		Write(w, r, VersionConflict)
	case errors.Is(err, configerr.ErrPreconditionRequired):
		Write(w, r, PreconditionRequired)
	case errors.As(err, &sa):
		Write(w, r, SpotHasFutureAllocs, SpotIDs(sa.SpotIDs))
	case errors.Is(err, configerr.ErrSpotHasFutureAllocations):
		Write(w, r, SpotHasFutureAllocs)
	case errors.Is(err, configerr.ErrNotFound):
		Write(w, r, NotFound)
	case errors.As(err, &dup):
		Write(w, r, Conflict, Detail(dup.Error()), ExistingCustomerID(dup.ExistingID))
	case errors.Is(err, configerr.ErrConflict):
		Write(w, r, Conflict, Detail(err.Error()))
	case errors.Is(err, pricing.ErrNoPrice):
		Write(w, r, NoPrice, Detail(err.Error()))
	case errors.As(err, &pve):
		vs := make([]api.PolicyViolation, len(pve.Violations))
		for i, v := range pve.Violations {
			vs[i] = api.PolicyViolation{Code: string(v.Code)}
			if len(v.Params) > 0 {
				params := v.Params
				vs[i].Params = &params
			}
		}
		Write(w, r, PolicyViolation, Violations(vs))
	case errors.Is(err, configerr.ErrNotImplemented):
		Write(w, r, NotImplemented)
	case errors.As(err, &ce):
		t := SpotUnavailable
		switch {
		case errors.Is(ce.Err, availability.ErrPoolExhausted):
			t = PoolExhausted
		case errors.Is(ce.Err, availability.ErrQuotaExceeded):
			t = QuotaExceeded
		}
		Write(w, r, t, Conflicts(ce.Conflicts))
	case errors.As(err, &sue):
		cs := make([]availability.Conflict, len(sue.Conflicts))
		for i, c := range sue.Conflicts {
			cs[i] = availability.Conflict{SpotID: sue.SpotID.String(), Date: c.Date, Slot: c.Slot, Reason: availability.ReasonSpotUnavailable}
		}
		Write(w, r, SpotUnavailable, Conflicts(cs))
	case errors.Is(err, availability.ErrSpotUnavailable):
		Write(w, r, SpotUnavailable)
	case errors.Is(err, availability.ErrPoolExhausted):
		Write(w, r, PoolExhausted)
	case errors.Is(err, availability.ErrQuotaExceeded):
		Write(w, r, QuotaExceeded)
	case errors.Is(err, availability.ErrHoldExpired):
		Write(w, r, HoldExpired)
	case errors.As(err, &te):
		Write(w, r, InvalidTransition, CurrentStatus(string(te.From)))
	case errors.Is(err, booking.ErrInvalidTransition):
		Write(w, r, InvalidTransition)
	case errors.As(err, &ope):
		Write(w, r, BookingOutsidePeriod, Period(ope.Today, ope.StartsOn, ope.EndsOn))
	case errors.Is(err, booking.ErrOutsidePeriod):
		Write(w, r, BookingOutsidePeriod)
	case errors.Is(err, cash.ErrSessionClosed):
		Write(w, r, CashSessionClosed)
	case errors.Is(err, cash.ErrSessionAlreadyOpen):
		Write(w, r, CashSessionOpen)
	case errors.Is(err, payment.ErrRefundInProgress):
		Write(w, r, RefundInProgress)
	case errors.Is(err, store.ErrConflict):
		Write(w, r, Conflict)
	case ClientGone(r, err):
		// The client went away: nobody reads the response, and it is not
		// a server error (no error log, 499 in the access log).
		if log != nil {
			log.DebugContext(r.Context(), "request cancelled by the client",
				slog.String("request_id", reqctx.From(r.Context()).RequestID))
		}
		w.WriteHeader(StatusClientClosedRequest)
	default:
		if log != nil {
			log.ErrorContext(r.Context(), "request failed", slog.String("error", err.Error()),
				slog.String("request_id", reqctx.From(r.Context()).RequestID))
		}
		Write(w, r, InternalError)
	}
}
