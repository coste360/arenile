package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

// In-panel notifications (admin_notifications, ADR 0019 F) and the owner
// email of an auto-disabled webhook endpoint.
//
// Producers call Notify in their own transaction; the dedupe key makes them
// idempotent. Params hold ids, counts and non-personal labels only (never
// emails, phones or tokens): the panel composes title and text from the i18n
// keys of the kind. Read state is shared by the staff of the establishment.
// Visibility: bookings.read; audience owner only with users.manage, audience
// manager only with refunds.issue.

// Notification kinds (api.NotificationKind).
const (
	KindWebhookEndpointDisabled = "webhook_endpoint_disabled"
	KindRefundFailed            = "refund_failed"
	KindImportCompleted         = "import_completed"
	KindImportFailed            = "import_failed"
	KindTestModeEnabled         = "test_mode_enabled"
)

// Notification severities (api.NotificationSeverity).
const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// TemplateManagerWebhookDisabled is the owner email of an auto-disabled
// webhook endpoint (payload: webhook_host, webhook_failures).
const TemplateManagerWebhookDisabled = "manager_webhook_disabled"

// PanelVars are the template variables of the panel templates (besides
// establishment.name).
var PanelVars = []string{"webhook.host", "webhook.failures"}

// panelTemplates are composed by panelContent (no booking).
var panelTemplates = map[string]bool{TemplateManagerWebhookDisabled: true}

func init() {
	Templates = append(Templates, TemplateManagerWebhookDisabled)
	managerTemplates[TemplateManagerWebhookDisabled] = true
}

// PanelNotice is a notification to show in the panel.
type PanelNotice struct {
	Kind     string
	Severity string // default SeverityInfo
	Owner    bool   // audience owner (users.manage) instead of the staff
	Params   map[string]string
	// ResourceType/ResourceID link the notification (e.g. webhook_endpoint).
	ResourceType string
	ResourceID   string
	// DedupeKey is unique per establishment (required).
	DedupeKey string
}

// Notify inserts n for es in tx (idempotent on DedupeKey) and reports
// whether it is new.
func (s *Service) Notify(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, n PanelNotice) (bool, error) {
	if n.Kind == "" || n.DedupeKey == "" {
		return false, errors.New("notify: panel notice: kind and dedupe key are required")
	}
	row := &store.AdminNotification{
		ID: store.NewID(), Kind: n.Kind, Severity: n.Severity, Audience: store.NotificationAudienceStaff,
		Params: n.Params, DedupeKey: n.DedupeKey, CreatedAt: s.now(),
	}
	if row.Severity == "" {
		row.Severity = SeverityInfo
	}
	if n.Owner {
		row.Audience = store.NotificationAudienceOwner
	}
	if n.ResourceType != "" {
		row.ResourceType = &n.ResourceType
	}
	if n.ResourceID != "" {
		row.ResourceID = &n.ResourceID
	}
	ok, err := store.InsertAdminNotification(ctx, tx, es, row)
	if err != nil {
		return false, fmt.Errorf("notify: panel notice: %w", err)
	}
	return ok, nil
}

// visibility is what p sees besides the staff notifications: owner rows
// with users.manage, manager rows (failed guaranteed refunds, ADR 0019 B)
// with refunds.issue.
func visibility(p *auth.Principal, es store.EstablishmentScope) store.NotificationVisibility {
	return store.NotificationVisibility{Owner: p.Can(es, auth.PermUsersManage), Manager: p.Can(es, auth.PermRefundsIssue)}
}

// PanelList is a page of notifications with the unread count.
type PanelList struct {
	Items       []store.AdminNotification
	UnreadCount int64
	NextCursor  string
}

// ListPanel returns the notifications visible to p, newest first.
func (s *Service) ListPanel(ctx context.Context, p *auth.Principal, establishmentID string, unreadOnly bool, cursor string, limit int) (*PanelList, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	v := visibility(p, es)
	page, err := store.ListAdminNotifications(ctx, s.d.DB, es, unreadOnly, v, cursor, limit)
	if err != nil {
		return nil, err
	}
	n, err := store.CountUnreadAdminNotifications(ctx, s.d.DB, es, v)
	if err != nil {
		return nil, err
	}
	return &PanelList{Items: page.Items, UnreadCount: n, NextCursor: page.NextCursor}, nil
}

// MarkPanelRead marks one notification as read (404 when not visible).
func (s *Service) MarkPanelRead(ctx context.Context, p *auth.Principal, establishmentID, id string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return err
	}
	return store.MarkAdminNotificationRead(ctx, s.d.DB, es, id, p.UserID, visibility(p, es), s.now())
}

// MarkAllPanelRead marks every visible notification as read.
func (s *Service) MarkAllPanelRead(ctx context.Context, p *auth.Principal, establishmentID string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return err
	}
	_, err = store.MarkAllAdminNotificationsRead(ctx, s.d.DB, es, p.UserID, visibility(p, es), s.now())
	return err
}

// WebhookEndpointDisabled implements event.EndpointDisabledFunc: inside the
// transaction that auto-disables ep it adds the owner notification and
// queues the email to every owner (when the manager notifications for
// webhook errors are on). Params carry the endpoint id, the host of its URL
// and the failure count only.
func (s *Service) WebhookEndpointDisabled(ctx context.Context, tx bun.IDB, ep *store.WebhookEndpoint, failures int, at time.Time) error {
	es, err := store.SystemEstablishmentScope(ctx, tx, ep.EstablishmentID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("notify: webhook disabled: %w", err)
	}
	host := endpointHost(ep.URL)
	key := KindWebhookEndpointDisabled + ":" + ep.ID + ":" + strconv.FormatInt(at.Unix(), 10)
	created, err := s.Notify(ctx, tx, es, PanelNotice{
		Kind: KindWebhookEndpointDisabled, Severity: SeverityError, Owner: true,
		Params:       map[string]string{"endpoint_id": ep.ID, "host": host, "failures": strconv.Itoa(failures)},
		ResourceType: "webhook_endpoint", ResourceID: ep.ID, DedupeKey: key,
	})
	if err != nil || !created {
		return err
	}
	cfg, err := s.emailConfig(ctx, tx, es)
	if err != nil {
		return err
	}
	if !cfg.ToManager.WebhookError {
		return nil
	}
	owners, err := store.OwnerRecipients(ctx, tx, es)
	if err != nil {
		return fmt.Errorf("notify: webhook disabled: %w", err)
	}
	for _, u := range owners {
		_, err := s.Enqueue(ctx, tx, es, Enqueue{
			Template: TemplateManagerWebhookDisabled, Recipient: u.Email, Locale: u.Locale,
			DedupeKey: key + ":" + u.ID,
			Payload:   map[string]any{"webhook_host": host, "webhook_failures": failures, "endpoint_id": ep.ID},
		})
		if errors.Is(err, errInvalidRecipient) {
			s.d.Log.WarnContext(ctx, "owner without a valid email", slog.String("user_id", u.ID))
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// endpointHost is the host of an endpoint URL (no path, query or userinfo).
func endpointHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "?"
	}
	return u.Host
}

// panelVars are the variables of a panel template from its payload.
func panelVars(m *store.Message, est *store.Establishment) map[string]string {
	out := map[string]string{"establishment.name": est.Name}
	if v, ok := m.Payload["webhook_host"].(string); ok {
		out["webhook.host"] = v
	}
	if v, ok := payloadInt(m.Payload["webhook_failures"]); ok {
		out["webhook.failures"] = strconv.FormatInt(v, 10)
	}
	return out
}
