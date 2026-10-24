/**
 * Form schemas of the cash desk (react-hook-form + zod). Amounts are integer
 * cents produced by MoneyInput; messages are i18n keys of the cash namespace.
 */
import { z } from 'zod';
import { DESK_METHODS } from './lib/totals';

const cents = (positive: boolean) =>
  z
    .number({ error: 'validation.amountRequired' })
    .int('validation.amountRequired')
    .refine((v) => (positive ? v > 0 : v >= 0), {
      message: positive ? 'validation.amountPositive' : 'validation.amountRequired',
    })
    .nullable()
    .refine((v) => v !== null, { message: 'validation.amountRequired' });

export const openSessionSchema = z.object({
  opening_float: cents(false),
  notes: z.string().trim().max(500, 'validation.tooLong'),
});
export type OpenSessionValues = z.input<typeof openSessionSchema>;

export const movementSchema = z.object({
  kind: z.enum(['in', 'out']),
  amount: cents(true),
  reason: z.string().trim().min(1, 'validation.reasonRequired').max(200, 'validation.tooLong'),
});
export type MovementValues = z.input<typeof movementSchema>;

export const quickPaymentSchema = z.object({
  amount: cents(true),
  method: z.enum(DESK_METHODS as [string, ...string[]]),
  note: z.string().trim().max(500, 'validation.tooLong'),
});
export type QuickPaymentValues = z.input<typeof quickPaymentSchema>;

/** Counted amount per method shown in the close dialog (cash is mandatory). */
export const closeSessionSchema = z.object({
  counted: z.record(z.string(), z.number().int().nullable()).refine((c) => c.cash != null, {
    message: 'validation.countedRequired',
    path: ['cash'],
  }),
  notes: z.string().trim().max(500, 'validation.tooLong'),
});
export type CloseSessionValues = z.input<typeof closeSessionSchema>;

/** Cash register form (Configurazione → Casse). Code only on creation, optional. */
export const registerSchema = z.object({
  name: z.string().trim().min(1, 'validation.nameRequired').max(60, 'validation.tooLong'),
  code: z
    .string()
    .trim()
    .regex(/^([a-z0-9][a-z0-9_-]{0,39})?$/, 'validation.codeFormat'),
  sort_order: z
    .number({ error: 'validation.sortOrder' })
    .int('validation.sortOrder')
    .min(0, 'validation.sortOrder')
    .max(10000, 'validation.sortOrder'),
  active: z.boolean(),
});
export type RegisterValues = z.infer<typeof registerSchema>;
