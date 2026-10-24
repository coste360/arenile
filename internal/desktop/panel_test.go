package desktop

import (
	"strings"
	"testing"
)

func TestIsPanelPath_Prefixes(t *testing.T) {
	for p, want := range map[string]bool{
		"/admin": true, "/admin/": true, "/admin/oggi": true, "/api/admin/v1/x": true, "/api": true,
		"": false, "/": false, "/b/lido": false, "/administrator": false, "/apis": false, "/embed.js": false,
		"/admin/../b/lido": false, "admin/oggi": true, "//admin": true,
	} {
		if got := IsPanelPath(p); got != want {
			t.Errorf("IsPanelPath(%q) = %v", p, got)
		}
	}
}

func TestClassifyLink_Policy(t *testing.T) {
	panel := "http://127.0.0.1:5173/admin/oggi"
	for target, want := range map[string]LinkKind{
		"http://127.0.0.1:5173/admin/":                LinkPanelPage,
		"http://127.0.0.1:5173/admin":                 LinkPanelPage,
		"http://127.0.0.1:5173/admin/desktop?x=1#y":   LinkPanelPage,
		"HTTP://127.0.0.1:5173/admin/":                LinkPanelPage,
		"http://127.0.0.1:5173/api/admin/v1/z-report": LinkPanelResource,
		"http://127.0.0.1:5173/b/bar-marini":          LinkLeaves,
		"http://127.0.0.1:5173/":                      LinkLeaves,
		"http://127.0.0.1:5173/administrator":         LinkLeaves,
		"http://127.0.0.1:9999/admin/":                LinkLeaves,
		"https://127.0.0.1:5173/admin/":               LinkLeaves,
		"https://dashboard.stripe.com/":               LinkLeaves,
		"https://arenile.app/download/":               LinkLeaves,
		"mailto:info@example.it":                      LinkLeaves,
		"blob:http://127.0.0.1:5173/0b1c":             LinkOther,
		"data:text/csv,a":                             LinkOther,
		"about:blank":                                 LinkOther,
		"/b/bar-marini":                               LinkOther,
		"file:///etc/passwd":                          LinkOther,
		"javascript:alert(1)":                         LinkOther,
	} {
		if got := ClassifyLink(panel, target); got != want {
			t.Errorf("ClassifyLink(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestLeavesPanel_RelativeAndInvalid(t *testing.T) {
	panel := "http://127.0.0.1:5173/admin/"
	for _, target := range []string{"/b/x", "b/x", "", "%zz", "http://127.0.0.1:5173/admin/x"} {
		if LeavesPanel(panel, target) {
			t.Errorf("LeavesPanel(%q) = true", target)
		}
	}
	if !LeavesPanel(panel, "http://127.0.0.1:5173/b/x") {
		t.Error("public page stays in the window")
	}
}

// TestLinkScript_MirrorsTheGoRule: the injected script keeps the panel
// prefixes of IsPanelPath, never intercepts <a download>, and covers clicks,
// window.open and the Navigation API.
func TestLinkScript_MirrorsTheGoRule(t *testing.T) {
	for _, pre := range panelPrefixes {
		for _, want := range []string{`p === "` + pre + `"`, `p.indexOf("` + pre + `/") === 0`} {
			if !strings.Contains(LeavesPanelJS, want) {
				t.Errorf("LeavesPanelJS lacks %s", want)
			}
		}
	}
	for _, want := range []string{`a.hasAttribute("download")`, "window.open = function", `addEventListener("navigate"`, "e.downloadRequest", "arenileOpenExternal"} {
		if !strings.Contains(LinkScript, want) {
			t.Errorf("LinkScript lacks %s", want)
		}
	}
}
