import { z } from 'zod';
import { V } from '../../configuration/lib/validation';
import type {
  EmailLocale,
  EmailSettings,
  EmailSettingsInput,
  EmailTemplate,
  EmailTemplateKind,
  EmailTemplateTexts,
} from '../api';
import { toEmailInput, toEmailValues } from './email.schema';

/** Customer email kinds with editable texts, in the order of the page (ADR 0022). */
export const TEMPLATE_KINDS: readonly EmailTemplateKind[] = [
  'booking_confirmed',
  'booking_modified',
  'booking_cancelled',
  'booking_rejected',
  'booking_approval_pending',
  'refund_issued',
  'manage_link',
  'balance_reminder',
  'day_before_reminder',
];

export const EMAIL_LOCALES: readonly EmailLocale[] = ['it', 'en', 'de', 'fr'];

/** Length limits, the same as the server (internal/domain/notify). */
export const LIMITS = { subject: 200, opening: 2000, closing: 2000, signature: 500 } as const;

export type TemplateField = keyof typeof LIMITS;

const PLACEHOLDER_RE = /\{([A-Za-z_][A-Za-z0-9_]*)\}/g;

/** Placeholder names of `text` outside `allowed` (deduplicated). */
export function unknownPlaceholders(text: string, allowed: readonly string[]): string[] {
  const out: string[] = [];
  for (const m of text.matchAll(PLACEHOLDER_RE)) {
    const name = m[1]!;
    if (!allowed.includes(name) && !out.includes(name)) out.push(name);
  }
  return out;
}

/**
 * Texts form: the server validates again (422 email_template_invalid); the
 * client checks the same rules to answer while typing.
 */
export function templateSchema(placeholders: readonly string[]) {
  const text = (max: number) =>
    z
      .string()
      .max(max, 'online:customerEmails.validation.tooLong')
      .refine(
        (v) => unknownPlaceholders(v, placeholders).length === 0,
        'online:customerEmails.validation.placeholder',
      );
  return z.object({
    subject: text(LIMITS.subject)
      .refine((v) => v.trim() !== '', V.required)
      .refine((v) => !/[\r\n]/.test(v), 'online:customerEmails.validation.oneLine'),
    opening: text(LIMITS.opening),
    closing: text(LIMITS.closing),
    signature: text(LIMITS.signature),
  });
}

export type TemplateValues = EmailTemplateTexts;

export function toTemplateValues(t: EmailTemplate): TemplateValues {
  return { subject: t.subject, opening: t.opening, closing: t.closing, signature: t.signature };
}

/** Inserts `token` at the selection of a text control and returns the new value and caret. */
export function insertAt(
  value: string,
  token: string,
  start: number | null,
  end: number | null,
): { value: string; caret: number } {
  const s = start ?? value.length;
  const e = end ?? s;
  return { value: value.slice(0, s) + token + value.slice(e), caret: s + token.length };
}

/** Send times offered by the page (every 30 minutes); a stored value outside is kept. */
export function timeOptions(current: string): string[] {
  const out: string[] = [];
  for (let h = 0; h < 24; h++) {
    for (const m of ['00', '30']) out.push(`${String(h).padStart(2, '0')}:${m}`);
  }
  return out.includes(current) ? out : [...out, current].sort();
}

/** Lead days of the balance reminder (0 = on the due date). */
export const LEAD_DAYS = Array.from({ length: 31 }, (_, i) => i);

/** Automatic reminders form: toggles and schedule (stored in the email settings). */
export const remindersSchema = z.object({
  day_before_reminder: z.boolean(),
  balance_reminder: z.boolean(),
  day_before_time: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/),
  balance_time: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/),
  balance_lead_days: z.string().regex(/^([0-9]|[12][0-9]|30)$/),
});

export type RemindersValues = z.infer<typeof remindersSchema>;

export function toRemindersValues(s: EmailSettings): RemindersValues {
  return {
    day_before_reminder: s.customer_notifications.day_before_reminder,
    balance_reminder: s.customer_notifications.balance_reminder,
    day_before_time: s.reminders.day_before_time,
    balance_time: s.reminders.balance_time,
    balance_lead_days: String(s.reminders.balance_lead_days),
  };
}

/** Full email settings input with the reminder fields replaced (PUT email-settings). */
export function toRemindersInput(s: EmailSettings, v: RemindersValues): EmailSettingsInput {
  const base = toEmailInput(toEmailValues(s));
  return {
    ...base,
    customer_notifications: {
      ...base.customer_notifications,
      day_before_reminder: v.day_before_reminder,
      balance_reminder: v.balance_reminder,
    },
    reminders: {
      day_before_time: v.day_before_time,
      balance_time: v.balance_time,
      balance_lead_days: Number(v.balance_lead_days),
    },
  };
}
