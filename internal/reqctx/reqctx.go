// Package reqctx carries per-request metadata (request id, client IP,
// scheme, authentication method) through the request context.
package reqctx

import "context"

// AuthMethod tells how the principal authenticated.
type AuthMethod string

const (
	AuthNone   AuthMethod = ""
	AuthCookie AuthMethod = "cookie"
	AuthBearer AuthMethod = "bearer"
)

// Info is the per-request metadata. It is created by the request context
// middleware and mutated only by middlewares, before handlers run.
type Info struct {
	RequestID  string
	ClientIP   string
	HTTPS      bool
	AuthMethod AuthMethod
	// SessionToken is the raw session cookie when AuthMethod is AuthCookie.
	SessionToken string
}

type key struct{}

// With stores info in ctx.
func With(ctx context.Context, info *Info) context.Context {
	return context.WithValue(ctx, key{}, info)
}

// From returns the request info, or an empty Info when missing.
func From(ctx context.Context) *Info {
	if i, ok := ctx.Value(key{}).(*Info); ok && i != nil {
		return i
	}
	return &Info{}
}
