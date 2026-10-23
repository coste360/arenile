package fiscalbridge

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/pkg/ext"
)

// AgentDeps are the dependencies of the desktop-side Agent.
type AgentDeps struct {
	// BaseURL of the remote instance (from the pairing QR).
	BaseURL string
	// Token is the device token (read from the OS keyring by the launcher;
	// never logged).
	Token string
	// HTTPClient must pin the certificate fingerprint of the QR when the
	// instance uses a self-signed certificate (see desktop.PinnedClient).
	HTTPClient *http.Client
	// Driver is the local FiscalDriver (printer / registratore telematico).
	Driver ext.FiscalDriver
	Clock  clock.Clock
	Log    *slog.Logger
}

// Agent runs on the cash desk PC (desktop app in remote mode): it registers
// as the bridge, long-polls the instance, executes jobs on the local driver
// and submits the outcome, with exponential backoff on network errors.
type Agent struct{ d AgentDeps }

// NewAgent returns an Agent.
func NewAgent(d AgentDeps) *Agent {
	if d.HTTPClient == nil {
		d.HTTPClient = http.DefaultClient
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Agent{d: d}
}

// Run blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	return ErrNotImplemented
}
