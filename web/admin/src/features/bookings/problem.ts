import { ApiProblem, problemMessageKey, toApiProblem, type Schemas } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import type { TFunction } from 'i18next';

export type AllocationConflict = Schemas['AllocationConflict'];
export type PolicyViolation = Schemas['PolicyViolation'];
type BookingStatus = Schemas['BookingStatus'];
type Slots = Schemas['Slots'];

/**
 * Problem of an M3 operation keeping the extension members that the generic
 * ApiProblem drops: `conflicts` (spot_unavailable / pool_exhausted /
 * quota_exceeded), `current_status` (invalid_transition), `violations`
 * (policy_violation) and `current_version` (version_conflict).
 */
export class BookingProblem extends ApiProblem {
  readonly conflicts: AllocationConflict[];
  readonly currentStatus?: BookingStatus;
  readonly violations: PolicyViolation[];
  readonly currentVersion?: number;

  constructor(base: ApiProblem, raw: unknown) {
    super({
      type: base.type,
      status: base.status,
      title: base.title,
      detail: base.detail,
      retry_after: base.retryAfter,
      errors: base.fieldErrors,
      request_id: base.requestId,
    });
    const body = (typeof raw === 'object' && raw !== null ? raw : {}) as Record<string, unknown>;
    this.conflicts = Array.isArray(body.conflicts)
      ? (body.conflicts as unknown[]).filter(isConflict)
      : [];
    this.currentStatus =
      typeof body.current_status === 'string' ? (body.current_status as BookingStatus) : undefined;
    this.violations = Array.isArray(body.violations)
      ? (body.violations as unknown[]).filter(
          (v): v is PolicyViolation =>
            typeof v === 'object' && v !== null && typeof (v as PolicyViolation).code === 'string',
        )
      : [];
    this.currentVersion =
      typeof body.current_version === 'number' ? body.current_version : undefined;
  }
}

function isConflict(v: unknown): v is AllocationConflict {
  if (typeof v !== 'object' || v === null) return false;
  const c = v as AllocationConflict;
  return typeof c.date === 'string' && typeof c.slot === 'string' && typeof c.reason === 'string';
}

type FetchResult<T> = { data?: T; error?: unknown; response: Response };

/**
 * Runs an openapi-fetch call like `call()` from the shared API layer, but
 * throws a BookingProblem that keeps the raw problem members.
 */
export async function callBooking<T>(fn: () => Promise<FetchResult<T>>): Promise<T> {
  let result: FetchResult<T>;
  try {
    result = await fn();
  } catch (err) {
    const base =
      err instanceof ApiProblem
        ? err
        : new ApiProblem({
            type: 'network_error',
            status: 0,
            title: 'Network error',
            detail: err instanceof Error ? err.message : undefined,
          });
    throw new BookingProblem(base, undefined);
  }
  if (result.error !== undefined || !result.response.ok) {
    throw new BookingProblem(toApiProblem(result.error, result.response), result.error);
  }
  return result.data as T;
}

/** Conflicting half-days of one resource, AM+PM of the same date merged into FULL. */
export interface ConflictGroup {
  /** spot_id, else pool_id, else zone_id. */
  resourceId: string;
  kind: 'spot' | 'pool' | 'zone';
  reason: AllocationConflict['reason'];
  days: { date: string; slots: Slots }[];
}

/** Groups conflicts by resource and merges the two halves of a day (sorted by date). */
export function groupConflicts(conflicts: readonly AllocationConflict[]): ConflictGroup[] {
  const groups = new Map<string, ConflictGroup & { halves: Map<string, Set<string>> }>();
  for (const c of conflicts) {
    const kind: ConflictGroup['kind'] = c.spot_id ? 'spot' : c.pool_id ? 'pool' : 'zone';
    const resourceId = c.spot_id ?? c.pool_id ?? c.zone_id ?? '';
    const key = `${kind}:${resourceId}`;
    let g = groups.get(key);
    if (!g) {
      g = { resourceId, kind, reason: c.reason, days: [], halves: new Map() };
      groups.set(key, g);
    }
    const set = g.halves.get(c.date) ?? new Set<string>();
    set.add(c.slot);
    g.halves.set(c.date, set);
  }
  return [...groups.values()].map(({ halves, ...g }) => ({
    ...g,
    days: [...halves.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([date, set]) => ({
        date,
        slots: (set.size === 2 ? 'FULL' : ([...set][0] ?? 'FULL')) as Slots,
      })),
  }));
}

