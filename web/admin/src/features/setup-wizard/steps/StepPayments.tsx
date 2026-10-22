import { Badge } from '@arenile/shared/ui/badge';
import {
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
  Field as UiField,
} from '@arenile/shared/ui/field';
import { RadioGroup, RadioGroupItem } from '@arenile/shared/ui/radio-group';
import { CreditCardIcon, WalletIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * Step 6: payments. Manual payments (desk) need no configuration; online
 * payments with Stripe are not available yet (spec 13, 28), so the option is
 * shown disabled. The page footer closes the setup.
 */
export function StepPayments() {
  const { t } = useTranslation('setupWizard');
  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">{t('payments.intro')}</p>
      <FieldSet>
        <FieldLegend variant="label">{t('payments.method')}</FieldLegend>
        <RadioGroup defaultValue="manual" className="grid gap-3 sm:grid-cols-2">
          <FieldLabel htmlFor="pay-manual">
            <UiField orientation="horizontal">
              <WalletIcon aria-hidden="true" className="mt-0.5 size-5 text-primary-on-surface" />
              <FieldContent>
                <FieldTitle>{t('payments.manual')}</FieldTitle>
                <FieldDescription>{t('payments.manualHint')}</FieldDescription>
              </FieldContent>
              <RadioGroupItem value="manual" id="pay-manual" />
            </UiField>
          </FieldLabel>
          <FieldLabel htmlFor="pay-stripe">
            <UiField orientation="horizontal" data-disabled="true">
              <CreditCardIcon aria-hidden="true" className="mt-0.5 size-5 text-muted-foreground" />
              <FieldContent>
                <FieldTitle>
                  {t('payments.stripe')}
                  <Badge variant="outline">{t('common.inArrivo', { ns: 'translation' })}</Badge>
                </FieldTitle>
                <FieldDescription id="pay-stripe-hint">
                  {t('payments.stripeLater')}
                </FieldDescription>
              </FieldContent>
              <RadioGroupItem
                value="stripe"
                id="pay-stripe"
                disabled
                aria-describedby="pay-stripe-hint"
              />
            </UiField>
          </FieldLabel>
        </RadioGroup>
      </FieldSet>
    </div>
  );
}
