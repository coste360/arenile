// Package online owns the configuration of the public booking page (spec
// 12, 13.1, 19, 23): online flow settings, theme and embedding origins,
// versioned terms, the active payment provider with its encrypted
// credentials, and the email settings of an establishment.
//
// Ownership (M4): the "online settings" builder owns this package, the
// handlers in internal/http/admin/online.go, paymentsettings.go and
// email.go (settings part), the repository functions in
// internal/store/online.go and the models terms_versions, online_settings,
// payment_provider_settings (internal/store/models_online.go).
//
// # Architecture (functional core, imperative shell)
//
//   - config.go (pure): Config / EmailConfig defaults, validation of
//     origins (https://host[:port], no paths, no wildcards), theme colours
//     (#RRGGBB) and contrast-safe text colour, formulas and slots;
//     ParseConfig tolerates unknown fields (forward compatible JSON).
//   - service.go (shell): admin use cases with *auth.Principal and
//     optimistic concurrency (If-Match on online_settings.version), each in
//     one store.RunInTx with audit ("online.settings_updated",
//     "online.terms_published", "payments.settings_updated" without secret
//     values) and an outbox event (event.ConfigOnlineChanged).
//   - ports for the other M4 packages (read side, no principal, called by
//     public flows and jobs): ResolvePublic, ActiveProvider,
//     ProviderConfig, CurrentTerms, EmailConfigFor. They are declared as
//     interfaces in the consumers (checkout.Settings,
//     gateway.ProviderSettings, notify.Settings) and asserted in
//     internal/app/m4.go.
//
// # Secrets
//
// Provider credentials (ext.Credentials) are encrypted with the key ring
// (internal/crypto, AES-256-GCM) with AAD "<establishment_id>:<provider>:<mode>"
// before being written to payment_provider_settings.credentials; they are
// decrypted only inside ActiveProvider / ProviderConfig and never returned
// by the API (only "configured", last4, publishable key). A missing secret
// field in an update keeps the stored value, an empty string clears it.
// Never log credentials or tokens.
//
// # Permissions (spec 16.1)
//
// Online settings, terms and email settings: auth.PermEstablishmentConfigure
// (reads auth.PermBookingsRead); payment settings: auth.PermPaymentsConfigure.
package online
