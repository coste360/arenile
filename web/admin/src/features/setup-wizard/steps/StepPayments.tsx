import { Button } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';
import type { StepProps } from './types';

/**
 * Step 6: payments. Manual payments (desk) need no configuration; online
 * payments with Stripe arrive with M4 (spec 13, 28), so the option is shown
 * disabled.
 */
export function StepPayments({ dispatch }: StepProps) {
  const { t } = useTranslation('setupWizard');
  return (
    <div className="space-y-4">
      <p className="text-sm text-slate-600">{t('payments.intro')}</p>
      <fieldset className="space-y-3">
        <legend className="text-sm font-medium text-slate-800">{t('payments.method')}</legend>
        <label
          htmlFor="pay-manual"
          className="flex items-start gap-2 rounded-md border border-slate-200 p-3 text-sm"
        >
          <input
            id="pay-manual"
            type="radio"
            name="payment"
            defaultChecked
            className="mt-0.5 accent-[var(--ar-primary)]"
          />
          <span>
            <span className="font-medium">{t('payments.manual')}</span>
            <span className="block text-xs text-slate-600">{t('payments.manualHint')}</span>
          </span>
        </label>
        <label
          htmlFor="pay-stripe"
          className="flex items-start gap-2 rounded-md border border-dashed border-slate-300 p-3 text-sm text-slate-500"
        >
          <input
            id="pay-stripe"
            type="radio"
            name="payment"
            disabled
            aria-describedby="pay-stripe-hint"
            className="mt-0.5"
          />
          <span>
            <span className="font-medium">{t('payments.stripe')}</span>
            <span id="pay-stripe-hint" className="block text-xs">
              {t('payments.stripeLater')}
            </span>
          </span>
        </label>
      </fieldset>
      <Button onClick={() => dispatch({ type: 'complete' })}>{t('finish')}</Button>
    </div>
  );
}
