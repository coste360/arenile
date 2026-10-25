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

/** All versions and systems (arenile.app): the secondary download link. */
export const DOWNLOADS_PAGE = 'https://arenile.app/download/';

/** Refresh of the status while the server's first check is pending. */
export const PENDING_REFETCH_MS = 15_000;

/** Regular refresh: the server checks once a day, hourly is plenty. */
export const STATUS_REFETCH_MS = 60 * 60_000;

/**
 * Refetch cadence of the update status (pure): every 15 s while the server
 * has not completed its first check after start-up (the status would say
 * "not checked yet" for an hour otherwise), then hourly.
 */
export function updateRefetchInterval(
  data: { pending?: boolean; check_enabled?: boolean } | undefined,
): number {
  return data?.pending && data.check_enabled ? PENDING_REFETCH_MS : STATUS_REFETCH_MS;
}

/**
 * Human name of the platform of the installation (pure), for "Scarica
 * Arenile X per macOS (Apple Silicon)"; undefined when unknown. Product
 * and system names are not translated.
 */
export function platformLabel(
  os: string | undefined,
  arch: string | undefined,
): string | undefined {
  switch (os) {
    case 'darwin':
      if (arch === 'arm64') return 'macOS (Apple Silicon)';
      if (arch === 'amd64') return 'macOS (Intel)';
      return 'macOS';
    case 'windows':
      return arch === 'arm64' ? 'Windows (ARM)' : 'Windows';
    case 'linux':
      return arch === 'arm64' ? 'Linux (ARM64)' : 'Linux';
    default:
      return undefined;
  }
}

/**
 * Whether the download URL is a package file rather than the downloads page
 * (pure): the label names the system only for a package.
 */
export function isPackageUrl(url: string | undefined): boolean {
  return !!url && url.replace(/\/+$/, '') !== DOWNLOADS_PAGE.replace(/\/+$/, '');
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
