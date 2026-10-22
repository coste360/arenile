import type { Establishment } from '@arenile/shared/api';
import { readableTextColor } from '@arenile/shared/theme';
import { Button } from '@arenile/shared/ui/button';
import { Calendar, CalendarDayButton } from '@arenile/shared/ui/calendar';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { FieldDescription, FieldLegend, FieldSet } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import { cn } from '@arenile/shared/lib/utils';
import { dateFnsLocale, formatIsoDate, isoToDate } from '@arenile/shared/lib/dates';
import {
  DataTable,
  DataTableColumnHeader,
  DatePicker,
  EmptyState,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate, useSearch } from '@tanstack/react-router';
import {
  CalendarDaysIcon,
  CalendarRangeIcon,
  ChartGanttIcon,
  PlusIcon,
  SunIcon,
  TriangleAlertIcon,
  XIcon,
} from 'lucide-react';
import { createContext, useContext, useState, type ComponentProps } from 'react';
import { useController, useFieldArray, useForm, useWatch, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ratePeriods, seasons, type RatePeriod, type Season } from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { useEstablishmentSection } from '../api/useEstablishmentSection';
import { ConfigPage, QueryState } from '../components/ConfigPage';
import { FormActions, FormDialog, RowActions } from '../components/dialogs';
import {
  DateRangeField,
  Field,
  FieldError,
  I18nTextField,
  SelectField,
} from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { errorAt, fieldAria } from '../lib/aria';
import { monthStarts, daysBetween, addDays } from '../lib/dates';
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
  type RatePeriodValues,
} from './seasons.schema';

const STATUS_TONE = { draft: 'neutral', active: 'success', closed: 'warning' } as const;

/** Picks the season shown by default: the active one, else the most recent. */
function defaultSeason(list: Season[]): Season | undefined {
  return list.find((s) => s.status === 'active') ?? [...list].sort((a, b) => b.year - a.year)[0];
}

