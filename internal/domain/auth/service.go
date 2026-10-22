// Package auth implements staff authentication and authorization: Argon2id
// passwords, cookie sessions (only token hashes are stored), API keys, TOTP
// and the role/permission matrix of spec 16.1.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/store"
)

// Options configure the Service.
type Options struct {
	// SessionTTL is the sliding idle lifetime of a session (default 7 days).
	SessionTTL time.Duration
	// SessionMaxAge caps the absolute lifetime of a session (default 30 days).
	SessionMaxAge time.Duration
	// RequireTOTPForOwners forces owners to enrol TOTP (ARENILE_REQUIRE_TOTP_FOR_OWNERS).
	RequireTOTPForOwners bool
	// TOTPIssuer is the issuer shown in authenticator apps.
	TOTPIssuer string
	// Argon2 overrides the password hashing parameters (tests).
	Argon2 *Argon2Params
}

// Service is the authentication service.
type Service struct {
	db       bun.IDB
	clock    clock.Clock
	keys     *crypto.KeyRing
	hasher   *Hasher
	throttle *LoginThrottle
	audit    *audit.Writer
	log      *slog.Logger
	opts     Options
	dummy    string
}

// NewService builds the service. keys may be nil (TOTP is then unavailable).
func NewService(db bun.IDB, c clock.Clock, keys *crypto.KeyRing, aw *audit.Writer, log *slog.Logger, opts Options) (*Service, error) {
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 7 * 24 * time.Hour
	}
	if opts.SessionMaxAge <= 0 {
		opts.SessionMaxAge = 30 * 24 * time.Hour
	}
	if opts.TOTPIssuer == "" {
		opts.TOTPIssuer = "Arenile"
	}
	params := DefaultArgon2Params
	if opts.Argon2 != nil {
		params = *opts.Argon2
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	h := NewHasher(params)
	dummy, err := h.Hash("arenile-timing-equalizer")
	if err != nil {
		return nil, err
	}
	return &Service{db: db, clock: c, keys: keys, hasher: h, throttle: NewLoginThrottle(c), audit: aw, log: log, opts: opts, dummy: dummy}, nil
}

// Hasher exposes the password hasher (CLI).
func (s *Service) Hasher() *Hasher { return s.hasher }

// SessionTTL returns the configured session lifetime (cookie Max-Age).
func (s *Service) SessionTTL() time.Duration { return s.opts.SessionTTL }

// LoginInput is a login attempt.
type LoginInput struct {
	Email     string
	Password  string
	TOTPCode  string
	IP        string
	UserAgent string
	// PreviousToken is the session cookie presented with the request, if
	// any: it is revoked so that the token always rotates at login.
	PreviousToken string
}

// LoginResult is a successful login.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	Principal *Principal
}

// Login authenticates a user and creates a new session.
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	email := store.NormalizeEmail(in.Email)
	if email == "" || in.Password == "" || len(in.Password) > 1024 || len(email) > 254 {
		return nil, ErrInvalidCredentials
	}
	if ok, retry := s.throttle.Check(in.IP, email); !ok {
		return nil, &RateLimitError{RetryAfter: retry}
	}
	user, err := store.GetUserByEmail(ctx, s.db, email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("auth: login: %w", err)
		}
		// Equalise timing with the existing-user path.
		_, _ = s.hasher.Verify(in.Password, s.dummy)
		s.throttle.Failure(email)
		return nil, ErrInvalidCredentials
	}
	ok, err := s.hasher.Verify(in.Password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("auth: verify password: %w", err)
	}
	if !ok || !user.Active {
		s.throttle.Failure(email)
		s.recordFailure(ctx, user.ID, in.IP, "password")
		return nil, ErrInvalidCredentials
	}
	var totpStep int64
	if user.TOTPEnabled() {
		if in.TOTPCode == "" {
			return nil, ErrTOTPRequired
		}
		secret, err := s.decryptTOTP(user)
		if err != nil {
			return nil, err
		}
		step, valid := VerifyTOTP(secret, in.TOTPCode, s.clock.Now())
		if !valid || step <= user.TOTPLastStep {
			s.throttle.Failure(email)
			s.recordFailure(ctx, user.ID, in.IP, "totp")
			return nil, ErrInvalidTOTP
		}
		totpStep = step
	}

	token, err := NewSessionToken()
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	sess := &store.Session{
		ID:         store.NewID(),
		TokenHash:  HashToken(token),
		UserID:     user.ID,
		ExpiresAt:  now.Add(s.opts.SessionTTL),
		LastSeenAt: now,
		IP:         truncate(in.IP, 64),
		UserAgent:  truncate(in.UserAgent, 255),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	err = store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if totpStep > 0 {
			fresh, err := store.AdvanceUserTOTPStep(ctx, tx, user.ID, totpStep, now)
			if err != nil {
				return err
			}
			if !fresh {
				return ErrInvalidTOTP
			}
		}
		if in.PreviousToken != "" {
			if err := store.DeleteSessionByTokenHash(ctx, tx, HashToken(in.PreviousToken)); err != nil {
				return err
			}
		}
		if err := store.InsertSession(ctx, tx, sess); err != nil {
			return err
		}
		if err := store.UpdateUserLogin(ctx, tx, user.ID, now); err != nil {
			return err
		}
		if s.hasher.NeedsRehash(user.PasswordHash) {
			if h, err := s.hasher.Hash(in.Password); err == nil {
				if err := store.UpdateUserPasswordHash(ctx, tx, user.ID, h, now); err != nil {
					return err
				}
			}
		}
		return s.audit.Record(ctx, tx, audit.Entry{
			Actor: audit.Actor{Type: audit.ActorUser, ID: user.ID, IP: in.IP}, Action: "auth.login",
			EntityType: "user", EntityID: user.ID,
		})
	})
	if err != nil {
		if errors.Is(err, ErrInvalidTOTP) {
			return nil, err
		}
		return nil, fmt.Errorf("auth: create session: %w", err)
	}
	s.throttle.Success(email)
	user.LastLoginAt = &now
	p, err := s.userPrincipal(ctx, user, sess.ID)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Token: token, ExpiresAt: sess.ExpiresAt, Principal: p}, nil
}

