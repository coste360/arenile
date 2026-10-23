package notify

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// A deliberately small, safe Markdown subset for the notification
// templates (built-in and edited by the manager, spec 19): headings (#,
// ##, ###), paragraphs (single newlines become line breaks), bullet lists
// (- or *), **bold**, *italic* / _italic_ and [links](https://...). Raw HTML
// is never passed through: every character of the source is escaped, and
// links accept only https, http, mailto and tel URLs or a template variable.
// Each block renders to email-safe HTML with inline styles and to plain
// text (the text/plain part).
//
// Template variables ({{name}}) are replaced by sentinels before parsing
// and restored afterwards with their values (HTML-escaped in the HTML
// part), so a value is never interpreted as Markdown or HTML.

var (
	reVar    = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_.]*)\s*\}\}`)
	reLink   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	reBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic = regexp.MustCompile(`(^|[^\w*])[*_]([^*_\s][^*_]*?)[*_]($|[^\w*])`)
	reSent   = regexp.MustCompile("\x00([0-9]+)\x00")
	reLinkS  = regexp.MustCompile("\x01([0-9]+)\x01")
)

// mdStyle carries the colours used by inline styles.
type mdStyle struct {
	Link string
}

// mdBlock is one parsed block.
type mdBlock struct {
	kind  string // h1 | h2 | h3 | p | ul
	lines []string
}

// parseBlocks splits Markdown into blocks separated by blank lines.
func parseBlocks(src string) []mdBlock {
	var out []mdBlock
	for _, chunk := range splitBlocks(src) {
		lines := strings.Split(chunk, "\n")
		first := lines[0]
		switch {
		case len(lines) == 1 && strings.HasPrefix(first, "### "):
			out = append(out, mdBlock{kind: "h3", lines: []string{strings.TrimPrefix(first, "### ")}})
		case len(lines) == 1 && strings.HasPrefix(first, "## "):
			out = append(out, mdBlock{kind: "h2", lines: []string{strings.TrimPrefix(first, "## ")}})
		case len(lines) == 1 && strings.HasPrefix(first, "# "):
			out = append(out, mdBlock{kind: "h1", lines: []string{strings.TrimPrefix(first, "# ")}})
		case allItems(lines):
			items := make([]string, len(lines))
			for i, l := range lines {
				items[i] = strings.TrimSpace(l[2:])
			}
			out = append(out, mdBlock{kind: "ul", lines: items})
		default:
			out = append(out, mdBlock{kind: "p", lines: lines})
		}
	}
	return out
}

// splitBlocks returns the non-empty chunks between blank lines, trimmed.
func splitBlocks(src string) []string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimRight(l, " \t")
		if strings.TrimSpace(l) == "" {
			flush()
			continue
		}
		cur = append(cur, strings.TrimLeft(l, " \t"))
	}
	flush()
	return out
}

func allItems(lines []string) bool {
	for _, l := range lines {
		if !strings.HasPrefix(l, "- ") && !strings.HasPrefix(l, "* ") {
			return false
		}
	}
	return true
}

// protectVars replaces {{vars}} with sentinels and returns their names.
func protectVars(s string) (string, []string) {
	var names []string
	out := reVar.ReplaceAllStringFunc(s, func(m string) string {
		names = append(names, reVar.FindStringSubmatch(m)[1])
		return "\x00" + strconv.Itoa(len(names)-1) + "\x00"
	})
	return out, names
}

// restoreVars puts the values back, transformed by esc.
func restoreVars(s string, names []string, vars map[string]string, esc func(string) string) string {
	return reSent.ReplaceAllStringFunc(s, func(m string) string {
		i, _ := strconv.Atoi(reSent.FindStringSubmatch(m)[1])
		if i < 0 || i >= len(names) {
			return ""
		}
		return esc(vars[names[i]])
	})
}

// safeURL reports whether a link target is allowed (after escaping).
func safeURL(u string) bool {
	if reSent.MatchString(u) && strings.HasPrefix(u, "\x00") {
		return true // a template variable such as {{manage_url}}
	}
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") ||
		strings.HasPrefix(l, "mailto:") || strings.HasPrefix(l, "tel:")
}

// inlineHTML renders the inline syntax of an escaped line.
func inlineHTML(s string, st mdStyle) string {
	s = html.EscapeString(s)
	var links []string
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		p := reLink.FindStringSubmatch(m)
		text, target := p[1], p[2]
		if !safeURL(target) {
			links = append(links, text)
		} else {
			links = append(links, `<a href="`+target+`" style="color:`+st.Link+`;text-decoration:underline">`+text+`</a>`)
		}
		return "\x01" + strconv.Itoa(len(links)-1) + "\x01"
	})
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItalic.ReplaceAllString(s, "$1<em>$2</em>$3")
	return reLinkS.ReplaceAllStringFunc(s, func(m string) string {
		i, _ := strconv.Atoi(reLinkS.FindStringSubmatch(m)[1])
		return links[i]
	})
}

// inlineText renders the inline syntax as plain text.
func inlineText(s string) string {
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		p := reLink.FindStringSubmatch(m)
		if p[1] == p[2] || !safeURL(p[2]) {
			return p[1]
		}
		return p[1] + " (" + p[2] + ")"
	})
	s = reBold.ReplaceAllString(s, "$1")
	return reItalic.ReplaceAllString(s, "$1$2$3")
}

const (
	styleH1 = "margin:0 0 16px;font-size:24px;line-height:1.3;font-weight:700;color:#111111"
	styleH2 = "margin:24px 0 12px;font-size:19px;line-height:1.35;font-weight:700;color:#111111"
	styleH3 = "margin:20px 0 8px;font-size:16px;line-height:1.4;font-weight:700;color:#111111"
	styleP  = "margin:0 0 16px;font-size:16px;line-height:1.55;color:#222222"
	styleUL = "margin:0 0 16px;padding-left:22px;font-size:16px;line-height:1.55;color:#222222"
)

// blockHTML renders one block; vars are restored HTML-escaped.
func blockHTML(b mdBlock, st mdStyle, vars map[string]string) string {
	render := func(line string) string {
		p, names := protectVars(line)
		return restoreVars(inlineHTML(p, st), names, vars, html.EscapeString)
	}
	switch b.kind {
	case "h1", "h2", "h3":
		style := map[string]string{"h1": styleH1, "h2": styleH2, "h3": styleH3}[b.kind]
		return "<" + b.kind + ` style="` + style + `">` + render(b.lines[0]) + "</" + b.kind + ">"
	case "ul":
		var sb strings.Builder
		sb.WriteString(`<ul style="` + styleUL + `">`)
		for _, it := range b.lines {
			sb.WriteString(`<li style="margin:0 0 6px">` + render(it) + "</li>")
		}
		sb.WriteString("</ul>")
		return sb.String()
	default:
		parts := make([]string, len(b.lines))
		for i, l := range b.lines {
			parts[i] = render(l)
		}
		return `<p style="` + styleP + `">` + strings.Join(parts, "<br>") + "</p>"
	}
}

// blockText renders one block as plain text; vars are restored as is.
func blockText(b mdBlock, vars map[string]string) string {
	render := func(line string) string {
		p, names := protectVars(line)
		return restoreVars(inlineText(p), names, vars, func(s string) string { return s })
	}
	switch b.kind {
	case "h1", "h2":
		t := render(b.lines[0])
		ch := "="
		if b.kind == "h2" {
			ch = "-"
		}
		return t + "\n" + strings.Repeat(ch, max(len([]rune(t)), 3))
	case "h3":
		return render(b.lines[0])
	case "ul":
		items := make([]string, len(b.lines))
		for i, it := range b.lines {
			items[i] = "- " + render(it)
		}
		return strings.Join(items, "\n")
	default:
		parts := make([]string, len(b.lines))
		for i, l := range b.lines {
			parts[i] = render(l)
		}
		return strings.Join(parts, "\n")
	}
}
