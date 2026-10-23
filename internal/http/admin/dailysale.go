package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Daily sale handlers ("Vendita giornaliera", spec 14.3, ADR 0020). Owner:
// internal/domain/dailysale. Contract scaffolding: every operation answers
// 501 not_implemented until the domain service is wired.

// QuoteDailySale implements POST .../daily-sales/quote.
func (h *Handlers) QuoteDailySale(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.QuoteDailySaleParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateDailySale implements POST .../daily-sales.
func (h *Handlers) CreateDailySale(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateDailySaleParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetDailySaleSettings implements GET .../daily-sale-settings.
func (h *Handlers) GetDailySaleSettings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateDailySaleSettings implements PUT .../daily-sale-settings.
func (h *Handlers) UpdateDailySaleSettings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.UpdateDailySaleSettingsParams) {
	problem.Write(w, r, problem.NotImplemented)
}
