import { z } from 'zod';
import { CODE_RE, V, intField, isoDate, optionalIsoDate } from '../configuration/lib/validation';
import type { SimulateRequest } from './api';
import { CHANNELS, FORMULAS, splitCodes } from './rules.schema';

export const simulatorSchema = z
  .object({
    channel: z.enum(CHANNELS),
    starts_on: isoDate,
    ends_on: isoDate,
    slots: z.enum(['FULL', 'AM', 'PM']),
    formula: z.enum(FORMULAS),
    pack_id: z.string().min(1, V.required),
    zone_id: z.string().min(1, V.required),
    extra_id: z.string(),
    extra_qty: intField({ min: 1, max: 100, optional: true }),
    promo_code: z.string().trim().max(40),
    agreement_id: z.string(),
    customer_tags: z
      .string()
      .trim()
      .refine((v) => splitCodes(v).every((c) => CODE_RE.test(c)), V.code),
    today: optionalIsoDate,
  })
  .superRefine((v, ctx) => {
    if (v.ends_on < v.starts_on) {
      ctx.addIssue({ code: 'custom', path: ['ends_on'], message: V.dateOrder });
    }
    if (v.extra_id && !v.extra_qty) {
      ctx.addIssue({ code: 'custom', path: ['extra_qty'], message: V.required });
    }
  });

export type SimulatorValues = z.infer<typeof simulatorSchema>;

export function simulatorDefaults(opts: {
  start: string;
  packId: string;
  zoneId: string;
}): SimulatorValues {
  return {
    channel: 'desk',
    starts_on: opts.start,
    ends_on: opts.start,
    slots: 'FULL',
    formula: 'day',
    pack_id: opts.packId,
    zone_id: opts.zoneId,
    extra_id: '',
    extra_qty: '1',
    promo_code: '',
    agreement_id: '',
    customer_tags: '',
    today: '',
  };
}

export function toSimulateRequest(
  seasonId: string | undefined,
  v: SimulatorValues,
): SimulateRequest {
  const req: SimulateRequest = {
    channel: v.channel,
    starts_on: v.starts_on,
    ends_on: v.ends_on,
    slots: v.slots,
    formula: v.formula,
    lines: [{ pack_id: v.pack_id, zone_id: v.zone_id }],
  };
  if (seasonId) req.season_id = seasonId;
  if (v.extra_id) req.extras = [{ extra_id: v.extra_id, quantity: Number(v.extra_qty) }];
  if (v.promo_code) req.promo_code = v.promo_code;
  if (v.agreement_id) req.agreement_id = v.agreement_id;
  const tags = splitCodes(v.customer_tags);
  if (tags.length) req.customer_tags = tags;
  if (v.today) req.today = v.today;
  return req;
}