const range = (from: string, to: string) => `${formatIsoDate(from)} – ${formatIsoDate(to)}`;

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

  const select = (seasonId: string) =>
    void navigate({ to: '.', search: { seasonId } as never, replace: true });

  const columns: ColumnDef<Season>[] = [
    {
      accessorKey: 'year',
      meta: { label: t('seasons.fields.year') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('seasons.fields.year')} />
      ),
      cell: ({ row }) => {
        const s = row.original;
        const active = s.id === selected?.id;
        return (
          <Button
            variant={active ? 'secondary' : 'ghost'}
            size="sm"
            className="font-semibold tabular-nums"
            onClick={() => select(s.id)}
            aria-pressed={active}
          >
            {s.year}
          </Button>
        );
      },
    },
    {
      id: 'dates',
      meta: { label: t('seasons.dates') },
      header: t('seasons.dates'),
      cell: ({ row }) => (
        <span className="tabular-nums">{range(row.original.opens_on, row.original.closes_on)}</span>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('seasons.fields.status') },
      header: t('seasons.fields.status'),
      cell: ({ row }) => (
        <StatusBadge tone={STATUS_TONE[row.original.status]}>
          {t(`seasons.status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) =>
        readOnly ? null : (
          <RowActions
            editLabel={t('seasons.editAria', { year: row.original.year })}
            deleteLabel={t('seasons.deleteAria', { year: row.original.year })}
            deleteMessage={t('seasons.deleteConfirm', { year: row.original.year })}
            onEdit={() => editor.edit(row.original)}
            onDelete={() =>
              remove.mutateAsync({ id: row.original.id, version: row.original.version })
            }
          />
        ),
    },
  ];

  return (
    <ConfigPage
      title={t('seasons.title')}
      description={t('seasons.description')}
      readOnly={readOnly}
      actions={
        !readOnly && (
          <Button onClick={editor.create}>
            <PlusIcon aria-hidden="true" />
            {t('seasons.add')}
          </Button>
        )
      }
    >
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('seasons.title')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(s) => s.id}
          pageSize={0}
          columnToggle={false}
          empty={
            <EmptyState icon={CalendarRangeIcon} title={t('seasons.empty')} bordered={false} />
          }
        />
      )}
      {selected && (
        <RatePeriodsCard
          key={selected.id}
          season={selected}
          establishmentId={ctx.establishmentId}
          readOnly={readOnly}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          uiLocale={ctx.uiLocale}
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
    control,
    formState: { errors },
  } = form;
  return (
    <form noValidate onSubmit={s.submit} className="space-y-5">
      <FormAlert state={s.alert} onReload={onCancel} />
      <div className="grid gap-5 sm:grid-cols-2">
        <Field id="season-year" label={t('seasons.fields.year')} error={errors.year}>
          <Input
            {...fieldAria('season-year', errors.year)}
            inputMode="numeric"
            {...register('year')}
          />
        </Field>
        <SelectField
          control={control}
          name="status"
          id="season-status"
          label={t('seasons.fields.status')}
          options={SEASON_STATUSES.map((st) => ({ value: st, label: t(`seasons.status.${st}`) }))}
        />
      </div>
      <Field id="season-dates" label={t('seasons.dates')}>
        <DateRangeField
          control={control}
          fromName="opens_on"
          toName="closes_on"
          id="season-dates"
          label={t('seasons.dates')}
        />
      </Field>
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

/** Colour swatch drawn in SVG (period colours are data, not theme tokens). */
function Swatch({ color, className }: { color: string; className?: string }) {
  return (
    <svg viewBox="0 0 12 12" aria-hidden="true" className={cn('size-3 shrink-0', className)}>
      <rect width="12" height="12" rx="3" fill={color} />
    </svg>
  );
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
  const gaps = list.data ? uncoveredDays(season, items) : 0;
  const name = (p: RatePeriod) => localize(p.name, uiLocale, defaultLocale) || p.code;

  const columns: ColumnDef<RatePeriod>[] = [
    {
      accessorKey: 'code',
      meta: { label: t('periods.fields.code') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('periods.fields.code')} />
      ),
      cell: ({ row }) => (
        <span className="flex items-center gap-2">
          <Swatch color={row.original.color} />
          <span className="font-mono text-xs">{row.original.code}</span>
        </span>
      ),
    },
    {
      id: 'name',
      meta: { label: t('fields.name') },
      accessorFn: (p) => name(p),
      header: t('fields.name'),
      cell: ({ row }) => <span className="font-medium">{name(row.original)}</span>,
    },
    {
      id: 'ranges',
      meta: { label: t('periods.fields.ranges') },
      header: t('periods.fields.ranges'),
      cell: ({ row }) => (
        <span className="text-muted-foreground tabular-nums">
          {row.original.ranges.map((r) => range(r.from, r.to)).join(', ')}
        </span>
      ),
    },
    {
      id: 'days',
      meta: { label: t('periods.days') },
      header: () => <span className="block text-right">{t('periods.days')}</span>,
      cell: ({ row }) => (
        <span className="block text-right tabular-nums">
          {row.original.ranges.reduce((n, r) => n + daysBetween(r.from, r.to) + 1, 0)}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) =>
        readOnly ? null : (
          <RowActions
            editLabel={t('periods.editAria', { code: row.original.code })}
            deleteLabel={t('periods.deleteAria', { code: row.original.code })}
            deleteMessage={t('periods.deleteConfirm', { code: row.original.code })}
            onEdit={() => editor.edit(row.original)}
            onDelete={() =>
              remove.mutateAsync({ id: row.original.id, version: row.original.version })
            }
          />
        ),
    },
  ];

  return (
    <Card role="region" aria-labelledby="periods-title">
      <CardHeader>
        <CardTitle id="periods-title">{t('periods.title', { year: season.year })}</CardTitle>
        <CardDescription>{t('periods.description')}</CardDescription>
        {!readOnly && (
          <CardAction>
            <Button size="sm" onClick={editor.create}>
              <PlusIcon aria-hidden="true" />
              {t('periods.add')}
            </Button>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="space-y-5">
        <QueryState
          isLoading={list.isPending}
          error={list.error}
          onRetry={() => void list.refetch()}
        />
        {list.data && (
          <Tabs defaultValue="timeline" className="gap-4">
            <TabsList aria-label={t('periods.views')}>
              <TabsTrigger value="timeline">
                <ChartGanttIcon aria-hidden="true" />
                {t('periods.viewTimeline')}
              </TabsTrigger>
              <TabsTrigger value="calendar">
                <CalendarDaysIcon aria-hidden="true" />
                {t('periods.viewCalendar')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="timeline">
              <PeriodTimeline season={season} periods={items} />
            </TabsContent>
            <TabsContent value="calendar">
              <PeriodCalendar season={season} periods={items} name={name} />
            </TabsContent>
          </Tabs>
        )}
        {gaps > 0 && (
          <p role="status" className="flex items-center gap-2 text-sm">
            <TriangleAlertIcon aria-hidden="true" className="size-4 text-warning" />
            {t('periods.uncovered', { count: gaps })}
          </p>
        )}
        {list.data && (
          <DataTable
            caption={t('periods.title', { year: season.year })}
            columns={columns}
            data={items}
            getRowId={(p) => p.id}
            pageSize={0}
            columnToggle={false}
            empty={<EmptyState icon={ChartGanttIcon} title={t('periods.empty')} bordered={false} />}
          />
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

/**
 * Season bar with one coloured segment per date range (SVG: positions are
 * percentages of the season, colours come from the periods), month ticks
 * underneath. Uncovered days show the muted background.
 */
function PeriodTimeline({ season, periods }: { season: Season; periods: RatePeriod[] }) {
  const { t, i18n } = useTranslation('configuration');
  const segments = timelineSegments(season, periods);
  const label = (seg: { periodId: string; code: string; width: number }) => {
    const p = periods.find((x) => x.id === seg.periodId);
    const name = p ? localize(p.name, i18n.language || 'it', 'it') : '';
    return name && seg.width > 12 ? name : seg.code;
  };
  const total = daysBetween(season.opens_on, season.closes_on) + 1;
  const ticks = monthStarts(season.opens_on, season.closes_on);
  const monthFmt = new Intl.DateTimeFormat(i18n.language || 'it', {
    month: 'short',
    timeZone: 'UTC',
  });
  return (
    <figure className="space-y-1">
      <figcaption className="sr-only">{t('periods.timeline')}</figcaption>
      <svg
        role="list"
        aria-label={t('periods.timeline')}
        width="100%"
        height="44"
        className="overflow-hidden rounded-lg border"
      >
        <rect width="100%" height="100%" className="fill-muted" />
        {segments.map((seg) => (
          <g
            key={`${seg.periodId}-${seg.from}`}
            role="listitem"
            aria-label={t('periods.segmentAria', {
              code: seg.code,
              from: formatIsoDate(seg.from),
              to: formatIsoDate(seg.to),
            })}
          >
            <title>{`${seg.code}: ${range(seg.from, seg.to)}`}</title>
            <rect
              x={`${seg.left}%`}
              width={`${seg.width}%`}
              height="100%"
              fill={seg.color}
              className="stroke-background"
              strokeWidth="1"
            />
            {seg.width > 5 && (
              <text
                x={`${seg.left + seg.width / 2}%`}
                y="50%"
                dominantBaseline="central"
                textAnchor="middle"
                fill={readableTextColor(seg.color)}
                className="text-xs font-semibold"
              >
                {label(seg)}
              </text>
            )}
          </g>
        ))}
      </svg>
      <svg width="100%" height="16" aria-hidden="true">
        {ticks.map((d) => {
          const offset = (daysBetween(season.opens_on, d) / total) * 100;
          return (
            <g key={d}>
              <line x1={`${offset}%`} x2={`${offset}%`} y1="0" y2="4" className="stroke-border" />
              <text x={`${offset}%`} y="14" className="fill-muted-foreground text-[10px]">
                {monthFmt.format(new Date(`${d}T00:00:00Z`))}
              </text>
            </g>
          );
        })}
      </svg>
    </figure>
  );
}

/** Day → period colour, read by the calendar day buttons. */
const PeriodColors = createContext<Map<string, string>>(new Map());

/** shadcn calendar day with a dot in the colour of its rate period. */
function PeriodDayButton(props: ComponentProps<typeof CalendarDayButton>) {
  const color = useContext(PeriodColors).get(toIsoLocal(props.day.date));
  return (
    <CalendarDayButton {...props}>
      {props.children}
      {color && (
        <svg viewBox="0 0 6 6" aria-hidden="true" className="size-1.5">
          <circle cx="3" cy="3" r="3" fill={color} />
        </svg>
      )}
    </CalendarDayButton>
  );
}

/** Month calendar of the season: every day carries a dot in its period colour. */
function PeriodCalendar({
  season,
  periods,
  name,
}: {
  season: Season;
  periods: RatePeriod[];
  name: (p: RatePeriod) => string;
}) {
  const { i18n } = useTranslation();
  const colorOf = new Map<string, string>();
  for (const p of periods) {
    for (const r of p.ranges) {
      for (let d = r.from; d <= r.to; d = addDays(d, 1)) colorOf.set(d, p.color);
    }
  }
  const start = isoToDate(season.opens_on);
  const end = isoToDate(season.closes_on);
  const months = Math.min(ticksBetween(season.opens_on, season.closes_on), 12);
  return (
    <div className="space-y-3">
      <div className="overflow-x-auto rounded-lg border">
        <PeriodColors.Provider value={colorOf}>
          <Calendar
            defaultMonth={start}
            startMonth={start}
            endMonth={end}
            numberOfMonths={months}
            locale={dateFnsLocale(i18n.language)}
            weekStartsOn={1}
            disabled={[...(start ? [{ before: start }] : []), ...(end ? [{ after: end }] : [])]}
            components={{ DayButton: PeriodDayButton }}
            className="w-full [&_.rdp-months]:flex-wrap"
          />
        </PeriodColors.Provider>
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
        {periods.map((p) => (
          <li key={p.id} className="flex items-center gap-1.5">
            <Swatch color={p.color} />
            {name(p)}
          </li>
        ))}
      </ul>
    </div>
  );
}

function ticksBetween(from: string, to: string): number {
  const [fy, fm] = from.split('-').map(Number) as [number, number];
  const [ty, tm] = to.split('-').map(Number) as [number, number];
  return (ty - fy) * 12 + (tm - fm) + 1;
}

function toIsoLocal(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function ColorField({
  control,
  id,
  label,
  error,
}: {
  control: Control<RatePeriodValues>;
  id: string;
  label: string;
  error: ReturnType<typeof errorAt>;
}) {
  const { t } = useTranslation('configuration');
  const {
    field: { ref, ...field },
  } = useController({ control, name: 'color' });
  const color = String(field.value ?? '');
  return (
    <Field id={id} label={label} error={error}>
      <InputGroup>
        <InputGroupAddon>
          <input
            type="color"
            aria-label={t('theme.picker', { field: label })}
            value={/^#[0-9a-f]{6}$/i.test(color) ? color : '#000000'}
            onChange={(e) => field.onChange(e.target.value)}
            className="size-6 cursor-pointer rounded-md border-0 bg-transparent p-0"
          />
        </InputGroupAddon>
        <InputGroupInput
          {...fieldAria(id, error)}
          ref={ref}
          className="font-mono uppercase"
          maxLength={7}
          value={color}
          onChange={field.onChange}
          onBlur={field.onBlur}
        />
      </InputGroup>
    </Field>
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
  const { t } = useTranslation('configuration');
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
    control,
    formState: { errors },
  } = form;
  const code = useWatch({ control, name: 'code' });
  const rangeValues = useWatch({ control, name: 'ranges' });
  return (
    <form noValidate onSubmit={s.submit} className="space-y-5">
      <FormAlert state={s.alert} onReload={onCancel} />
      <div className="grid gap-5 sm:grid-cols-3">
        <Field
          id="rp-code"
          label={t('periods.fields.code')}
          error={errors.code}
          hint={t('periods.codeHint')}
        >
          <Input {...fieldAria('rp-code', errors.code, true)} {...register('code')} />
        </Field>
        <ColorField
          control={control}
          id="rp-color"
          label={t('periods.fields.color')}
          error={errors.color}
        />
        <Field id="rp-sort" label={t('fields.sort_order')} error={errors.sort_order}>
          <Input
            {...fieldAria('rp-sort', errors.sort_order)}
            inputMode="numeric"
            {...register('sort_order')}
          />
        </Field>
      </div>
      {!item && (
        <div
          className="flex flex-wrap items-center gap-2"
          role="group"
          aria-label={t('periods.presetsLabel')}
        >
          <span className="text-sm text-muted-foreground">{t('periods.presetsLabel')}</span>
          {PERIOD_PRESETS.map((p) => (
            <Button
              key={p.code}
              size="sm"
              variant={code === p.code ? 'secondary' : 'outline'}
              aria-pressed={code === p.code}
              onClick={() => {
                setValue('code', p.code, { shouldDirty: true, shouldValidate: true });
                setValue('color', p.color, { shouldDirty: true });
              }}
            >
              <Swatch color={p.color} />
              {t(`periods.presets.${p.code}`)}
            </Button>
          ))}
        </div>
      )}
      <I18nTextField
        control={control}
        name="name"
        id="rp-name"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
      />
      <FieldSet className="gap-3">
        <FieldLegend variant="label">{t('periods.fields.ranges')}</FieldLegend>
        <FieldDescription>
          {t('periods.rangesHint', {
            from: formatIsoDate(season.opens_on),
            to: formatIsoDate(season.closes_on),
          })}
        </FieldDescription>
        {ranges.fields.map((f, i) => {
          const value = rangeValues[i];
          const label = t('periods.rangeAria', { index: i + 1 });
          return (
            <div key={f.id} className="flex flex-wrap items-start gap-2">
              <DateRangeField
                control={control}
                fromName={`ranges.${i}.from`}
                toName={`ranges.${i}.to`}
                id={`rp-r${i}`}
                label={
                  value?.from ? `${label}: ${range(value.from, value.to || value.from)}` : label
                }
                min={season.opens_on}
                max={season.closes_on}
              />
              {ranges.fields.length > 1 && (
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => ranges.remove(i)}
                  aria-label={t('periods.removeRange', { index: i + 1 })}
                >
                  <XIcon aria-hidden="true" />
                </Button>
              )}
            </div>
          );
        })}
        <FieldError id="rp-ranges" error={errorAt(errors, 'ranges')} />
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => ranges.append({ from: season.opens_on, to: season.opens_on })}
          >
            <PlusIcon aria-hidden="true" />
            {t('periods.addRange')}
          </Button>
        </div>
      </FieldSet>
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
    control,
    formState: { errors },
  } = form;
  return (
    <form noValidate onSubmit={s.submit}>
      <Card role="region" aria-labelledby="holidays-title">
        <CardHeader>
          <CardTitle id="holidays-title" className="flex items-center gap-2">
            <SunIcon aria-hidden="true" className="size-4 text-muted-foreground" />
            {t('holidays.title')}
          </CardTitle>
          <CardDescription>{t('holidays.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <FormAlert state={s.alert} onReload={s.reload} />
          <div className="grid gap-8 sm:grid-cols-2">
            <FieldSet className="gap-3">
              <FieldLegend variant="label">{t('holidays.dates')}</FieldLegend>
              {dates.fields.map((f, i) => (
                <HolidayDate
                  key={f.id}
                  control={control}
                  index={i}
                  readOnly={readOnly}
                  onRemove={() => dates.remove(i)}
                />
              ))}
              {!readOnly && (
                <div>
                  <Button variant="outline" size="sm" onClick={() => dates.append({ value: '' })}>
                    <PlusIcon aria-hidden="true" />
                    {t('holidays.addDate')}
                  </Button>
                </div>
              )}
            </FieldSet>
            <FieldSet className="gap-3">
              <FieldLegend variant="label">{t('holidays.recurring')}</FieldLegend>
              {recurring.fields.map((f, i) => {
                const err = errorAt(errors, `recurring_local_holidays.${i}.value`);
                return (
                  <div key={f.id} className="space-y-1">
                    <div className="flex items-center gap-2">
                      <Input
                        {...fieldAria(`hol-r${i}`, err)}
                        aria-label={t('holidays.recurringAria', { index: i + 1 })}
                        placeholder="MM-GG"
                        maxLength={5}
                        className="w-28 font-mono"
                        disabled={readOnly}
                        {...register(`recurring_local_holidays.${i}.value`)}
                      />
                      {!readOnly && (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => recurring.remove(i)}
                          aria-label={t('holidays.removeRecurringAria', { index: i + 1 })}
                        >
                          <XIcon aria-hidden="true" />
                        </Button>
                      )}
                    </div>
                    <FieldError id={`hol-r${i}`} error={err} />
                  </div>
                );
              })}
              {!readOnly && (
                <div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => recurring.append({ value: '' })}
                  >
                    <PlusIcon aria-hidden="true" />
                    {t('holidays.addRecurring')}
                  </Button>
                </div>
              )}
              <FieldDescription>{t('holidays.recurringHint')}</FieldDescription>
            </FieldSet>
          </div>
        </CardContent>
        {!readOnly && (
          <CardFooter className="justify-end border-t">
            <Button type="submit" disabled={s.pending}>
              {s.pending && <Spinner />}
              {t('common.actions.save', { ns: 'translation' })}
            </Button>
          </CardFooter>
        )}
      </Card>
    </form>
  );
}

function HolidayDate({
  control,
  index,
  readOnly,
  onRemove,
}: {
  control: Control<ReturnType<typeof toHolidaysValues>>;
  index: number;
  readOnly: boolean;
  onRemove: () => void;
}) {
  const { t } = useTranslation('configuration');
  const { field, fieldState } = useController({
    control,
    name: `local_holidays.${index}.value`,
  });
  const id = `hol-d${index}`;
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <DatePicker
          {...fieldAria(id, fieldState.error)}
          aria-label={
            field.value
              ? `${t('holidays.dateAria', { index: index + 1 })}: ${formatIsoDate(field.value)}`
              : t('holidays.dateAria', { index: index + 1 })
          }
          value={field.value || undefined}
          onChange={(v) => field.onChange(v ?? '')}
          disabled={readOnly}
        />
        {!readOnly && (
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onRemove}
            aria-label={t('holidays.removeAria', { index: index + 1 })}
          >
            <XIcon aria-hidden="true" />
          </Button>
        )}
      </div>
      <FieldError id={id} error={fieldState.error} />
    </div>
  );
}
