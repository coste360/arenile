import { Button, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormActions } from '../configuration/components/dialogs';
import { Field, Select } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { I18nTextInput } from '../configuration/components/I18nTextInput';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { useRowPreview } from './api';
import {
  generatorDefaults,
  generatorSchema,
  mapSettingsDefaults,
  mapSettingsSchema,
  renumberDefaults,
  renumberSchema,
  toGeneratorRequest,
  toMapSettings,
  type GeneratorValues,
  type MapSettingsValues,
  type RenumberValues,
} from './editor.schema';
import { spotFromInput } from './model/document';
import { conflictingLabels, formatSpotLabel, type RenumberOptions } from './model/editor';
import {
  SEA_SIDES,
  SPOT_KINDS,
  type DraftDocument,
  type DraftMapSettings,
  type DraftSpot,
  type RowGeneratorResult,
} from './model/types';

export interface ZoneOption {
  id: string;
  name: string;
  disabled?: boolean;
}

/**
 * Row generator (spec 11.2): parameters → server preview (nothing written)
 * → the spots are added to the draft and saved with the map.
 */
export function RowGeneratorForm({
  establishmentId,
  mapId,
  doc,
  zones,
  onAdd,
  onCancel,
}: {
  establishmentId: string;
  mapId: string;
  doc: DraftDocument;
  zones: ZoneOption[];
  onAdd: (spots: DraftSpot[]) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('mapEditor');
  const firstZone = zones.find((z) => !z.disabled)?.id ?? '';
  const form = useForm<GeneratorValues>({
    resolver: zodResolver(generatorSchema),
    defaultValues: generatorDefaults(doc.map, firstZone),
  });
  const preview = useRowPreview(establishmentId, mapId);
  const [result, setResult] = useState<RowGeneratorResult | null>(null);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const {
    register,
    formState: { errors },
  } = form;

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      setResult(await preview.mutateAsync(toGeneratorRequest(values)));
    } catch (err) {
      setResult(null);
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }));
    }
  });

  // Labels clashing with the server map or with unsaved spots of the draft.
  const conflicts = result
    ? [...new Set([...result.conflicting_labels, ...conflictingLabels(doc, result.spots)])]
    : [];

  const field = (name: keyof GeneratorValues, label: string, numeric = true) => (
    <Field id={`gen-${name}`} label={label} error={errors[name]}>
      <Input
        {...fieldAria(`gen-${name}`, errors[name])}
        inputMode={numeric ? 'decimal' : undefined}
        {...register(name)}
      />
    </Field>
  );

  return (
    <form noValidate onSubmit={submit} className="space-y-4">
      <FormAlert state={alert} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="gen-zone_id" label={t('generator.zone')} error={errors.zone_id}>
          <Select {...fieldAria('gen-zone_id', errors.zone_id)} {...register('zone_id')}>
            {zones.map((z) => (
              <option key={z.id} value={z.id} disabled={z.disabled}>
                {z.name}
              </option>
            ))}
          </Select>
        </Field>
        {field('rows', t('generator.rows'))}
        {field('spots_per_row', t('generator.spotsPerRow'))}
        {field('origin_x', t('generator.originX'))}
        {field('origin_y', t('generator.originY'))}
        <Field id="gen-kind" label={t('fields.kind')} error={errors.kind}>
          <Select {...fieldAria('gen-kind', errors.kind)} {...register('kind')}>
            {SPOT_KINDS.map((k) => (
              <option key={k} value={k}>
                {t(`kinds.${k}`)}
              </option>
            ))}
          </Select>
        </Field>
        {field('spacing_x', t('generator.spacingX'))}
        {field('spacing_y', t('generator.spacingY'))}
        <Field id="gen-direction" label={t('generator.direction')} error={errors.direction}>
          <Select {...fieldAria('gen-direction', errors.direction)} {...register('direction')}>
            <option value="ltr">{t('generator.ltr')}</option>
            <option value="rtl">{t('generator.rtl')}</option>
          </Select>
        </Field>
        <Field
          id="gen-row_label_style"
          label={t('generator.rowLabels')}
          error={errors.row_label_style}
        >
          <Select
            {...fieldAria('gen-row_label_style', errors.row_label_style)}
            {...register('row_label_style')}
          >
            <option value="letters">{t('generator.letters')}</option>
            <option value="numbers">{t('generator.numbers')}</option>
          </Select>
        </Field>
        {field('first_row_label', t('generator.firstRow'), false)}
        {field('number_start', t('generator.numberStart'))}
        <Field id="gen-numbering" label={t('generator.numbering')} error={errors.numbering}>
          <Select {...fieldAria('gen-numbering', errors.numbering)} {...register('numbering')}>
            <option value="per_row">{t('generator.perRow')}</option>
            <option value="continuous">{t('generator.continuous')}</option>
          </Select>
        </Field>
        {field('label_format', t('generator.labelFormat'), false)}
      </div>
      <p className="text-xs text-slate-600">{t('generator.seaHint')}</p>
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="outline" onClick={onCancel}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
        <Button type="submit" variant="secondary" disabled={preview.isPending}>
          {t('generator.preview')}
        </Button>
      </div>
      {result && (
        <div role="status" className="space-y-2 rounded-md border border-slate-200 p-3 text-sm">
          <p>
            {t('generator.previewCount', {
              count: result.spots.length,
              first: result.spots[0]?.label ?? '',
              last: result.spots[result.spots.length - 1]?.label ?? '',
            })}
          </p>
          {conflicts.length > 0 && (
            <p className="text-red-700">
              {t('generator.conflicts', { labels: conflicts.slice(0, 10).join(', ') })}
            </p>
          )}
          <div className="flex justify-end">
            <Button
              disabled={conflicts.length > 0 || result.spots.length === 0}
              onClick={() => onAdd(result.spots.map(spotFromInput))}
            >
              {t('generator.add', { count: result.spots.length })}
            </Button>
          </div>
        </div>
      )}
    </form>
  );
}

