/**
 * Test-mode preview link of the public page (spec 15.4, ADR 0019):
 * `/b/{slug}?preview=<token>` created by the panel. The token is kept in
 * sessionStorage for the slug (it must survive the return from the payment
 * provider) and sent as `X-Arenile-Preview` on every public request; the
 * server decides whether the page may book in test mode. `?preview=1` is
 * the theme preview of the panel (location.ts), never a token.
 */

export const PREVIEW_HEADER = 'X-Arenile-Preview';

/** Shape of a key-ring sealed token ("v1:<kid>:<base64url>"), never logged. */
const TOKEN_RE = /^v\d+:[A-Za-z0-9_-]{1,64}:[A-Za-z0-9_-]{16,400}$/;

const storageKey = (slug: string) => `arenile.testPreview.${slug}`;

/** Reads the preview token of `slug` from the query (storing it) or from the session. */
export function resolveTestPreviewToken(
  slug: string | null,
  search: string,
  storage: Pick<Storage, 'getItem' | 'setItem'> | undefined = safeSession(),
): string | null {
  if (!slug) return null;
  const fromQuery = new URLSearchParams(search).get('preview');
  if (fromQuery && TOKEN_RE.test(fromQuery)) {
    try {
      storage?.setItem(storageKey(slug), fromQuery);
    } catch {
      // Private mode: the token still works for this page load.
    }
    return fromQuery;
  }
  try {
    const stored = storage?.getItem(storageKey(slug)) ?? null;
    return stored && TOKEN_RE.test(stored) ? stored : null;
  } catch {
    return null;
  }
}

function safeSession(): Storage | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.sessionStorage;
  } catch {
    return undefined;
  }
}
