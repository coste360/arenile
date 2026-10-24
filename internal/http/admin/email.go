package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Email settings and message log (M4, spec 19). Owners: online settings
// builder (GetEmailSettings, UpdateEmailSettings -> h.Online) and
// notifications builder (SendTestEmail, ListMessages -> h.Notify).
//
//	GetEmailSettings    -> Online.GetEmail     -> api.EmailSettings + ETag
//	UpdateEmailSettings -> Online.UpdateEmail  -> api.EmailSettings + ETag (If-Match)
//	SendTestEmail       -> Notify.SendTest     -> 202 api.MessageQueued
//	ListMessages        -> Notify.ListMessages -> api.MessageLogList (recipients masked)
//
// Responses never carry recipients in clear nor message bodies.

// GetEmailSettings implements GET /api/admin/v1/establishments/{establishmentId}/email-settings.
func (h *Handlers) GetEmailSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	s, err := h.Online.GetEmail(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, emailSettingsToAPI(s))
}

// UpdateEmailSettings implements PUT /api/admin/v1/establishments/{establishmentId}/email-settings.
func (h *Handlers) UpdateEmailSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateEmailSettingsParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.EmailSettingsInput
	if !decode(w, r, &req) {
		return
	}
	in := online.EmailConfig{
		SenderName: req.SenderName,
		ToManager: online.ManagerNotifications{
			NewOnlineBooking: req.ManagerNotifications.NewOnlineBooking, ApprovalRequested: req.ManagerNotifications.ApprovalRequested,
			PaymentFailed: req.ManagerNotifications.PaymentFailed, WebhookError: req.ManagerNotifications.WebhookError,
		},
		ToCustomer: online.CustomerNotifications{
			Confirmation: req.CustomerNotifications.Confirmation, BalanceReminder: req.CustomerNotifications.BalanceReminder,
			DayBeforeReminder: req.CustomerNotifications.DayBeforeReminder, Cancellation: req.CustomerNotifications.Cancellation,
			Refund: req.CustomerNotifications.Refund,
		},
	}
	if req.ReplyTo != nil {
		in.ReplyTo = string(*req.ReplyTo)
	}
	if r := req.Reminders; r != nil {
		in.Reminders = &online.ReminderSchedule{DayBeforeTime: r.DayBeforeTime, BalanceTime: r.BalanceTime, BalanceLeadDays: r.BalanceLeadDays}
	}
	if req.ManagerEmail != nil {
		in.Manager = string(*req.ManagerEmail)
	}
	s, err := h.Online.UpdateEmail(r.Context(), principalOf(r), establishmentID, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, emailSettingsToAPI(s))
}

// SendTestEmail implements POST /api/admin/v1/establishments/{establishmentId}/email-settings/test.
func (h *Handlers) SendTestEmail(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.SendTestEmailParams) {
	var req api.EmailTestRequest
	if !decode(w, r, &req) {
		return
	}
	m, err := h.Notify.SendTest(r.Context(), principalOf(r), establishmentID, string(req.To))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, api.MessageQueued{MessageId: m.ID})
}

// ListMessages implements GET /api/admin/v1/establishments/{establishmentId}/messages.
func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListMessagesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var f notify.MessageFilter
	if params.BookingId != nil {
		f.BookingID = *params.BookingId
	}
	if params.Status != nil {
		if !params.Status.Valid() {
			problem.Error(w, r, h.Log, configerr.Invalid("status", "Stato non valido."))
			return
		}
		f.Status = string(*params.Status)
	}
	page, err := h.Notify.ListMessages(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.MessageLogList{Items: make([]api.MessageLogEntry, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, messageToAPI(&page.Items[i]))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func messageToAPI(v *notify.MessageView) api.MessageLogEntry {
	m := &v.Message
	e := api.MessageLogEntry{
		Id: m.ID, Attempts: m.Attempts, BookingId: m.BookingID, Channel: m.Channel, CreatedAt: m.CreatedAt,
		LastError: m.LastError, Locale: m.Locale, RecipientMasked: v.RecipientMasked, SentAt: m.SentAt,
		Status: api.MessageStatus(m.Status), Template: m.Template,
	}
	if v.BookingCode != "" {
		code := v.BookingCode
		e.BookingCode = &code
	}
	return e
}

func emailSettingsToAPI(s *online.EmailSettings) api.EmailSettings {
	c := s.Config
	out := api.EmailSettings{
		SenderName: c.SenderName,
		CustomerNotifications: api.CustomerNotifications{
			Confirmation: c.ToCustomer.Confirmation, BalanceReminder: c.ToCustomer.BalanceReminder,
			DayBeforeReminder: c.ToCustomer.DayBeforeReminder, Cancellation: c.ToCustomer.Cancellation, Refund: c.ToCustomer.Refund,
		},
		ManagerNotifications: api.ManagerNotifications{
			NewOnlineBooking: c.ToManager.NewOnlineBooking, ApprovalRequested: c.ToManager.ApprovalRequested,
			PaymentFailed: c.ToManager.PaymentFailed, WebhookError: c.ToManager.WebhookError,
		},
		Version: s.Version,
	}
	sched := c.Schedule()
	out.Reminders = api.EmailReminderSchedule{DayBeforeTime: sched.DayBeforeTime, BalanceTime: sched.BalanceTime, BalanceLeadDays: sched.BalanceLeadDays}
	if c.ReplyTo != "" {
		v := c.ReplyTo
		out.ReplyTo = &v
	}
	if c.Manager != "" {
		v := c.Manager
		out.ManagerEmail = &v
	}
	if s.Transport.MailFrom != "" {
		v := s.Transport.MailFrom
		out.MailFrom = &v
	}
	smtp := s.Transport.SMTPConfigured
	out.SmtpConfigured = &smtp
	if s.Transport.Kind != "" {
		k := api.EmailSettingsTransport(s.Transport.Kind)
		out.Transport = &k
	}
	return out
}
