package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// F. In-panel notifications (notify panel.go, ADR 0019 F). The service
// checks bookings.read; owner-audience rows only with users.manage.

// ListNotifications implements GET /notifications.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListNotificationsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	list, err := h.Notify.ListPanel(r.Context(), principalOf(r), establishmentID, params.Unread != nil && *params.Unread, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.NotificationList{Items: make([]api.Notification, 0, len(list.Items)), UnreadCount: list.UnreadCount, NextCursor: nextCursor(list.NextCursor)}
	for i := range list.Items {
		out.Items = append(out.Items, notificationToAPI(&list.Items[i]))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// MarkNotificationRead implements POST /notifications/{notificationId}/read.
func (h *Handlers) MarkNotificationRead(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.NotificationID, _ api.MarkNotificationReadParams) {
	if err := h.Notify.MarkPanelRead(r.Context(), principalOf(r), establishmentID, id); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllNotificationsRead implements POST /notifications/read-all.
func (h *Handlers) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.MarkAllNotificationsReadParams) {
	if err := h.Notify.MarkAllPanelRead(r.Context(), principalOf(r), establishmentID); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func notificationToAPI(n *store.AdminNotification) api.Notification {
	params := n.Params
	if params == nil {
		params = map[string]string{}
	}
	return api.Notification{
		Id: n.ID, Kind: api.NotificationKind(n.Kind), Severity: api.NotificationSeverity(n.Severity), Params: params,
		ResourceType: n.ResourceType, ResourceId: n.ResourceID, ReadAt: n.ReadAt, CreatedAt: n.CreatedAt,
	}
}
