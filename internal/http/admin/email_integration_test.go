//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestEmailHTTP_TestEmailAndMessageLog(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
		org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
		if err != nil {
			t.Fatal(err)
		}
		c := clock.NewFake(now)
		keys, err := crypto.NewKeyRing(bytes.Repeat([]byte{3}, 32))
		if err != nil {
			t.Fatal(err)
		}
		tr := &notify.MemoryTransport{}
		svc := notify.NewService(notify.Deps{
			DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c), Keys: keys,
			Transport: tr, MailFrom: "noreply@arenile.example", BaseURL: "https://arenile.example",
		})
		h := &admin.Handlers{Notify: svc, Log: slog.New(slog.DiscardHandler)}
		user := func(role auth.Role) *auth.Principal {
			return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, Role: role}}}
		}
		manager, viewer := user(auth.RoleManager), user(auth.RoleViewer)
		base := "/api/admin/v1/establishments/" + est.ID

		send := func(p *auth.Principal, body any) (int, map[string]any) {
			code, _, out := callTarget(t, p, http.MethodPost, base+"/email-settings/test", body, func(w http.ResponseWriter, r *http.Request) {
				h.SendTestEmail(w, r, est.ID, api.SendTestEmailParams{})
			})
			return code, out
		}
		if code, _ := send(viewer, map[string]string{"to": "ops@lido.example"}); code != http.StatusForbidden {
			t.Fatalf("viewer: %d", code)
		}
		if code, _ := send(manager, map[string]string{"to": "nope"}); code != http.StatusBadRequest {
			t.Fatalf("invalid address: %d", code)
		}
		code, out := send(manager, map[string]string{"to": "ops@lido.example"})
		if code != http.StatusAccepted || out["message_id"] == "" {
			t.Fatalf("send: %d %v", code, out)
		}
		if _, err := svc.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
		if sent := tr.Sent(); len(sent) != 1 || sent[0].To != "ops@lido.example" || !strings.Contains(sent[0].Subject, "Lido A") {
			t.Fatalf("sent %+v", sent)
		}

		list := func(p *auth.Principal, query string, params api.ListMessagesParams) (int, http.Header, map[string]any) {
			return callTarget(t, p, http.MethodGet, base+"/messages"+query, nil, func(w http.ResponseWriter, r *http.Request) {
				h.ListMessages(w, r, est.ID, params)
			})
		}
		code, hdr, page := list(viewer, "", api.ListMessagesParams{})
		if code != http.StatusOK || hdr.Get("Cache-Control") != "no-store" {
			t.Fatalf("list: %d %v", code, hdr)
		}
		items, _ := page["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("items %v", page)
		}
		item := items[0].(map[string]any)
		if item["recipient_masked"] != "o***s@l***.example" || item["status"] != "sent" || item["template"] != notify.TemplateTest {
			t.Fatalf("item %v", item)
		}
		bad := api.MessageStatus("bogus")
		if code, _, _ := list(viewer, "?status=bogus", api.ListMessagesParams{Status: &bad}); code != http.StatusBadRequest {
			t.Fatalf("bad status: %d", code)
		}
		other := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: store.NewID(), Role: auth.RoleOwner}}}
		if code, _, _ := list(other, "", api.ListMessagesParams{}); code != http.StatusNotFound {
			t.Fatalf("outsider: %d", code)
		}
	})
}

