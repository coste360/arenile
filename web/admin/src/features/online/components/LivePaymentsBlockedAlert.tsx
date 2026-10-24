import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Link } from '@tanstack/react-router';
import { BanknoteIcon, FlaskConicalIcon, TriangleAlertIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * Warning shown when the payment provider is configured with test keys while
 * the test mode (spec 15.4) is off: online payments of real customers are
 * blocked (INV-88). The way out is live keys, or the test mode to try the
 * flow. `showPaymentsLink` adds the shortcut to the payment settings (pages
 * other than the payment settings themselves).
 */
export function LivePaymentsBlockedAlert({
  establishmentId,
  showPaymentsLink = false,
}: {
  establishmentId: string;
  showPaymentsLink?: boolean;
}) {
  const { t } = useTranslation('online');
  return (
    <Alert variant="destructive" role="alert">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertTitle>{t('payments.blocked.title')}</AlertTitle>
      <AlertDescription>
        <p>{t('payments.blocked.description')}</p>
        <div className="mt-2 flex flex-wrap gap-2">
          {showPaymentsLink && (
            <Button asChild size="sm" variant="outline">
              <Link to="/e/$establishmentId/config/payments" params={{ establishmentId }}>
                <BanknoteIcon aria-hidden="true" />
                {t('payments.blocked.toPayments')}
              </Link>
            </Button>
          )}
          <Button asChild size="sm" variant="outline">
            <Link to="/e/$establishmentId/config/test-mode" params={{ establishmentId }}>
              <FlaskConicalIcon aria-hidden="true" />
              {t('payments.blocked.toTestMode')}
            </Link>
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  );
}
