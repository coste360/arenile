/**
 * Magic link token store of the manage area (spec 12.4, 22): the token of
 * `/b/{slug}/m/{token}` is kept in `sessionStorage` (per tab) so that the
 * page can resume at `/b/{slug}/m` after a balance payment, whose return
 * URL carries no token (the provider would store it with the session and
 * show it in its dashboard and logs). Never a cookie, never logged.
 */
const PREFIX = 'arenile:manage:';

export interface ManageTokenStore {
  load(slug: string): string | null;
  save(slug: string, token: string): void;
}

export function createManageTokenStore(storage?: Storage | null): ManageTokenStore {
  const memory = new Map<string, string>();
  return {
    load(slug) {
      try {
        const stored = storage?.getItem(PREFIX + slug);
        if (stored) return stored;
      } catch {
        // storage blocked: memory only
      }
      return memory.get(slug) ?? null;
    },
    save(slug, token) {
      memory.set(slug, token);
      try {
        storage?.setItem(PREFIX + slug, token);
      } catch {
        // memory already updated
      }
    },
  };
}

function storageOf(win: Window): Storage | null {
  try {
    return win.sessionStorage;
  } catch {
    return null;
  }
}

/**
 * Resolves the manage token of the page: the one in the path (stored for
 * later), else the one stored for this slug in this tab.
 */
export function resolveManageToken(win: Window, slug: string, fromPath: string | null): string | null {
  const store = createManageTokenStore(storageOf(win));
  if (fromPath) {
    store.save(slug, fromPath);
    return fromPath;
  }
  return store.load(slug);
}
