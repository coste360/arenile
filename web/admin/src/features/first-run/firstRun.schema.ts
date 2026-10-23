import { z } from 'zod';

/** Minimum password length (server: auth.MinPasswordLength). */
export const MIN_PASSWORD_LENGTH = 10;

/** First-run form; messages are `firstRun:` i18n keys resolved by the form. */
export const firstRunSchema = z
  .object({
    organization_name: z
      .string()
      .trim()
      .min(1, 'firstRun:validation.organizationRequired')
      .max(200, 'firstRun:validation.tooLong'),
    name: z
      .string()
      .trim()
      .min(1, 'firstRun:validation.nameRequired')
      .max(200, 'firstRun:validation.tooLong'),
    email: z.string().trim().max(254).email('firstRun:validation.emailInvalid'),
    password: z
      .string()
      .min(MIN_PASSWORD_LENGTH, 'firstRun:validation.passwordTooShort')
      .max(1024, 'firstRun:validation.tooLong'),
    password_confirm: z.string(),
  })
  .refine((v) => v.password === v.password_confirm, {
    path: ['password_confirm'],
    message: 'firstRun:validation.passwordMismatch',
  });

export type FirstRunValues = z.infer<typeof firstRunSchema>;

/** Reads the one-time token from the URL fragment (`#token=...`). */
export function tokenFromHash(hash: string): string {
  return new URLSearchParams(hash.replace(/^#/, '')).get('token')?.trim() ?? '';
}
