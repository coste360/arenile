package refund

import (
	"fmt"

	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/payment"
)

var (
	// ErrRefundInProgress: the booking already has a live refund request
	// for the same payment (409 refund_in_progress).
	ErrRefundInProgress = payment.ErrRefundInProgress
	// ErrNotRetryable: only a failed request can be retried (409 invalid_transition).
	ErrNotRetryable = fmt.Errorf("%w: solo un rimborso fallito può essere riprovato", booking.ErrInvalidTransition)
)
