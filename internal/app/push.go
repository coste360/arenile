package app

import (
	"log/slog"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/domain/webhook"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/pushrelay"
	"github.com/coste360/arenile/pkg/ext"
)

// Compile-time checks of the push ports (ADR 0027).
var (
	_ push.Relay                     = pushrelay.Client{}
	_ push.Principals                = (*auth.Service)(nil)
	_ interface{ Jobs() []jobs.Job } = (*push.Service)(nil)
)

// wirePush builds the push notification service. The relay client exists
// only when ARENILE_PUSH_RELAY_KEY is set on the server edition
// (config.PushRelayConfigured): otherwise nothing is queued and no request
// leaves the instance. Requires a.Auth.
func (a *App) wirePush(aw *audit.Writer) {
	var relay push.Relay
	if a.cfg.PushRelayConfigured() {
		relay = pushrelay.Client{URL: a.cfg.PushRelayURL, Key: a.cfg.PushRelayKey, Version: a.opts.version, Now: a.opts.clock.Now}
		// The key logs as "<id>.***" (pushrelay.Key.LogValue); the URL without
		// its query string, which could carry a credential.
		a.log.Info("push notifications available through the relay",
			slog.String("relay_url", webhook.RedactURL(a.cfg.PushRelayURL)), slog.Any("relay_key", a.cfg.PushRelayKey))
	}
	a.Push = push.NewService(push.Deps{
		DB: a.db, Clock: a.opts.clock, Audit: aw, Log: a.log, Keys: a.keys,
		Principals: a.Auth, Relay: relay, IdleExpired: device.IdleExpired,
	})
}

// pushSinks are the outbox sinks of the push service.
func (a *App) pushSinks() []ext.EventSink { return []ext.EventSink{a.Push.Sink()} }
