/** Parameters of the public page URL (spec 12.1). */
export interface BookingLocation {
  slug: string | null;
  embed: boolean;
  lang: string | null;
  from: string | null;
  to: string | null;
  pack: string | null;
  /** Origin of the host page, as declared by embed.js (verified separately). */
  parentOrigin: string | null;
}

const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

function date(value: string | null): string | null {
  return value && DATE_RE.test(value) ? value : null;
}

/** Parses `/b/{slug}[/...]?embed=1&lang=..&from=..&to=..&pack=..`. */
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
  return {
    slug,
    embed: embed === '1' || embed === 'true',
    lang: q.get('lang'),
    from: date(q.get('from')),
    to: date(q.get('to')),
    pack: q.get('pack'),
    parentOrigin: q.get('origin'),
  };
}
