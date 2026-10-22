import { z } from 'zod';
import type { TeamMemberCreate } from '../api/team';
import { V, requiredText } from '../lib/validation';

export const ROLES = ['owner', 'manager', 'cashier', 'beach_staff', 'viewer'] as const;
export const TEAM_SCOPES = ['establishment', 'organization'] as const;

export const inviteSchema = z.object({
  email: z
    .string()
    .trim()
    .max(254)
    .refine((v) => z.email().safeParse(v).success, V.email),
  name: requiredText(120),
  role: z.enum(ROLES),
  scope: z.enum(TEAM_SCOPES),
  locale: z.enum(['it', 'en', 'de', 'fr']),
  password: z
    .string()
    .max(1024)
    .refine((v) => v === '' || v.length >= 12, 'configuration:validation.passwordLength'),
});

export type InviteValues = z.infer<typeof inviteSchema>;

export const inviteDefaults: InviteValues = {
  email: '',
  name: '',
  role: 'cashier',
  scope: 'establishment',
  locale: 'it',
  password: '',
};

export function toTeamMemberCreate(v: InviteValues): TeamMemberCreate {
  return {
    email: v.email,
    name: v.name,
    role: v.role,
    scope: v.scope,
    locale: v.locale,
    ...(v.password ? { password: v.password } : {}),
  };
}
