import { z } from 'zod';
import type { SlotChoice } from './status';

/** Error message keys (operations namespace), translated by the form. */
export const BLOCK_ERRORS = {
  period: 'block.errors.period',
  order: 'block.errors.order',
  reason: 'block.errors.reason',
} as const;

const iso = /^\d{4}-\d{2}-\d{2}$/;

/** Manual block of spots from the map (POST /spot-blocks; ADR 0019 C). */
export const blockSchema = z
  .object({
    starts_on: z.string().regex(iso, BLOCK_ERRORS.period),
    ends_on: z.string().regex(iso, BLOCK_ERRORS.period),
    slots: z.enum(['FULL', 'AM', 'PM']),
    reason: z.string().trim().max(200, BLOCK_ERRORS.reason),
  })
  .refine((v) => v.ends_on >= v.starts_on, { message: BLOCK_ERRORS.order, path: ['ends_on'] });

export type BlockValues = z.infer<typeof blockSchema>;

export function blockValues(date: string, slot: SlotChoice): BlockValues {
  return { starts_on: date, ends_on: date, slots: slot, reason: '' };
}

/** Request body for the selected spots. */
export function toSpotBlockCreate(spotIds: readonly string[], v: BlockValues) {
  const reason = v.reason.trim();
  return {
    spot_ids: [...spotIds],
    starts_on: v.starts_on,
    ends_on: v.ends_on,
    slots: v.slots,
    ...(reason ? { reason } : {}),
  };
}
