#!/usr/bin/env bash
# M4 real-binary acceptance (spec 28, M4): drives a freshly built
# `bin/arenile` with curl only, like an integrator would.
#
#   1. serve with a temporary data dir, `seed demo`, fake Stripe as a process
#      (ARENILE_STRIPE_API_URL, credentials of internal/.../stripetest);
#   2. owner login, Stripe (fake, live-mode key: INV-88) through the admin API;
#   3. public flow: create (hold) -> customer details -> pay -> the fake
#      Stripe page completes the session and posts a SIGNED webhook ->
#      confirmed (polling never confirms by itself);
#   4. confirmation email with magic link in ARENILE_MAIL_DIR, manage page,
#      QR (svg) and .ics, "send me a new link" answers 202;
#   5. cancellation from the manage area refunded through the provider;
#   6. a second booking with the manual policy (pay at the desk).
#
# Usage: scripts/acceptance/m4-real-binary.sh   (after `make build`)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN="$ROOT/bin/arenile"
PORT="${M4_PORT:-8193}"
SPORT="${M4_STRIPE_PORT:-12113}"
BASE="http://127.0.0.1:$PORT"
SLUG=lido-demo
PUB="$BASE/api/public/v1/$SLUG"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/arenile-m4-XXXXXX")"
JAR="$WORK/cookies"
PASSWORD='Acceptance-Owner-2026!'
STRIPE_SK=sk_live_arenile_fake_0000
STRIPE_WHSEC=whsec_arenile_test_secret
pass=0

say() { printf '\n== %s\n' "$*"; }
ok() { pass=$((pass + 1)); printf '   ok %s\n' "$*"; }
die() { printf '   FAIL %s\n' "$*" >&2; exit 1; }
cleanup() {
  [[ -n "${SRV:-}" ]] && kill "$SRV" 2>/dev/null || true
  [[ -n "${FAKE:-}" ]] && kill "$FAKE" 2>/dev/null || true
  wait 2>/dev/null || true
  [[ "${KEEP:-}" == 1 ]] && echo "work dir kept: $WORK" || rm -rf "$WORK"
}
trap cleanup EXIT

[[ -x "$BIN" ]] || die "missing $BIN: run make build"
export ARENILE_DATA_DIR="$WORK/data" ARENILE_ADDR="127.0.0.1:$PORT" ARENILE_BASE_URL="$BASE" \
  ARENILE_SECRET_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' ARENILE_TELEMETRY=false ARENILE_UPDATE_CHECK=false \
  ARENILE_MAIL_DIR="$WORK/mail" ARENILE_ADMIN_PASSWORD="$PASSWORD" ARENILE_LOG_LEVEL=warn \
  ARENILE_STRIPE_API_URL="http://127.0.0.1:$SPORT"
mkdir -p "$ARENILE_DATA_DIR" "$ARENILE_MAIL_DIR"

say "build fake Stripe, migrate, seed demo, serve"
(cd "$ROOT" && go build -o "$WORK/stripefake" ./internal/testtools/stripefake)
"$WORK/stripefake" -addr "127.0.0.1:$SPORT" -arenile "$BASE" >"$WORK/stripe.log" 2>&1 &
FAKE=$!
"$BIN" migrate up >/dev/null
"$BIN" seed demo >"$WORK/seed.log"
"$BIN" serve >"$WORK/server.log" 2>&1 &
SRV=$!
for _ in $(seq 1 50); do curl -fs "$BASE/healthz" >/dev/null 2>&1 && break; sleep 0.2; done
curl -fs "$BASE/healthz" >/dev/null || die "server not up"
ok "serve on $BASE, fake Stripe on 127.0.0.1:$SPORT"
[[ -z "$(ls -A "$ARENILE_MAIL_DIR")" ]] && ok "seeded demo history sends no email" || die "emails sent for seeded data"

