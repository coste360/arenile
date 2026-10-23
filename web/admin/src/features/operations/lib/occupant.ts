import type { MapLabelMode } from '@arenile/shared/map';
import { useCallback, useMemo, useState } from 'react';
import {
  slotStatus,
  type OperationalBooking,
  type OperationalSpot,
  type SlotChoice,
} from './status';

type Translate = (key: string, options?: Record<string, unknown>) => string;

/** Booking fields the occupant label is made of. */
export type OccupantFields = Pick<
  OperationalBooking,
  'occupant_label' | 'customer_display_name' | 'customer_anonymized'
>;

/**
 * Short name drawn on the map for the occupant of a spot (spec 11.3): the
 * surname from the server (`occupant_label`: surname, or first name when
 * there is none; the pass holder for passes, the main customer for groups),
 * "Anonimo" for an anonymised customer, undefined without a name. The map
 * truncates it to the room it has.
 */
export function occupantShortLabel(b: OccupantFields, t: Translate): string | undefined {
  if (b.customer_anonymized) return t('occupant.anonymousShort');
  const label = b.occupant_label.trim();
  return label || undefined;
}

/**
 * Full occupant name for the accessible name and the tooltip ("Rossi M.",
 * "Cliente anonimo"); undefined without a name.
 */
export function occupantFullName(b: OccupantFields, t: Translate): string | undefined {
  if (b.customer_anonymized) return t('occupant.anonymous');
  return b.customer_display_name.trim() || b.occupant_label.trim() || undefined;
}

/**
 * Captions of the map per spot id: the occupant of every spot that is taken
 * for the slot (booked, present, pass holder, hold, renewal offer); none for
 * free spots and for the day a pass holder released. Empty when the user
 * may not see customers.
 */
export function occupantCaptions(
  spots: readonly OperationalSpot[],
  slot: SlotChoice,
  t: Translate,
  canSeeCustomers: boolean,
): Record<string, string> {
  const out: Record<string, string> = {};
  if (!canSeeCustomers) return out;
  for (const s of spots) {
    if (!s.booking) continue;
    const st = slotStatus(s, slot);
    if (st === 'free' || st === 'released' || st === 'closed' || st === 'inactive') continue;
    const label = occupantShortLabel(s.booking, t);
    if (label) out[s.spot_id] = label;
  }
  return out;
}

const STORAGE_PREFIX = 'arenile.map.labelMode';

function isMode(v: unknown): v is MapLabelMode {
  return v === 'code' || v === 'customer' || v === 'both';
}

/** localStorage key of the label mode of `userId` (per user on a shared desk PC). */
export function labelModeKey(userId: string | undefined): string {
  return userId ? `${STORAGE_PREFIX}.${userId}` : STORAGE_PREFIX;
}

/**
 * Label mode of the operational map ("Etichette: Codice / Cliente /
 * Entrambi"), remembered per user in localStorage. Default `both`: the map
 * itself falls back to the codes while the names are not legible at the
 * current zoom. Storage errors (private mode, quota) keep it in memory.
 */
export function useLabelMode(
  userId: string | undefined,
): [MapLabelMode, (m: MapLabelMode) => void] {
  const key = labelModeKey(userId);
  // Read once per user (the user loads after the first render, or changes).
  const stored = useMemo((): MapLabelMode => {
    if (!userId) return 'both';
    try {
      const v = window.localStorage.getItem(key);
      return isMode(v) ? v : 'both';
    } catch {
      return 'both';
    }
  }, [key, userId]);
  const [chosen, setChosen] = useState<{ key: string; mode: MapLabelMode } | null>(null);
  const mode = chosen?.key === key ? chosen.mode : stored;
  const setMode = useCallback(
    (m: MapLabelMode) => {
      setChosen({ key, mode: m });
      // Persisted only once the user is known: never under a shared key.
      if (!userId) return;
      try {
        window.localStorage.setItem(key, m);
      } catch {
        // Not persisted: the choice lasts for this page.
      }
    },
    [key, userId],
  );
  return [mode, setMode];
}
