package notify

import (
	"bytes"
	_ "embed"
	"html"
	"html/template"
	"regexp"
	"strings"
)

// Rendering (functional core): template source + catalog + branding +
// already formatted content -> subject, text/plain and text/html parts. No
// I/O, no clock: golden tests cover every template in every locale.
//
// Template syntax: Markdown subset (markdown.go), variables {{name}} (see
// Content.Vars) and partials on a line of their own:
//
//	{{> summary}}        booking summary and totals
//	{{> qr}}             inline QR image (cid:qr) with the booking code
//	{{> manage_button}}  button to the manage area (magic link)
//	{{> policy}}         cancellation and modification terms
//	{{> ics_note}}       note about the attached .ics file
//
// A partial without data (no QR, no link, ...) renders nothing, so an
// edited template can never show an empty button or a broken image.

//go:embed layout.html
var layoutHTML string

var layoutTmpl = template.Must(template.New("layout").Parse(layoutHTML))

// QRContentID is the Content-ID of the inline QR image.
const QRContentID = "qr@arenile"

// Product signature in the footer of every email ("Arenile — Coste360.it"),
// small and muted under the establishment footer: the establishment branding
// stays primary. The mark is an inline image (cid:, like the QR), so it shows
// without remote content and with no base URL configured.
const (
	ProductLine        = "Arenile — Coste360.it"
	ProductURL         = "https://arenile.app"
	BrandMarkContentID = "arenile-mark@arenile"
	brandMarkFilename  = "arenile.png"
)

// brandMarkPNG is assets/brand/arenile/png/email-mark-64.png (the Arenile
// app icon, outlined, 64 px for a 20 px slot on high-density screens);
// TestBrandMarkMatchesBrandKit keeps the copy in sync.
//
//go:embed brand/arenile-mark-64.png
var brandMarkPNG []byte

// BrandMarkAttachment is the inline attachment the footer image refers to.
func BrandMarkAttachment() Attachment {
	return Attachment{Filename: brandMarkFilename, ContentType: contentTypePNG, ContentID: BrandMarkContentID, Inline: true, Data: brandMarkPNG}
}

// Row is a label/value line of the summary.
type Row struct {
	Label string
	Value string
}

// Content is the formatted, locale-specific data of one message.
type Content struct {
	Locale string
	// Vars are the template variables (booking.code, customer.first_name,
	// manage_url, ...), already formatted; values are plain text.
	Vars map[string]string
	// Summary and Totals feed {{> summary}}.
	Summary []Row
	Totals  []Row
	// BookingCode with QR = true feeds {{> qr}} (image attached inline).
	BookingCode string
	QR          bool
	// ManageURL feeds {{> manage_button}}.
	ManageURL string
	// Policy lines feed {{> policy}}.
	Policy []string
	// ICS reports that the .ics file is attached ({{> ics_note}}).
	ICS bool
	// Contacts are footer lines (establishment phone, email, address).
	Contacts []string
	// Manager selects the manager footer instead of the customer one.
	Manager bool
}

// Rendered is a rendered message.
type Rendered struct {
	Subject string
	Text    string
	HTML    string
}

var rePartial = regexp.MustCompile(`^\{\{>\s*([a-z_]+)\s*\}\}$`)

// Render renders src with the content c.
func Render(src templateSource, cat catalog, br Branding, c Content) (Rendered, error) {
	st := mdStyle{Link: br.Link}
	var htmlParts, textParts []string
	for _, b := range parseBlocks(src.Body) {
		if b.kind == "p" && len(b.lines) == 1 {
			if m := rePartial.FindStringSubmatch(b.lines[0]); m != nil {
				h, t := renderPartial(m[1], cat, br, c)
				if h != "" {
					htmlParts = append(htmlParts, h)
					textParts = append(textParts, t)
				}
				continue
			}
		}
		h, t := blockHTML(b, st, c.Vars), blockText(b, c.Vars)
		if strings.TrimSpace(t) == "" {
			continue // a paragraph made only of empty variables
		}
		htmlParts = append(htmlParts, h)
		textParts = append(textParts, t)
	}
	footer := cat.t("footer_transactional", map[string]string{"name": br.Name})
	if c.Manager {
		footer = cat.t("footer_manager", map[string]string{"name": br.Name})
	}
	subject := headerSafe(substitute(src.Subject, c.Vars))
	data := struct {
		Lang, Subject, Preheader string
		Brand                    Branding
		Body                     template.HTML
		Contacts                 []string
		Footer                   string
		ProductLine, ProductURL  string
		MarkCID                  string
	}{
		Lang: cat.locale, Subject: subject, Preheader: headerSafe(substitute(src.Preheader, c.Vars)),
		Brand: br, Body: template.HTML(strings.Join(htmlParts, "\n")), //nolint:gosec // G203: built from escaped blocks
		Contacts: c.Contacts, Footer: footer,
		ProductLine: ProductLine, ProductURL: ProductURL, MarkCID: BrandMarkContentID,
	}
	var buf bytes.Buffer
	if err := layoutTmpl.Execute(&buf, data); err != nil {
		return Rendered{}, err
	}
	text := strings.Join(textParts, "\n\n")
	text += "\n\n-- \n" + br.Name
	for _, l := range c.Contacts {
		text += "\n" + l
	}
	text += "\n\n" + footer + "\n\n" + ProductLine + " · " + ProductURL + "\n"
	return Rendered{Subject: subject, Text: text, HTML: buf.String()}, nil
}

