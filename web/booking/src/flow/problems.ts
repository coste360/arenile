import { ApiProblem } from '@arenile/shared/api';

/** Problem types with a dedicated message in the `public` namespace (errors.*). */
const PUBLIC_ERRORS = new Set([
  'hold_expired',
  'spot_unavailable',
  'quota_exceeded',
  'pool_exhausted',
  'policy_violation',
  'checkout_not_found',
  'payment_provider_unavailable',
  'payment_provider_test_mode',
  'rate_limited',
  'invalid_transition',
  'version_conflict',
  'conflict',
  'concurrent_update',
  'validation_failed',
]);

/** Problems after which the chosen resources must be picked again (back to "Dove"). */
const RESOURCE_PROBLEMS = new Set(['spot_unavailable', 'quota_exceeded', 'pool_exhausted']);

/**
 * i18n key (with namespace) of the message for a failed public call:
 * dedicated texts for the booking problems, the shared ones otherwise.
 * Never shows server details verbatim (no personal data, stable wording).
 */
export function problemMessageKey(error: unknown): string {
  if (error instanceof ApiProblem) {
    if (error.isNetworkError) return 'common.errors.network';
    if (PUBLIC_ERRORS.has(error.type)) return `public:errors.${error.type}`;
    if (error.status === 410) return 'public:errors.hold_expired';
    if (error.status === 404) return 'public:errors.checkout_not_found';
  }
  return 'common.errors.generic';
}

export function isResourceProblem(error: unknown): boolean {
  return error instanceof ApiProblem && RESOURCE_PROBLEMS.has(error.type);
}

export function isHoldExpired(error: unknown): boolean {
  return error instanceof ApiProblem && (error.type === 'hold_expired' || error.status === 410);
}

export function isSessionLost(error: unknown): boolean {
  return error instanceof ApiProblem && error.type === 'checkout_not_found';
}
