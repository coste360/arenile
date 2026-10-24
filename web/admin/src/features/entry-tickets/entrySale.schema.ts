import { z } from 'zod';
import type { EntrySaleCreate, EntrySaleQuoteRequest, EntryTicketType } from './api';
import { toSaleItems, totalQuantity } from './lib';

/** Validation messages: i18n keys of the `entryTickets` namespace. */
export const SM = {
  noItems: 'entryTickets:validation.noItems',
  name: 'entryTickets:validation.holderName',
  tendered: 'entryTickets:validation.tendered',
} as const;

export const SLOT_VALUES = ['FULL', 'AM', 'PM'] as const;
export const METHOD_VALUES = ['cash', 'card'] as const;

/**
 * Entry sale form ("Vendita rapida" → Ingressi): slot, quantity per ticket
 * type, cash or card and an optional holder name (no phone, no email).
 */
export const entrySaleSchema = z
  .object({
    slots: z.enum(SLOT_VALUES),
    quantities: z.record(z.string(), z.number().int().min(0).max(100)),
    method: z.enum(METHOD_VALUES),
    /** Cash received in cents (optional, to compute the change). */
    tendered: z.number().int().min(0, SM.tendered).nullable(),
    first_name: z.string().trim().max(100, SM.name),
    last_name: z.string().trim().max(100, SM.name),
  })
  .refine((v) => totalQuantity(v.quantities) > 0, { message: SM.noItems, path: ['quantities'] });

export type EntrySaleValues = z.infer<typeof entrySaleSchema>;

export const EMPTY_ENTRY_SALE: EntrySaleValues = {
  slots: 'FULL',
  quantities: {},
  method: 'cash',
  tendered: null,
  first_name: '',
  last_name: '',
};

export function toEntrySaleQuote(
  v: Pick<EntrySaleValues, 'slots' | 'quantities'>,
  types: readonly EntryTicketType[],
  date: string,
): EntrySaleQuoteRequest | null {
  const items = toSaleItems(types, v.quantities);
  return items.length ? { date, slots: v.slots, items } : null;
}

/** Request body of the sale; `expectedTotal` is the quoted total (409 price_changed if it moved). */
export function toEntrySaleCreate(
  v: EntrySaleValues,
  types: readonly EntryTicketType[],
  date: string,
  expectedTotal?: number,
): EntrySaleCreate {
  const first = v.first_name.trim();
  const last = v.last_name.trim();
  return {
    date,
    slots: v.slots,
    items: toSaleItems(types, v.quantities),
    payment: {
      method: v.method,
      ...(v.method === 'cash' && v.tendered != null ? { tendered_amount: v.tendered } : {}),
    },
    ...(first || last
      ? {
          holder: { ...(first ? { first_name: first } : {}), ...(last ? { last_name: last } : {}) },
        }
      : {}),
    ...(expectedTotal != null ? { expected_total: expectedTotal } : {}),
  };
}

/** Server field paths of the sale → form fields, for mapProblemToForm. */
export const SALE_FIELD_ALIASES: Record<string, string> = {
  'holder.first_name': 'first_name',
  'holder.last_name': 'last_name',
  'payment.tendered_amount': 'tendered',
  'payment.method': 'method',
  items: 'quantities',
};
