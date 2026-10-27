/**
 * Views of the public SPA, chosen from the path without a router library
 * (first-screen budget, spec 12.6):
 * - `book`: `/b/{slug}` the four-step flow (and its confirmation);
 * - `manage`: `/b/{slug}/m/{token}` the customer manage area (spec 12.4);
 * - `waitlist`: `/b/{slug}/m/w/{token}` the personal waitlist link (spec
 *   10.5, M5B): under `/m/` so it gets the same no-referrer / no-store
 *   headers as the manage area.
 */
export type BookingView = 'book' | 'manage' | 'waitlist';

/** Parameters of the public page URL (spec 12.1). */
export interface BookingLocation {
  slug: string | null;
  view: BookingView;
  /**
   * Magic link token of the manage area (`/b/{slug}/m/{token}`), never
   * logged. `/b/{slug}/m` without a token (the return from a balance
   * payment: the token is never sent to the provider) resumes from the
   * token the page stored when the link was opened.
   */
  manageToken: string | null;
  /** Personal waitlist token of `/b/{slug}/m/w/{token}` (never logged). */
  waitlistToken: string | null;
  /**
   * Booking id of a checkout to resume (`?checkout=<id>`): the return URL of
   * the payment provider. The page reads the session token from
   * sessionStorage and polls the status; it never confirms (INV-19).
   */
  checkoutBookingId: string | null;
  embed: boolean;
  lang: string | null;
  from: string | null;
  to: string | null;
  pack: string | null;
  /** Origin of the host page, as declared by embed.js (verified separately). */
  parentOrigin: string | null;
  /**
   * Host page of the widget (`?return=`, set by embed.js): the provider
   * returns there after a payment, with `arenile_checkout=<id>` that embed.js
   * turns back into `?checkout=` of the iframe. The server accepts it only
   * on an allowed embed origin (no open redirect).
   */
  hostPage: string | null;
  /**
   * Theme preview of the admin panel (`?preview=1`, spec 12.5): the real page
   * in test mode, receiving the theme being edited from the same-origin panel;
   * nothing is booked.
   */
  preview: boolean;
  /** Forced colour scheme (`?theme=light|dark`, screenshots and host pages); null = system. */
  colorScheme: 'light' | 'dark' | null;
  /**
   * Agreement link `?c={code}` (spec 30.2.2): the partner publishes it or
   * sends it to its guests; the code goes into the quote as `agreement_code`.
   */
  agreementCode: string | null;
}

const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;
const TOKEN_RE = /^[A-Za-z0-9_-]{32,128}$/;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const AGREEMENT_CODE_RE = /^[A-Za-z0-9][A-Za-z0-9_-]{1,39}$/;

/** Absolute http(s) URL or null. */
function httpUrl(value: string | null): string | null {
  if (!value) return null;
  try {
    const u = new URL(value);
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : null;
  } catch {
    return null;
  }
}

/** Agreement code of `?c=` (case-insensitive, stored upper case), null when malformed. */
function agreementCode(value: string | null): string | null {
  const v = value?.trim() ?? '';
  return AGREEMENT_CODE_RE.test(v) ? v.toUpperCase() : null;
}

function date(value: string | null): string | null {
  return value && DATE_RE.test(value) ? value : null;
}

/** Parses `/b/{slug}[/m/{token}]?embed=1&lang=..&from=..&to=..&pack=..&checkout=..&preview=1&theme=..&c=..`. */
export function parseBookingLocation(pathname: string, search: string): BookingLocation {
  const match = /^\/b\/([^/?#]+)/.exec(pathname);
  let slug: string | null = null;
  if (match?.[1]) {
    try {
      const decoded = decodeURIComponent(match[1]).toLowerCase();
      slug = SLUG_RE.test(decoded) ? decoded : null;
    } catch {
      slug = null;
    }
  }
  const q = new URLSearchParams(search);
  const embed = q.get('embed');
  const waitlist = /^\/b\/[^/?#]+\/m\/w\/([^/?#]+)\/?$/.exec(pathname);
  const manage = /^\/b\/[^/?#]+\/m(?:\/([^/?#]*))?\/?$/.exec(pathname);
  const manageToken = manage?.[1] && TOKEN_RE.test(manage[1]) ? manage[1] : null;
  const checkout = q.get('checkout');
  const preview = q.get('preview');
  const scheme = q.get('theme');
  return {
    slug,
    view: waitlist ? 'waitlist' : manage ? 'manage' : 'book',
    manageToken,
    waitlistToken: waitlist?.[1] && TOKEN_RE.test(waitlist[1]) ? waitlist[1] : null,
    checkoutBookingId: checkout && UUID_RE.test(checkout) ? checkout.toLowerCase() : null,
    embed: embed === '1' || embed === 'true',
    lang: q.get('lang'),
    from: date(q.get('from')),
    to: date(q.get('to')),
    pack: q.get('pack'),
    parentOrigin: q.get('origin'),
    hostPage: embed === '1' || embed === 'true' ? httpUrl(q.get('return')) : null,
    preview: preview === '1' || preview === 'true',
    colorScheme: scheme === 'light' || scheme === 'dark' ? scheme : null,
    agreementCode: agreementCode(q.get('c')),
  };
}
