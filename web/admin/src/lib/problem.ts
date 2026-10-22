import { ApiProblem } from '@arenile/shared/api';

/** i18n key for a generic problem shown outside of specific forms. */
export function problemMessageKey(error: unknown): string {
  if (!(error instanceof ApiProblem)) return 'common.errors.generic';
  if (error.isNetworkError) return 'common.errors.network';
  switch (error.type) {
    case 'not_found':
      return 'common.errors.notFound';
    case 'forbidden':
    case 'csrf_failed':
      return 'common.errors.forbidden';
    case 'rate_limited':
      return 'common.errors.tooManyRequests';
    case 'validation_failed':
      return 'common.errors.validation';
    case 'conflict':
      return 'common.errors.conflict';
    case 'version_conflict':
      return 'common.errors.versionConflict';
    case 'not_implemented':
      return 'common.errors.notImplemented';
    case 'spot_has_future_allocations':
      return 'common.errors.spotHasFutureAllocations';
    case 'no_price':
      return 'common.errors.noPrice';
    default:
      return 'common.errors.generic';
  }
}
