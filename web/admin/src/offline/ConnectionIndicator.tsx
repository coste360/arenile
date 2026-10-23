import { Button } from '@arenile/shared/ui/button';
import { StatusBadge, type StatusTone } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import { useOfflineStatus, useQueueSheet } from './context';

/**
 * Connection status, always visible in the top bar (spec 21.2): online,
 * offline or syncing, with the number of queued operations and of those
 * the server refused. With operations in the queue it opens their list.
 */
export function ConnectionIndicator() {
  const { t } = useTranslation('offline');
  const { state, pending, rejected } = useOfflineStatus();
  const sheet = useQueueSheet();

  const tone: StatusTone =
    rejected > 0
      ? 'danger'
      : state === 'offline'
        ? 'warning'
        : state === 'syncing'
          ? 'info'
          : 'success';
  const parts = [t(`indicator.${state}`)];
  if (pending > 0) parts.push(t('indicator.pending', { count: pending }));
  if (rejected > 0) parts.push(t('indicator.rejected', { count: rejected }));
  const badge = (
    <span role="status" aria-live="polite">
      <StatusBadge tone={tone}>{parts.join(' · ')}</StatusBadge>
    </span>
  );

  if (pending + rejected === 0) return badge;
  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-auto px-1"
      aria-haspopup="dialog"
      onClick={() => sheet.setOpen(true)}
    >
      {badge}
      <span className="sr-only">{t('indicator.open')}</span>
    </Button>
  );
}