func (s *Service) recordFailure(ctx context.Context, userID, ip, reason string) {
	err := s.audit.Record(ctx, s.db, audit.Entry{
		Actor: audit.Actor{Type: audit.ActorUser, ID: userID, IP: ip}, Action: "auth.login_failed",
		EntityType: "user", EntityID: userID, Diff: map[string]string{"reason": reason},
	})
	if err != nil {
		s.log.WarnContext(ctx, "audit login failure", slog.String("error", err.Error()))
	}
}

// Logout revokes the session of token.
func (s *Service) Logout(ctx context.Context, token string, actor audit.Actor) error {
	if token == "" {
		return nil
	}
	return store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.DeleteSessionByTokenHash(ctx, tx, HashToken(token)); err != nil {
			return err
		}
		if actor.ID == "" {
			return nil
		}
		return s.audit.Record(ctx, tx, audit.Entry{Actor: actor, Action: "auth.logout", EntityType: "user", EntityID: actor.ID})
	})
}

// AuthenticateSession resolves a session token to its principal and slides
// the session expiry.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (*Principal, error) {
	if token == "" || len(token) > 128 {
		return nil, ErrUnauthenticated
	}
	now := s.clock.Now()
	sess, err := store.GetSessionByTokenHash(ctx, s.db, HashToken(token), now)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("auth: session: %w", err)
	}
	if now.Sub(sess.CreatedAt) > s.opts.SessionMaxAge {
		_ = store.DeleteSessionByTokenHash(ctx, s.db, sess.TokenHash)
		return nil, ErrUnauthenticated
	}
	user, err := store.GetUser(ctx, s.db, sess.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("auth: session user: %w", err)
	}
	if !user.Active {
		return nil, ErrUnauthenticated
	}
	if now.Sub(sess.LastSeenAt) > 5*time.Minute {
		if err := store.TouchSession(ctx, s.db, sess.ID, now, now.Add(s.opts.SessionTTL)); err != nil {
			s.log.WarnContext(ctx, "touch session", slog.String("error", err.Error()))
		}
	}
	return s.userPrincipal(ctx, user, sess.ID)
}

// AuthenticateAPIKey resolves an API key to its principal.
func (s *Service) AuthenticateAPIKey(ctx context.Context, key string) (*Principal, error) {
	prefix, ok := ParseAPIKey(key)
	if !ok {
		return nil, ErrUnauthenticated
	}
	k, err := store.GetActiveAPIKeyByPrefix(ctx, s.db, prefix)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("auth: api key: %w", err)
	}
	if !constantTimeEqual(HashToken(key), k.KeyHash) {
		return nil, ErrUnauthenticated
	}
	es, err := store.SystemEstablishmentScope(ctx, s.db, k.EstablishmentID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	now := s.clock.Now()
	if k.LastUsedAt == nil || now.Sub(*k.LastUsedAt) > time.Minute {
		if err := store.TouchAPIKey(ctx, s.db, k.ID, now); err != nil {
			s.log.WarnContext(ctx, "touch api key", slog.String("error", err.Error()))
		}
	}
	scopes := make([]Permission, 0, len(k.Scopes))
	for _, sc := range k.Scopes {
		if p := Permission(sc); p.Valid() {
			scopes = append(scopes, p)
		}
	}
	return &Principal{
		Kind: PrincipalAPIKey, APIKeyID: k.ID, APIKeyName: k.Name, APIKeyPrefix: DisplayPrefix(k.Prefix),
		APIKeyEstablishmentID: es.ID(), APIKeyOrganizationID: es.OrganizationID(), Scopes: scopes,
	}, nil
}

