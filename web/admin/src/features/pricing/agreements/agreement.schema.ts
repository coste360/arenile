import { z } from 'zod';
import type { Schemas } from '@arenile/shared/api';
import { INT_RE } from '../../configuration/lib/validation';

/**
 * M8 fields of an agreement (spec 30.2.2, ADR 0028): recipients (partner,
 * customer category or individual customers), fixed price per pack, scope
 * (packs and extras), seasons, quota (max and reserved per day, returned to
 * general sale N days before), requirement checked at the desk, channels
 * (desk, online code, agreement link) and the category of the register.
 * Merged into the agreement form of Prezzi → Sconti (rules.schema.ts).
 */

type Agreement = Schemas['Agreement'];
type AgreementInput = Schemas['AgreementInput'];
export type AgreementTarget = Schemas['AgreementTarget'];
export type AgreementChannel = Schemas['AgreementChannel'];

export const TARGETS: readonly AgreementTarget[] = [
  'partner',
  'customer_category',
  'individual_customers',
];
export const AGREEMENT_CHANNELS: readonly AgreementChannel[] = [
  'desk',
  'online_code',
  'online_link',
];
export const REGISTER_CATEGORIES = [
  'residents',
  'accommodation',
  'protected',
  'companies',
  'associations',
  'other',
] as const;

/** Default of quota.release_days_before (decision 4 of 30.13, ADR 0028). */
export const DEFAULT_RELEASE_DAYS = 3;

/** Validation messages: i18n keys of the `partners` namespace. */
export const AM = {
  partner: 'partners:validation.partnerRequired',
  category: 'partners:validation.categoryRequired',
  codeForOnline: 'partners:validation.codeForOnline',
  channels: 'partners:validation.channels',
  integer: 'partners:validation.integer',
  reserved: 'partners:validation.reservedOverMax',
  packPrice: 'partners:validation.packPrice',
  packDuplicate: 'partners:validation.packDuplicate',
} as const;

const optionalInt = z
  .string()
  .trim()
  .refine((v) => v === '' || INT_RE.test(v), AM.integer);

export const agreementM8Fields = {
  target: z.enum(TARGETS as [AgreementTarget, ...AgreementTarget[]]),
  partner_id: z.string(),
  customer_category_id: z.string(),
  pack_prices: z.array(
    z.object({
      pack_id: z.string(),
      amount: z.number().int().nonnegative().nullable(),
    }),
  ),
  scope_pack_ids: z.array(z.string()),
  scope_extra_ids: z.array(z.string()),
  apply_to_extras: z.boolean(),
  season_ids: z.array(z.string()),
  quota_zone_ids: z.array(z.string()),
  quota_max: optionalInt,
  quota_reserved: optionalInt,
  quota_release_days: optionalInt,
  requirement: z.string().trim().max(200),
  agreement_channels: z.array(
    z.enum(AGREEMENT_CHANNELS as [AgreementChannel, ...AgreementChannel[]]),
  ),
  register_category: z.enum(REGISTER_CATEGORIES),
};

export interface AgreementM8Values {
  target: AgreementTarget;
  partner_id: string;
  customer_category_id: string;
  pack_prices: { pack_id: string; amount: number | null }[];
  scope_pack_ids: string[];
  scope_extra_ids: string[];
  apply_to_extras: boolean;
  season_ids: string[];
  quota_zone_ids: string[];
  quota_max: string;
  quota_reserved: string;
  quota_release_days: string;
  requirement: string;
  agreement_channels: AgreementChannel[];
  register_category: (typeof REGISTER_CATEGORIES)[number];
}

