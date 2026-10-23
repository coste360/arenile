import { useTranslation } from 'react-i18next';
import { useInstantFormat } from '../../establishment';
import { remaining } from './lib';

/**
 * Time left until a deadline (pre-emption, offer hold): "3 g 4 h", "2 h 10
 * min", "Scaduta"; the exact instant is in the tooltip and for screen readers.
 * Shared by the renewals and waitlist pages.
 */
export function Countdown({ deadline, now }: { deadline: string; now: number }) {
  const { t } = useTranslation('renewals');
  const fmt = useInstantFormat();
  const left = remaining(deadline, now);
  const text = left.expired
    ? t('countdown.expired')
    : left.days > 0
      ? t('countdown.days', { days: left.days, hours: left.hours })
      : left.hours > 0
        ? t('countdown.hours', { hours: left.hours, minutes: left.minutes })
        : t('countdown.minutes', { minutes: left.minutes });
  const exact = fmt.dateTime(deadline);
  const urgent = !left.expired && left.days === 0;
  return (
    <span
      title={exact}
      className={
        left.expired
          ? 'text-muted-foreground tabular-nums'
          : urgent
            ? 'font-medium text-destructive tabular-nums'
            : 'tabular-nums'
      }
    >
      {text}
      <span className="sr-only"> ({exact})</span>
    </span>
  );
}