// substitute replaces {{vars}} in a plain-text string (subject, preheader).
func substitute(s string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(s, func(m string) string {
		return vars[reVar.FindStringSubmatch(m)[1]]
	})
}

// headerSafe collapses whitespace and removes CR/LF (no header injection).
func headerSafe(s string) string { return strings.Join(strings.Fields(s), " ") }

func renderPartial(name string, cat catalog, br Branding, c Content) (string, string) {
	esc := html.EscapeString
	switch name {
	case "summary":
		if len(c.Summary) == 0 && len(c.Totals) == 0 {
			return "", ""
		}
		var h strings.Builder
		var t []string
		h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 20px;border:1px solid #E3E6EA;border-radius:8px;border-collapse:separate">`)
		row := func(r Row, strong bool) {
			weight := "400"
			if strong {
				weight = "700"
			}
			h.WriteString(`<tr><td style="padding:10px 14px;border-bottom:1px solid #EEF0F2;font-size:14px;color:#5A6270;width:40%;vertical-align:top">` + esc(r.Label) +
				`</td><td style="padding:10px 14px;border-bottom:1px solid #EEF0F2;font-size:15px;color:#111111;font-weight:` + weight + `;vertical-align:top">` + esc(r.Value) + `</td></tr>`)
			t = append(t, r.Label+": "+r.Value)
		}
		for _, r := range c.Summary {
			row(r, false)
		}
		for _, r := range c.Totals {
			row(r, true)
		}
		h.WriteString(`</table>`)
		return h.String(), strings.Join(t, "\n")
	case "qr":
		if !c.QR || c.BookingCode == "" {
			return "", ""
		}
		alt := cat.t("qr_alt", map[string]string{"code": c.BookingCode})
		caption := cat.t("qr_caption", nil)
		h := `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 20px"><tr><td align="center" style="padding:16px;background:#F6F7F9;border-radius:8px">` +
			`<img src="cid:` + QRContentID + `" width="180" height="180" alt="` + esc(alt) + `" style="display:block;width:180px;height:180px;border:0;background:#FFFFFF">` +
			`<p style="margin:12px 0 4px;font-size:20px;line-height:1.3;font-weight:700;letter-spacing:1px;font-family:Menlo,Consolas,monospace;color:#111111">` + esc(c.BookingCode) + `</p>` +
			`<p style="margin:0;font-size:14px;line-height:1.4;color:#5A6270">` + esc(caption) + `</p></td></tr></table>`
		return h, caption + "\n" + c.BookingCode
	case "manage_button":
		if c.ManageURL == "" {
			return "", ""
		}
		label := cat.t("manage_button", nil)
		u := esc(c.ManageURL)
		h := `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 12px"><tr><td style="border-radius:8px;background:` + br.Primary + `">` +
			`<a href="` + u + `" style="display:inline-block;padding:14px 26px;font-size:16px;font-weight:700;line-height:1.2;color:` + br.OnPrimary + `;text-decoration:none;border-radius:8px">` + esc(label) + `</a></td></tr></table>` +
			`<p style="margin:0 0 16px;font-size:13px;line-height:1.5;color:#5A6270;word-break:break-all"><a href="` + u + `" style="color:` + br.Link + `">` + u + `</a></p>`
		return h, label + ":\n" + c.ManageURL
	case "policy":
		if len(c.Policy) == 0 {
			return "", ""
		}
		title := cat.t("policy_title", nil)
		var h strings.Builder
		h.WriteString(`<h3 style="` + styleH3 + `">` + esc(title) + `</h3><ul style="` + styleUL + `">`)
		t := []string{title}
		for _, l := range c.Policy {
			h.WriteString(`<li style="margin:0 0 6px">` + esc(l) + `</li>`)
			t = append(t, "- "+l)
		}
		h.WriteString(`</ul>`)
		return h.String(), strings.Join(t, "\n")
	case "ics_note":
		if !c.ICS {
			return "", ""
		}
		note := cat.t("ics_note", nil)
		return `<p style="` + styleP + `">` + esc(note) + `</p>`, note
	}
	return "", ""
}
