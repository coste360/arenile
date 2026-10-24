import { Button } from '@arenile/shared/ui/button';
import { Link } from '@tanstack/react-router';
import { CreditCardIcon, WalletIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { StepProps } from './types';

/**
 * Step 6: payments. Desk payments need no configuration; online payments with
 * Stripe are set up on their own page (keys and webhook, spec 13), linked from
 * here. The page footer closes the setup.
 */
export function StepPayments({ ctx }: StepProps) {
  const { t } = useTranslation('setupWizard');
  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">{t('payments.intro')}</p>
      <ul className="grid gap-3 sm:grid-cols-2">
        <li className="flex gap-3 rounded-lg border p-4">
          <WalletIcon
            aria-hidden="true"
            className="mt-0.5 size-5 shrink-0 text-primary-on-surface"
          />
          <div className="space-y-1">
            <h3 className="text-sm font-medium">{t('payments.manual')}</h3>
            <p className="text-sm text-muted-foreground">{t('payments.manualHint')}</p>
          </div>
        </li>
        <li className="flex gap-3 rounded-lg border p-4">
          <CreditCardIcon
            aria-hidden="true"
            className="mt-0.5 size-5 shrink-0 text-primary-on-surface"
          />
          <div className="space-y-3">
            <div className="space-y-1">
              <h3 className="text-sm font-medium">{t('payments.stripe')}</h3>
              <p className="text-sm text-muted-foreground">{t('payments.stripeHint')}</p>
            </div>
            <Button asChild variant="outline" size="sm">
              <Link
                to="/e/$establishmentId/config/payments"
                params={{ establishmentId: ctx.establishmentId }}
              >
                {t('payments.stripeSetup')}
              </Link>
            </Button>
          </div>
        </li>
      </ul>
    </div>
  );
}
