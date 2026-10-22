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
    default:
      return 'common.errors.generic';
  }
}