/** Name, size, grid and sea side of a map (create on the list page, edit in the editor). */
export function MapSettingsForm({
  initial,
  locales,
  defaultLocale,
  pending,
  alert,
  onSubmit,
  onCancel,
  submitLabel,
}: {
  initial?: Pick<DraftMapSettings, 'name' | 'width' | 'height' | 'grid_size' | 'sea_side'>;
  locales: string[];
  defaultLocale: string;
  pending?: boolean;
  alert?: FormAlertState | null;
  onSubmit: (values: ReturnType<typeof toMapSettings>) => void | Promise<void>;
  onCancel: () => void;
  submitLabel?: string;
}) {
  const { t } = useTranslation('mapEditor');
  const form = useForm<MapSettingsValues>({
    resolver: zodResolver(mapSettingsSchema(defaultLocale)),
    defaultValues: mapSettingsDefaults(initial, locales),
  });
  const {
    register,
    formState: { errors },
  } = form;
  return (
    <form
      noValidate
      onSubmit={form.handleSubmit((v) => onSubmit(toMapSettings(v)))}
      className="space-y-4"
    >
      <FormAlert state={alert ?? null} />
      <I18nTextInput
        name="name"
        idPrefix="map"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
        register={register}
        errors={errors}
      />
      <div className="grid gap-4 sm:grid-cols-4">
        {(['width', 'height', 'grid_size'] as const).map((name) => (
          <Field key={name} id={`map-${name}`} label={t(`fields.${name}`)} error={errors[name]}>
            <Input
              {...fieldAria(`map-${name}`, errors[name])}
              inputMode="numeric"
              {...register(name)}
            />
          </Field>
        ))}
        <Field id="map-sea_side" label={t('fields.sea_side')} error={errors.sea_side}>
          <Select {...fieldAria('map-sea_side', errors.sea_side)} {...register('sea_side')}>
            {SEA_SIDES.map((s) => (
              <option key={s} value={s}>
                {t(`seaSides.${s}`)}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <FormActions onCancel={onCancel} pending={!!pending} submitLabel={submitLabel} />
    </form>
  );
}

/** Renumbers the selected spots as one row, ordered along the shoreline. */
export function RenumberRowForm({
  spots,
  onSubmit,
  onCancel,
}: {
  spots: readonly DraftSpot[];
  onSubmit: (opts: RenumberOptions) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('mapEditor');
  const form = useForm<RenumberValues>({
    resolver: zodResolver(renumberSchema),
    defaultValues: renumberDefaults(spots),
  });
  const {
    register,
    formState: { errors },
  } = form;
  const [row, start, format] = useWatch({
    control: form.control,
    name: ['row_label', 'start', 'format'],
  });
  const first = Number(start);
  const example = Number.isInteger(first)
    ? `${formatSpotLabel(format, row.trim(), first)} … ${formatSpotLabel(
        format,
        row.trim(),
        first + spots.length - 1,
      )}`
    : '';
  return (
    <form
      noValidate
      className="space-y-4"
      onSubmit={form.handleSubmit((v) =>
        onSubmit({
          rowLabel: v.row_label,
          start: Number(v.start),
          reverse: v.reverse,
          format: v.format || undefined,
        }),
      )}
    >
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="ren-row_label" label={t('fields.row_label')} error={errors.row_label}>
          <Input {...fieldAria('ren-row_label', errors.row_label)} {...register('row_label')} />
        </Field>
        <Field id="ren-start" label={t('generator.numberStart')} error={errors.start}>
          <Input
            {...fieldAria('ren-start', errors.start)}
            inputMode="numeric"
            {...register('start')}
          />
        </Field>
        <Field id="ren-format" label={t('generator.labelFormat')} error={errors.format}>
          <Input {...fieldAria('ren-format', errors.format)} {...register('format')} />
        </Field>
      </div>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          className="h-4 w-4 accent-[var(--ar-primary)]"
          {...register('reverse')}
        />
        {t('renumber.reverse')}
      </label>
      <p className="text-sm text-slate-600" aria-live="polite">
        {t('renumber.example', { count: spots.length, example })}
      </p>
      <FormActions onCancel={onCancel} pending={false} submitLabel={t('renumber.apply')} />
    </form>
  );
}
