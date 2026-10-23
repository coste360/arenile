package webhook

import (
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
)

// Validation bounds.
const (
	MaxURLBytes        = 2048
	MaxDescriptionRune = 200
	MaxEventFilters    = 50
)

// localHosts may be reached over plain http (development receivers).
var localHosts = []string{"localhost", "127.0.0.1", "::1"}

// ValidateURL checks an endpoint URL and returns it trimmed ("" when
// invalid, with the error added to ve): absolute https (http only towards
// localhost, 127.0.0.1 and ::1), a host, no userinfo, no fragment, at most
// MaxURLBytes. An IP literal host must also be allowed by the egress policy
// (a private literal is refused at once instead of failing every delivery).
// Host names are checked by the Deliverer at connect time, after DNS
// resolution, because their addresses can change.
func ValidateURL(ve *configerr.ValidationError, raw string, policy event.EgressPolicy) string {
	s := strings.TrimSpace(raw)
	const field = "url"
	if s == "" || len(s) > MaxURLBytes {
		ve.Add(field, fmt.Sprintf("Indica un URL di al massimo %d caratteri.", MaxURLBytes))
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || !u.IsAbs() || u.Host == "" || u.Opaque != "" {
		ve.Add(field, "URL non valido: usa la forma https://esempio.it/webhook.")
		return ""
	}
	if u.User != nil {
		ve.Add(field, "L'URL non può contenere utente e password: usa il segreto di firma.")
		return ""
	}
	if u.Fragment != "" || strings.Contains(s, "#") {
		ve.Add(field, "L'URL non può contenere un frammento (#).")
		return ""
	}
	host := strings.ToLower(u.Hostname())
	switch u.Scheme {
	case "https":
	case "http":
		if !slices.Contains(localHosts, host) {
			ve.Add(field, "Usa https: http è ammesso solo verso localhost per le prove.")
			return ""
		}
	default:
		ve.Add(field, "Usa https.")
		return ""
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if err := policy.Check(a); err != nil {
			ve.Add(field, "Indirizzo non raggiungibile dai webhook: è privato, di loopback o riservato.")
			return ""
		}
	}
	return s
}

// ValidateEvents checks the event filters and returns them deduplicated in
// the given order: a known type (EventTypes), "<group>.*" with a known group
// (e.g. "booking.*") or "*" (all events). At least one is required: "every
// event" is the explicit "*", never an empty list.
func ValidateEvents(ve *configerr.ValidationError, in []string) []string {
	if len(in) == 0 {
		ve.Add("events", "Scegli almeno un evento (oppure * per tutti).")
		return nil
	}
	if len(in) > MaxEventFilters {
		ve.Add("events", fmt.Sprintf("Al massimo %d filtri.", MaxEventFilters))
		return nil
	}
	out := make([]string, 0, len(in))
	for _, f := range in {
		f = strings.TrimSpace(f)
		if !validFilter(f) {
			ve.Add("events", fmt.Sprintf("Evento sconosciuto: %q.", f))
			return nil
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

func validFilter(f string) bool {
	if f == "*" || slices.Contains(EventTypes, f) {
		return true
	}
	group, ok := strings.CutSuffix(f, ".*")
	if !ok || group == "" {
		return false
	}
	for _, t := range EventTypes {
		if strings.HasPrefix(t, group+".") {
			return true
		}
	}
	return false
}

func validateDescription(ve *configerr.ValidationError, raw string) string {
	d := strings.TrimSpace(raw)
	if utf8.RuneCountInString(d) > MaxDescriptionRune {
		ve.Add("description", fmt.Sprintf("Al massimo %d caratteri.", MaxDescriptionRune))
		return ""
	}
	return d
}
