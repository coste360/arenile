import { z } from 'zod';
import { isCalendarDate } from '../configuration/lib/validation';
import type { Customer } from '../customers/api';
import type { BookingCancelRequest, BookingCreate, ManualPaymentInput, RefundInput } from './api';
import { defaultFormula, FORMULAS, MANUAL_METHODS, SLOTS } from './model';

/** Validation messages: i18n keys of the `bookings` namespace (translated by FormMessage). */
export const M = {
  required: 'bookings:validation.required',
  date: 'bookings:validation.date',
  dateOrder: 'bookings:validation.dateOrder',
  pack: 'bookings:validation.pack',
  spots: 'bookings:validation.spots',
  zone: 'bookings:validation.zone',
  quantity: 'bookings:validation.quantity',
  customer: 'bookings:validation.customer',
  lastName: 'bookings:validation.lastName',
  contact: 'bookings:validation.contact',
  email: 'bookings:validation.email',
  phone: 'bookings:validation.phone',
  amount: 'bookings:validation.amount',
  amountMax: 'bookings:validation.amountMax',
  method: 'bookings:validation.method',
  text: 'bookings:validation.text',
} as const;

const PHONE_RE = /^\+[1-9][0-9]{6,14}$/;
const isoDate = z.string().refine(isCalendarDate, M.date);
const cents = z.number({ error: M.amount }).int(M.amount).positive(M.amount);
const slots = z.enum(SLOTS as [string, ...string[]]);
const formula = z.enum(FORMULAS as [string, ...string[]]);
const method = z.enum(MANUAL_METHODS as [string, ...string[]], { error: M.method });

/** "+39 333 123 4567" → "+393331234567"; numbers without prefix get +39 (Italian desk). */
export function normalizePhone(input: string): string {
  const compact = input.replace(/[\s().-]/g, '');
  if (compact === '') return '';
  if (compact.startsWith('00')) return `+${compact.slice(2)}`;
  if (compact.startsWith('+')) return compact;
  return `+39${compact}`;
}

/** Inline customer of the sheet (spec 23: name, surname, email or phone). */
export const newCustomerSchema = z
  .object({
    first_name: z.string().trim().max(100),
    last_name: z.string().trim().min(1, M.lastName).max(100),
    email: z
      .string()
      .trim()
      .toLowerCase()
      .refine((v) => v === '' || z.email().safeParse(v).success, M.email),
    phone: z
      .string()
      .trim()
      .refine((v) => v === '' || PHONE_RE.test(normalizePhone(v)), M.phone),
  })
  .refine((c) => c.email !== '' || c.phone !== '', { message: M.contact, path: ['phone'] });

export type NewCustomerValues = z.infer<typeof newCustomerSchema>;

export const newBookingSchema = z
  .object({
    starts_on: isoDate,
    ends_on: isoDate,
    slots,
    formula,
    pack_id: z.string().min(1, M.pack),
    /** spots: one line per selected spot; zone: a zone line (pending or quantity). */
    target: z.enum(['spots', 'zone']),
    spot_ids: z.array(z.string()),
    zone_id: z.string(),
    quantity: z.number().int().min(1, M.quantity).max(100, M.quantity),
    extras: z.record(z.string(), z.number().int().min(0).max(100)),
    customer_mode: z.enum(['existing', 'new']),
    customer_id: z.string(),
    /** Validated only when customer_mode = new (hidden fields never block the submit). */
    customer: z.custom<NewCustomerValues>().optional(),
    pay_now: z.boolean(),
    payment_amount: z.number().int().nullable(),
    payment_method: method,
    notes_internal: z.string().max(2000),
    force: z.boolean(),
  })
  .superRefine((v, ctx) => {
    if (isCalendarDate(v.starts_on) && isCalendarDate(v.ends_on) && v.ends_on < v.starts_on) {
      ctx.addIssue({ code: 'custom', message: M.dateOrder, path: ['ends_on'] });
    }
    if (v.target === 'spots' && v.spot_ids.length === 0) {
      ctx.addIssue({ code: 'custom', message: M.spots, path: ['spot_ids'] });
    }
    if (v.target === 'zone' && !v.zone_id) {
      ctx.addIssue({ code: 'custom', message: M.zone, path: ['zone_id'] });
    }
    if (v.customer_mode === 'existing' && !v.customer_id) {
      ctx.addIssue({ code: 'custom', message: M.customer, path: ['customer_id'] });
    }
    if (v.customer_mode === 'new') {
      const parsed = newCustomerSchema.safeParse(
        v.customer ?? { first_name: '', last_name: '', email: '', phone: '' },
      );
      if (!parsed.success) {
        for (const issue of parsed.error.issues) {
          ctx.addIssue({
            code: 'custom',
            message: issue.message,
            path: ['customer', ...issue.path],
          });
        }
      }
    }
    if (v.pay_now && (v.payment_amount === null || v.payment_amount <= 0)) {
      ctx.addIssue({ code: 'custom', message: M.amount, path: ['payment_amount'] });
    }
  });

