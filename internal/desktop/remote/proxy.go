package remote

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Proxy serves an instance with a self-signed certificate on
// http://127.0.0.1:<random port> for the panel window. The system webview
// (WebKit, WebView2) cannot pin a certificate, and the operator must not
// be trained to click through certificate warnings: the proxy talks to the
// instance with the pinned client (same fingerprint as the device token
// and the fiscal bridge) and the window talks to the proxy over loopback.
//
// Security: it listens on loopback only, refuses any Host other than its
// own address (DNS rebinding), and never adds credentials: requests carry
// only the window's own session cookie, so another local process gains
// nothing it could not get from the instance directly. Set-Cookie loses
// the Secure attribute (the loopback hop is plain HTTP; the network hop is
// TLS) and redirects to the instance are rewritten to the proxy.
type Proxy struct {
	URL      string
	srv      *http.Server
	upstream *url.URL
}

// StartProxy starts the proxy towards upstream (scheme://host[:port])
// with the pinned client transport. It stops when ctx is done or Close.
func StartProxy(ctx context.Context, upstream string, transport http.RoundTripper, log *slog.Logger) (*Proxy, error) {
	up, err := url.Parse(upstream)
	if err != nil || up.Host == "" {
		return nil, errors.New("remote: invalid upstream")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{URL: "http://" + ln.Addr().String(), upstream: up}
	rp := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(up)
			r.Out.Host = up.Host
			if r.In.Header.Get("Origin") != "" {
				r.Out.Header.Set("Origin", up.Scheme+"://"+up.Host)
			}
			if ref := r.In.Header.Get("Referer"); ref != "" {
				r.Out.Header.Set("Referer", strings.Replace(ref, p.URL, up.Scheme+"://"+up.Host, 1))
			}
			r.Out.Header.Del("X-Forwarded-For")
		},
		ModifyResponse: func(resp *http.Response) error {
			RewriteResponse(resp.Header, up.Scheme+"://"+up.Host, p.URL)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if log != nil {
				log.Warn("remote proxy: instance not reachable", slog.String("error", err.Error()))
			}
			http.Error(w, "Istanza Arenile non raggiungibile: controlla la connessione.", http.StatusBadGateway)
		},
	}
	self := ln.Addr().String()
	p.srv = &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != self {
				http.Error(w, "host non valido", http.StatusMisdirectedRequest)
				return
			}
			rp.ServeHTTP(w, r)
		}),
	}
	go func() { _ = p.srv.Serve(ln) }()
	go func() {
		<-ctx.Done()
		_ = p.Close()
	}()
	return p, nil
}

// Close stops the proxy.
func (p *Proxy) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.srv.Shutdown(ctx)
}

// RewriteResponse adapts the response headers of the instance to the
// loopback hop (pure over the header map): Location pointing to the
// instance points to the proxy, and cookies lose Secure and Domain.
func RewriteResponse(h http.Header, upstreamOrigin, proxyOrigin string) {
	if loc := h.Get("Location"); strings.HasPrefix(loc, upstreamOrigin) {
		h.Set("Location", proxyOrigin+strings.TrimPrefix(loc, upstreamOrigin))
	}
	cookies := h.Values("Set-Cookie")
	if len(cookies) == 0 {
		return
	}
	h.Del("Set-Cookie")
	for _, c := range cookies {
		parts := strings.Split(c, ";")
		kept := parts[:1]
		for _, a := range parts[1:] {
			name := strings.ToLower(strings.TrimSpace(strings.SplitN(a, "=", 2)[0]))
			if name == "secure" || name == "domain" {
				continue
			}
			kept = append(kept, a)
		}
		h.Add("Set-Cookie", strings.Join(kept, ";"))
	}
}
