package cash

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Cash registers (ADR 0019, INV-73): an establishment has one or more tills.
// The default register (code main, "Cassa principale") always exists: the
// migration 00009 creates it for the establishments of that time, the
// service creates it lazily (EnsureDefaultRegister) for newer ones. Code is
// stable and equals CashSession.Register; at most one open session per
// register (unique partial index cash_sessions_register_open_uidx).

// Errors of the registers.
var (
	// ErrRegisterNotFound: unknown register (404).
	ErrRegisterNotFound = fmt.Errorf("%w: cash register", configerr.ErrNotFound)
	// ErrDefaultRegister: the default register cannot be deactivated or deleted (409 conflict).
	ErrDefaultRegister = fmt.Errorf("%w: la cassa predefinita non si può disattivare né eliminare", configerr.ErrConflict)
	// ErrRegisterHasSessions: a register with history can only be deactivated (409 conflict).
	ErrRegisterHasSessions = fmt.Errorf("%w: la cassa ha sessioni nello storico, puoi solo disattivarla", configerr.ErrConflict)
	// ErrRegisterCodeTaken: duplicate code (409 conflict).
	ErrRegisterCodeTaken = fmt.Errorf("%w: esiste già una cassa con questo codice", configerr.ErrConflict)
)

const (
	maxRegisterName = 60
	maxSortOrder    = 10_000
)

// Audit actions of the registers.
const (
	AuditRegisterCreated = "cash_register.created"
	AuditRegisterUpdated = "cash_register.updated"
	AuditRegisterDeleted = "cash_register.deleted"
)

// RegisterView is a register with its open session (if any).
type RegisterView struct {
	store.CashRegister
	OpenSessionID *string
}

// EnsureDefaultRegister returns the default register of es, creating it when
// missing (establishments created after 00009). Concurrent calls create it
// once (unique index on (establishment_id, code)).
func (s *Service) EnsureDefaultRegister(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*store.CashRegister, error) {
	r, err := store.GetCashRegisterByCode(ctx, db, es, store.DefaultCashRegister)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("cash: %w", err)
	}
	now := s.now()
	r = &store.CashRegister{
		ID: store.NewID(), Code: store.DefaultCashRegister, Name: store.DefaultCashRegisterName,
		Active: true, IsDefault: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertCashRegisterIfAbsent(ctx, db, es, r); err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	r, err = store.GetCashRegisterByCode(ctx, db, es, store.DefaultCashRegister)
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	return r, nil
}