// TestEmailTemplatesHTTP_ValidationAndETag: 422 email_template_invalid with
// the field on an unknown placeholder, ETag/If-Match on save and restore,
// 412 on a stale version, preview rendering.
func TestEmailTemplatesHTTP_ValidationAndETag(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
		org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
		if err != nil {
			t.Fatal(err)
		}
		c := clock.NewFake(now)
		keys, err := crypto.NewKeyRing(bytes.Repeat([]byte{3}, 32))
		if err != nil {
			t.Fatal(err)
		}
		svc := notify.NewService(notify.Deps{
			DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c), Keys: keys,
			Transport: &notify.MemoryTransport{}, MailFrom: "noreply@arenile.example", BaseURL: "https://arenile.example",
		})
		h := &admin.Handlers{Notify: svc, Log: slog.New(slog.DiscardHandler)}
		manager := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, Role: auth.RoleManager}}}
		if _, err := db.NewInsert().Model(&store.User{ID: manager.UserID, Email: "m@lido.example", Name: "M", Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now}).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		path := "/api/admin/v1/establishments/" + est.ID + "/email-templates/booking_confirmed/it"
		kind, locale := api.EmailTemplateKind("booking_confirmed"), api.EmailLocale("it")
		put := func(ifMatch string, body any) (int, http.Header, map[string]any) {
			return callTarget(t, manager, http.MethodPut, path, body, func(w http.ResponseWriter, r *http.Request) {
				h.UpdateEmailTemplate(w, r, est.ID, kind, locale, api.UpdateEmailTemplateParams{IfMatch: &ifMatch})
			})
		}
		texts := map[string]string{"subject": "Ok {codice}", "opening": "Ciao {nome}", "closing": "", "signature": ""}
		bad := map[string]string{"subject": "Ok {codice}", "opening": "Ciao {telefono}", "closing": "", "signature": ""}
		code, _, out := put(`"0"`, bad)
		if code != http.StatusUnprocessableEntity || out["type"] != "email_template_invalid" {
			t.Fatalf("unknown placeholder: %d %v", code, out)
		}
		if errs, _ := out["errors"].([]any); len(errs) != 1 || errs[0].(map[string]any)["field"] != "opening" {
			t.Fatalf("errors %v", out["errors"])
		}
		code, hdr, out := put(`"0"`, texts)
		if code != http.StatusOK || hdr.Get("ETag") != `"1"` || out["customized"] != true {
			t.Fatalf("save: %d %v %v", code, hdr, out)
		}
		if code, _, out := put(`"0"`, texts); code != http.StatusPreconditionFailed || out["type"] != "version_conflict" {
			t.Fatalf("stale: %d %v", code, out)
		}
		code, hdr, _ = callTarget(t, manager, http.MethodGet, path, nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetEmailTemplate(w, r, est.ID, kind, locale)
		})
		if code != http.StatusOK || hdr.Get("ETag") != `"1"` {
			t.Fatalf("get: %d %v", code, hdr)
		}
		code, _, out = callTarget(t, manager, http.MethodPost, path+"/preview", texts, func(w http.ResponseWriter, r *http.Request) {
			h.PreviewEmailTemplate(w, r, est.ID, kind, locale, api.PreviewEmailTemplateParams{})
		})
		if subject, _ := out["subject"].(string); code != http.StatusOK || !strings.HasPrefix(subject, "Ok PROVA-") {
			t.Fatalf("preview: %d %v", code, out)
		}
		if html, _ := out["html"].(string); !strings.Contains(html, "Ciao Mario") {
			t.Fatal("preview html misses the sample name")
		}
		one := `"1"`
		code, hdr, out = callTarget(t, manager, http.MethodDelete, path, nil, func(w http.ResponseWriter, r *http.Request) {
			h.ResetEmailTemplate(w, r, est.ID, kind, locale, api.ResetEmailTemplateParams{IfMatch: &one})
		})
		if code != http.StatusOK || hdr.Get("ETag") != `"0"` || out["customized"] != false {
			t.Fatalf("reset: %d %v %v", code, hdr, out)
		}
		code, _, out = callTarget(t, manager, http.MethodGet, "/api/admin/v1/establishments/"+est.ID+"/email-templates", nil, func(w http.ResponseWriter, r *http.Request) {
			h.ListEmailTemplates(w, r, est.ID)
		})
		if items, _ := out["items"].([]any); code != http.StatusOK || len(items) != len(notify.EditableKinds)*len(notify.Locales) {
			t.Fatalf("list: %d %d", code, len(items))
		}
	})
}
