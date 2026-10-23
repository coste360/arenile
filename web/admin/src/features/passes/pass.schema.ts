import { z } from 'zod';
import type { BookingCreate } from '../bookings/api';
import type { SeasonPassCreate } from './api';

/** Validation messages: i18n keys of the `passes` namespace. */
export const PM = {
  customer: 'passes:validation.customer',
  season: 'passes:validation.season',
  spot: 'passes:validation.spot',
  pack: 'passes:validation.pack',
  dates: 'passes:validation.dates',
  note: 'passes:validation.note',
} as const;

export const createPassSchema = z
  .object({
    customer_id: z.string().min(1, PM.customer),
    season_id: z.string().min(1, PM.season),
    spot_id: z.string().min(1, PM.spot),
    pack_id: z.string().min(1, PM.pack),
    slots: z.enum(['FULL', 'AM', 'PM']),
    /** Empty = the whole season. */
    starts_on: z.string(),
    ends_on: z.string(),
    notes_internal: z.string().trim().max(2000),
  })
  .refine((v) => !v.starts_on || !v.ends_on || v.ends_on >= v.starts_on, {
    message: PM.dates,
    path: ['ends_on'],
  });

export type CreatePassValues = z.infer<typeof createPassSchema>;

export const EMPTY_PASS: CreatePassValues = {
  customer_id: '',
  season_id: '',
  spot_id: '',
  pack_id: '',
  slots: 'FULL',
  starts_on: '',
  ends_on: '',
  notes_internal: '',
};

export interface SeasonWindow {
  opens_on: string;
  closes_on: string;
}

export function toPassCreate(v: CreatePassValues): SeasonPassCreate {
  return {
    customer_id: v.customer_id,
    season_id: v.season_id,
    spot_id: v.spot_id,
    pack_id: v.pack_id,
    ...(v.slots !== 'FULL' ? { slots: v.slots } : {}),
    ...(v.starts_on ? { starts_on: v.starts_on } : {}),
    ...(v.ends_on ? { ends_on: v.ends_on } : {}),
    ...(v.notes_internal ? { notes_internal: v.notes_internal } : {}),
  };
}

/**
 * Quote request for the engine price of a pass (formula season over the
 * season window, or the chosen dates): the same pure engine used at creation.
 */
export function toPassQuote(
  v: Pick<CreatePassValues, 'spot_id' | 'pack_id' | 'slots' | 'starts_on' | 'ends_on'>,
  season: SeasonWindow | undefined,
  zoneId: string | undefined,
): BookingCreate | null {
  if (!season || !v.spot_id || !v.pack_id) return null;
  return {
    channel: 'desk',
    starts_on: v.starts_on || season.opens_on,
    ends_on: v.ends_on || season.closes_on,
    slots: v.slots,
    formula: 'season',
    lines: [{ pack_id: v.pack_id, spot_id: v.spot_id, ...(zoneId ? { zone_id: zoneId } : {}) }],
  };
}

export const releaseNoteSchema = z.string().trim().max(500, PM.note);
