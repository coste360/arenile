import type { Establishment } from '@arenile/shared/api';
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
} from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useState } from 'react';
import { useFieldArray, useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ratePeriods, seasons, type RatePeriod, type Season } from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { useEstablishmentSection } from '../api/useEstablishmentSection';
import { Badge, ConfigPage, EmptyState, QueryState } from '../components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../components/dialogs';
import { Field, FieldError, Select } from '../components/fields';
import { FormAlert, SavedNotice } from '../components/FormAlert';
import { I18nTextInput } from '../components/I18nTextInput';
import { errorAt, fieldAria } from '../lib/aria';
import { formatDate, formatDayMonth, monthStarts } from '../lib/dates';
import { localize } from '../lib/i18nText';
import {
  PERIOD_PRESETS,
  SEASON_STATUSES,
  holidaysAliases,
  holidaysPatch,
  holidaysSchema,
  ratePeriodSchema,
  seasonSchema,
  timelineSegments,
  toHolidaysValues,
  toRatePeriodInput,
  toRatePeriodValues,
  toSeasonInput,
  toSeasonValues,
  uncoveredDays,
} from './seasons.schema';

const STATUS_TONE = { draft: 'slate', active: 'green', closed: 'amber' } as const;

/** Picks the season shown by default: the active one, else the most recent. */
function defaultSeason(list: Season[]): Season | undefined {
  return list.find((s) => s.status === 'active') ?? [...list].sort((a, b) => b.year - a.year)[0];
}

