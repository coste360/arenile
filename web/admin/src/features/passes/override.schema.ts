import { z } from 'zod';

/** Manual price of a season pass (ADR 0019 E); messages are `passes` i18n keys. */
export const overrideSchema = z.object({
  amount: z
    .number({ error: 'passes:override.validation.amount' })
    .int()
    .min(0, 'passes:override.validation.amount')
    .max(100_000_000, 'passes:override.validation.amount')
    .nullable()
    .refine((v) => v !== null, 'passes:override.validation.amount'),
  reason: z
    .string()
    .trim()
    .min(3, 'passes:override.validation.reason')
    .max(500, 'passes:override.validation.reason'),
});

export type OverrideValues = z.input<typeof overrideSchema>;
