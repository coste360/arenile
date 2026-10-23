package device

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Pure rules of the device use cases (functional core): input validation,
// effective role, listing order and pagination. No I/O, no clock.

// Field limits (api.DevicePairRequest).
const (
	MaxNameLen       = 80
	MaxAppVersionLen = 32
)

// IdleExpiry: a device token that has not been used for this long no
// longer authenticates (spec 29.2 gives the token the role permissions
// with no second factor; an idle phone in a drawer must not keep them
// forever). The device stays listed until the owner revokes it, and a new
// pairing is needed to use it again.
const IdleExpiry = 90 * 24 * time.Hour

// IdleExpired reports whether d has been idle longer than IdleExpiry at
// now (pure): idle since last_used_at, or since the pairing when the
// token was never used.
func IdleExpired(d *store.Device, now time.Time) bool {
	last := d.CreatedAt
	if d.LastUsedAt != nil && d.LastUsedAt.After(last) {
		last = *d.LastUsedAt
	}
	return now.Sub(last) > IdleExpiry
}

var (
	platforms = []string{
		store.DevicePlatformIOS, store.DevicePlatformAndroid, store.DevicePlatformWindows,
		store.DevicePlatformMacOS, store.DevicePlatformLinux, store.DevicePlatformOther,
	}
	kinds = []string{store.DeviceKindMobile, store.DeviceKindDesktop}
)

// ValidKind reports whether k is a device kind.
func ValidKind(k string) bool { return slices.Contains(kinds, k) }

// ValidPlatform reports whether p is a device platform.
func ValidPlatform(p string) bool { return slices.Contains(platforms, p) }

// pairingTTL resolves the requested lifetime of a code (0 = default).
func pairingTTL(ttl time.Duration) (time.Duration, error) {
	if ttl == 0 {
		return DefaultPairingTTL, nil
	}
	if ttl < MinPairingTTL || ttl > MaxPairingTTL {
		return 0, configerr.Invalid("ttl_seconds", "deve essere tra 60 e 900 secondi")
	}
	return ttl, nil
}

// normalizePairInput trims and validates the device fields of PairInput
// (the code is checked separately, so that a bad code is always
// ErrPairingCodeInvalid).
func normalizePairInput(in PairInput) (PairInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.AppVersion = strings.TrimSpace(in.AppVersion)
	ve := &configerr.ValidationError{}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > MaxNameLen || strings.IndexFunc(in.Name, unicode.IsControl) >= 0 {
		ve.Add("name", "nome del dispositivo obbligatorio, al massimo 80 caratteri")
	}
	if !ValidPlatform(in.Platform) {
		ve.Add("platform", "piattaforma non valida")
	}
	if !ValidKind(in.Kind) {
		ve.Add("kind", "tipo di dispositivo non valido")
	}
	if len(in.AppVersion) > MaxAppVersionLen || strings.IndexFunc(in.AppVersion, unicode.IsControl) >= 0 {
		ve.Add("app_version", "al massimo 32 caratteri")
	}
	return in, ve.OrNil()
}

// effectiveRole returns the most privileged role among the grants of
// userID (auth.Roles is ordered most privileged first). ok is false when
// the user has no role on the establishment.
func effectiveRole(grants []store.TeamGrant, userID string) (auth.Role, bool) {
	best := -1
	for _, g := range grants {
		if g.UserID != userID {
			continue
		}
		if i := slices.Index(auth.Roles, auth.Role(g.Role)); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	return auth.Roles[best], true
}

// activeMember reports whether userID is active and has a role on the
// establishment of grants.
func activeMember(grants []store.TeamGrant, userID string) bool {
	for _, g := range grants {
		if g.UserID == userID && g.UserActive && auth.Role(g.Role).Valid() {
			return true
		}
	}
	return false
}

// sortKey is the listing order of a device: last use (or pairing, if
// never used) most recent first, then id descending for a total order.
type sortKey struct {
	at time.Time
	id string
}

func keyOf(d store.Device) sortKey {
	if d.LastUsedAt != nil {
		return sortKey{at: *d.LastUsedAt, id: d.ID}
	}
	return sortKey{at: d.CreatedAt, id: d.ID}
}

// compare orders a before b (negative) when a comes first in the listing.
func (a sortKey) compare(b sortKey) int {
	if c := b.at.Compare(a.at); c != 0 {
		return c
	}
	return strings.Compare(b.id, a.id)
}

// String is the cursor form of the key ("<unix nanos>.<id>").
func (a sortKey) String() string { return strconv.FormatInt(a.at.UnixNano(), 10) + "." + a.id }

func parseSortKey(s string) (sortKey, bool) {
	ns, id, ok := strings.Cut(s, ".")
	n, err := strconv.ParseInt(ns, 10, 64)
	if !ok || err != nil || id == "" {
		return sortKey{}, false
	}
	return sortKey{at: time.Unix(0, n).UTC(), id: id}, true
}

// sortViews orders devices for the listing (see sortKey).
func sortViews(vs []View) {
	slices.SortStableFunc(vs, func(a, b View) int { return keyOf(a.Device).compare(keyOf(b.Device)) })
}

// paginate returns the page of sorted vs that follows the key encoded in
// after (empty: first page) and the key to continue from ("" at the end).
// Keyset on the sort key, so devices revoked or used between two pages do
// not break the cursor (a device used meanwhile moves to the top and is
// not repeated). ok is false for a malformed key.
func paginate(vs []View, after string, limit int) (page []View, next string, ok bool) {
	start := 0
	if after != "" {
		k, valid := parseSortKey(after)
		if !valid {
			return nil, "", false
		}
		start = len(vs)
		if i := slices.IndexFunc(vs, func(v View) bool { return k.compare(keyOf(v.Device)) < 0 }); i >= 0 {
			start = i
		}
	}
	end := min(start+limit, len(vs))
	page = vs[start:end]
	if end < len(vs) && len(page) > 0 {
		next = keyOf(page[len(page)-1].Device).String()
	}
	return page, next, true
}
