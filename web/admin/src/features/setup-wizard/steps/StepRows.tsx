import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { FieldLegend, FieldSet } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CircleCheckIcon, InfoIcon, MapIcon } from 'lucide-react';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { DateRangeField, Field } from '../../configuration/components/fields';
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
      className="h-auto max-h-48 w-full max-w-md rounded-lg bg-map-sand/50 p-2"
      aria-hidden="true"
    >
      <rect x="0" y="0" width={s * cell + 4} height="8" className="fill-map-sea" rx="2" />
      {Array.from({ length: r }, (_, i) =>
        Array.from({ length: s }, (_, j) => (
          <circle
            key={`${i}-${j}`}
            cx={j * cell + cell / 2 + 2}
            cy={i * cell + 16 + cell / 2 - 2}
            r={cell / 2 - 1.5}
            className="fill-primary"
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
  if (templates.isPending) {
    return (
      <div role="status" className="space-y-3">
        <span className="sr-only">{t('loading')}</span>
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-32 w-full" />
      </div>
    );
  }
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
        <Alert role="status" className="border-success/40 bg-success/10">
          <CircleCheckIcon aria-hidden="true" className="text-success" />
          <AlertTitle>{t('rows.applied')}</AlertTitle>
          <AlertDescription>
            {t('rows.appliedSummary', {
              spots: result.spots_created,
              zones: result.zone_ids.length,
              packs: result.pack_ids.length,
              periods: result.rate_period_ids.length,
            })}
          </AlertDescription>
        </Alert>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => dispatch({ type: 'complete' })}>{t('continue')}</Button>
          {result.map_id && (
            <Button asChild variant="outline">
              <Link
                to="/e/$establishmentId/maps/$mapId"
                params={{ establishmentId: ctx.establishmentId, mapId: result.map_id }}
              >
                <MapIcon aria-hidden="true" />
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
        <Alert role="status">
          <InfoIcon aria-hidden="true" />
          <AlertDescription>
            {t('rows.alreadyConfigured', { spots: status?.spot_count ?? 0 })}
          </AlertDescription>
        </Alert>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => dispatch({ type: 'complete' })}>{t('continue')}</Button>
          <Button asChild variant="outline">
            <Link to="/e/$establishmentId/maps" params={{ establishmentId: ctx.establishmentId }}>
              <MapIcon aria-hidden="true" />
              {t('rows.openMaps')}
            </Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <form noValidate onSubmit={submit} className="space-y-6">
      <p className="text-sm text-muted-foreground">
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
          <p className="pb-2 text-sm font-medium tabular-nums" aria-live="polite">
            {t('rows.total', { count: rows * spots })}
          </p>
        </div>
      )}
      {withRows && <RowsPreview rows={rows} spots={spots} />}
      {!withRows && <p className="text-sm text-muted-foreground">{t('rows.noRows')}</p>}
      <FieldSet className="gap-4">
        <FieldLegend variant="label">{t('rows.season')}</FieldLegend>
        <div className="grid gap-5 sm:grid-cols-[10rem_1fr]">
          <Field id="rows-year" label={t('rows.year')} error={errors.season_year}>
            <Input
              {...fieldAria('rows-year', errors.season_year)}
              inputMode="numeric"
              {...register('season_year')}
            />
          </Field>
          <Field id="rows-dates" label={t('rows.dates')}>
            <DateRangeField
              control={form.control}
              fromName="opens_on"
              toName="closes_on"
              id="rows-dates"
              label={t('rows.dates')}
            />
          </Field>
        </div>
      </FieldSet>
      <Button type="submit" disabled={apply.isPending}>
        {apply.isPending && <Spinner />}
        {apply.isPending ? t('rows.applying') : t('rows.apply')}
      </Button>
    </form>
  );
}
