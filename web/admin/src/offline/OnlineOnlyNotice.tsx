import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { WifiOffIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useOnlineStatus } from '../lib/online';

/**
 * Explains, while offline, why creating or moving a booking is not possible
 * (spec 21.2: availability is guaranteed only by the server). Place it at
 * the top of the form or dialog and disable the submit with useOnlineStatus.
 * Renders nothing online.
 */
export function OnlineOnlyNotice() {
  const { t } = useTranslation('offline');
  const online = useOnlineStatus();
  if (online) return null;
  return (
    <Alert>
      <WifiOffIcon aria-hidden="true" />
      <AlertTitle>{t('onlineOnly.title')}</AlertTitle>
      <AlertDescription>{t('onlineOnly.body')}</AlertDescription>
    </Alert>
  );
}
