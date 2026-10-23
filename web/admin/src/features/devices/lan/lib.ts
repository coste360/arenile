/**
 * Pure helpers of the LAN info page (spec 29.1, "Modalità rete locale").
 */

/** Address of the panel on the LAN listener (`lan_url` + `/admin/`). */
export function panelAddress(lanUrl: string): string {
  return `${lanUrl.replace(/\/+$/, '')}/admin/`;
}

/**
 * Splits a SHA-256 fingerprint (`AB:CD:…`, 32 bytes) into lines of 8 bytes,
 * the way browsers show it in the certificate details, so the operator can
 * compare it line by line.
 */
export function fingerprintLines(fingerprint: string, bytesPerLine = 8): string[] {
  const bytes = fingerprint
    .toUpperCase()
    .split(/[:\s]+/)
    .filter((b) => b.length > 0);
  const lines: string[] = [];
  for (let i = 0; i < bytes.length; i += bytesPerLine) {
    lines.push(bytes.slice(i, i + bytesPerLine).join(':'));
  }
  return lines;
}

/** Host and port of the LAN URL, shown large (e.g. `192.168.1.20:8443`). */
export function hostOf(lanUrl: string): string {
  try {
    return new URL(lanUrl).host;
  } catch {
    return lanUrl;
  }
}

/** The panel warns this many days before the certificate is regenerated (lan.RenewWarnBefore). */
export const CERT_RENEWAL_WARN_DAYS = 60;

/**
 * True when the self-signed certificate will be regenerated within
 * CERT_RENEWAL_WARN_DAYS (meta desktop.lan_cert_renews_at): paired devices
 * must then be paired again and browsers accept the new certificate, so the
 * page says so ahead of time. Pure: `now` is injected for tests.
 */
export function certRenewalSoon(renewsAt: string | undefined, now: number = Date.now()): boolean {
  if (!renewsAt) return false;
  const at = new Date(renewsAt).getTime();
  if (Number.isNaN(at)) return false;
  return at - now <= CERT_RENEWAL_WARN_DAYS * 86_400_000;
}