/** Configurazione → Stagioni e periodi (spec 6.1, 8.1). */
export function SeasonsPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const search = useSearch({ strict: false }) as { seasonId?: string };
  const navigate = useNavigate();
  const list = seasons.useList(ctx.establishmentId);
  const remove = seasons.useRemove(ctx.establishmentId);
  const editor = useEditor<Season>();
  const items = [...(list.data ?? [])].sort((a, b) => b.year - a.year);
  const selected = items.find((s) => s.id === search.seasonId) ?? defaultSeason(items);
  const readOnly = !ctx.canConfigure;
  const uiLocale = ctx.uiLocale;

  const select = (seasonId: string) =>
    void navigate({ to: '.', search: { seasonId } as never, replace: true });

  return (
    <ConfigPage
      title={t('seasons.title')}
      description={t('seasons.description')}
      readOnly={readOnly}
      actions={!readOnly && <Button onClick={editor.create}>{t('seasons.add')}</Button>}
    >
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('seasons.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[34rem] text-sm">
            <caption className="sr-only">{t('seasons.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('seasons.fields.year')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('seasons.dates')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('seasons.fields.status')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  <span className="sr-only">{t('actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((s) => (
                <tr
                  key={s.id}
                  className={`border-t border-slate-100 ${s.id === selected?.id ? 'bg-sky-50' : ''}`}
                  aria-selected={s.id === selected?.id}
                >
                  <th scope="row" className="px-4 py-2 text-left font-medium">
                    <button
                      type="button"
                      className="underline-offset-2 hover:underline"
                      onClick={() => select(s.id)}
                      aria-pressed={s.id === selected?.id}
                    >
                      {s.year}
                    </button>
                  </th>
                  <td className="px-4 py-2">
                    {formatDate(s.opens_on, uiLocale)} – {formatDate(s.closes_on, uiLocale)}
                  </td>
                  <td className="px-4 py-2">
                    <Badge tone={STATUS_TONE[s.status]}>{t(`seasons.status.${s.status}`)}</Badge>
                  </td>
                  <td className="px-4 py-2 text-right">
                    {!readOnly && (
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => editor.edit(s)}
                          aria-label={t('seasons.editAria', { year: s.year })}
                        >
                          {t('common.actions.edit', { ns: 'translation' })}
                        </Button>
                        <ConfirmDelete
                          label={t('seasons.deleteAria', { year: s.year })}
                          message={t('seasons.deleteConfirm', { year: s.year })}
                          onConfirm={() => remove.mutateAsync({ id: s.id, version: s.version })}
                        />
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      {selected && (
        <RatePeriodsCard
          key={selected.id}
          season={selected}
          establishmentId={ctx.establishmentId}
          readOnly={readOnly}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          uiLocale={uiLocale}
        />
      )}
      {ctx.establishment && (
        <LocalHolidaysCard
          key={ctx.establishment.id}
          establishment={ctx.establishment}
          readOnly={readOnly}
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('seasons.editTitle', { year: editor.item.year }) : t('seasons.add')}
      >
        <SeasonForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          item={editor.item}
          onDone={(s) => {
            editor.close();
            select(s.id);
          }}
          onCancel={editor.close}
        />
      </FormDialog>
    </ConfigPage>
  );
}

function SeasonForm({
  establishmentId,
  item,
  onDone,
  onCancel,
}: {
  establishmentId: string;
  item?: Season;
  onDone: (s: Season) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(seasonSchema),
    defaultValues: toSeasonValues(item),
  });
  const create = seasons.useCreate(establishmentId);
  const update = seasons.useUpdate(establishmentId);
  const s = useCrudSubmit({ form, item, create, update, toInput: toSeasonInput, onDone });
  const {
    register,
    formState: { errors },
  } = form;
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onCancel} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="season-year" label={t('seasons.fields.year')} error={errors.year}>
          <Input
            {...fieldAria('season-year', errors.year)}
            inputMode="numeric"
            {...register('year')}
          />
        </Field>
        <Field id="season-status" label={t('seasons.fields.status')} error={errors.status}>
          <Select {...fieldAria('season-status', errors.status)} {...register('status')}>
            {SEASON_STATUSES.map((st) => (
              <option key={st} value={st}>
                {t(`seasons.status.${st}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="season-opens" label={t('seasons.fields.opens_on')} error={errors.opens_on}>
          <Input
            {...fieldAria('season-opens', errors.opens_on)}
            type="date"
            {...register('opens_on')}
          />
        </Field>
        <Field id="season-closes" label={t('seasons.fields.closes_on')} error={errors.closes_on}>
          <Input
            {...fieldAria('season-closes', errors.closes_on)}
            type="date"
            {...register('closes_on')}
          />
        </Field>
      </div>
      <FormActions onCancel={onCancel} pending={s.pending} />
    </form>
  );
}

// ---- Rate periods -----------------------------------------------------------------

interface RatePeriodsCardProps {
  season: Season;
  establishmentId: string;
  readOnly: boolean;
  locales: string[];
  defaultLocale: string;
  uiLocale: string;
}

function RatePeriodsCard({
  season,
  establishmentId,
  readOnly,
  locales,
  defaultLocale,
  uiLocale,
}: RatePeriodsCardProps) {
  const { t } = useTranslation('configuration');
  const list = ratePeriods.useList(establishmentId, { season_id: season.id });
  const remove = ratePeriods.useRemove(establishmentId);
  const editor = useEditor<RatePeriod>();
  const items = [...(list.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const segments = timelineSegments(season, items);
  const gaps = list.data ? uncoveredDays(season, items) : 0;
  const ticks = monthStarts(season.opens_on, season.closes_on);
  const totalDays =
    (Date.parse(`${season.closes_on}T00:00:00Z`) - Date.parse(`${season.opens_on}T00:00:00Z`)) /
      86_400_000 +
    1;

  return (
    <Card role="region" aria-labelledby="periods-title">
      <CardHeader className="flex-row flex-wrap items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle id="periods-title">{t('periods.title', { year: season.year })}</CardTitle>
          <CardDescription>{t('periods.description')}</CardDescription>
        </div>
        {!readOnly && (
          <Button size="sm" onClick={editor.create}>
            {t('periods.add')}
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <QueryState
          isLoading={list.isPending}
          error={list.error}
          onRetry={() => void list.refetch()}
        />
        {list.data && (
          <figure className="space-y-1">
            <figcaption className="sr-only">{t('periods.timeline')}</figcaption>
            <div
              className="relative h-10 overflow-hidden rounded-md border border-slate-200 bg-[repeating-linear-gradient(45deg,#f8fafc,#f8fafc_6px,#e2e8f0_6px,#e2e8f0_12px)]"
              role="list"
              aria-label={t('periods.timeline')}
            >
              {segments.map((seg) => (
                <div
                  key={`${seg.periodId}-${seg.from}`}
                  role="listitem"
                  title={`${seg.code}: ${formatDayMonth(seg.from, uiLocale)} – ${formatDayMonth(seg.to, uiLocale)}`}
                  aria-label={t('periods.segmentAria', {
                    code: seg.code,
                    from: formatDate(seg.from, uiLocale),
                    to: formatDate(seg.to, uiLocale),
                  })}
                  className="absolute inset-y-0 flex items-center justify-center overflow-hidden border-r border-white text-xs font-semibold text-slate-900"
                  style={{
                    left: `${seg.left}%`,
                    width: `${seg.width}%`,
                    backgroundColor: seg.color,
                  }}
                >
                  <span className="truncate px-1">{seg.code}</span>
                </div>
              ))}
            </div>
            <div className="relative h-4 text-[10px] text-slate-500" aria-hidden="true">
              {ticks.map((d) => {
                const offset =
                  ((Date.parse(`${d}T00:00:00Z`) - Date.parse(`${season.opens_on}T00:00:00Z`)) /
                    86_400_000 /
                    totalDays) *
                  100;
                return (
                  <span key={d} className="absolute" style={{ left: `${offset}%` }}>
                    {formatDayMonth(d, uiLocale)}
                  </span>
                );
              })}
            </div>
            {gaps > 0 && (
              <p role="status" className="text-sm text-amber-800">
                {t('periods.uncovered', { count: gaps })}
              </p>
            )}
          </figure>
        )}
        {list.data && items.length === 0 && <EmptyState>{t('periods.empty')}</EmptyState>}
        {items.length > 0 && (
          <ul className="divide-y divide-slate-100">
            {items.map((p) => (
              <li key={p.id} className="flex flex-wrap items-center gap-3 py-2">
                <span
                  aria-hidden="true"
                  className="h-4 w-4 shrink-0 rounded"
                  style={{ backgroundColor: p.color }}
                />
                <span className="font-mono text-xs text-slate-600">{p.code}</span>
                <span className="font-medium">{localize(p.name, uiLocale, defaultLocale)}</span>
                <span className="text-sm text-slate-600">
                  {p.ranges
                    .map(
                      (r) =>
                        `${formatDayMonth(r.from, uiLocale)} – ${formatDayMonth(r.to, uiLocale)}`,
                    )
                    .join(', ')}
                </span>
                {!readOnly && (
                  <span className="ml-auto flex gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => editor.edit(p)}
                      aria-label={t('periods.editAria', { code: p.code })}
                    >
                      {t('common.actions.edit', { ns: 'translation' })}
                    </Button>
                    <ConfirmDelete
                      label={t('periods.deleteAria', { code: p.code })}
                      message={t('periods.deleteConfirm', { code: p.code })}
                      onConfirm={() => remove.mutateAsync({ id: p.id, version: p.version })}
                    />
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('periods.editTitle', { code: editor.item.code }) : t('periods.add')}
      >
        <RatePeriodForm
          key={editor.key}
          establishmentId={establishmentId}
          season={season}
          item={editor.item}
          others={items.filter((p) => p.id !== editor.item?.id)}
          locales={locales}
          defaultLocale={defaultLocale}
          onDone={editor.close}
          onCancel={editor.close}
        />
      </FormDialog>
    </Card>
  );
}

function RatePeriodForm({
  establishmentId,
  season,
  item,
  others,
  locales,
  defaultLocale,
  onDone,
  onCancel,
}: {
  establishmentId: string;
  season: Season;
  item?: RatePeriod;
  others: RatePeriod[];
  locales: string[];
  defaultLocale: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t, i18n } = useTranslation('configuration');
  const schema = ratePeriodSchema({ defaultLocale, season, others });
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: toRatePeriodValues(item, locales, season, others.length),
  });
  const ranges = useFieldArray({ control: form.control, name: 'ranges' });
  const create = ratePeriods.useCreate(establishmentId);
  const update = ratePeriods.useUpdate(establishmentId);
  const s = useCrudSubmit({
    form,
    item,
    create,
    update,
    toInput: (v) => toRatePeriodInput(v, season.id),
    onDone,
  });
  const {
    register,
    setValue,
    formState: { errors },
  } = form;
  const color = useWatch({ control: form.control, name: 'color' });
  const codeField = register('code');
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onCancel} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field
          id="rp-code"
          label={t('periods.fields.code')}
          error={errors.code}
          hint={t('periods.codeHint')}
        >
          <Input
            {...fieldAria('rp-code', errors.code, true)}
            list="rp-code-presets"
            {...codeField}
            onChange={(e) => {
              void codeField.onChange(e);
              const preset = PERIOD_PRESETS.find((p) => p.code === e.target.value);
              if (preset && !item) setValue('color', preset.color);
            }}
          />
          <datalist id="rp-code-presets">
            {PERIOD_PRESETS.map((p) => (
              <option key={p.code} value={p.code}>
                {t(`periods.presets.${p.code}`)}
              </option>
            ))}
          </datalist>
        </Field>
        <Field id="rp-color" label={t('periods.fields.color')} error={errors.color}>
          <div className="flex items-center gap-2">
            <input
              type="color"
              aria-label={t('theme.picker', { field: t('periods.fields.color') })}
              value={/^#[0-9a-f]{6}$/i.test(color) ? color : '#000000'}
              onChange={(e) => setValue('color', e.target.value, { shouldDirty: true })}
              className="h-10 w-12 cursor-pointer rounded border border-slate-300 bg-white p-1"
            />
            <Input
              {...fieldAria('rp-color', errors.color)}
              className="font-mono"
              maxLength={7}
              {...register('color')}
            />
          </div>
        </Field>
        <Field id="rp-sort" label={t('fields.sort_order')} error={errors.sort_order}>
          <Input
            {...fieldAria('rp-sort', errors.sort_order)}
            inputMode="numeric"
            {...register('sort_order')}
          />
        </Field>
      </div>
      <I18nTextInput
        name="name"
        idPrefix="rp"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
        register={register}
        errors={errors}
      />
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-slate-800">{t('periods.fields.ranges')}</legend>
        <p className="text-xs text-slate-600">
          {t('periods.rangesHint', {
            from: formatDate(season.opens_on, i18n.language || 'it'),
            to: formatDate(season.closes_on, i18n.language || 'it'),
          })}
        </p>
        {ranges.fields.map((f, i) => {
          const fromErr = errorAt(errors, `ranges.${i}.from`);
          const toErr = errorAt(errors, `ranges.${i}.to`);
          return (
            <div key={f.id} className="flex flex-wrap items-end gap-2">
              <div className="space-y-1">
                <label htmlFor={`rp-r${i}-from`} className="text-xs text-slate-600">
                  {t('periods.from')}
                </label>
                <Input
                  {...fieldAria(`rp-r${i}-from`, fromErr)}
                  type="date"
                  min={season.opens_on}
                  max={season.closes_on}
                  {...register(`ranges.${i}.from`)}
                />
              </div>
              <div className="space-y-1">
                <label htmlFor={`rp-r${i}-to`} className="text-xs text-slate-600">
                  {t('periods.to')}
                </label>
                <Input
                  {...fieldAria(`rp-r${i}-to`, toErr)}
                  type="date"
                  min={season.opens_on}
                  max={season.closes_on}
                  {...register(`ranges.${i}.to`)}
                />
              </div>
              {ranges.fields.length > 1 && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => ranges.remove(i)}
                  aria-label={t('periods.removeRange', { index: i + 1 })}
                >
                  {t('common.actions.delete', { ns: 'translation' })}
                </Button>
              )}
              <div className="w-full">
                <FieldError id={`rp-r${i}-from`} error={fromErr} />
                <FieldError id={`rp-r${i}-to`} error={toErr} />
              </div>
            </div>
          );
        })}
        <FieldError id="rp-ranges" error={errorAt(errors, 'ranges')} />
        <Button
          variant="outline"
          size="sm"
          onClick={() => ranges.append({ from: season.opens_on, to: season.opens_on })}
        >
          {t('periods.addRange')}
        </Button>
      </fieldset>
      <FormActions onCancel={onCancel} pending={s.pending} />
    </form>
  );
}

// ---- Local holidays -----------------------------------------------------------------

function LocalHolidaysCard({
  establishment,
  readOnly,
}: {
  establishment: Establishment;
  readOnly: boolean;
}) {
  const { t } = useTranslation('configuration');
  const [version, setVersion] = useState(establishment.version);
  const form = useForm({
    resolver: zodResolver(holidaysSchema),
    defaultValues: toHolidaysValues(establishment),
  });
  const dates = useFieldArray({ control: form.control, name: 'local_holidays' });
  const recurring = useFieldArray({ control: form.control, name: 'recurring_local_holidays' });
  const s = useEstablishmentSection({
    establishmentId: establishment.id,
    form,
    version,
    onVersion: setVersion,
    toPatch: holidaysPatch,
    toValues: toHolidaysValues,
    aliases: holidaysAliases,
  });
  const {
    register,
    formState: { errors },
  } = form;
  return (
    <Card role="region" aria-labelledby="holidays-title">
      <CardHeader>
        <CardTitle id="holidays-title">{t('holidays.title')}</CardTitle>
        <CardDescription>{t('holidays.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <form noValidate onSubmit={s.submit} className="space-y-4">
          <FormAlert state={s.alert} onReload={s.reload} />
          <div className="grid gap-6 sm:grid-cols-2">
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium text-slate-800">{t('holidays.dates')}</legend>
              {dates.fields.map((f, i) => {
                const err = errorAt(errors, `local_holidays.${i}.value`);
                return (
                  <div key={f.id} className="flex items-center gap-2">
                    <Input
                      {...fieldAria(`hol-d${i}`, err)}
                      aria-label={t('holidays.dateAria', { index: i + 1 })}
                      type="date"
                      disabled={readOnly}
                      {...register(`local_holidays.${i}.value`)}
                    />
                    {!readOnly && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => dates.remove(i)}
                        aria-label={t('holidays.removeAria', { index: i + 1 })}
                      >
                        ×
                      </Button>
                    )}
                    <FieldError id={`hol-d${i}`} error={err} />
                  </div>
                );
              })}
              {!readOnly && (
                <Button variant="outline" size="sm" onClick={() => dates.append({ value: '' })}>
                  {t('holidays.addDate')}
                </Button>
              )}
            </fieldset>
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium text-slate-800">
                {t('holidays.recurring')}
              </legend>
              {recurring.fields.map((f, i) => {
                const err = errorAt(errors, `recurring_local_holidays.${i}.value`);
                return (
                  <div key={f.id} className="flex items-center gap-2">
                    <Input
                      {...fieldAria(`hol-r${i}`, err)}
                      aria-label={t('holidays.recurringAria', { index: i + 1 })}
                      placeholder="MM-GG"
                      maxLength={5}
                      disabled={readOnly}
                      {...register(`recurring_local_holidays.${i}.value`)}
                    />
                    {!readOnly && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => recurring.remove(i)}
                        aria-label={t('holidays.removeRecurringAria', { index: i + 1 })}
                      >
                        ×
                      </Button>
                    )}
                    <FieldError id={`hol-r${i}`} error={err} />
                  </div>
                );
              })}
              {!readOnly && (
                <Button variant="outline" size="sm" onClick={() => recurring.append({ value: '' })}>
                  {t('holidays.addRecurring')}
                </Button>
              )}
              <p className="text-xs text-slate-600">{t('holidays.recurringHint')}</p>
            </fieldset>
          </div>
          {!readOnly && (
            <div className="flex items-center gap-3">
              <Button type="submit" disabled={s.pending}>
                {s.pending
                  ? t('common.actions.saving', { ns: 'translation' })
                  : t('common.actions.save', { ns: 'translation' })}
              </Button>
              <SavedNotice show={s.saved} />
            </div>
          )}
        </form>
      </CardContent>
    </Card>
  );
}
