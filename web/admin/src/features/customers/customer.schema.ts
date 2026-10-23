import { z } from 'zod';
import { normalizePhone } from '../bookings/booking.schema';
import type { Customer, CustomerInput } from './api';

/** Validation messages: i18n keys of the `customers` namespace. */
export const CM = {
  lastName: 'customers:validation.lastName',
  email: 'customers:validation.email',
  phone: 'customers:validation.phone',
  tag: 'customers:validation.tag',
  tags: 'customers:validation.tags',
  confirm: 'customers:validation.confirm',
} as const;

const PHONE_RE = /^\+[1-9][0-9]{6,14}$/;
export const LOCALES = ['it', 'en', 'de', 'fr'] as const;

/** Source and text version recorded for a consent collected at the desk (spec 23). */
export const DESK_CONSENT = { source: 'desk', text_version: 'panel-1' } as const;

/** "estate, vip ,  estate" → ["estate", "vip"] (trimmed, lowercase, unique). */
export function parseTags(input: string): string[] {
  return [
    ...new Set(
      input
        .split(',')
        .map((s) => s.trim().toLowerCase())
        .filter(Boolean),
    ),
  ];
}

export const customerSchema = z.object({
  first_name: z.string().trim().max(100),
  last_name: z.string().trim().min(1, CM.lastName).max(100),
  email: z
    .string()
    .trim()
    .toLowerCase()
    .refine((v) => v === '' || z.email().safeParse(v).success, CM.email),
  phone: z
    .string()
    .trim()
    .refine((v) => v === '' || PHONE_RE.test(normalizePhone(v)), CM.phone),
  locale: z.enum(LOCALES),
  notes: z.string().max(2000),
  tags: z
    .string()
    .refine((v) => parseTags(v).every((tag) => tag.length <= 30), CM.tag)
    .refine((v) => parseTags(v).length <= 20, CM.tags),
  marketing: z.boolean(),
});

export type CustomerValues = z.infer<typeof customerSchema>;

export function customerValues(c?: Customer): CustomerValues {
  return {
    first_name: c?.first_name ?? '',
    last_name: c?.last_name ?? '',
    email: c?.email ?? '',
    phone: c?.phone ?? '',
    locale: (LOCALES as readonly string[]).includes(c?.locale ?? '')
      ? (c?.locale as CustomerValues['locale'])
      : 'it',
    notes: c?.notes ?? '',
    tags: (c?.tags ?? []).join(', '),
    marketing: !!c?.marketing_consent,
  };
}

/**
 * Full replacement body (PUT) or creation body. An existing consent is kept
 * as recorded; a new one gets the current instant and the desk source.
 */
export function toCustomerInput(
  v: CustomerValues,
  previous?: Customer,
  now = new Date(),
): CustomerInput {
  const consent = v.marketing
    ? (previous?.marketing_consent ?? { given_at: now.toISOString(), ...DESK_CONSENT })
    : undefined;
  const tags = parseTags(v.tags);
  return {
    last_name: v.last_name.trim(),
    ...(v.first_name.trim() ? { first_name: v.first_name.trim() } : {}),
    ...(v.email.trim() ? { email: v.email.trim().toLowerCase() } : {}),
    ...(v.phone.trim() ? { phone: normalizePhone(v.phone) } : {}),
    locale: v.locale,
    ...(v.notes.trim() ? { notes: v.notes.trim() } : {}),
    ...(tags.length ? { tags } : {}),
    ...(previous?.agreement_id ? { agreement_id: previous.agreement_id } : {}),
    ...(consent ? { marketing_consent: consent } : {}),
  };
}

/**
 * Strong confirmation of the anonymisation (irreversible): the operator types
 * the customer's surname exactly (case and surrounding spaces ignored).
 */
export function anonymizeSchema(lastName: string) {
  return z.object({
    confirmation: z
      .string()
      .refine(
        (v) => v.trim().toLocaleLowerCase('it') === lastName.trim().toLocaleLowerCase('it'),
        CM.confirm,
      ),
  });
}
