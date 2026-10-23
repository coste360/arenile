package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// D. Cash registers and the printable Z closing: contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// ListCashRegisters is not implemented yet.
func (h *Handlers) ListCashRegisters(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListCashRegistersParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateCashRegister is not implemented yet.
func (h *Handlers) CreateCashRegister(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateCashRegisterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetCashRegister is not implemented yet.
func (h *Handlers) GetCashRegister(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashRegisterID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateCashRegister is not implemented yet.
func (h *Handlers) UpdateCashRegister(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashRegisterID, _ api.UpdateCashRegisterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// DeleteCashRegister is not implemented yet.
func (h *Handlers) DeleteCashRegister(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashRegisterID, _ api.DeleteCashRegisterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetCashSessionZReport is not implemented yet.
func (h *Handlers) GetCashSessionZReport(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashSessionID, _ api.GetCashSessionZReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}
