package admin

import (
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Editable customer email texts (ADR 0022) -> h.Notify.
//
//	ListEmailTemplates    -> ListEmailTemplates    -> api.EmailTemplateList
//	GetEmailTemplate      -> GetEmailTemplate      -> api.EmailTemplate + ETag
//	UpdateEmailTemplate   -> UpdateEmailTemplate   -> api.EmailTemplate + ETag (If-Match; 422 email_template_invalid)
//	ResetEmailTemplate    -> ResetEmailTemplate    -> api.EmailTemplate + ETag (If-Match)
//	PreviewEmailTemplate  -> PreviewEmailTemplate  -> api.EmailTemplatePreview (sample data)
//	SendEmailTemplateTest -> SendEmailTemplateTest -> 202 api.MessageQueued (to the current user)

// ListEmailTemplates implements GET /api/admin/v1/establishments/{establishmentId}/email-templates.
func (h *Handlers) ListEmailTemplates(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	items, err := h.Notify.ListEmailTemplates(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.EmailTemplateList{Placeholders: append([]string(nil), notify.Placeholders...), Items: make([]api.EmailTemplate, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, emailTemplateToAPI(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetEmailTemplate implements GET .../email-templates/{kind}/{locale}.
func (h *Handlers) GetEmailTemplate(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, kind api.EmailTemplateKindParam, locale api.EmailLocaleParam) {
	v, err := h.Notify.GetEmailTemplate(r.Context(), principalOf(r), establishmentID, string(kind), string(locale))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, emailTemplateToAPI(v))
}

// UpdateEmailTemplate implements PUT .../email-templates/{kind}/{locale}.
func (h *Handlers) UpdateEmailTemplate(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, kind api.EmailTemplateKindParam, locale api.EmailLocaleParam, params api.UpdateEmailTemplateParams) {
	expected, err := templateIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.EmailTemplateTexts
	if !decode(w, r, &req) {
		return
	}
	v, err := h.Notify.UpdateEmailTemplate(r.Context(), principalOf(r), establishmentID, string(kind), string(locale), expected, textsFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, emailTemplateToAPI(v))
}

// ResetEmailTemplate implements DELETE .../email-templates/{kind}/{locale}.
func (h *Handlers) ResetEmailTemplate(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, kind api.EmailTemplateKindParam, locale api.EmailLocaleParam, params api.ResetEmailTemplateParams) {
	expected, err := templateIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	v, err := h.Notify.ResetEmailTemplate(r.Context(), principalOf(r), establishmentID, string(kind), string(locale), expected)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, emailTemplateToAPI(v))
}

// PreviewEmailTemplate implements POST .../email-templates/{kind}/{locale}/preview.
func (h *Handlers) PreviewEmailTemplate(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, kind api.EmailTemplateKindParam, locale api.EmailLocaleParam, _ api.PreviewEmailTemplateParams) {
	var req api.EmailTemplateTexts
	if !decode(w, r, &req) {
		return
	}
	p, err := h.Notify.PreviewEmailTemplate(r.Context(), principalOf(r), establishmentID, string(kind), string(locale), textsFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.EmailTemplatePreview{Subject: p.Subject, Html: p.HTML, Text: p.Text})
}

// SendEmailTemplateTest implements POST .../email-templates/{kind}/{locale}/test.
func (h *Handlers) SendEmailTemplateTest(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, kind api.EmailTemplateKindParam, locale api.EmailLocaleParam, _ api.SendEmailTemplateTestParams) {
	var req api.EmailTemplateTexts
	if !decode(w, r, &req) {
		return
	}
	m, err := h.Notify.SendEmailTemplateTest(r.Context(), principalOf(r), establishmentID, string(kind), string(locale), textsFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, api.MessageQueued{MessageId: m.ID})
}

// templateIfMatch is ParseIfMatch that also accepts "0": the ETag of a
// template still on its built-in texts (no override row yet).
func templateIfMatch(h *string) (*int64, error) {
	if h != nil && strings.Trim(strings.TrimPrefix(strings.TrimSpace(*h), "W/"), `"`) == "0" {
		zero := int64(0)
		return &zero, nil
	}
	return ParseIfMatch(h)
}

func textsFromAPI(t api.EmailTemplateTexts) notify.Texts {
	return notify.Texts{Subject: t.Subject, Opening: t.Opening, Closing: t.Closing, Signature: t.Signature}
}

func emailTemplateToAPI(v *notify.TemplateView) api.EmailTemplate {
	blocks := make([]api.EmailTemplateFixedBlock, 0, len(v.FixedBlocks))
	for _, b := range v.FixedBlocks {
		blocks = append(blocks, api.EmailTemplateFixedBlock(b))
	}
	return api.EmailTemplate{
		Kind: api.EmailTemplateKind(v.Kind), Locale: api.EmailLocale(v.Locale),
		Subject: v.Texts.Subject, Opening: v.Texts.Opening, Closing: v.Texts.Closing, Signature: v.Texts.Signature,
		Customized: v.Customized, FixedBlocks: blocks, Version: v.Version, UpdatedAt: v.UpdatedAt,
		Defaults: api.EmailTemplateTexts{
			Subject: v.Defaults.Subject, Opening: v.Defaults.Opening, Closing: v.Defaults.Closing, Signature: v.Defaults.Signature,
		},
	}
}