# --- admin -------------------------------------------------------------------
say "owner login, Stripe in live mode (fake)"
curl -fs -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d "{\"email\":\"demo@lido-demo.example\",\"password\":\"$PASSWORD\"}" "$BASE/api/admin/v1/auth/login" >/dev/null
CSRF="$(awk '$6=="arenile_csrf"{print $7}' "$JAR")"
EST="$(curl -fs "$PUB" | jq -r '.establishment_id // empty')"
if [[ -z "$EST" ]]; then
  EST="$(curl -fs -b "$JAR" "$BASE/api/admin/v1/establishments" | jq -r '.items[0].id')"
fi
admin() { # method path [json]
  curl -fsS -b "$JAR" -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' -X "$1" "${@:4}" \
    ${3:+-d "$3"} "$BASE/api/admin/v1/establishments/$EST$2"
}
ETAG="$(curl -fsS -b "$JAR" -D - -o /dev/null "$BASE/api/admin/v1/establishments/$EST/payment-settings" | awk 'tolower($1)=="etag:"{print $2}' | tr -d '\r')"
admin PUT /payment-settings "{\"provider\":\"stripe\",\"mode\":\"live\",\"stripe\":{\"secret_key\":\"$STRIPE_SK\",\"webhook_secret\":\"$STRIPE_WHSEC\"}}" -H "If-Match: $ETAG" >"$WORK/ps.json"
[[ "$(jq -r .provider "$WORK/ps.json")" == stripe ]] || die "payment settings: $(cat "$WORK/ps.json")"
grep -q "$STRIPE_SK" "$WORK/ps.json" && die "secret key returned by the API"
ok "stripe configured (secret never returned), webhook $(jq -r .webhook_url "$WORK/ps.json")"
[[ "$(admin POST /payment-settings/test '{}' | jq -r .ok)" == true ]] && ok "credentials verified against the (fake) provider"
[[ "$(curl -fs "$PUB" | jq -r .online.payment.online)" == true ]] && ok "public profile: online payment on, test mode $(curl -fs "$PUB" | jq -r .online.payment.test_mode)"

# --- public flow -------------------------------------------------------------
DAY="$(curl -fs "$PUB" | jq -r '.season.opens_on' | awk -F- '{printf "%s-06-15", $1}')"
PACK="$(curl -fs "$PUB/catalog?starts_on=$DAY&ends_on=$DAY&formula=day" | jq -r '.packs[] | select(.code=="base") | .id')"
free_spot() { curl -fs "$PUB/availability?from=$DAY&to=$DAY&slots=FULL&formula=day" | jq -r "[.zones[] | select(.assignment_mode==\"seat\") | .spots[] | select(.available)] | .[$1].spot_id"; }
create() { # spot -> CheckoutSession json
  curl -fsS -H 'Content-Type: application/json' -H "Idempotency-Key: $(uuidgen)" \
    -d "{\"starts_on\":\"$DAY\",\"ends_on\":\"$DAY\",\"slots\":\"FULL\",\"formula\":\"day\",\"lines\":[{\"pack_id\":\"$PACK\",\"spot_id\":\"$1\"}],\"locale\":\"it\"}" \
    "$PUB/bookings"
}
say "online booking paid with Stripe Checkout ($DAY)"
SPOT="$(free_spot 0)"
create "$SPOT" >"$WORK/c1.json"
BID="$(jq -r .booking_id "$WORK/c1.json")"
TOK="$(jq -r .checkout_token "$WORK/c1.json")"
[[ "$(jq -r .status "$WORK/c1.json")" == held ]] && ok "hold created ($BID), total $(jq -r .quote.total "$WORK/c1.json") cents"
code="$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -H "Idempotency-Key: $(uuidgen)" \
  -d "{\"starts_on\":\"$DAY\",\"ends_on\":\"$DAY\",\"slots\":\"FULL\",\"formula\":\"day\",\"lines\":[{\"pack_id\":\"$PACK\",\"spot_id\":\"$SPOT\"}]}" "$PUB/bookings")"
[[ "$code" == 409 ]] && ok "same spot while held -> 409" || die "second hold on the same spot answered $code"
TERMS="$(curl -fs "$PUB" | jq -r '.online.terms.version // ""')"
curl -fsS -X PATCH -H 'Content-Type: application/json' -H "X-Arenile-Checkout: $TOK" \
  -d "{\"first_name\":\"Marta\",\"last_name\":\"Verdi\",\"email\":\"marta.verdi@mailbox.test\",\"phone\":\"+393331234567\",\"accept_terms_version\":\"$TERMS\",\"marketing_consent\":false}" \
  "$PUB/bookings/$BID" >/dev/null
