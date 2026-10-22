import { z } from 'zod';
import type { EstablishmentCreate } from '../api/establishment';
import { SLUG_RE, V, requiredText } from '../lib/validation';
import { SUPPORTED_LOCALES } from './establishment.schema';

export const newEstablishmentSchema = z
  .object({
    organization_id: z.string(),
    name: requiredText(120),
    slug: z.string().trim().regex(SLUG_RE, V.slug),
    timezone: z.string().trim().min(1, V.required),
    default_locale: z.enum(SUPPORTED_LOCALES),
    enabled_locales: z.array(z.enum(SUPPORTED_LOCALES)).min(1, V.localeRequired),
    contact_email: z
      .string()
      .trim()
      .max(254)
      .refine((v) => v === '' || z.email().safeParse(v).success, V.email),
    contact_phone: z.string().trim().max(40),
  })
  .refine((v) => v.enabled_locales.includes(v.default_locale), {
    message: V.localeRequired,
    path: ['enabled_locales'],
  });

export type NewEstablishmentValues = z.infer<typeof newEstablishmentSchema>;

export const newEstablishmentDefaults = (organizationId: string): NewEstablishmentValues => ({
  organization_id: organizationId,
  name: '',
  slug: '',
  timezone: 'Europe/Rome',
  default_locale: 'it',
  enabled_locales: ['it', 'en'],
  contact_email: '',
  contact_phone: '',
});

export function toEstablishmentCreate(v: NewEstablishmentValues): EstablishmentCreate {
  return {
    ...(v.organization_id ? { organization_id: v.organization_id } : {}),
    name: v.name,
    slug: v.slug,
    timezone: v.timezone,
    currency: 'EUR',
    default_locale: v.default_locale,
    enabled_locales: v.enabled_locales,
    ...(v.contact_email ? { contact_email: v.contact_email } : {}),
    ...(v.contact_phone ? { contact_phone: v.contact_phone } : {}),
  };
}
