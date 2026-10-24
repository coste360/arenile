import { z } from 'zod';
import { compactI18n, i18nFormValue } from '../configuration/lib/i18nText';
import type { EntryTicketType, EntryTicketTypeInput, EntryTicketTypeUpdate } from './api';

/** Validation messages: i18n keys of the `entryTickets` namespace. */
export const EM = {
  required: 'entryTickets:validation.required',
  code: 'entryTickets:validation.code',
  name: 'entryTickets:validation.name',
  pack: 'entryTickets:validation.pack',
  zone: 'entryTickets:validation.zone',
  sortOrder: 'entryTickets:validation.sortOrder',
} as const;

/** EntryTicketTypeInput.code pattern (openapi). */
export const ENTRY_CODE_RE = /^[a-z0-9][a-z0-9_-]{0,39}$/;

export function entryTypeSchema(defaultLocale: string) {
  return z.object({
    code: z.string().trim().toLowerCase().regex(ENTRY_CODE_RE, EM.code),
    name: z
      .record(z.string(), z.string().max(200))
      .refine((t) => (t[defaultLocale] ?? '').trim() !== '', EM.name),
    description: z.record(z.string(), z.string().max(2000)),
    pack_id: z.string().min(1, EM.pack),
    zone_id: z.string().min(1, EM.zone),
    sort_order: z
      .string()
      .trim()
      .refine((v) => v === '' || /^-?\d{1,6}$/.test(v), EM.sortOrder),
    active: z.boolean(),
  });
}

export type EntryTypeValues = z.infer<ReturnType<typeof entryTypeSchema>>;

export function toEntryTypeValues(
  item: EntryTicketType | undefined,
  locales: readonly string[],
): EntryTypeValues {
  return {
    code: item?.code ?? '',
    name: i18nFormValue(item?.name, locales),
    description: i18nFormValue(item?.description, locales),
    pack_id: item?.pack_id ?? '',
    zone_id: item?.zone_id ?? '',
    sort_order: item ? String(item.sort_order) : '',
    active: item?.active ?? true,
  };
}

function common(v: EntryTypeValues) {
  const description = compactI18n(v.description);
  return {
    name: compactI18n(v.name),
    ...(Object.keys(description).length ? { description } : {}),
    pack_id: v.pack_id,
    zone_id: v.zone_id,
    sort_order: v.sort_order.trim() === '' ? 0 : Number(v.sort_order.trim()),
    active: v.active,
  };
}

/** Creation body (the code is fixed after creation). */
export function toEntryTypeInput(v: EntryTypeValues): EntryTicketTypeInput {
  return { code: v.code.trim().toLowerCase(), ...common(v) };
}

/** PATCH body: every editable field (the code cannot change). */
export function toEntryTypeUpdate(v: EntryTypeValues): EntryTicketTypeUpdate {
  return { ...common(v), description: compactI18n(v.description) };
}
