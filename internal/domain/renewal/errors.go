package renewal

import (
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/domain/configerr"
)

var (
	// ErrExpired: the pre-emption deadline passed (410 renewal_expired).
	ErrExpired = errors.New("renewal: proposal expired")
	// ErrNotOpen: the proposal is not proposed/accepted (409 invalid_transition
	// is reported by booking.TransitionError; this one maps to 409 conflict).
	ErrNotOpen = fmt.Errorf("%w: la proposta di rinnovo non è più aperta", configerr.ErrConflict)
	// ErrCampaignExists: a campaign for the same seasons exists (409).
	ErrCampaignExists = fmt.Errorf("%w: esiste già una campagna di rinnovo per queste stagioni", configerr.ErrConflict)
	// ErrSourceCancelled: the source pass was cancelled after the proposal
	// was made, so there is nothing to renew (409; the sync job expires the
	// proposal and releases the spot).
	ErrSourceCancelled = fmt.Errorf("%w: l'abbonamento da rinnovare è stato annullato", configerr.ErrConflict)
	// ErrNotRenewal: the magic link booking is not a renewal proposal (404).
	ErrNotRenewal = fmt.Errorf("%w: proposta di rinnovo", configerr.ErrNotFound)
)