func (s *Service) userPrincipal(ctx context.Context, user *store.User, sessionID string) (*Principal, error) {
	ms, err := store.ListUserMemberships(ctx, s.db, user.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: memberships: %w", err)
	}
	p := &Principal{
		Kind: PrincipalUser, UserID: user.ID, Email: user.Email, Name: user.Name, Locale: user.Locale,
		TOTPEnabled: user.TOTPEnabled(), LastLoginAt: user.LastLoginAt, SessionID: sessionID,
	}
	for _, m := range ms {
		g := Grant{OrganizationID: m.OrganizationID, Role: Role(m.Role)}
		if m.EstablishmentID != nil {
			g.EstablishmentID = *m.EstablishmentID
		}
		if g.Role.Valid() {
			p.Grants = append(p.Grants, g)
		}
	}
	p.TOTPEnrolmentRequired = s.opts.RequireTOTPForOwners && !p.TOTPEnabled && p.HasRole(RoleOwner)
	return p, nil
}

// ---- TOTP ---------------------------------------------------------------------

func (s *Service) decryptTOTP(u *store.User) (string, error) {
	if s.keys == nil {
		return "", ErrNoSecretKey
	}
	if u.TOTPSecret == nil {
		return "", ErrTOTPNotEnrolled
	}
	b, err := s.keys.Decrypt(*u.TOTPSecret, []byte(u.ID))
	if err != nil {
		return "", fmt.Errorf("auth: decrypt totp: %w", err)
	}
	return string(b), nil
}

// TOTPEnrolment is the data needed to configure an authenticator app.
type TOTPEnrolment struct {
	Secret     string
	OTPAuthURL string
}

// BeginTOTPEnrolment generates a pending TOTP secret for the current user.
func (s *Service) BeginTOTPEnrolment(ctx context.Context, p *Principal, ip string) (*TOTPEnrolment, error) {
	if p == nil || p.Kind != PrincipalUser {
		return nil, ErrForbidden
	}
	if s.keys == nil {
		return nil, ErrNoSecretKey
	}
	user, err := store.GetUser(ctx, s.db, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: totp user: %w", err)
	}
	if user.TOTPEnabled() {
		return nil, ErrTOTPAlreadyEnabled
	}
	secret, err := NewTOTPSecret()
	if err != nil {
		return nil, err
	}
	enc, err := s.keys.Encrypt([]byte(secret), []byte(user.ID))
	if err != nil {
		return nil, fmt.Errorf("auth: encrypt totp: %w", err)
	}
	now := s.clock.Now()
	err = store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.SetUserTOTP(ctx, tx, user.ID, &enc, nil, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{Actor: p.Actor(ip), Action: "auth.totp_enrolment_started", EntityType: "user", EntityID: user.ID})
	})
	if err != nil {
		return nil, fmt.Errorf("auth: begin totp: %w", err)
	}
	return &TOTPEnrolment{Secret: secret, OTPAuthURL: TOTPURL(s.opts.TOTPIssuer, user.Email, secret)}, nil
}

// ConfirmTOTP activates the pending TOTP secret if code is valid.
func (s *Service) ConfirmTOTP(ctx context.Context, p *Principal, code, ip string) error {
	if p == nil || p.Kind != PrincipalUser {
		return ErrForbidden
	}
	user, err := store.GetUser(ctx, s.db, p.UserID)
	if err != nil {
		return fmt.Errorf("auth: totp user: %w", err)
	}
	if user.TOTPEnabled() {
		return ErrTOTPAlreadyEnabled
	}
	secret, err := s.decryptTOTP(user)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	step, ok := VerifyTOTP(secret, code, now)
	if !ok {
		return ErrInvalidTOTP
	}
	return store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.SetUserTOTP(ctx, tx, user.ID, user.TOTPSecret, &now, now); err != nil {
			return err
		}
		if _, err := store.AdvanceUserTOTPStep(ctx, tx, user.ID, step, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{Actor: p.Actor(ip), Action: "auth.totp_enabled", EntityType: "user", EntityID: user.ID})
	})
}

// ---- API keys -------------------------------------------------------------------

// CreateAPIKeyInput describes a new API key.
type CreateAPIKeyInput struct {
	EstablishmentID string
	Name            string
	Scopes          []Permission
	IP              string
}