export type NewBookingValues = z.infer<typeof newBookingSchema>;

export interface NewBookingDefaults {
  spotIds?: string[];
  startsOn?: string;
  endsOn?: string;
  slots?: 'AM' | 'PM' | 'FULL';
  holdId?: string;
  /** Preselected customer (e.g. from the customer page). */
  customer?: Customer;
}

export function newBookingValues(today: string, d: NewBookingDefaults = {}): NewBookingValues {
  const startsOn = d.startsOn ?? today;
  const endsOn = d.endsOn ?? startsOn;
  const s = d.slots ?? 'FULL';
  return {
    starts_on: startsOn,
    ends_on: endsOn,
    slots: s,
    formula: defaultFormula(startsOn, endsOn, s),
    pack_id: '',
    target: 'spots',
    spot_ids: d.spotIds ?? [],
    zone_id: '',
    quantity: 1,
    extras: {},
    customer_mode: 'existing',
    customer_id: d.customer?.id ?? '',
    customer: undefined,
    pay_now: false,
    payment_amount: null,
    payment_method: 'cash',
    notes_internal: '',
    force: false,
  };
}

/** Context needed to turn the form into lines (spot → zone of the map). */
export interface LineContext {
  zoneOfSpot: (spotId: string) => string | undefined;
  /** Extras with attach_to = line: repeated on every line (quantity per spot). */
  lineExtraIds?: ReadonlySet<string>;
  holdId?: string;
}

/** Selection part of the request, shared by quote and create (no customer needed for a quote). */
export function toSelection(
  v: Pick<
    NewBookingValues,
    | 'starts_on'
    | 'ends_on'
    | 'slots'
    | 'formula'
    | 'pack_id'
    | 'target'
    | 'spot_ids'
    | 'zone_id'
    | 'quantity'
    | 'extras'
  >,
  ctx: LineContext,
): BookingCreate | null {
  if (!isCalendarDate(v.starts_on) || !isCalendarDate(v.ends_on) || v.ends_on < v.starts_on) {
    return null;
  }
  if (!v.pack_id) return null;
  const chosen = Object.entries(v.extras)
    .filter(([, q]) => q > 0)
    .map(([extra_id, quantity]) => ({ extra_id, quantity }));
  const lineExtras = chosen.filter((x) => ctx.lineExtraIds?.has(x.extra_id));
  const extras = chosen.filter((x) => !ctx.lineExtraIds?.has(x.extra_id));
  const withExtras = lineExtras.length ? { extras: lineExtras } : {};
  const lines: BookingCreate['lines'] =
    v.target === 'spots'
      ? v.spot_ids.map((spotId) => ({
          pack_id: v.pack_id,
          spot_id: spotId,
          zone_id: ctx.zoneOfSpot(spotId),
          ...withExtras,
        }))
      : v.zone_id
        ? [{ pack_id: v.pack_id, zone_id: v.zone_id, quantity: v.quantity, ...withExtras }]
        : [];
  if (lines.length === 0) return null;
  return {
    channel: 'desk',
    starts_on: v.starts_on,
    ends_on: v.ends_on,
    slots: v.slots as BookingCreate['slots'],
    formula: v.formula as BookingCreate['formula'],
    lines,
    ...(extras.length ? { extras } : {}),
    ...(ctx.holdId ? { hold_id: ctx.holdId } : {}),
  };
}

