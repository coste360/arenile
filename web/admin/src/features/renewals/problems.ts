import { ApiProblem } from '@arenile/shared/api';
import type { TFunction } from 'i18next';
import { describeProblem, type ProblemText } from '../bookings/problem';

/** Problem types of the M5B contract with a dedicated message (renewals:problem.*). */
const M5B_PROBLEMS = new Set([
  'renewal_expired',
  'waitlist_offer_expired',
  'waitlist_disabled',
  'credit_insufficient',
  'season_rollover_conflict',
]);

/**
 * Friendly text of a failed renewal / waitlist / credit / rollover call:
 * the M5B problem types first, then the booking problems (conflicts,
 * versions, policy) of describeProblem.
 */
export function describeM5bProblem(error: unknown, t: TFunction): ProblemText {
  if (error instanceof ApiProblem && M5B_PROBLEMS.has(error.type)) {
    return {
      title: t(`renewals:problem.${error.type}`),
      details: error.detail ? [error.detail] : [],
      reload: error.type === 'renewal_expired' || error.type === 'waitlist_offer_expired',
    };
  }
  return describeProblem(error, t);
}
