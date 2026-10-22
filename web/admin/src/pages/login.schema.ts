import { z } from 'zod';

/** Login form schema; messages are i18n keys resolved by the form. */
export const loginSchema = z.object({
  email: z.string().trim().max(254).email('auth.validation.emailInvalid'),
  password: z.string().min(1, 'auth.validation.passwordRequired').max(1024),
  totp_code: z
    .string()
    .trim()
    .regex(/^[0-9]{6}$/, 'auth.validation.totpInvalid')
    .optional()
    .or(z.literal('')),
});

export type LoginValues = z.infer<typeof loginSchema>;
