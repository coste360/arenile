import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { CircleAlertIcon, LogInIcon, WifiOffIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useOfflineRuntime, useOfflineStatus, useQueueSheet } from './context';

/**
 * Shell banners of the offline mode (shared Alert, spec 21.2): offline with
 * the time of the saved data and what can still be done; operations the
 * server refused; queue paused by an expired session. Renders nothing when
 * there is nothing to say.
 */
export function OfflineBanner() {
  const { t, i18n } = useTranslation('offline');
  const runtime = useOfflineRuntime();
  const { online, rejected, paused } = useOfflineStatus();
  const sheet = useQueueSheet();

  if (online && rejected === 0 && !paused) return null;
  const savedAt = runtime?.persister.lastSavedAt();

  return (
    <div className="space-y-3 px-4 pt-4 md:px-6">
      {!online && (
        <Alert>
          <WifiOffIcon aria-hidden="true" />
          <AlertTitle>{t('banner.offlineTitle')}</AlertTitle>
          <AlertDescription>
            <p>
              {savedAt !== undefined
                ? t('banner.savedAt', {
                    time: new Date(savedAt).toLocaleTimeString(i18n.language, {
                      hour: '2-digit',
                      minute: '2-digit',
                    }),
                  })
                : t('banner.noData')}
            </p>
            <p>{t('banner.allowed')}</p>
          </AlertDescription>
        </Alert>
      )}
      {paused && (
        <Alert>
          <LogInIcon aria-hidden="true" />
          <AlertDescription>{t('banner.paused')}</AlertDescription>
        </Alert>
      )}
      {rejected > 0 && (
        <Alert variant="destructive">
          <CircleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('banner.rejectedTitle')}</AlertTitle>
          <AlertDescription>
            <p>{t('banner.rejected', { count: rejected })}</p>
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => sheet.setOpen(true)}
            >
              {t('banner.review')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}