// ListRegisters returns the registers with their open session. Requires
// auth.PermCashOperate or auth.PermReportsRead.
func (s *Service) ListRegisters(ctx context.Context, p *auth.Principal, establishmentID string, includeInactive bool) ([]RegisterView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if _, err := s.EnsureDefaultRegister(ctx, s.d.DB, es); err != nil {
		return nil, err
	}
	regs, err := store.ListCashRegisters(ctx, s.d.DB, es, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	open, err := store.OpenCashSessionsByRegister(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	out := make([]RegisterView, 0, len(regs))
	for _, r := range regs {
		v := RegisterView{CashRegister: r}
		if id, ok := open[r.ID]; ok {
			v.OpenSessionID = &id
		}
		out = append(out, v)
	}
	return out, nil
}

// GetRegister returns one register. Same permissions as ListRegisters.
func (s *Service) GetRegister(ctx context.Context, p *auth.Principal, establishmentID, id string) (*RegisterView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	return s.registerView(ctx, s.d.DB, es, id)
}

func (s *Service) registerView(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*RegisterView, error) {
	r, err := store.GetCashRegister(ctx, db, es, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrRegisterNotFound
		}
		return nil, fmt.Errorf("cash: %w", err)
	}
	v := &RegisterView{CashRegister: *r}
	cs, err := store.GetOpenCashSessionOfRegister(ctx, db, es, r.ID)
	switch {
	case err == nil:
		v.OpenSessionID = &cs.ID
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("cash: %w", err)
	}
	return v, nil
}

// RegisterInput creates or updates a register. Code is used only on create
// (default: derived from the name); nil Active/SortOrder keep the current
// value (create: active, 0).
type RegisterInput struct {
	Code      *string
	Name      string
	Active    *bool
	SortOrder *int
}

func (in *RegisterInput) validate(create bool) error {
	in.Name = strings.TrimSpace(in.Name)
	var ve configerr.ValidationError
	if in.Name == "" {
		ve.Add("name", "Indica il nome della cassa.")
	} else if len([]rune(in.Name)) > maxRegisterName {
		ve.Add("name", fmt.Sprintf("Massimo %d caratteri.", maxRegisterName))
	}
	if in.SortOrder != nil && (*in.SortOrder < 0 || *in.SortOrder > maxSortOrder) {
		ve.Add("sort_order", fmt.Sprintf("L'ordine deve essere tra 0 e %d.", maxSortOrder))
	}
	if create && in.Code != nil {
		c := strings.TrimSpace(*in.Code)
		in.Code = &c
		if c != "" && !registerRe.MatchString(c) {
			ve.Add("code", "Codice cassa: lettere minuscole, cifre, - e _ (massimo 40).")
		}
	}
	return ve.OrNil()
}

// RegisterCode derives a register code from a name: lower-case ASCII
// letters and digits, other runs become "-", at most 40 characters;
// "cassa" when nothing is left. Pure function.
func RegisterCode(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if plain, ok := accents[r]; ok {
			r = plain
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	code := strings.TrimRight(b.String(), "-")
	if len(code) > 40 {
		code = strings.TrimRight(code[:40], "-")
	}
	if code == "" {
		return "cassa"
	}
	return code
}

// accents folds the accented letters of it/de/fr names to ASCII.
var accents = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ä': 'a', 'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ì': 'i', 'í': 'i', 'î': 'i',
	'ï': 'i', 'ò': 'o', 'ó': 'o', 'ô': 'o', 'ö': 'o', 'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ç': 'c', 'ß': 's',
	'ñ': 'n', 'ÿ': 'y',
}

// CreateRegister creates a register (auth.PermEstablishmentConfigure). A
// given code already used is ErrRegisterCodeTaken; a derived code gets a
// numeric suffix until free.
func (s *Service) CreateRegister(ctx context.Context, p *auth.Principal, ip, establishmentID string, in RegisterInput) (*RegisterView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if err := in.validate(true); err != nil {
		return nil, err
	}
	if _, err := s.EnsureDefaultRegister(ctx, s.d.DB, es); err != nil {
		return nil, err
	}
	explicit := in.Code != nil && *in.Code != ""
	base := RegisterCode(in.Name)
	if explicit {
		base = *in.Code
	}
	now := s.now()
	r := &store.CashRegister{
		ID: store.NewID(), Name: in.Name, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if in.Active != nil {
		r.Active = *in.Active
	}
	if in.SortOrder != nil {
		r.SortOrder = *in.SortOrder
	}
	for attempt := 1; ; attempt++ {
		r.Code = base
		if attempt > 1 {
			suffix := "-" + strconv.Itoa(attempt)
			r.Code = strings.TrimRight(base[:min(len(base), 40-len(suffix))], "-") + suffix
		}
		err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			if err := store.InsertCashRegister(ctx, tx, es, r); err != nil {
				return err
			}
			return s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: AuditRegisterCreated,
				EntityType: "cash_register", EntityID: r.ID,
				Diff: map[string]any{"code": r.Code, "name": r.Name, "active": r.Active, "sort_order": r.SortOrder},
			})
		})
		if err == nil {
			return &RegisterView{CashRegister: *r}, nil
		}
		if !errors.Is(err, store.ErrConflict) && !store.IsUniqueViolation(err) {
			return nil, fmt.Errorf("cash: %w", err)
		}
		if explicit || attempt >= 50 {
			return nil, ErrRegisterCodeTaken
		}
	}
}

