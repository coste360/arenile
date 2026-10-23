package waitlist

import (
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/domain/configerr"
)

var (
	// ErrOfferExpired: the offer is no longer pending (410 waitlist_offer_expired).
	ErrOfferExpired = errors.New("waitlist: offer expired")
	// ErrDisabled: the waitlist (or public join) is off (409 waitlist_disabled).
	ErrDisabled = errors.New("waitlist: disabled")
	// ErrNoOffer: accept/decline without a pending offer (409 conflict).
	ErrNoOffer = fmt.Errorf("%w: nessuna offerta attiva", configerr.ErrConflict)
)
