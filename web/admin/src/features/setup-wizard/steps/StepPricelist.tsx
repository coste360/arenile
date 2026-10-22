import { Button, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ratePeriods, seasons, zones, type RatePeriod } from '../../configuration/api/resources';
import { FieldError } from '../../configuration/components/fields';
import { FormAlert } from '../../configuration/components/FormAlert';
import { errorAt, fieldAria } from '../../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../../configuration/lib/formErrors';
import { localize } from '../../configuration/lib/i18nText';
import { formatCents } from '../../configuration/lib/money';
import { useQuickPricelist, type QuickPricelistResult } from '../api';
import { emptyGrid, pricelistSchema, toQuickPricelistRequest } from '../wizard.schema';
import type { StepProps } from './types';

/** Step 4: quick price list — daily price per price category and rate period. */
export function StepPricelist(props: StepProps) {
  const { t } = useTranslation('setupWizard');
  const { ctx, progress } = props;
  const seasonList = seasons.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const season =
    seasonList.data?.find((s) => s.id === progress.seasonId) ??
    seasonList.data?.find((s) => s.status === 'active') ??
    [...(seasonList.data ?? [])].sort((a, b) => b.year - a.year)[0];
  const periodList = ratePeriods.useList(
    ctx.establishmentId,
    { season_id: season?.id },
    { enabled: !!season },
  );

  if (seasonList.isPending || zoneList.isPending || (season && periodList.isPending)) {
    return <p className="text-sm text-slate-600">{t('loading')}</p>;
  }
  const categories = [
    ...new Set(
      [...(zoneList.data ?? [])]
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((z) => z.price_category),
    ),
  ];
  const periods = [...(periodList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  if (!season || categories.length === 0 || periods.length === 0) {
    return (
      <div className="space-y-3">
        <p
          role="status"
          className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900"
        >
          {!season
            ? t('pricelist.noSeason')
            : categories.length === 0
              ? t('pricelist.noZones')
              : t('pricelist.noPeriods')}
        </p>
      </div>
    );
  }
  return (
    <PricelistGrid
      {...props}
      key={`${season.id}-${categories.join()}-${periods.map((p) => p.code).join()}`}
      seasonId={season.id}
      seasonYear={season.year}
      categories={categories}
      periods={periods}
    />
  );
}

function PricelistGrid({
  ctx,
  dispatch,
  seasonId,
  seasonYear,
  categories,
  periods,
}: StepProps & {
  seasonId: string;
  seasonYear: number;
  categories: string[];
  periods: RatePeriod[];
}) {
  const { t } = useTranslation('setupWizard');
  const quick = useQuickPricelist(ctx.establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const [result, setResult] = useState<QuickPricelistResult | null>(null);
  const form = useForm({
    resolver: zodResolver(pricelistSchema),
    defaultValues: emptyGrid(categories.length, periods.length),
  });
  const {
    register,
    formState: { errors },
  } = form;
  const currency = ctx.establishment?.currency ?? 'EUR';

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      const req = toQuickPricelistRequest(
        seasonId,
        categories,
        periods.map((p) => p.code),
        values,
      );
      setResult(await quick.mutateAsync(req));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(values) }));
    }
  });

  if (result) {
    return (
      <div className="space-y-4">
        <div
          role="status"
          className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-900"
        >
          {t('pricelist.created', { count: result.rules.length, replaced: result.replaced })}
        </div>
        <Button onClick={() => dispatch({ type: 'complete' })}>{t('continue')}</Button>
      </div>
    );
  }

  return (
    <form noValidate onSubmit={submit} className="space-y-4">
      <p className="text-sm text-slate-600">{t('pricelist.intro', { year: seasonYear })}</p>
      <FormAlert state={alert} />
      <div className="overflow-x-auto">
        <table className="text-sm">
          <caption className="sr-only">{t('pricelist.caption', { currency })}</caption>
          <thead>
            <tr>
              <th scope="col" className="px-2 py-1 text-left font-medium text-slate-600">
                {t('pricelist.category')}
              </th>
              {periods.map((p) => (
                <th key={p.id} scope="col" className="px-2 py-1 text-left font-medium">
                  <span className="inline-flex items-center gap-1.5">
                    <span
                      aria-hidden="true"
                      className="h-3 w-3 rounded"
                      style={{ backgroundColor: p.color }}
                    />
                    {localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.code}
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {categories.map((cat, i) => (
              <tr key={cat}>
                <th scope="row" className="px-2 py-1 text-left font-mono text-xs font-medium">
                  {cat}
                </th>
                {periods.map((p, j) => {
                  const id = `pl-${i}-${j}`;
                  const err = errorAt(errors, `cells.${i}.${j}`);
                  return (
                    <td key={p.id} className="px-2 py-1 align-top">
                      <div className="flex items-center gap-1">
                        <span aria-hidden="true" className="text-slate-500">
                          €
                        </span>
                        <Input
                          {...fieldAria(id, err)}
                          aria-label={t('pricelist.cellAria', {
                            category: cat,
                            period: localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.code,
                          })}
                          className="w-24"
                          inputMode="decimal"
                          placeholder="0,00"
                          {...register(`cells.${i}.${j}`)}
                        />
                      </div>
                      <FieldError id={id} error={err} />
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <FieldError id="pl-cells" error={errorAt(errors, 'cells')} />
      <p className="text-xs text-slate-600">
        {t('pricelist.hint', { example: formatCents(2500, ctx.uiLocale, currency) })}
      </p>
      <Button type="submit" disabled={quick.isPending}>
        {quick.isPending ? t('pricelist.generating') : t('pricelist.generate')}
      </Button>
    </form>
  );
}
