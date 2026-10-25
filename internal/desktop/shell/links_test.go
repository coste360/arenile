package shell

import (
	"testing"
	"unsafe"
)

func TestNavigationAction_Policy(t *testing.T) {
	const panel = "http://127.0.0.1:51234/admin/oggi"
	cases := []struct {
		name, panel, target string
		want                LinkAction
	}{
		{"panel page", panel, "http://127.0.0.1:51234/admin/prenotazioni", LinkAllow},
		{"api download", panel, "http://127.0.0.1:51234/api/admin/v1/export.csv", LinkAllow},
		{"redirect to another origin", panel, "https://checkout.stripe.com/c/pay", LinkExternal},
		{"public page on the same origin", panel, "http://127.0.0.1:51234/b/lido-sole", LinkExternal},
		{"dot segments leave the panel", panel, "http://127.0.0.1:51234/admin/%2e%2e/b/lido", LinkExternal},
		{"mailto", panel, "mailto:info@example.com", LinkExternal},
		{"other port", panel, "http://127.0.0.1:9/admin/", LinkExternal},
		{"about:blank of the quit", panel, "about:blank", LinkAllow},
		{"blob", panel, "blob:http://127.0.0.1:51234/1f0e", LinkAllow},
		{"first load, nothing shown", "", "http://127.0.0.1:51234/admin/", LinkAllow},
		{"no web page shown", "about:blank", "https://example.com/", LinkAllow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NavigationAction(c.panel, c.target); got != c.want {
				t.Fatalf("NavigationAction(%q, %q) = %d, want %d", c.panel, c.target, got, c.want)
			}
		})
	}
}

func TestNewWindowAction_Policy(t *testing.T) {
	const panel = "https://lido.example.it/admin/"
	cases := []struct {
		name, panel, target string
		want                LinkAction
	}{
		{"other origin", panel, "https://docs.arenile.app/", LinkExternal},
		{"public page", panel, "https://lido.example.it/b/lido-sole", LinkExternal},
		{"panel page in place", panel, "https://lido.example.it/admin/cassa", LinkInPlace},
		{"server document keeps the popup", panel, "https://lido.example.it/api/admin/v1/z-report/1.pdf", LinkAllow},
		{"blob keeps the popup", panel, "blob:https://lido.example.it/1f0e", LinkAllow},
		{"about:blank keeps the popup", panel, "about:blank", LinkAllow},
		{"mailto", panel, "mailto:info@example.com", LinkExternal},
		{"nothing shown yet", "", "https://docs.arenile.app/", LinkAllow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NewWindowAction(c.panel, c.target); got != c.want {
				t.Fatalf("NewWindowAction(%q, %q) = %d, want %d", c.panel, c.target, got, c.want)
			}
		})
	}
}

func TestWebviewHandle_Shapes(t *testing.T) {
	// fakeWebview has the shape of webview_go's unexported struct.
	type fakeWebview struct{ w unsafe.Pointer }
	x := 1
	p := unsafe.Pointer(&x)
	type twoFields struct {
		w unsafe.Pointer
		n int
	}
	type wrongKind struct{ w uintptr }
	cases := []struct {
		name string
		in   any
		want unsafe.Pointer
		ok   bool
	}{
		{"webview_go shape", &fakeWebview{w: p}, p, true},
		{"nil handle", &fakeWebview{}, nil, false},
		{"nil pointer", (*fakeWebview)(nil), nil, false},
		{"not a pointer", fakeWebview{w: p}, nil, false},
		{"more fields", &twoFields{w: p}, nil, false},
		{"not a pointer field", &wrongKind{w: 1}, nil, false},
		{"nil", nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := webviewHandle(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("webviewHandle = %v %v, want %v %v", got, ok, c.want, c.ok)
			}
		})
	}
}
