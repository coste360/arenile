import { ApiProblem } from '@arenile/shared/api';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../../../lib/problem';
import { isVersionConflict } from './formErrors';

/** Human message of a failed mutation (server detail when present). */
export function useProblemText() {
  const { t } = useTranslation();
  return (error: unknown) =>
    isVersionConflict(error)
      ? t('common.errors.versionConflict')
      : error instanceof ApiProblem && error.detail
        ? error.detail
        : t(problemMessageKey(error instanceof ApiProblem ? error : undefined));
}
