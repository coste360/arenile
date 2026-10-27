import { ApiProblem, useMe, type Me } from '@arenile/shared/api';
import type { TFunction } from 'i18next';
import { useCurrentEstablishment } from '../../establishment';
import { describeProblem, type DescribeOptions, type ProblemText } from '../bookings/problem';

/** What the current user may do in "Partner e convenzioni" (spec 16.1; the server checks again). */
export interface PartnerPermissions {
  /** Partners, categories, agreements (establishment.configure). */
  canConfigure: boolean;
  /** Statement, reports and register (reports.read). */
  canReport: boolean;
  /** Record and void partner payments (cash.operate). */
  canTakePayments: boolean;
  /** Bookings and customers: agreement, verifications, group (bookings.write). */
  canWrite: boolean;
}

export function partnerPermissions(
  me: Me | undefined,
  establishment: { id: string; organization_id: string } | undefined,
): PartnerPermissions {
  const granted = new Set<string>();
  if (me && establishment) {
    if (me.principal_type === 'api_key' && me.api_key?.establishment_id === establishment.id) {
      for (const p of me.api_key.scopes) granted.add(p);
    }
    for (const m of me.memberships) {
      const applies = m.establishment_id
        ? m.establishment_id === establishment.id
        : m.organization_id === establishment.organization_id;
      if (applies) for (const p of m.permissions) granted.add(p);
    }
  }
  return {
    canConfigure: granted.has('establishment.configure'),
    canReport: granted.has('reports.read'),
    canTakePayments: granted.has('cash.operate'),
    canWrite: granted.has('bookings.write'),
  };
}

export function usePartnerPermissions(): PartnerPermissions {
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  return partnerPermissions(me.data, establishment);
}

/** Problem types introduced by M8 (internal/http/problem/m8.go) with their own message. */
export const M8_PROBLEMS = [
  'agreement_quota_exceeded',
  'agreement_not_applicable',
  'requirement_not_verified',
  'partner_in_use',
  'customer_category_in_use',
] as const;

/**
 * Friendly text of a failed M8 operation: the M8 problem types get their own
 * message (namespace `partners`), everything else the booking mapping.
 */
export function describeM8Problem(
  error: unknown,
  t: TFunction,
  options?: DescribeOptions,
): ProblemText {
  if (error instanceof ApiProblem && (M8_PROBLEMS as readonly string[]).includes(error.type)) {
    return { title: t(`partners:problems.${error.type}`), details: [], reload: false };
  }
  return describeProblem(error, t, options);
}

/** "Mario, Lucia ,, Paolo" (one per line or comma) → ["Mario", "Lucia", "Paolo"]. */
export function parseNames(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/** Inclusive calendar year of a date: { from: "2026-01-01", to: "2026-12-31" }. */
export function yearOf(isoDate: string): { from: string; to: string } {
  const y = isoDate.slice(0, 4);
  return { from: `${y}-01-01`, to: `${y}-12-31` };
}
