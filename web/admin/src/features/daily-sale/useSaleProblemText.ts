import { ApiProblem, useProblemText } from '@arenile/shared/api';
import { useTranslation } from 'react-i18next';
import { BookingProblem } from '../bookings/problem';

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
  return (error: unknown): string => {
    // Same-day rule of the booking policy: say what to do instead of the
    // generic "does not respect the booking rules".
    if (error instanceof BookingProblem && error.type === 'policy_violation') {
      const sameDay = error.violations.find((v) => v.code === 'same_day_closed');
      const reason = sameDay?.params?.reason;
      if (reason === 'time' || reason === 'slot' || reason === 'disabled') {
        return t(`problems.sameDay.${reason}`);
      }
    }
    return error instanceof ApiProblem && OWN.has(error.type)
      ? t(`problems.${error.type}`)
      : shared(error);
  };
}
