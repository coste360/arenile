import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Link } from '@tanstack/react-router';
import { FlaskConicalIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEstablishmentId } from '../../establishment';
import { useTestMode } from './api';

/**
 * Persistent banner of the shell while test mode is on (spec 15.4): every
 * staff member of the establishment sees that what they create now is test
 * data. Rendered by AppShell under the header; nothing outside
 * establishment-scoped routes or while test mode is off.
 */
export function TestModeBanner() {
  const { t } = useTranslation('testMode');
  const establishmentId = useEstablishmentId();
  const query = useTestMode(establishmentId, { poll: true });
  if (!establishmentId || !query.data?.enabled) return null;
  return (
    <div className="px-4 pt-4 md:px-6 print:hidden" data-testid="test-mode-banner">
      <Alert variant="warning">
        <FlaskConicalIcon aria-hidden="true" />
        <AlertTitle>{t('banner.title')}</AlertTitle>
        <AlertDescription>
          <p>
            {query.data.public_enabled ? t('banner.descriptionPublic') : t('banner.description')}
          </p>
          <Button asChild variant="link" className="h-auto p-0">
            <Link to="/e/$establishmentId/config/test-mode" params={{ establishmentId }}>
              {t('banner.manage')}
            </Link>
          </Button>
        </AlertDescription>
      </Alert>
    </div>
  );
}
