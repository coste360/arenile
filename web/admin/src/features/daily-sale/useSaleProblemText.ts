import { ApiProblem, useProblemText } from '@arenile/shared/api';
import { useTranslation } from 'react-i18next';

/** Problem types of the daily sale with their own message (dailySale `problems.*`). */
const OWN = new Set([
  'zone_full',
  'assignment_conflict',
  'ticket_number_conflict',
  'ticket_voided',
  'ticket_already_entered',
  'price_changed',
  'spot_unavailable',
  'cash_session_closed',
  'ticket_owned',
]);

/**
 * Message of a failed daily-sale request: the feature's wording for the
 * ticket problems (zone full, spot just taken, price changed...), else the
 * shared text (server detail or generic message).
 */
export function useSaleProblemText() {
  const { t } = useTranslation('dailySale');
  const shared = useProblemText();
  return (error: unknown): string =>
    error instanceof ApiProblem && OWN.has(error.type)
      ? t(`problems.${error.type}`)
      : shared(error);
}