/** User-facing text of a failed booking operation. */
export interface ProblemText {
  title: string;
  /** Extra lines (conflicting dates, violated rules, field errors). */
  details: string[];
  /** The data on screen is stale: offer to reload. */
  reload: boolean;
}

export interface DescribeOptions {
  /** Readable label of a spot / pool / zone id (falls back to a generic word). */
  resourceLabel?: (group: ConflictGroup) => string | undefined;
}

const MAX_DAYS_SHOWN = 6;

/** One line per resource: "A12: 14/07/2026, 15/07/2026 (mattina)". */
export function formatConflicts(
  conflicts: readonly AllocationConflict[],
  t: TFunction,
  resourceLabel?: (group: ConflictGroup) => string | undefined,
): string[] {
  return groupConflicts(conflicts).map((g) => {
    const label =
      resourceLabel?.(g) ?? t(`bookings:problem.resource.${g.kind}`, { id: g.resourceId });
    const shown = g.days
      .slice(0, MAX_DAYS_SHOWN)
      .map((d) =>
        d.slots === 'FULL'
          ? formatIsoDate(d.date)
          : `${formatIsoDate(d.date)} (${t(`bookings:slots.${d.slots}`)})`,
      );
    const more = g.days.length - shown.length;
    const days =
      more > 0
        ? `${shown.join(', ')} ${t('bookings:problem.more', { count: more })}`
        : shown.join(', ');
    return t('bookings:problem.conflictLine', { label, days });
  });
}

/**
 * Maps a problem of the M3 contract to a friendly message (spec 17 problem
 * types). `t` must resolve the `bookings` namespace.
 */
export function describeProblem(
  error: unknown,
  t: TFunction,
  options: DescribeOptions = {},
): ProblemText {
  const problem = error instanceof ApiProblem ? error : undefined;
  const conflicts = error instanceof BookingProblem ? error.conflicts : [];
  const conflictLines = () => formatConflicts(conflicts, t, options.resourceLabel);

  if (!problem)
    return { title: t('common.errors.generic', { ns: 'translation' }), details: [], reload: false };

  switch (problem.type) {
    case 'spot_unavailable':
      return {
        title: t('bookings:problem.spotUnavailable'),
        details: conflictLines(),
        reload: false,
      };
    case 'pool_exhausted':
      return {
        title: t('bookings:problem.poolExhausted'),
        details: conflictLines(),
        reload: false,
      };
    case 'quota_exceeded':
      return {
        title: t('bookings:problem.quotaExceeded'),
        details: conflictLines(),
        reload: false,
      };
    case 'hold_expired':
      return { title: t('bookings:problem.holdExpired'), details: [], reload: false };
    case 'invalid_transition': {
      const current = error instanceof BookingProblem ? error.currentStatus : undefined;
      return {
        title: current
          ? t('bookings:problem.invalidTransitionNow', { status: t(`bookings:status.${current}`) })
          : t('bookings:problem.invalidTransition'),
        details: [],
        reload: true,
      };
    }
    case 'version_conflict':
    case 'precondition_required':
      return { title: t('bookings:problem.versionConflict'), details: [], reload: true };
    case 'policy_violation': {
      const violations = error instanceof BookingProblem ? error.violations : [];
      return {
        title: t('bookings:problem.policyViolation'),
        details: violations.map((v) =>
          t(`pricing:violations.${v.code}`, { defaultValue: v.code, ...(v.params ?? {}) }),
        ),
        reload: false,
      };
    }
    case 'no_price':
      return {
        title: t('bookings:problem.noPrice'),
        details: problem.detail ? [problem.detail] : [],
        reload: false,
      };
    case 'cash_session_closed':
      return { title: t('bookings:problem.cashSessionClosed'), details: [], reload: false };
    case 'booking_outside_period':
      return { title: t('bookings:problem.outsidePeriod'), details: [], reload: true };
    case 'validation_failed':
      return {
        title: t('common.errors.validation', { ns: 'translation' }),
        details: problem.fieldErrors.map((f) => (f.field ? `${f.field}: ${f.message}` : f.message)),
        reload: false,
      };
    case 'conflict':
      return {
        title: problem.detail ?? t('common.errors.conflict', { ns: 'translation' }),
        details: [],
        reload: false,
      };
    default:
      if (problem.status === 412) {
        return { title: t('bookings:problem.versionConflict'), details: [], reload: true };
      }
      return {
        title: t(problemMessageKey(problem), { ns: 'translation' }),
        details: [],
        reload: false,
      };
  }
}
