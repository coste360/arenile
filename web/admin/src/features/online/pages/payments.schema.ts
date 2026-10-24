import { z } from 'zod';
import type { PaymentSettings, PaymentSettingsInput } from '../api';

const PK_RE = /^pk_(test|live)_[A-Za-z0-9]+$/;
const SK_RE = /^(sk|rk)_(test|live)_[A-Za-z0-9]+$/;
const WHSEC_RE = /^whsec_[A-Za-z0-9]+$/;

/**
 * Payment provider form (spec 13). Secret fields are write-only: empty means
 * "keep the saved value", the `clear_*` flags remove it.
 */
export const paymentSchema = z
  .object({
    provider: z.string().min(1, 'configuration:validation.required'),
    mode: z.enum(['test', 'live']),
    publishable_key: z
      .string()
      .trim()
      .refine((v) => v === '' || PK_RE.test(v), 'online:payments.validation.publishable'),
    secret_key: z
      .string()
      .trim()
      .refine((v) => v === '' || SK_RE.test(v), 'online:payments.validation.secret'),
    webhook_secret: z
      .string()
      .trim()
      .refine((v) => v === '' || WHSEC_RE.test(v), 'online:payments.validation.webhook'),
    clear_secret_key: z.boolean(),
    clear_webhook_secret: z.boolean(),
    embedded_mode: z.boolean(),
  })
  .superRefine((v, ctx) => {
    if (v.provider !== 'stripe') return;
    // Keys must match the chosen mode: a live key in test mode is a costly mistake.
    const check = (field: 'publishable_key' | 'secret_key', value: string) => {
      if (value && !value.includes(`_${v.mode}_`)) {
        ctx.addIssue({ code: 'custom', path: [field], message: 'online:payments.validation.mode' });
      }
    };
    check('publishable_key', v.publishable_key);
    check('secret_key', v.secret_key);
  });

export type PaymentValues = z.infer<typeof paymentSchema>;

export function toPaymentValues(s: PaymentSettings): PaymentValues {
  return {
    provider: s.provider,
    mode: s.mode,
    publishable_key: s.stripe.publishable_key ?? '',
    secret_key: '',
    webhook_secret: '',
    clear_secret_key: false,
    clear_webhook_secret: false,
    embedded_mode: s.stripe.embedded_mode,
  };
}

export function toPaymentInput(v: PaymentValues): PaymentSettingsInput {
  if (v.provider !== 'stripe') return { provider: v.provider, mode: v.mode };
  return {
    provider: v.provider,
    mode: v.mode,
    stripe: {
      publishable_key: v.publishable_key,
      embedded_mode: v.embedded_mode,
      ...(v.clear_secret_key
        ? { secret_key: '' }
        : v.secret_key
          ? { secret_key: v.secret_key }
          : {}),
      ...(v.clear_webhook_secret
        ? { webhook_secret: '' }
        : v.webhook_secret
          ? { webhook_secret: v.webhook_secret }
          : {}),
    },
  };
}

/** Stripe events the webhook endpoint must receive (Dashboard → Developers → Webhooks). */
export const STRIPE_EVENTS = [
  'checkout.session.completed',
  'checkout.session.async_payment_succeeded',
  'checkout.session.async_payment_failed',
  'checkout.session.expired',
  'charge.refunded',
  'refund.failed',
] as const;