ok "customer details accepted"
curl -fsS -H 'Content-Type: application/json' -H "X-Arenile-Checkout: $TOK" -H "Idempotency-Key: $(uuidgen)" -d '{}' "$PUB/bookings/$BID/pay" >"$WORK/pay.json"
[[ "$(jq -r .mode "$WORK/pay.json")" == redirect ]] || die "pay: $(cat "$WORK/pay.json")"
REDIRECT="$(jq -r .redirect_url "$WORK/pay.json")"
ok "pay -> redirect to provider ($REDIRECT), hold until $(jq -r .hold_expires_at "$WORK/pay.json")"
[[ "$(curl -fs -H "X-Arenile-Checkout: $TOK" "$PUB/bookings/$BID" | jq -r .status)" != confirmed ]] && ok "status polling alone does not confirm"
LOC="$(curl -s -o /dev/null -w '%{redirect_url}' -d action=pay "$REDIRECT")"
[[ "$LOC" == "$BASE/b/$SLUG"* ]] && ok "fake Stripe posted the signed webhook, browser back to $LOC" || die "hosted page: $LOC ($(tail -3 "$WORK/stripe.log"))"
ST="$(curl -fs -H "X-Arenile-Checkout: $TOK" "$PUB/bookings/$BID")"
[[ "$(jq -r .status <<<"$ST")" == confirmed ]] && ok "confirmed by the webhook: $(jq -r .confirmation.code <<<"$ST"), paid $(jq -r .summary.paid <<<"$ST")" || die "state: $ST"
CODE="$(jq -r .confirmation.code <<<"$ST")"
bad="$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -H 'Stripe-Signature: t=1,v1=00' -d '{"id":"evt_forged","type":"checkout.session.completed"}' "$BASE/webhooks/payments/stripe/$EST")"
[[ "$bad" == 400 || "$bad" == 401 ]] && ok "forged webhook signature rejected ($bad)" || die "forged webhook answered $bad"
for p in qr ics; do
  ctype="$(curl -fs -o "$WORK/b.$p" -w '%{content_type}' -H "X-Arenile-Checkout: $TOK" "$PUB/bookings/$BID/$p")"
  ok "checkout $p: $ctype, $(wc -c <"$WORK/b.$p" | tr -d ' ') bytes"
done

say "confirmation email, magic link, manage area"
for _ in $(seq 1 40); do grep -lq "$CODE" "$ARENILE_MAIL_DIR"/*.eml 2>/dev/null && break; sleep 0.5; done
MAIL="$(grep -l "$CODE" "$ARENILE_MAIL_DIR"/*.eml | head -1)"
[[ -n "$MAIL" ]] || die "no confirmation email"
LINK="$(python3 - "$MAIL" <<'PY'
import email, re, sys
m = email.message_from_bytes(open(sys.argv[1], 'rb').read())
for part in m.walk():
    if part.get_content_type() == 'text/plain':
        r = re.search(r'https?://\S+/m/[A-Za-z0-9_-]+', part.get_payload(decode=True).decode())
        if r: print(r.group(0)); break
PY
)"
MTOK="${LINK##*/m/}"
ok "email $(basename "$MAIL") with QR + .ics attachments: $(grep -c 'Content-Disposition' "$MAIL") parts"
grep -q "$MTOK" "$WORK/server.log" && die "magic link token found in the server log"
ok "magic link token never logged"
MB="$(curl -fsS "$PUB/manage/$MTOK")"
[[ "$(jq -r .code <<<"$MB")" == "$CODE" ]] && ok "manage area: $(jq -r .status <<<"$MB"), actions $(jq -c .allowed <<<"$MB")"
[[ "$(curl -s -o /dev/null -w '%{http_code}' "$PUB/manage/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")" == 404 ]] && ok "unknown token -> 404"
for p in qr ics; do
  ctype="$(curl -fs -o "$WORK/m.$p" -w '%{content_type}' "$PUB/manage/$MTOK/$p")"
  ok "manage $p: $ctype"
