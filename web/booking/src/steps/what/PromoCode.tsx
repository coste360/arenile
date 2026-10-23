import type { Schemas } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import { Field, FieldDescription, FieldError, FieldLabel } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PUBLIC_NS } from '../../i18n';

export interface PromoCodeProps {
  value: string;
  status?: Schemas['PromoStatus'];
  onApply: (code: string) => void;
}

/**
 * Promo code field (spec 12.3 passo 3). An invalid code is not an error of
 * the request: the quote reports `promo.status`, shown under the field.
 */
export function PromoCode({ value, status, onApply }: PromoCodeProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const id = useId();
  const [draft, setDraft] = useState(value);
  const current = status && status.code === value ? status.status : undefined;
  return (
    <Field data-invalid={current === 'invalid' || undefined}>
      <FieldLabel htmlFor={id}>{t('what.promo')}</FieldLabel>
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          onApply(draft);
        }}
      >
        <Input
          id={id}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          autoCapitalize="characters"
          autoComplete="off"
          maxLength={40}
          className="h-11 uppercase"
          aria-invalid={current === 'invalid' || undefined}
          aria-describedby={current ? `${id}-status` : undefined}
        />
        {value ? (
          <Button
            type="button"
            variant="outline"
            className="h-11"
            onClick={() => {
              setDraft('');
              onApply('');
            }}
          >
            {t('what.removePromo')}
          </Button>
        ) : (
          <Button type="submit" variant="outline" className="h-11" disabled={!draft.trim()}>
            {t('what.applyPromo')}
          </Button>
        )}
      </form>
      <div aria-live="polite">
        {current === 'applied' && (
          <FieldDescription id={`${id}-status`}>
            {t('what.promoApplied', { code: value })}
          </FieldDescription>
        )}
        {current === 'not_applicable' && (
          <FieldDescription id={`${id}-status`}>
            {t('what.promoNotApplicable', { code: value })}
          </FieldDescription>
        )}
        {current === 'invalid' && (
          <FieldError id={`${id}-status`}>{t('what.promoInvalid')}</FieldError>
        )}
      </div>
    </Field>
  );
}
