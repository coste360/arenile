package desktop

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// GetFirstRunStatus implements GET /api/desktop/v1/first-run: whether the
// owner still has to be created (public, loopback only).
func (h *Handlers) GetFirstRunStatus(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	hasUsers, err := h.Users.AnyUser(r.Context())
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.FirstRunStatus{Required: !hasUsers})
}

// CompleteFirstRun implements POST /api/desktop/v1/first-run (spec 29.1,
// "Primo avvio"): with the one-time setup token the launcher put in the
// URL fragment, it creates the owner and the organization and logs the
// browser in (same cookies as admin.Login).
//
// Order matters: the input is validated BEFORE the token is consumed (a
// typo in the password must not burn the link), "already set up" is
// answered without touching the token, and the creation itself is
// conditional on an empty users table (CreateOwner OnlyIfNoUsers), so two
// concurrent requests can never create two owners.
func (h *Handlers) CompleteFirstRun(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	var req api.FirstRunRequest
	if !decode(w, r, &req) {
		return
	}
	if fe := validateFirstRun(req); len(fe) > 0 {
		problem.Write(w, r, problem.ValidationFailed, problem.Fields(fe...))
		return
	}
	hasUsers, err := h.Users.AnyUser(r.Context())
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if hasUsers {
		problem.Error(w, r, h.Log, control.ErrSetupCompleted)
		return
	}
	if err := h.Control.ConsumeSetupToken(req.SetupToken); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	email := string(req.Email)
	if _, err := h.Auth.CreateOwner(r.Context(), auth.CreateOwnerInput{
		Email: email, Name: req.Name, Password: req.Password, OrganizationName: req.OrganizationName,
		OnlyIfNoUsers: true, Source: "desktop_first_run",
	}); err != nil {
		if errors.Is(err, auth.ErrUsersExist) || errors.Is(err, auth.ErrUserExists) {
			err = control.ErrSetupCompleted
		}
		problem.Error(w, r, h.Log, err)
		return
	}
	info := reqctx.From(r.Context())
	res, err := h.Auth.Login(r.Context(), auth.LoginInput{
		Email: email, Password: req.Password, IP: info.ClientIP, UserAgent: r.UserAgent(),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	ttl := h.Auth.SessionTTL()
	secure := middleware.CookieSecure(r, h.SecureCookies)
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure follows the configuration (loopback HTTP on the desktop) or the TLS request
		Name: middleware.SessionCookie, Value: res.Token, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds()), Expires: res.ExpiresAt,
	})
	middleware.SetCSRFCookie(w, middleware.NewCSRFToken(), secure, ttl)
	h.Log.Info("desktop first run completed", "user_id", res.Principal.UserID)
	writeJSON(w, http.StatusCreated, admin.MeFromPrincipal(res.Principal))
}

// validateFirstRun checks the request before the token is consumed
// (pure). Messages are Italian: they are shown next to the fields.
func validateFirstRun(req api.FirstRunRequest) []api.FieldError {
	var fe []api.FieldError
	if strings.TrimSpace(req.SetupToken) == "" {
		fe = append(fe, api.FieldError{Field: "setup_token", Message: "Collegamento di configurazione mancante."})
	}
	if a, err := mail.ParseAddress(string(req.Email)); err != nil || a.Address != strings.TrimSpace(string(req.Email)) || !strings.Contains(a.Address, ".") {
		fe = append(fe, api.FieldError{Field: "email", Message: "Inserisci un indirizzo email valido."})
	}
	if n := strings.TrimSpace(req.Name); n == "" || len(n) > 200 {
		fe = append(fe, api.FieldError{Field: "name", Message: "Inserisci il tuo nome."})
	}
	if o := strings.TrimSpace(req.OrganizationName); o == "" || len(o) > 200 {
		fe = append(fe, api.FieldError{Field: "organization_name", Message: "Inserisci il nome dell'azienda."})
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		fe = append(fe, api.FieldError{Field: "password", Message: "La password deve avere almeno 10 caratteri."})
	}
	return fe
}
