package team

import (
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
)

// Deps are the dependencies of the team service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// Auth hashes passwords and validates the password policy.
	Auth *auth.Service
}

// Service implements the team use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service { return &Service{d: d} }

// Errors of the team package.
var (
	// ErrLastOwner: the change would leave the organization or the
	// establishment without an active owner (409).
	ErrLastOwner = fmt.Errorf("%w: deve restare almeno un proprietario attivo", configerr.ErrConflict)
	// ErrAlreadyMember: the user already has access to the establishment;
	// change the role instead (409).
	ErrAlreadyMember = fmt.Errorf("%w: l'utente ha già accesso a questo stabilimento", configerr.ErrConflict)
	// ErrSelfDeactivation: an owner cannot deactivate their own account (409).
	ErrSelfDeactivation = fmt.Errorf("%w: non puoi disattivare il tuo account", configerr.ErrConflict)
	// ErrOtherOrganizations: the account also works for other organizations,
	// so it cannot be deactivated from here; remove its access instead (409).
	ErrOtherOrganizations = fmt.Errorf("%w: l'utente lavora anche per altre organizzazioni: revoca l'accesso invece di disattivarlo", configerr.ErrConflict)
)

// Audit actions.
const (
	ActionMemberAdded       = "team.member_added"
	ActionRoleChanged       = "team.role_changed"
	ActionMemberDeactivated = "team.member_deactivated"
	ActionMemberReactivated = "team.member_reactivated"
	ActionMemberRemoved     = "team.member_removed"
)

// Page size bounds.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)