// UpdateRegister changes name, sort order and active state
// (auth.PermEstablishmentConfigure); the code is immutable. The default
// register cannot be deactivated (ErrDefaultRegister); a register with an
// open session cannot be deactivated (ErrSessionAlreadyOpen).
func (s *Service) UpdateRegister(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in RegisterInput) (*RegisterView, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if err := in.validate(false); err != nil {
		return nil, err
	}
	var out *RegisterView
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetCashRegister(ctx, tx, es, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrRegisterNotFound
			}
			return fmt.Errorf("cash: %w", err)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		next := *cur
		next.Name = in.Name
		if in.Active != nil {
			next.Active = *in.Active
		}
		if in.SortOrder != nil {
			next.SortOrder = *in.SortOrder
		}
		if cur.IsDefault && !next.Active {
			return ErrDefaultRegister
		}
		old := cur.Version
		next.Version, next.UpdatedAt = old+1, s.now()
		// The UPDATE takes the row lock first: an opening in flight either
		// committed (its session is seen below) or waits and then finds the
		// register inactive.
		ok, err := store.UpdateCashRegister(ctx, tx, es, &next, old)
		if err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		if !ok {
			return &configerr.VersionConflictError{Current: old}
		}
		if cur.Active && !next.Active {
			if _, err := store.GetOpenCashSessionOfRegister(ctx, tx, es, id); err == nil {
				return ErrSessionAlreadyOpen
			} else if !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("cash: %w", err)
			}
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: AuditRegisterUpdated,
			EntityType: "cash_register", EntityID: id,
			Diff: map[string]any{
				"before": map[string]any{"name": cur.Name, "active": cur.Active, "sort_order": cur.SortOrder},
				"after":  map[string]any{"name": next.Name, "active": next.Active, "sort_order": next.SortOrder},
			},
		}); err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		out, err = s.registerView(ctx, tx, es, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteRegister deletes a register that was never used
// (auth.PermEstablishmentConfigure): the default register is
// ErrDefaultRegister, a register with sessions ErrRegisterHasSessions.
func (s *Service) DeleteRegister(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetCashRegister(ctx, tx, es, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrRegisterNotFound
			}
			return fmt.Errorf("cash: %w", err)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		if cur.IsDefault {
			return ErrDefaultRegister
		}
		n, err := store.CountCashRegisterSessions(ctx, tx, es, id)
		if err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		if n > 0 {
			return ErrRegisterHasSessions
		}
		ok, err := store.DeleteCashRegister(ctx, tx, es, id, cur.Version)
		if err != nil {
			// A session opened meanwhile: the foreign key refuses the delete.
			return ErrRegisterHasSessions
		}
		if !ok {
			return &configerr.VersionConflictError{Current: cur.Version}
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: AuditRegisterDeleted,
			EntityType: "cash_register", EntityID: id,
			Diff: map[string]any{"code": cur.Code, "name": cur.Name},
		})
	})
}

// resolveRegister finds the register to open: by id, else by code (the
// deprecated `register` field), else the default one. Unknown -> a
// validation error on register_id.
func (s *Service) resolveRegister(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id, code string) (*store.CashRegister, error) {
	if id != "" {
		r, err := store.GetCashRegister(ctx, db, es, id)
		if errors.Is(err, store.ErrNotFound) {
			return nil, configerr.Invalid("register_id", "Cassa sconosciuta.")
		}
		if err != nil {
			return nil, fmt.Errorf("cash: %w", err)
		}
		return r, nil
	}
	if code == "" || code == store.DefaultCashRegister {
		return s.EnsureDefaultRegister(ctx, db, es)
	}
	r, err := store.GetCashRegisterByCode(ctx, db, es, code)
	if errors.Is(err, store.ErrNotFound) {
		return nil, configerr.Invalid("register", "Cassa sconosciuta: creala in Configurazione → Casse.")
	}
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	return r, nil
}

// registerNames maps register id -> name for es.
func (s *Service) registerNames(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (map[string]string, error) {
	regs, err := store.ListCashRegisters(ctx, db, es, true)
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	out := make(map[string]string, len(regs))
	for _, r := range regs {
		out[r.ID] = r.Name
	}
	return out, nil
}

func nameSession(cs *store.CashSession, names map[string]string) {
	if cs.RegisterID != nil {
		cs.RegisterName = names[*cs.RegisterID]
	}
}
