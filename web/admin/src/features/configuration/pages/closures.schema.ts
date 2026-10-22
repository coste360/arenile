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
