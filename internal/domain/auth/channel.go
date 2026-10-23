package auth

import "github.com/coste360/arenile/internal/store"

// ChannelFor returns the sales channel of a booking or hold created by p.
// A request authenticated with an API key is always channel "api" whatever
// it asks for: an integration is an external channel like the public page
// and must respect the online quota (ADR 0011 §1, INV-62); only staff from
// the panel may choose "desk" (or "online"/"api" when re-keying a sale).
func ChannelFor(p *Principal, requested string) string {
	if p != nil && p.Kind == PrincipalAPIKey {
		return store.ChannelAPI
	}
	return requested
}
