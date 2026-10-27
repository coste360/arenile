import type { Schemas } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import { Field, FieldDescription, FieldError, FieldLabel } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PUBLIC_NS } from '../../i18n';

export interface AgreementCodeProps {
  value: string;
  status?: Schemas['AgreementStatus'];
  onApply: (code: string) => void;
}

/**
 * Agreement code field (spec 30.2.2, channel "online con codice"): the code
 * of a hotel, a body or a category. Like the promo code, an unknown code is
 * not an error of the request: the quote reports `agreement.status`.
 */
export function AgreementCode({ value, status, onApply }: AgreementCodeProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const id = useId();
  const [draft, setDraft] = useState(value);
  const current = status && status.code.toUpperCase() === value ? status.status : undefined;
  const failed = current === 'invalid' || current === 'quota_exhausted';
  return (
    <Field data-invalid={failed || undefined}>
      <FieldLabel htmlFor={id}>{t('agreement.label')}</FieldLabel>
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
          aria-invalid={failed || undefined}
          aria-describedby={current ? `${id}-status` : undefined}
        />
        {value ? (
          <Button
            type="button"
            variant="outline"
            className="h-11"
            aria-label={t('agreement.removeAria')}
            onClick={() => {
              setDraft('');
              onApply('');
            }}
          >
            {t('what.removePromo')}
          </Button>
        ) : (
          <Button
            type="submit"
            variant="outline"
            className="h-11"
            aria-label={t('agreement.applyAria')}
            disabled={!draft.trim()}
          >
            {t('what.applyPromo')}
          </Button>
        )}
      </form>
      <div aria-live="polite">
        {current === 'applied' && (
          <FieldDescription id={`${id}-status`}>
            {t('agreement.applied', { name: status?.name ?? value })}
            {status?.requirement &&
              ` ${t('agreement.requirement', { requirement: status.requirement })}`}
          </FieldDescription>
        )}
        {current === 'not_applicable' && (
          <FieldDescription id={`${id}-status`}>
            {t('agreement.notApplicable', { code: value })}
          </FieldDescription>
        )}
        {current === 'invalid' && (
          <FieldError id={`${id}-status`}>{t('agreement.invalid')}</FieldError>
        )}
        {current === 'quota_exhausted' && (
          <FieldError id={`${id}-status`}>{t('agreement.quotaExhausted')}</FieldError>
        )}
      </div>
    </Field>
  );
}
