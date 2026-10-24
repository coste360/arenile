import type { StatusTone } from '@arenile/shared/ui/patterns';
import type { PrintDocumentKind, PrintJobStatus, PrintRoute, Printer, PrinterStatus } from './api';

/** Paper widths offered in the form (mm). */
export const PAPER_WIDTHS = [58, 80] as const;
export type PaperWidth = (typeof PAPER_WIDTHS)[number];

/** Font A characters per line by paper width (Epson defaults). */
export function defaultCharsPerLine(width: PaperWidth): number {
  return width === 58 ? 32 : 48;
}

/** Raw TCP port of ESC/POS network printers (JetDirect / AppSocket). */
export const DEFAULT_PORT = 9100;

/** Documents a route can be defined for (the test page ignores routing). */
export const ROUTABLE_KINDS = [
  'daily_ticket',
  'cash_z',
  'payment_receipt',
] as const satisfies readonly PrintDocumentKind[];

export const PRINTER_STATUS_TONE: Record<PrinterStatus, StatusTone> = {
  unknown: 'neutral',
  reachable: 'success',
  unreachable: 'danger',
};

export const JOB_STATUS_TONE: Record<PrintJobStatus, StatusTone> = {
  queued: 'info',
  printing: 'info',
  printed: 'success',
  failed: 'danger',
  cancelled: 'neutral',
};

// ---- Host validation (mirrors the server rule: LAN addresses only) ---------------------

export type HostCheck = 'ok' | 'public' | 'invalid';

const IPV4_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
const LABEL_RE = /^(?!-)[a-z0-9-]{1,63}(?<!-)$/i;

function ipv4Octets(host: string): number[] | null {
  const m = IPV4_RE.exec(host);
  if (!m) return null;
  const octets = m.slice(1).map(Number);
  return octets.every((o) => o <= 255) ? octets : null;
}

/** RFC 1918, RFC 6598 (100.64/10), loopback and link-local IPv4 ranges (same as the server). */
function isPrivateIPv4([a, b]: number[]): boolean {
  if (a === undefined || b === undefined) return false;
  return (
    a === 10 ||
    a === 127 ||
    (a === 100 && b >= 64 && b <= 127) ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 169 && b === 254)
  );
}

/** Loopback, unique local (fc00::/7) and link-local (fe80::/10) IPv6. */
function isPrivateIPv6(host: string): boolean {
  const h = host.toLowerCase().replace(/^\[|\]$/g, '');
  if (h === '::1') return true;
  return /^f[cd][0-9a-f]{0,2}:/.test(h) || /^fe[89ab][0-9a-f]?:/.test(h);
}

/**
 * Client-side check of the printer address, the same rule the server applies
 * on save and before every connection (ADR 0021): IP literals must be private
 * (LAN, loopback, link-local). A hostname is accepted here; the server
 * resolves it and refuses it when it points outside the LAN
 * (422 printer_address_not_allowed).
 */
export function checkHost(raw: string): HostCheck {
  const host = raw.trim();
  if (host === '') return 'invalid';
  const v4 = ipv4Octets(host);
  if (v4) return isPrivateIPv4(v4) ? 'ok' : 'public';
  if (IPV4_RE.test(host)) return 'invalid';
  if (host.includes(':')) {
    if (!/^\[?[0-9a-f:.]+\]?$/i.test(host)) return 'invalid';
    return isPrivateIPv6(host) ? 'ok' : 'public';
  }
  const labels = host.replace(/\.$/, '').split('.');
  return labels.every((l) => LABEL_RE.test(l)) ? 'ok' : 'invalid';
}

/** "192.168.1.50:9100" (port omitted when it is the default). */
export function printerAddress(p: Pick<Printer, 'host' | 'port'>): string {
  const host = p.host.includes(':') ? `[${p.host}]` : p.host;
  return p.port === DEFAULT_PORT ? host : `${host}:${p.port}`;
}

// ---- Routing rows ------------------------------------------------------------------------

/** One editable routing rule; registerId '' is the default rule of the document. */
export interface RouteRow {
  kind: (typeof ROUTABLE_KINDS)[number];
  registerId: string;
  printerId: string;
  copies: number;
  kickDrawer: boolean;
}

export function toRows(routes: PrintRoute[]): RouteRow[] {
  return routes
    .filter((r): r is PrintRoute & { document_kind: RouteRow['kind'] } =>
      (ROUTABLE_KINDS as readonly string[]).includes(r.document_kind),
    )
    .map((r) => ({
      kind: r.document_kind,
      registerId: r.register_id ?? '',
      printerId: r.printer_id,
      copies: r.copies,
      kickDrawer: r.kick_drawer ?? false,
    }))
    .sort(compareRows);
}

/** Document order, default rule first, then by register. */
export function compareRows(a: RouteRow, b: RouteRow): number {
  const k = ROUTABLE_KINDS.indexOf(a.kind) - ROUTABLE_KINDS.indexOf(b.kind);
  if (k !== 0) return k;
  if (a.registerId === b.registerId) return 0;
  if (a.registerId === '') return -1;
  if (b.registerId === '') return 1;
  return a.registerId.localeCompare(b.registerId);
}

export function toRoutes(rows: RouteRow[]): PrintRoute[] {
  return rows.map((r) => ({
    document_kind: r.kind,
    ...(r.registerId ? { register_id: r.registerId } : {}),
    printer_id: r.printerId,
    copies: r.copies,
    kick_drawer: r.kickDrawer,
  }));
}
