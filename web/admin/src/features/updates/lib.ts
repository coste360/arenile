/** Pure helpers of the update notice (ADR 0025). */

/** localStorage key of the version whose banner was dismissed. */
export const DISMISSED_KEY = 'arenile.update.dismissed';

/** Upgrade guide of the documentation (Docker: backup, then pull and up). */
export function upgradeDocsUrl(language: string): string {
  const base = 'https://docs.arenile.app';
  return language.startsWith('it')
    ? `${base}/installazione/aggiornamenti/`
    : `${base}/en/installazione/aggiornamenti/`;
}

/** The dismissed version, or null (storage may be unavailable). */
export function readDismissed(storage: Pick<Storage, 'getItem'> | undefined): string | null {
  try {
    return storage?.getItem(DISMISSED_KEY) ?? null;
  } catch {
    return null;
  }
}

/** Remembers the dismissed version; a failing storage only loses the memory. */
export function writeDismissed(storage: Pick<Storage, 'setItem'> | undefined, version: string) {
  try {
    storage?.setItem(DISMISSED_KEY, version);
  } catch {
    // Private window or blocked storage: the banner comes back on reload.
  }
}

/** The browser storage, if reachable. */
export function browserStorage(): Storage | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
}
