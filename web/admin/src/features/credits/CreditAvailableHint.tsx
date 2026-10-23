import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { CircleAlertIcon, WalletIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCustomerCredits } from './api';

/**
 * Available credit of the customer under the "Credito cliente" payment
 * method (record-payment dialog): the amount cannot exceed it (the server
 * answers 422 credit_insufficient otherwise).
 */
export function CreditAvailableHint({
  establishmentId,
  customerId,
  amount,
}: {
  establishmentId: string;
  customerId: string;
  amount: number | null | undefined;
}) {
  const { t } = useTranslation('credits');
  const ledger = useCustomerCredits(establishmentId, customerId);
  const available = ledger.data?.pages[0]?.available;
  if (available === undefined) return null;
  const tooMuch = (amount ?? 0) > available;
  return (
    <Alert variant={tooMuch ? 'warning' : 'default'} role="status">
      {tooMuch ? <CircleAlertIcon aria-hidden="true" /> : <WalletIcon aria-hidden="true" />}
      <AlertDescription>
        {tooMuch
          ? t('hint.tooMuch', { available: formatCents(available) })
          : t('hint.available', { available: formatCents(available) })}
      </AlertDescription>
    </Alert>
  );
}
