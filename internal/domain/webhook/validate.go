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

// localHosts may be reached over plain http (development receivers) when
// the egress policy allows private targets.
var localHosts = []string{"localhost", "127.0.0.1", "::1"}

// ValidateURL checks an endpoint URL and returns it trimmed ("" when
// invalid, with the error added to ve): absolute https (http only towards
// localhost, 127.0.0.1 and ::1), a host, no userinfo, no fragment, at most
// MaxURLBytes. The host must also be allowed by the egress policy: an IP
// literal is checked directly and localhost, 127.0.0.1 and ::1 are refused
// unless the policy allows private targets, so that an endpoint the
// Deliverer would block at every attempt (and then disable) is refused at
// once with the same message. Other host names are checked by the
// Deliverer at connect time, after DNS resolution, because their addresses
// can change.
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
	const blocked = "Indirizzo non raggiungibile dai webhook: è privato, di loopback o riservato."
	if a, err := netip.ParseAddr(host); err == nil {
		if err := policy.Check(a); err != nil {
			ve.Add(field, blocked)
			return ""
		}
	} else if slices.Contains(localHosts, host) && !policy.AllowPrivate {
		ve.Add(field, blocked)
		return ""
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

// RedactURL returns u without its query string, "?…" marking that one was
// present: receivers often carry a token in the query, and tokens never
// reach the audit log or the logs in clear (AGENTS.md). The full URL lives
// only in the endpoint row. An unparsable value is returned as is up to
// the first '?'.
func RedactURL(u string) string {
	raw, query, hasQuery := strings.Cut(u, "?")
	if !hasQuery || query == "" {
		return raw
	}
	return raw + "?…"
}
