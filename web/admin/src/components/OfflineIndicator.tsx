import { StatusBadge } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import { useOnlineStatus } from '../lib/online';

/** Connection status, always visible in the top bar (spec 21.2). */
export function OfflineIndicator() {
  const { t } = useTranslation();
  const online = useOnlineStatus();
  return (
    <span role="status" aria-live="polite">
      <StatusBadge tone={online ? 'success' : 'warning'}>
        {online ? t('common.online') : t('common.offline')}
      </StatusBadge>
    </span>
  );
}