/** Full create request (confirmed at the desk, payment recorded separately). */
export function toBookingCreate(v: NewBookingValues, ctx: LineContext): BookingCreate | null {
  const selection = toSelection(v, ctx);
  if (!selection) return null;
  const body: BookingCreate = { ...selection, status: 'confirmed' };
  if (v.customer_mode === 'existing') {
    body.customer_id = v.customer_id;
  } else if (v.customer) {
    const c = v.customer;
    body.customer = {
      last_name: c.last_name.trim(),
      ...(c.first_name.trim() ? { first_name: c.first_name.trim() } : {}),
      ...(c.email.trim() ? { email: c.email.trim().toLowerCase() } : {}),
      ...(c.phone.trim() ? { phone: normalizePhone(c.phone) } : {}),
    };
  }
  if (v.notes_internal.trim()) body.notes_internal = v.notes_internal.trim();
  if (v.force) body.force = true;
  return body;
}

/** Payment taken at creation, or null for "pay later". */
export function toInitialPayment(v: NewBookingValues): ManualPaymentInput | null {
  if (!v.pay_now || v.payment_amount === null || v.payment_amount <= 0) return null;
  return { amount: v.payment_amount, method: v.payment_method as ManualPaymentInput['method'] };
}

// ---- detail page dialogs ----------------------------------------------------

export function paymentSchema(maxCents?: number) {
  return z.object({
    amount: cents.refine((v) => maxCents === undefined || v <= maxCents, M.amountMax),
    method,
    note: z.string().trim().max(500),
  });
}
export type PaymentValues = z.infer<ReturnType<typeof paymentSchema>>;

export function toPaymentInput(v: PaymentValues): ManualPaymentInput {
  return {
    amount: v.amount,
    method: v.method as ManualPaymentInput['method'],
    ...(v.note ? { note: v.note } : {}),
  };
}

/** Refund form; the amount never exceeds what was paid (INV-24, checked again by the server). */
export function refundSchema(paidCents: number) {
  return z.object({
    amount: cents.refine((v) => v <= paidCents, M.amountMax),
    method,
    as: z.enum(['refund', 'credit']),
    reason: z.string().trim().max(500),
  });
}
export type RefundValues = z.infer<ReturnType<typeof refundSchema>>;

export function toRefundInput(v: RefundValues): RefundInput {
  return {
    amount: v.amount,
    method: (v.as === 'credit' ? 'credit' : v.method) as RefundInput['method'],
    as: v.as,
    ...(v.reason ? { reason: v.reason } : {}),
  };
}

export const SETTLEMENTS = ['policy', 'refund', 'credit', 'none'] as const;

export function cancelSchema(paidCents: number) {
  return z
    .object({
      settlement: z.enum(SETTLEMENTS),
      refund_amount: z.number().int().nullable(),
      refund_method: method,
      weather: z.boolean(),
      reason: z.string().trim().max(500),
    })
    .superRefine((v, ctx) => {
      if (v.settlement !== 'refund' && v.settlement !== 'credit') return;
      if (v.refund_amount === null || v.refund_amount < 0) {
        ctx.addIssue({ code: 'custom', message: M.amount, path: ['refund_amount'] });
      } else if (v.refund_amount > paidCents) {
        ctx.addIssue({ code: 'custom', message: M.amountMax, path: ['refund_amount'] });
      }
    });
}
export type CancelValues = z.infer<ReturnType<typeof cancelSchema>>;

export function toCancelRequest(v: CancelValues, dryRun: boolean): BookingCancelRequest {
  const forced = v.settlement === 'refund' || v.settlement === 'credit';
  return {
    settlement: v.settlement,
    ...(forced && v.refund_amount !== null ? { refund_amount: v.refund_amount } : {}),
    ...(v.settlement !== 'none' && v.settlement !== 'credit'
      ? { refund_method: v.refund_method as BookingCancelRequest['refund_method'] }
      : {}),
    ...(v.weather ? { weather: true } : {}),
    ...(v.reason ? { reason: v.reason } : {}),
    ...(dryRun ? { dry_run: true } : {}),
  };
}

export const noteSchema = z.object({
  text: z.string().trim().min(1, M.text).max(2000),
  visibility: z.enum(['internal', 'customer']),
});
export type NoteValues = z.infer<typeof noteSchema>;

export const changeDatesSchema = z
  .object({ starts_on: isoDate, ends_on: isoDate, slots, force: z.boolean() })
  .refine((v) => v.ends_on >= v.starts_on, { message: M.dateOrder, path: ['ends_on'] });
export type ChangeDatesValues = z.infer<typeof changeDatesSchema>;
