/**
 * Pure helpers of the entry tickets feature (no React, unit tested).
 */
import type { EntrySaleItem, EntryTicketType } from './api';

/** Quantity per entry ticket type id, as edited by the steppers. */
export type Quantities = Record<string, number>;

/** Max tickets of one type in one sale (EntrySaleItem.quantity maximum). */
export const MAX_PER_TYPE = 100;

/** Active types in display order (sort_order, then code). */
export function sellableTypes(types: readonly EntryTicketType[]): EntryTicketType[] {
  return types
    .filter((t) => t.active)
    .sort((a, b) => a.sort_order - b.sort_order || a.code.localeCompare(b.code));
}

/** Sale lines for the quantities above zero, in the order of `types` (stable request body). */
export function toSaleItems(types: readonly EntryTicketType[], q: Quantities): EntrySaleItem[] {
  return types
    .map((t) => ({ entry_ticket_type_id: t.id, quantity: Math.trunc(q[t.id] ?? 0) }))
    .filter((i) => i.quantity > 0)
    .map((i) => ({ ...i, quantity: Math.min(i.quantity, MAX_PER_TYPE) }));
}

export function totalQuantity(q: Quantities): number {
  return Object.values(q).reduce((sum, n) => sum + (n > 0 ? Math.trunc(n) : 0), 0);
}

/** Change to give back in cents, or null when nothing (or not enough) was tendered. */
export function changeDue(total: number, tendered: number | null | undefined): number | null {
  if (tendered == null || tendered < total) return null;
  return tendered - total;
}

/**
 * The booking code carried by a scanned QR or typed at the gate. The QR holds
 * the bare code (qr_payload); a link with `?code=` or ending with the code is
 * accepted too, so a printed manage link also works. Returns "" when empty.
 */
export function extractTicketCode(raw: string): string {
  const text = raw.trim();
  if (text === '') return '';
  if (/^https?:\/\//i.test(text)) {
    try {
      const url = new URL(text);
      const fromQuery = url.searchParams.get('code') ?? url.searchParams.get('c');
      if (fromQuery?.trim()) return fromQuery.trim().slice(0, 200);
      const last = url.pathname.split('/').filter(Boolean).pop();
      if (last) return decodeURIComponent(last).slice(0, 200);
    } catch {
      // Not a valid URL: fall through and send the text as typed.
    }
  }
  return text.slice(0, 200);
}

/**
 * Suppresses repeated reads of the same QR while it stays in front of the
 * camera: the same code is accepted again only after `windowMs`.
 */
export function createScanDeduper(windowMs = 4000) {
  let last = '';
  let at = -Infinity;
  return (code: string, now: number): boolean => {
    if (code === last && now - at < windowMs) return false;
    last = code;
    at = now;
    return true;
  };
}
