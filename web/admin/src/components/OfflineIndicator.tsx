import { useTranslation } from 'react-i18next';
import { useOnlineStatus } from '../lib/online';

/** Connection status, always visible in the header (spec 21.2). */
export function OfflineIndicator() {
  const { t } = useTranslation();
  const online = useOnlineStatus();
  return (
    <span
      role="status"
      aria-live="polite"
      className={
        online
          ? 'inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-800'
          : 'inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-900'
      }
    >
      <span
        aria-hidden="true"
        className={
          online ? 'h-2 w-2 rounded-full bg-emerald-500' : 'h-2 w-2 rounded-full bg-amber-500'
        }
      />
      {online ? t('common.online') : t('common.offline')}
    </span>
  );
}
