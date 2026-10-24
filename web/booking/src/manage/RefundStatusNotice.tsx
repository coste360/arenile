import type { Schemas } from '@arenile/shared/api';
import { formatInstantDate } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { BanknoteIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { formatMoney, intlLocale } from '../flow/format';

type ManageRefundStatus = Schemas['ManageRefundStatus'];

/**
 * What the customer sees of the guaranteed refund of their cancellation
 * (ADR 0019): status, amount and dates only, never provider details.
 */
export function RefundStatusNotice({
  refund,
  currency,
  timeZone,
}: {
  refund: ManageRefundStatus;
  currency: string;
  timeZone?: string;
}) {
  const { t, i18n } = useTranslation();
  const locale = intlLocale(i18n.language);
  const date = (iso: string | undefined) => formatInstantDate(iso, { locale, timeZone });
  return (
    <Alert role="status">
      <BanknoteIcon aria-hidden="true" />
      <AlertTitle>
        {t('manage.refund.title', { amount: formatMoney(refund.amount, currency, i18n.language) })}
      </AlertTitle>
      <AlertDescription>
        <p>{t(`manage.refund.status.${refund.status}`)}</p>
        <p className="text-xs text-muted-foreground">
          {refund.status === 'succeeded' && refund.completed_at
            ? t('manage.refund.completedAt', { date: date(refund.completed_at) })
            : t('manage.refund.requestedAt', { date: date(refund.requested_at) })}
        </p>
      </AlertDescription>
    </Alert>
  );
}
