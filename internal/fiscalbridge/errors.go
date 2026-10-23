package fiscalbridge

import (
	"fmt"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// Errors of the instance side (mapped by internal/http/problem through the
// wrapped sentinels).
var (
	// ErrNotDevice: the operation needs a device principal (403).
	ErrNotDevice = fmt.Errorf("%w: fiscal bridge operations need a device token", auth.ErrForbidden)
	// ErrNotActiveBridge: the device is not the active bridge of its
	// establishment (another device registered later, or it unregistered):
	// the agent must register again (409).
	ErrNotActiveBridge = fmt.Errorf("%w: il dispositivo non è il ponte fiscale attivo", configerr.ErrConflict)
	// ErrLeaseLost: the result does not match the job state (completed with
	// another outcome or leased to another device) (409).
	ErrLeaseLost = fmt.Errorf("%w: il lavoro fiscale non è più assegnato a questo dispositivo", configerr.ErrConflict)
)

func errInvalid(field, msg string) error { return configerr.Invalid(field, msg) }
