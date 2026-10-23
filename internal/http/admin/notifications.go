package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// F. In-panel notifications: contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// ListNotifications is not implemented yet.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListNotificationsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// MarkNotificationRead is not implemented yet.
func (h *Handlers) MarkNotificationRead(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.NotificationID, _ api.MarkNotificationReadParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// MarkAllNotificationsRead is not implemented yet.
func (h *Handlers) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MarkAllNotificationsReadParams) {
	problem.Write(w, r, problem.NotImplemented)
}
