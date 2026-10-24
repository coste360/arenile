import { z } from 'zod';

/** Loose phone check: E.164 or an Italian number with spaces, dots or dashes (the server normalises). */
const PHONE_RE = /^\+?[0-9][0-9 .\-/]{5,19}$/;

/**
 * Customer form of the "Chi e pagamento" step (spec 12.3 passo 4, 23).
 * The phone is always optional (owner decision, ADR 0020: `require_phone` is
 * deprecated); the email stays required for the confirmation and magic link.
 * Messages are i18n keys of the `public` namespace.
 */
export function customerSchema(opts: { hasTerms: boolean }) {
  return z.object({
    first_name: z.string().trim().min(1, 'who.validation.required').max(100),
    last_name: z.string().trim().min(1, 'who.validation.required').max(100),
    email: z
      .string()
      .trim()
      .min(1, 'who.validation.required')
      .email('who.validation.email')
      .max(254),
    phone: z
      .string()
      .trim()
      .max(32)
      .refine((v) => v === '' || PHONE_RE.test(v), { message: 'who.validation.phone' }),
    accept_terms: opts.hasTerms
      ? z.boolean().refine((v) => v, 'who.validation.terms')
      : z.boolean(),
    marketing_consent: z.boolean(),
  });
}

export type CustomerValues = z.infer<ReturnType<typeof customerSchema>>;
