import { announcementBucket } from './countdown';
import { cn } from '@arenile/shared/lib/utils';
import { TimerIcon } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatCountdown } from '../../flow/format';
import { serverNow } from '../../flow/serverClock';
import { PUBLIC_NS } from '../../i18n';

export interface HoldCountdownProps {
  /** RFC 3339 hold expiry from the server. */
  expiresAt: string;
  onExpire: () => void;
  /** Current time in ms (default: the server's clock), injectable for tests. */
  now?: () => number;
}

/**
 * Visible countdown of the hold (spec 12.3 passo 4). The ticking text is
 * hidden from assistive technology; a polite live region announces it at
 * coarse steps. Calls `onExpire` once when the time is up.
 */
export function HoldCountdown({ expiresAt, onExpire, now = serverNow }: HoldCountdownProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const end = Date.parse(expiresAt);
  const [remaining, setRemaining] = useState(() => end - now());
  const expired = useRef(false);
  const onExpireRef = useRef(onExpire);
  useEffect(() => {
    onExpireRef.current = onExpire;
  });

  useEffect(() => {
    expired.current = false;
    const tick = () => {
      const r = end - now();
      setRemaining(r);
      if (r <= 0 && !expired.current) {
        expired.current = true;
        onExpireRef.current();
      }
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [end, now]);

  const bucket = announcementBucket(remaining);
  const urgent = remaining <= 60_000;
  return (
    <div
      className={cn(
        'flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium',
        urgent ? 'border-destructive/50 bg-destructive/10 text-destructive' : 'bg-muted/50',
      )}
      data-testid="hold-countdown"
    >
      <TimerIcon aria-hidden="true" className="size-4 shrink-0" />
      <span aria-hidden="true" className="tabular-nums">
        {remaining > 0
          ? t('who.countdown', { time: formatCountdown(remaining) })
          : t('who.expired')}
      </span>
      <span role="status" className="sr-only">
        {bucket === null
          ? t('who.expired')
          : bucket.unit === 'minutes'
            ? t('who.countdownSr', { minutes: bucket.value })
            : t('who.countdownSrSeconds', { seconds: bucket.value })}
      </span>
    </div>
  );
}