// CreateAPIKey creates an API key; the plaintext key is returned only once.
func (s *Service) CreateAPIKey(ctx context.Context, p *Principal, in CreateAPIKeyInput) (string, *store.APIKey, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 || len(in.Scopes) == 0 {
		return "", nil, ErrInvalidInput
	}
	scopes := make([]string, 0, len(in.Scopes))
	for _, sc := range in.Scopes {
		if !sc.Valid() {
			return "", nil, fmt.Errorf("%w: unknown scope %q", ErrInvalidInput, sc)
		}
		if !slices.Contains(scopes, string(sc)) {
			scopes = append(scopes, string(sc))
		}
	}
	es, err := p.Authorize(ctx, s.db, in.EstablishmentID, PermUsersManage)
	if err != nil {
		return "", nil, err
	}
	key, prefix, err := NewAPIKey()
	if err != nil {
		return "", nil, err
	}
	now := s.clock.Now()
	row := &store.APIKey{ID: store.NewID(), Name: name, Prefix: prefix, KeyHash: HashToken(key), Scopes: scopes, CreatedAt: now, UpdatedAt: now}
	if p.Kind == PrincipalUser {
		row.CreatedByUserID = &p.UserID
	}
	err = store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertAPIKey(ctx, tx, es, row); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: "api_key.created",
			EntityType: "api_key", EntityID: row.ID, Diff: map[string]any{"name": name, "scopes": scopes, "prefix": DisplayPrefix(prefix)},
		})
	})
	if err != nil {
		return "", nil, fmt.Errorf("auth: create api key: %w", err)
	}
	return key, row, nil
}

// ---- users (CLI) ----------------------------------------------------------------

// CreateOwnerInput describes the first owner created from the CLI.
type CreateOwnerInput struct {
	Email            string
	Name             string
	Password         string
	OrganizationName string
	Locale           string
}

// CreateOwnerResult reports what was created.
type CreateOwnerResult struct {
	User                *store.User
	Organization        *store.Organization
	OrganizationCreated bool
}

// CreateOwner creates a user with an organization-wide owner membership,
// creating the organization (by name) if it does not exist.
func (s *Service) CreateOwner(ctx context.Context, in CreateOwnerInput) (*CreateOwnerResult, error) {
	email := store.NormalizeEmail(in.Email)
	name := strings.TrimSpace(in.Name)
	orgName := strings.TrimSpace(in.OrganizationName)
	if !looksLikeEmail(email) || name == "" || orgName == "" {
		return nil, fmt.Errorf("%w: email, name and organization are required", ErrInvalidInput)
	}
	if err := ValidatePassword(in.Password); err != nil {
		return nil, err
	}
	locale := in.Locale
	if locale == "" {
		locale = "it"
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	res := &CreateOwnerResult{}
	err = store.RunInTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if _, err := store.GetUserByEmail(ctx, tx, email); err == nil {
			return ErrUserExists
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		org, err := store.FindOrganizationByName(ctx, tx, orgName)
		if errors.Is(err, store.ErrNotFound) {
			org = &store.Organization{ID: store.NewID(), Name: orgName, CreatedAt: now, UpdatedAt: now}
			if err := store.InsertOrganization(ctx, tx, org); err != nil {
				return err
			}
			res.OrganizationCreated = true
		} else if err != nil {
			return err
		}
		user := &store.User{ID: store.NewID(), Email: email, PasswordHash: hash, Name: name, Locale: locale, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := store.InsertUser(ctx, tx, user); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrUserExists
			}
			return err
		}
		m := &store.Membership{ID: store.NewID(), UserID: user.ID, OrganizationID: org.ID, Role: string(RoleOwner), CreatedAt: now, UpdatedAt: now}
		if err := store.InsertMembership(ctx, tx, m); err != nil {
			return err
		}
		res.User, res.Organization = user, org
		return s.audit.Record(ctx, tx, audit.Entry{
			Actor: audit.SystemActor, Action: "user.created", EntityType: "user", EntityID: user.ID,
			Diff: map[string]any{"organization_id": org.ID, "role": RoleOwner, "source": "cli"},
		})
	})
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			return nil, err
		}
		return nil, fmt.Errorf("auth: create owner: %w", err)
	}
	return res, nil
}

// CleanupExpiredSessions deletes expired sessions (periodic job).
func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return store.DeleteExpiredSessions(ctx, s.db, s.clock.Now())
}

func looksLikeEmail(e string) bool {
	at := strings.LastIndexByte(e, '@')
	return at > 0 && at < len(e)-3 && strings.Contains(e[at:], ".") && !strings.ContainsAny(e, " \t\r\n") && len(e) <= 254
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
