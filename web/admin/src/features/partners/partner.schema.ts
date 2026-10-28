import { z } from 'zod';
import { normalizePhone } from '../bookings/booking.schema';
import { codeField, isoDate, V } from '../configuration/lib/validation';
import {
  ACCOMMODATION_TYPES,
  PARTNER_KINDS,
  PAYMENT_METHODS,
  REGISTER_CATEGORIES,
  type CustomerCategory,
  type CustomerCategoryInput,
  type Partner,
  type PartnerInput,
  type PartnerPaymentInput,
} from './api';

/** Validation messages: i18n keys of the `partners` namespace. */
export const PM = {
  email: 'partners:validation.email',
  phone: 'partners:validation.phone',
  vat: 'partners:validation.vat',
  amount: 'partners:validation.amount',
  reason: 'partners:validation.reason',
} as const;

const PHONE_RE = /^\+[1-9][0-9]{6,14}$/;
const VAT_RE = /^[A-Za-z0-9]{0,16}$/;

// ---------------------------------------------------------------------------
// Partner (spec 30.2.1): contact data is optional and never logged.
// ---------------------------------------------------------------------------

export const partnerSchema = z.object({
  name: z.string().trim().min(1, V.required).max(200),
  kind: z.enum(PARTNER_KINDS as [Partner['kind'], ...Partner['kind'][]]),
  accommodation_type: z.enum(['', ...ACCOMMODATION_TYPES] as [string, ...string[]]),
  municipality: z.string().trim().max(120),
  address: z.string().trim().max(300),
  contact_name: z.string().trim().max(200),
  email: z
    .string()
    .trim()
    .toLowerCase()
    .refine((v) => v === '' || z.email().safeParse(v).success, PM.email),
  phone: z
    .string()
    .trim()
    .refine((v) => v === '' || PHONE_RE.test(normalizePhone(v)), PM.phone),
  vat_number: z.string().trim().regex(VAT_RE, PM.vat),
  tax_code: z.string().trim().regex(VAT_RE, PM.vat),
  notes: z.string().trim().max(2000),
  active: z.boolean(),
});

export type PartnerValues = z.infer<typeof partnerSchema>;

export function partnerValues(p?: Partner): PartnerValues {
  return {
    name: p?.name ?? '',
    kind: p?.kind ?? 'struttura_ricettiva',
    accommodation_type: p?.accommodation_type ?? '',
    municipality: p?.municipality ?? '',
    address: p?.address ?? '',
    contact_name: p?.contact_name ?? '',
    email: p?.email ?? '',
    phone: p?.phone ?? '',
    vat_number: p?.vat_number ?? '',
    tax_code: p?.tax_code ?? '',
    notes: p?.notes ?? '',
    active: p?.active ?? true,
  };
}

/** Full replacement body: empty optional fields are omitted. */
export function toPartnerInput(v: PartnerValues): PartnerInput {
  const opt = (s: string) => (s.trim() ? s.trim() : undefined);
  const input: PartnerInput = { name: v.name.trim(), kind: v.kind, active: v.active };
  if (v.kind === 'struttura_ricettiva' && v.accommodation_type) {
    input.accommodation_type = v.accommodation_type as PartnerInput['accommodation_type'];
  }
  const fields = {
    municipality: opt(v.municipality),
    address: opt(v.address),
    contact_name: opt(v.contact_name),
    email: opt(v.email)?.toLowerCase(),
    phone: v.phone.trim() ? normalizePhone(v.phone) : undefined,
    vat_number: opt(v.vat_number)?.toUpperCase(),
    tax_code: opt(v.tax_code)?.toUpperCase(),
    notes: opt(v.notes),
  };
  for (const [k, val] of Object.entries(fields)) {
    if (val !== undefined) (input as unknown as Record<string, unknown>)[k] = val;
  }
  return input;
}

// ---------------------------------------------------------------------------
// Customer category (residents, over 65, ...).
// ---------------------------------------------------------------------------

export const categorySchema = z.object({
  code: codeField,
  name: z.string().trim().min(1, V.required).max(120),
  description: z.string().trim().max(500),
  requirement: z.string().trim().max(200),
  register_category: z.enum(
    REGISTER_CATEGORIES as [CustomerCategory['register_category'], ...string[]],
  ),
  active: z.boolean(),
});

export type CategoryValues = z.infer<typeof categorySchema>;

export function categoryValues(c?: CustomerCategory): CategoryValues {
  return {
    code: c?.code ?? '',
    name: c?.name ?? '',
    description: c?.description ?? '',
    requirement: c?.requirement ?? '',
    register_category: c?.register_category ?? 'residents',
    active: c?.active ?? true,
  };
}

export function toCategoryInput(v: CategoryValues): CustomerCategoryInput {
  return {
    code: v.code,
    name: v.name.trim(),
    description: v.description.trim(),
    requirement: v.requirement.trim(),
    register_category: v.register_category as CustomerCategoryInput['register_category'],
    active: v.active,
  };
}

// ---------------------------------------------------------------------------
// Partner payment (decision 30.13: recorded payments, balance = charged - paid).
// ---------------------------------------------------------------------------

export const paymentSchema = z.object({
  amount: z.number({ error: PM.amount }).int(PM.amount).positive(PM.amount),
  received_on: isoDate,
  method: z.enum(PAYMENT_METHODS as [PartnerPaymentInput['method'], ...string[]]),
  reference: z.string().trim().max(120),
  note: z.string().trim().max(500),
});

export type PaymentValues = z.infer<typeof paymentSchema>;

export function paymentValues(today: string): PaymentValues {
  // Empty until typed (MoneyInput null); zod rejects it on submit.
  return {
    amount: null as unknown as number,
    received_on: today,
    method: 'bank_transfer',
    reference: '',
    note: '',
  };
}

export function toPaymentInput(v: PaymentValues): PartnerPaymentInput {
  return {
    amount: v.amount,
    received_on: v.received_on,
    method: v.method as PartnerPaymentInput['method'],
    ...(v.reference ? { reference: v.reference } : {}),
    ...(v.note ? { note: v.note } : {}),
  };
}

export const voidSchema = z.object({ reason: z.string().trim().min(3, PM.reason).max(300) });
export type VoidValues = z.infer<typeof voidSchema>;
