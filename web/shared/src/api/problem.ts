import { useTranslation } from 'react-i18next';
import { ApiProblem } from './client';

/** i18n key (shared `common.errors.*`) of a failed request shown outside a specific form. */
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

/** True for a stale `If-Match` (the resource changed since it was loaded). */
export function isVersionConflict(error: unknown): boolean {
  return error instanceof ApiProblem && (error.type === 'version_conflict' || error.status === 412);
}

/**
 * Human message of a failed request: the translated version-conflict notice,
 * else the server `detail` when present, else the translated generic message.
 */
export function useProblemText() {
  const { t } = useTranslation();
  return (error: unknown) =>
    isVersionConflict(error)
      ? t('common.errors.versionConflict')
      : error instanceof ApiProblem && error.detail
        ? error.detail
        : t(problemMessageKey(error));
}
