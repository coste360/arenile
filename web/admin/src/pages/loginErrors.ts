import { ApiProblem } from '@arenile/shared/api';

/** Maps a login problem+json to an i18n key (types from openapi ProblemType). */
export function loginErrorKey(error: unknown): string {
  if (!(error instanceof ApiProblem)) return 'auth.errors.generic';
  if (error.isNetworkError) return 'common.errors.network';
  switch (error.type) {
    case 'invalid_credentials':
    case 'unauthorized':
      return 'auth.errors.invalidCredentials';
    case 'rate_limited':
      return 'auth.errors.tooManyAttempts';
    case 'totp_required':
      return 'auth.errors.totpRequired';
    case 'invalid_totp':
      return 'auth.errors.invalidTotp';
    case 'validation_failed':
    case 'bad_request':
      return 'auth.errors.invalidCredentials';
    default:
      return 'auth.errors.generic';
  }
}
