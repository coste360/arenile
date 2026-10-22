import { Button, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Field } from '../../configuration/components/fields';
import { FormAlert } from '../../configuration/components/FormAlert';
import { fieldAria } from '../../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../../configuration/lib/formErrors';
import { defaultSeasonYear } from '../../configuration/pages/seasons.schema';
import {
  useApplyTemplate,
  useSetupTemplates,
  type ApplyTemplateResult,
  type SetupTemplate,
} from '../api';
import type { TemplateCode } from '../progress';
import {
  rowsDefaults,
  rowsSchema,
  templateHasRows,
  toApplyTemplateRequest,
} from '../wizard.schema';
import type { StepProps } from './types';

/** Miniature of the generated rows (decorative; the numbers are in the text). */
function RowsPreview({ rows, spots }: { rows: number; spots: number }) {
  const r = Math.min(Math.max(rows, 0), 50);
  const s = Math.min(Math.max(spots, 0), 100);
  if (!r || !s) return null;
  const cell = 10;
  return (
    <svg
      viewBox={`0 0 ${s * cell + 4} ${r * cell + 16}`}
      className="h-auto max-h-48 w-full max-w-md"
      aria-hidden="true"
    >
      <rect x="0" y="0" width={s * cell + 4} height="8" fill="#7dd3fc" rx="2" />
      {Array.from({ length: r }, (_, i) =>
        Array.from({ length: s }, (_, j) => (
          <circle
            key={`${i}-${j}`}
            cx={j * cell + cell / 2 + 2}
            cy={i * cell + 16 + cell / 2 - 2}
            r={cell / 2 - 1.5}
            fill="var(--ar-primary)"
            opacity={1 - i * (0.5 / Math.max(r, 1))}
          />
        )),
      )}
    </svg>
  );
}

/** Step 3: row generator — applies the chosen template with rows × spots (spec 11.2, 15.2). */
export function StepRows(props: StepProps) {
  const { t } = useTranslation('setupWizard');
  const templates = useSetupTemplates(props.ctx.establishmentId);
  // Wait for the template (its default rows × spots); a failed load falls back to 8 × 15.
  if (templates.isPending) return <p className="text-sm text-slate-600">{t('loading')}</p>;
  const template = props.progress.template ?? 'classic_rows';
  return (
    <RowsForm
      {...props}
      template={template}
      tpl={templates.data?.find((x) => x.code === template)}
    />
  );
}

function RowsForm({
  ctx,
  dispatch,
  status,
  template,
  tpl,
}: StepProps & { template: TemplateCode; tpl: SetupTemplate | undefined }) {
  const { t } = useTranslation('setupWizard');
  const apply = useApplyTemplate(ctx.establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const [result, setResult] = useState<ApplyTemplateResult | null>(null);
  const withRows = templateHasRows(template, tpl?.rows);
  const form = useForm({
    resolver: zodResolver(rowsSchema),
    defaultValues: rowsDefaults(withRows, defaultSeasonYear(), tpl?.rows, tpl?.spots_per_row),
  });
  const {
    register,
    formState: { errors },
  } = form;
  const rows = Number(useWatch({ control: form.control, name: 'rows' })) || 0;
  const spots = Number(useWatch({ control: form.control, name: 'spots_per_row' })) || 0;
  const alreadyApplied = !!status && (status.has_map || status.has_season) && !result;

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      const mapName = { [ctx.defaultLocale]: t('rows.mapName') };
      const res = await apply.mutateAsync(toApplyTemplateRequest(template, values, mapName));
      setResult(res);
      dispatch({ type: 'applied', seasonId: res.season_id, mapId: res.map_id });
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
          <p className="font-medium">{t('rows.applied')}</p>
          <p>
            {t('rows.appliedSummary', {
              spots: result.spots_created,
              zones: result.zone_ids.length,
              packs: result.pack_ids.length,
              periods: result.rate_period_ids.length,
            })}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => dispatch({ type: 'complete' })}>{t('continue')}</Button>
          {result.map_id && (
            <Button asChild variant="outline">
              <Link
                to="/e/$establishmentId/maps/$mapId"
                params={{ establishmentId: ctx.establishmentId, mapId: result.map_id }}
              >
                {t('rows.openMap')}
              </Link>
            </Button>
          )}
        </div>
      </div>
    );
  }

  if (alreadyApplied) {
    return (
      <div className="space-y-4">
        <p
          role="status"
          className="rounded-md border border-sky-200 bg-sky-50 px-3 py-2 text-sm text-sky-900"
        >
          {t('rows.alreadyConfigured', { spots: status?.spot_count ?? 0 })}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => dispatch({ type: 'complete' })}>{t('continue')}</Button>
          <Button asChild variant="outline">
            <Link to="/e/$establishmentId/maps" params={{ establishmentId: ctx.establishmentId }}>
              {t('rows.openMaps')}
            </Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <form noValidate onSubmit={submit} className="space-y-4">
      <p className="text-sm text-slate-600">
        {t('rows.intro', { template: t(`template.codes.${template}.name`) })}
      </p>
      <FormAlert state={alert} />
      {withRows && (
        <div className="grid gap-4 sm:grid-cols-[repeat(2,10rem)_1fr] sm:items-end">
          <Field id="rows-rows" label={t('rows.rows')} error={errors.rows}>
            <Input
              {...fieldAria('rows-rows', errors.rows)}
              inputMode="numeric"
              {...register('rows')}
            />
          </Field>
          <Field id="rows-spots" label={t('rows.spotsPerRow')} error={errors.spots_per_row}>
            <Input
              {...fieldAria('rows-spots', errors.spots_per_row)}
              inputMode="numeric"
              {...register('spots_per_row')}
            />
          </Field>
          <p className="text-sm font-medium text-slate-800" aria-live="polite">
            {t('rows.total', { count: rows * spots })}
          </p>
        </div>
      )}
      {withRows && <RowsPreview rows={rows} spots={spots} />}
      {!withRows && <p className="text-sm text-slate-600">{t('rows.noRows')}</p>}
      <fieldset className="grid gap-4 sm:grid-cols-3">
        <legend className="mb-2 text-sm font-medium text-slate-800">{t('rows.season')}</legend>
        <Field id="rows-year" label={t('rows.year')} error={errors.season_year}>
          <Input
            {...fieldAria('rows-year', errors.season_year)}
            inputMode="numeric"
            {...register('season_year')}
          />
        </Field>
        <Field id="rows-opens" label={t('rows.opensOn')} error={errors.opens_on}>
          <Input
            {...fieldAria('rows-opens', errors.opens_on)}
            type="date"
            {...register('opens_on')}
          />
        </Field>
        <Field id="rows-closes" label={t('rows.closesOn')} error={errors.closes_on}>
          <Input
            {...fieldAria('rows-closes', errors.closes_on)}
            type="date"
            {...register('closes_on')}
          />
        </Field>
      </fieldset>
      <Button type="submit" disabled={apply.isPending}>
        {apply.isPending ? t('rows.applying') : t('rows.apply')}
      </Button>
    </form>
  );
}
