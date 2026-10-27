import type { MapLabelMode, SpotIndicator } from '@arenile/shared/map';
import { useCallback, useMemo, useState } from 'react';
import { labelModeKey, occupantShortLabel } from './occupant';
import {
  slotStatus,
  type OperationalBooking,
  type OperationalSpot,
  type SlotChoice,
} from './status';

type Translate = (key: string, options?: Record<string, unknown>) => string;

/**
 * Parts of the spot labels on the operational map (spec 30.4): the spot
 * code, the customer, the end date of the stay ("16/8") and the setup
 * ("2L 1S"). Any combination, at least one; remembered per user.
 */
export type MapLabelPart = 'code' | 'customer' | 'end' | 'setup';

export const MAP_LABEL_PARTS: readonly MapLabelPart[] = ['code', 'customer', 'end', 'setup'];

export const DEFAULT_LABEL_PARTS: readonly MapLabelPart[] = ['code', 'customer'];

const STORAGE_PREFIX = 'arenile.map.labelParts';

function isPart(v: unknown): v is MapLabelPart {
  return v === 'code' || v === 'customer' || v === 'end' || v === 'setup';
}

/** Parts in canonical order, without duplicates; the default when empty. */
export function normalizeParts(parts: readonly unknown[]): MapLabelPart[] {
  const set = new Set(parts.filter(isPart));
  const out = MAP_LABEL_PARTS.filter((p) => set.has(p));
  return out.length > 0 ? out : [...DEFAULT_LABEL_PARTS];
}

/** localStorage key of the label parts of `userId` (per user on a shared desk PC). */
export function labelPartsKey(userId: string | undefined): string {
  return userId ? `${STORAGE_PREFIX}.${userId}` : STORAGE_PREFIX;
}

/** Reads the stored parts, migrating the previous single choice (Codice / Cliente / Entrambi). */
export function readLabelParts(userId: string | undefined): MapLabelPart[] {
  if (!userId) return [...DEFAULT_LABEL_PARTS];
  try {
    const raw = window.localStorage.getItem(labelPartsKey(userId));
    if (raw) {
      const parsed: unknown = JSON.parse(raw);
      if (Array.isArray(parsed)) return normalizeParts(parsed);
    }
    const legacy = window.localStorage.getItem(labelModeKey(userId));
    if (legacy === 'code') return ['code'];
    if (legacy === 'customer') return ['customer'];
  } catch {
    // Unreadable storage: the default.
  }
  return [...DEFAULT_LABEL_PARTS];
}

/**
 * Label parts of the operational map, remembered per user in localStorage
 * (spec 30.4 "memorizzato per utente"). Storage errors keep the choice in
 * memory for the page.
 */
export function useLabelParts(
  userId: string | undefined,
): [MapLabelPart[], (parts: readonly MapLabelPart[]) => void] {
  const key = labelPartsKey(userId);
  const stored = useMemo(() => readLabelParts(userId), [userId]);
  const [chosen, setChosen] = useState<{ key: string; parts: MapLabelPart[] } | null>(null);
  const parts = chosen?.key === key ? chosen.parts : stored;
  const setParts = useCallback(
    (next: readonly MapLabelPart[]) => {
      const value = normalizeParts(next);
      setChosen({ key, parts: value });
      if (!userId) return;
      try {
        window.localStorage.setItem(key, JSON.stringify(value));
      } catch {
        // Not persisted: the choice lasts for this page.
      }
    },
    [key, userId],
  );
  return [parts, setParts];
}

/** Which labels the renderer draws: the code pill, the caption, or both. */
export function labelModeOf(parts: readonly MapLabelPart[]): MapLabelMode {
  const code = parts.includes('code');
  const other = parts.some((p) => p !== 'code');
  if (code && other) return 'both';
  return other ? 'customer' : 'code';
}

/** End date of a stay as drawn on the map: "16/8" (day/month, no leading zeros). */
export function shortDate(date: string): string {
  const [, m, d] = date.split('-');
  return `${Number(d)}/${Number(m)}`;
}

/** Last day of the booking on this spot (the segment end for a split stay). */
export function bookingEnd(b: Pick<OperationalBooking, 'ends_on' | 'segment_ends_on'>): string {
  return b.segment_ends_on ?? b.ends_on;
}

/**
 * Captions per spot id built from the chosen parts (spec 30.4): customer
 * (only with bookings.read), end date and setup, joined by " · ". Taken
 * spots only; spots without any part get none (their code stays).
 */
export function labelCaptions(
  spots: readonly OperationalSpot[],
  slot: SlotChoice,
  parts: readonly MapLabelPart[],
  t: Translate,
  canSeeCustomers: boolean,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const s of spots) {
    const b = s.booking;
    if (!b) continue;
    const st = slotStatus(s, slot);
    if (st === 'free' || st === 'released' || st === 'closed' || st === 'inactive') continue;
    const text: string[] = [];
    for (const p of parts) {
      if (p === 'customer' && canSeeCustomers) {
        const name = occupantShortLabel(b, t);
        if (name) text.push(name);
      } else if (p === 'end') {
        text.push(shortDate(bookingEnd(b)));
      } else if (p === 'setup' && b.setup_label) {
        text.push(b.setup_label);
      }
    }
    if (text.length > 0) out[s.spot_id] = text.join(' · ');
  }
  return out;
}

/** Order in which indicators are drawn; the map shows at most MAX_MAP_INDICATORS. */
const INDICATOR_PRIORITY: readonly SpotIndicator[] = [
  'to_assign',
  'arrival',
  'departure',
  'vip',
  'note',
  'agreement',
  'group',
  'extra',
];

export const MAX_MAP_INDICATORS = 4;

/**
 * The indicators drawn on the art, by priority and capped so the dots never
 * reach the status badge; the full list stays in the accessible name.
 */
export function mapIndicators(
  all: Readonly<Record<string, readonly SpotIndicator[]>>,
): Record<string, readonly SpotIndicator[]> {
  const out: Record<string, readonly SpotIndicator[]> = {};
  for (const [id, list] of Object.entries(all)) {
    if (list.length <= MAX_MAP_INDICATORS) {
      out[id] = list;
      continue;
    }
    out[id] = INDICATOR_PRIORITY.filter((k) => list.includes(k)).slice(0, MAX_MAP_INDICATORS);
  }
  return out;
}
