import { z } from 'zod';
import type { ClosureDay, ClosureDayInput } from '../api/resources';
import { V, isoDate } from '../lib/validation';

export const SLOTS = ['AM', 'PM'] as const;

export const closureSchema = z.object({
  date: isoDate,
  slots: z.array(z.enum(SLOTS)).min(1, V.slotRequired),
  reason: z.string().trim().max(200),
});

export type ClosureValues = z.infer<typeof closureSchema>;

export function toClosureValues(c?: ClosureDay, today = ''): ClosureValues {
  return c
    ? { date: c.date, slots: [...c.slots], reason: c.reason ?? '' }
    : { date: today, slots: ['AM', 'PM'], reason: '' };
}

export function toClosureInput(v: ClosureValues): ClosureDayInput {
  const slots = SLOTS.filter((s) => v.slots.includes(s));
  return { date: v.date, slots, ...(v.reason ? { reason: v.reason } : {}) };
}

/** New closures: the same slots and reason on several days picked in the calendar. */
export const closureBatchSchema = z.object({
  dates: z.array(isoDate).min(1, V.dateRequired),
  slots: z.array(z.enum(SLOTS)).min(1, V.slotRequired),
  reason: z.string().trim().max(200),
});

export type ClosureBatchValues = z.infer<typeof closureBatchSchema>;

export function toClosureBatchValues(dates: string[] = []): ClosureBatchValues {
  return { dates: [...dates].sort(), slots: ['AM', 'PM'], reason: '' };
}

/** One input per selected day, in date order. */
export function toClosureInputs(v: ClosureBatchValues): ClosureDayInput[] {
  return [...new Set(v.dates)]
    .sort()
    .map((date) => toClosureInput({ date, slots: v.slots, reason: v.reason }));
}
