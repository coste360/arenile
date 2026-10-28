package problem

import (
	"errors"
	"net/http"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/http/api"
)

// M8 type slugs (spec 30, ADR 0028).
const (
	AgreementQuotaExceeded = api.ProblemType("agreement_quota_exceeded")
	AgreementNotApplicable = api.ProblemType("agreement_not_applicable")
	SwapConflict           = api.ProblemType("swap_conflict")
	MoveUndoExpired        = api.ProblemType("move_undo_expired")
	MoveUndoConflict       = api.ProblemType("move_undo_conflict")
	PartnerInUse           = api.ProblemType("partner_in_use")
	CustomerCategoryInUse  = api.ProblemType("customer_category_in_use")
)

func init() {
	catalog[AgreementQuotaExceeded] = meta{http.StatusConflict, "Contingente della convenzione esaurito per le date richieste"}
	catalog[AgreementNotApplicable] = meta{http.StatusUnprocessableEntity, "La convenzione non si applica a questa prenotazione"}
	catalog[SwapConflict] = meta{http.StatusConflict, "Le due prenotazioni non si possono scambiare"}
	catalog[MoveUndoExpired] = meta{http.StatusGone, "Lo spostamento non si può più annullare"}
	catalog[MoveUndoConflict] = meta{http.StatusConflict, "Lo spostamento non si può annullare: le postazioni di partenza non sono più libere"}
	catalog[PartnerInUse] = meta{http.StatusConflict, "Partner in uso: disattivalo invece di eliminarlo"}
	catalog[CustomerCategoryInUse] = meta{http.StatusConflict, "Categoria usata da una convenzione"}
}

// writeM8 writes the problem of an M8 domain error and reports whether err
// was one. Agreement quota conflicts carry their conflicts.
func writeM8(w http.ResponseWriter, r *http.Request, err error) bool {
	var ce *availability.ConflictError
	if errors.As(err, &ce) && errors.Is(ce.Err, availability.ErrAgreementQuotaExceeded) {
		Write(w, r, AgreementQuotaExceeded, Conflicts(ce.Conflicts))
		return true
	}
	var typ api.ProblemType
	switch {
	case errors.Is(err, availability.ErrAgreementQuotaExceeded):
		typ = AgreementQuotaExceeded
	case errors.Is(err, booking.ErrAgreementNotApplicable):
		typ = AgreementNotApplicable
	case errors.Is(err, booking.ErrSwapConflict):
		typ = SwapConflict
	case errors.Is(err, booking.ErrMoveUndoExpired):
		typ = MoveUndoExpired
	case errors.Is(err, booking.ErrMoveUndoConflict):
		typ = MoveUndoConflict
	case errors.Is(err, partner.ErrPartnerInUse):
		typ = PartnerInUse
	case errors.Is(err, partner.ErrCategoryInUse):
		typ = CustomerCategoryInUse
	default:
		return false
	}
	Write(w, r, typ)
	return true
}