done
grep -q 'BEGIN:VCALENDAR' "$WORK/m.ics" && grep -q "$CODE" "$WORK/m.ics" && ok ".ics is a VCALENDAR with the booking code"

say "cancel from the manage area, refund through the provider"
curl -fsS -H 'Content-Type: application/json' -H "If-Match: \"$(jq -r .version <<<"$MB")\"" -d '{"dry_run":true}' "$PUB/manage/$MTOK/cancel" >"$WORK/cprev.json"
ok "preview: refund $(jq -c .refund "$WORK/cprev.json")"
curl -fsS -H 'Content-Type: application/json' -H "If-Match: \"$(jq -r .version <<<"$MB")\"" -d '{"dry_run":false,"settlement":"refund"}' "$PUB/manage/$MTOK/cancel" >"$WORK/cancel.json"
[[ "$(jq -r .applied "$WORK/cancel.json")" == true ]] || die "cancel: $(cat "$WORK/cancel.json")"
ok "cancelled, refund_status $(jq -r .refund_status "$WORK/cancel.json")"
[[ "$(jq -r .refund_status "$WORK/cancel.json")" == provider_succeeded || "$(jq -r .refund_status "$WORK/cancel.json")" == provider_pending ]] || die "refund not through the provider"
for _ in $(seq 1 40); do
  PAID="$(curl -fs "$PUB/manage/$MTOK" | jq -r .summary.paid)"
  [[ "$PAID" == 0 ]] && break; sleep 0.5
done
[[ "$PAID" == 0 ]] && ok "provider refund recorded in the ledger (paid back to 0)" || die "refund not recorded, paid=$PAID"
[[ "$(free_spot 0)" != null ]] && curl -fs "$PUB/availability?from=$DAY&to=$DAY&slots=FULL&formula=day" | jq -e "[.zones[].spots[] | select(.spot_id==\"$SPOT\") | .available] | .[0]" >/dev/null && ok "spot $SPOT available again"

# "Send me a new link" revokes the previous links (spec 12.4): last.
for email in marta.verdi@mailbox.test nobody@mailbox.test; do
  c="$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d "{\"email\":\"$email\",\"code\":\"$CODE\"}" "$PUB/manage-links")"
  [[ "$c" == 202 ]] && ok "new link request for $email -> 202" || die "manage-links answered $c"
done

say "manual policy (pay at the desk)"
ETAG="$(curl -fsS -b "$JAR" -D - -o /dev/null "$BASE/api/admin/v1/establishments/$EST/payment-settings" | awk 'tolower($1)=="etag:"{print $2}' | tr -d '\r')"
admin PUT /payment-settings '{"provider":"manual","mode":"test"}' -H "If-Match: $ETAG" >/dev/null
create "$(free_spot 1)" >"$WORK/c2.json"
BID2="$(jq -r .booking_id "$WORK/c2.json")"
TOK2="$(jq -r .checkout_token "$WORK/c2.json")"
curl -fsS -X PATCH -H 'Content-Type: application/json' -H "X-Arenile-Checkout: $TOK2" \
  -d "{\"first_name\":\"Luca\",\"last_name\":\"Neri\",\"email\":\"luca.neri@mailbox.test\",\"accept_terms_version\":\"$TERMS\",\"marketing_consent\":true}" "$PUB/bookings/$BID2" >/dev/null
curl -fsS -H 'Content-Type: application/json' -H "X-Arenile-Checkout: $TOK2" -H "Idempotency-Key: $(uuidgen)" -d '{}' "$PUB/bookings/$BID2/pay" >"$WORK/pay2.json"
[[ "$(jq -r .mode "$WORK/pay2.json")/$(jq -r .status "$WORK/pay2.json")" == none/confirmed ]] && ok "manual driver confirms without online payment" || die "manual: $(cat "$WORK/pay2.json")"

printf '\nM4 real-binary acceptance: %d checks passed\n' "$pass"
