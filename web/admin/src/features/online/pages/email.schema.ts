import { z } from 'zod';
import { V } from '../../configuration/lib/validation';
import type { EmailSettings, EmailSettingsInput } from '../api';

const optionalEmail = z
  .string()
  .trim()
  .refine((v) => v === '' || z.email().safeParse(v).success, V.email);

export const MANAGER_NOTIFICATIONS = [
  'new_online_booking',
  'approval_requested',
  'payment_failed',
  'webhook_error',
] as const;
export const CUSTOMER_NOTIFICATIONS = [
  'confirmation',
  'balance_reminder',
  'day_before_reminder',
  'cancellation',
  'refund',
] as const;

/** Email settings form (spec 19): sender name, reply-to, manager address and notifications. */
export const emailSchema = z.object({
  sender_name: z.string().trim().min(1, V.required).max(100),
  reply_to: optionalEmail,
  manager_email: optionalEmail,
  manager_notifications: z.object(
    Object.fromEntries(MANAGER_NOTIFICATIONS.map((k) => [k, z.boolean()])) as Record<
      (typeof MANAGER_NOTIFICATIONS)[number],
      z.ZodBoolean
    >,
  ),
  customer_notifications: z.object(
    Object.fromEntries(CUSTOMER_NOTIFICATIONS.map((k) => [k, z.boolean()])) as Record<
      (typeof CUSTOMER_NOTIFICATIONS)[number],
      z.ZodBoolean
    >,
  ),
});

export type EmailValues = z.infer<typeof emailSchema>;

export function toEmailValues(s: EmailSettings): EmailValues {
  return {
    sender_name: s.sender_name,
    reply_to: s.reply_to ?? '',
    manager_email: s.manager_email ?? '',
    manager_notifications: { ...s.manager_notifications },
    customer_notifications: { ...s.customer_notifications },
  };
}

export function toEmailInput(v: EmailValues): EmailSettingsInput {
  return {
    sender_name: v.sender_name.trim(),
    ...(v.reply_to ? { reply_to: v.reply_to } : {}),
    ...(v.manager_email ? { manager_email: v.manager_email } : {}),
    manager_notifications: v.manager_notifications,
    customer_notifications: v.customer_notifications,
  };
}

export const testEmailSchema = z.object({
  to: z.string().trim().min(1, V.required).pipe(z.email(V.email)),
});
