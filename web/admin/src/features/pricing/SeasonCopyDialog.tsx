import { Button, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { Season } from '../configuration/api/resources';
import { FormActions, FormDialog } from '../configuration/components/dialogs';
import { Checkbox, Field } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { useCopySeason, type SeasonCopyResult } from './api';
import { copySchema, type CopyValues } from './rules.schema';

/**
 * "Copia stagione" action (spec 8.7): dialog asking the target year and what
 * to copy; dates move keeping the weekday and the new season starts as draft.
 * After the copy the dialog shows the summary and links to the new season.
 */
export function SeasonCopyDialog({
  establishmentId,
  season,
  onOpenSeason,
}: {
  establishmentId: string;
  season: Season;
  onOpenSeason: (id: string) => void;
}) {
  const { t } = useTranslation('pricing');
  const [open, setOpen] = useState(false);
  const [nonce, setNonce] = useState(0);
  return (
    <>
      <Button
        variant="outline"
        onClick={() => {
          setNonce((n) => n + 1);
          setOpen(true);
        }}
      >
        {t('copy.action')}
      </Button>
      <FormDialog
        open={open}
        onOpenChange={setOpen}
        title={t('copy.title', { year: season.year })}
        description={t('copy.description', { year: season.year })}
      >
        <SeasonCopyForm
          key={nonce}
          establishmentId={establishmentId}
          season={season}
          onClose={() => setOpen(false)}
          onOpenSeason={(id) => {
            setOpen(false);
            onOpenSeason(id);
          }}
        />
      </FormDialog>
    </>
  );
}

export function SeasonCopyForm({
  establishmentId,
  season,
  onClose,
  onOpenSeason,
}: {
  establishmentId: string;
  season: Season;
  onClose: () => void;
  onOpenSeason: (id: string) => void;
}) {
  const { t } = useTranslation('pricing');
  const copy = useCopySeason(establishmentId);
  const [result, setResult] = useState<SeasonCopyResult | null>(null);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const form = useForm<CopyValues>({
    resolver: zodResolver(copySchema),
    defaultValues: {
      target_year: String(season.year + 1),
      include_rate_periods: true,
      include_price_rules: true,
      include_fixed_prices: true,
      include_policies: true,
    },
  });
  const {
    register,
    formState: { errors },
  } = form;
  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      setResult(
        await copy.mutateAsync({
          seasonId: season.id,
          body: {
            target_year: Number(v.target_year),
            include_rate_periods: v.include_rate_periods,
            include_price_rules: v.include_price_rules,
            include_fixed_prices: v.include_fixed_prices,
            include_policies: v.include_policies,
          },
        }),
      );
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }));
    }
  });

  if (result) {
    return (
      <div role="status" className="space-y-3 text-sm">
        <p className="font-medium">
          {t('copy.done', {
            year: result.season.year,
            periods: result.rate_periods,
            rules: result.price_rules,
            fixed: result.fixed_prices,
            policies: result.policies,
          })}
        </p>
        {result.warnings.length > 0 && (
          <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-amber-900">
            <p className="font-medium">{t('copy.warnings')}</p>
            <ul className="list-inside list-disc">
              {result.warnings.map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose}>
            {t('common.close', { ns: 'translation' })}
          </Button>
          <Button onClick={() => onOpenSeason(result.season.id)}>
            {t('copy.open', { year: result.season.year })}
          </Button>
        </div>
      </div>
    );
  }
  return (
    <form noValidate onSubmit={submit} className="space-y-4">
      <FormAlert state={alert} />
      <Field
        id="copy-year"
        label={t('copy.targetYear')}
        error={errors.target_year}
        className="max-w-48"
      >
        <Input
          {...fieldAria('copy-year', errors.target_year)}
          inputMode="numeric"
          {...register('target_year')}
        />
      </Field>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-slate-800">{t('copy.include')}</legend>
        <Checkbox
          id="copy-periods"
          label={t('copy.periods')}
          {...register('include_rate_periods')}
        />
        <Checkbox id="copy-rules" label={t('copy.rules')} {...register('include_price_rules')} />
        <Checkbox id="copy-fixed" label={t('copy.fixed')} {...register('include_fixed_prices')} />
        <Checkbox id="copy-policies" label={t('copy.policies')} {...register('include_policies')} />
      </fieldset>
      <FormActions onCancel={onClose} pending={copy.isPending} submitLabel={t('copy.submit')} />
    </form>
  );
}