/** Cross-field checks, attached to the right input. */
export function refineAgreementM8(v: AgreementM8Values & { code: string }, ctx: z.RefinementCtx) {
  const issue = (path: string, message: string) =>
    ctx.addIssue({ code: 'custom', path: [path], message });
  if (v.target === 'partner' && !v.partner_id) issue('partner_id', AM.partner);
  if (v.target === 'customer_category' && !v.customer_category_id) {
    issue('customer_category_id', AM.category);
  }
  if (v.agreement_channels.length === 0) issue('agreement_channels', AM.channels);
  if (v.agreement_channels.some((c) => c !== 'desk') && !v.code.trim())
    issue('code', AM.codeForOnline);
  const max = v.quota_max ? Number(v.quota_max) : undefined;
  const reserved = v.quota_reserved ? Number(v.quota_reserved) : undefined;
  if (max !== undefined && reserved !== undefined && reserved > max) {
    issue('quota_reserved', AM.reserved);
  }
  const seen = new Set<string>();
  v.pack_prices.forEach((p, i) => {
    if (!p.pack_id || p.amount === null) {
      ctx.addIssue({ code: 'custom', path: ['pack_prices', i, 'amount'], message: AM.packPrice });
    } else if (seen.has(p.pack_id)) {
      ctx.addIssue({
        code: 'custom',
        path: ['pack_prices', i, 'pack_id'],
        message: AM.packDuplicate,
      });
    }
    seen.add(p.pack_id);
  });
}

export function agreementM8Values(a: Agreement | undefined): AgreementM8Values {
  const q = a?.quota;
  return {
    // Agreements before M8 had no target: they were assigned to customers.
    target: a?.target ?? (a?.partner_id ? 'partner' : 'individual_customers'),
    partner_id: a?.partner_id ?? '',
    customer_category_id: a?.customer_category_id ?? '',
    pack_prices: (a?.pack_prices ?? []).map((p) => ({ pack_id: p.pack_id, amount: p.amount })),
    scope_pack_ids: a?.scope?.pack_ids ?? [],
    scope_extra_ids: a?.scope?.extra_ids ?? [],
    apply_to_extras: a?.scope?.apply_to_extras ?? false,
    season_ids: a?.season_ids ?? [],
    quota_zone_ids: q?.zone_ids ?? [],
    quota_max: q?.max_per_day !== undefined ? String(q.max_per_day) : '',
    quota_reserved: q?.reserved_per_day !== undefined ? String(q.reserved_per_day) : '',
    quota_release_days:
      q?.release_days_before !== undefined
        ? String(q.release_days_before)
        : q?.reserved_per_day
          ? String(DEFAULT_RELEASE_DAYS)
          : '',
    requirement: a?.requirement ?? '',
    agreement_channels: a?.channels?.length ? a.channels : ['desk'],
    register_category: a?.register_category ?? 'other',
  };
}

/** M8 part of the full replacement body (PUT keeps nothing that is not sent). */
export function toAgreementM8Input(v: AgreementM8Values): Partial<AgreementInput> {
  const out: Partial<AgreementInput> = {
    target: v.target,
    channels: v.agreement_channels,
    register_category: v.register_category,
  };
  if (v.target === 'partner' && v.partner_id) out.partner_id = v.partner_id;
  if (v.target === 'customer_category' && v.customer_category_id) {
    out.customer_category_id = v.customer_category_id;
  }
  const packPrices = v.pack_prices
    .filter((p) => p.pack_id && p.amount !== null)
    .map((p) => ({ pack_id: p.pack_id, amount: p.amount as number, per: 'day' as const }));
  if (packPrices.length) out.pack_prices = packPrices;
  if (v.scope_pack_ids.length || v.scope_extra_ids.length || v.apply_to_extras) {
    out.scope = {
      ...(v.scope_pack_ids.length ? { pack_ids: v.scope_pack_ids } : {}),
      ...(v.scope_extra_ids.length ? { extra_ids: v.scope_extra_ids } : {}),
      ...(v.apply_to_extras ? { apply_to_extras: true } : {}),
    };
  }
  if (v.season_ids.length) out.season_ids = v.season_ids;
  const max = v.quota_max ? Number(v.quota_max) : undefined;
  const reserved = v.quota_reserved ? Number(v.quota_reserved) : undefined;
  if (max !== undefined || reserved) {
    out.quota = {
      ...(v.quota_zone_ids.length ? { zone_ids: v.quota_zone_ids } : {}),
      ...(max !== undefined ? { max_per_day: max } : {}),
      ...(reserved ? { reserved_per_day: reserved } : {}),
      ...(reserved
        ? {
            release_days_before: v.quota_release_days
              ? Number(v.quota_release_days)
              : DEFAULT_RELEASE_DAYS,
          }
        : {}),
    };
  }
  if (v.requirement.trim()) out.requirement = v.requirement.trim();
  return out;
}
