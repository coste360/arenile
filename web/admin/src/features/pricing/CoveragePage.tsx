import { dateFnsLocale, dateToIso, formatIsoDate, isoToDate } from '@arenile/shared/lib/dates';
import {
  Alert,
  AlertDescription,
  Badge,
  Button,
  Calendar,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  FormSheet,
  QueryState,
} from '@arenile/shared/ui/patterns';
import { CalendarXIcon, CircleCheckIcon, LockIcon, PlusIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useEditor } from '../configuration/api/useCrud';
import { useCoverage, type CoverageReport, type Slots } from './api';
import { calendarMonths, gapCountsByDay, heatLevel } from './coverage';
import { PricingLayout, SeasonSelect, SingleToggle } from './kit';
import type { RuleValues } from './rules.schema';
import { RuleForm } from './RulesTab';
import { useSelectedSeason } from './season';
import { usePricingData } from './usePricingData';

type Gap = CoverageReport['gaps'][number];

/** Verifica di copertura (spec 8.7): season days without a price per active pack and category. */
export function CoveragePage() {
  const { t } = useTranslation('pricing');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const [slots, setSlots] = useState<Slots>('FULL');
  const [day, setDay] = useState<string>();
  const allowed = ctx.canConfigure;
  const report = useCoverage(
    ctx.establishmentId,
    sel.season && allowed ? { season_id: sel.season.id, slots } : null,
  );
  const creating = useEditor<never>();
  const [prefill, setPrefill] = useState<Partial<RuleValues>>();
  const packName = (id: string) => {
    const p = data.packs.find((x) => x.id === id);
    return p ? data.name(p.name) || p.code : id.slice(0, 8);
  };
  const createRule = (g: Gap) => {
    setPrefill({
      name: t('coverage.ruleName', { pack: packName(g.pack_id), category: g.price_category }),
      stage: 'base',
      action_type: 'set',
      target_type: 'pack',
      target_ids: [g.pack_id],
      price_categories: g.price_category,
      channels: [g.channel],
      slots: slots === 'FULL' ? [] : [slots],
      date_ranges: [{ from: g.from, to: g.to }],
    });
    creating.create();
  };
  const range = (g: Gap) =>
    g.from === g.to ? formatIsoDate(g.from) : `${formatIsoDate(g.from)} → ${formatIsoDate(g.to)}`;

  const columns: ColumnDef<Gap>[] = [
    {
      id: 'pack',
      accessorFn: (g) => packName(g.pack_id),
      meta: { label: t('fields.pack') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.pack')} />,
      cell: ({ getValue }) => <span className="font-medium">{getValue<string>()}</span>,
    },
    {
      accessorKey: 'price_category',
      meta: { label: t('fields.price_category') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('fields.price_category')} />
      ),
      cell: ({ row }) => (
        <Badge variant="secondary" className="font-mono">
          {row.original.price_category}
        </Badge>
      ),
    },
    {
      id: 'channel',
      accessorFn: (g) => t(`channels.${g.channel}`),
      meta: { label: t('fields.channel') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.channel')} />,
    },
    {
      accessorKey: 'from',
      meta: { label: t('coverage.days') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('coverage.days')} />,
      cell: ({ row }) => <span className="tabular-nums">{range(row.original)}</span>,
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actionsColumn')}</span>,
      cell: ({ row }) => (
        <div className="text-right">
          <Button
            variant="outline"
            size="sm"
            onClick={() => createRule(row.original)}
            aria-label={t('coverage.createRuleAria', {
              pack: packName(row.original.pack_id),
              category: row.original.price_category,
              from: formatIsoDate(row.original.from),
            })}
          >
            <PlusIcon aria-hidden="true" />
            {t('coverage.createRule')}
          </Button>
        </div>
      ),
    },
  ];

  const gaps = (report.data?.gaps ?? []).filter((g) => !day || (g.from <= day && day <= g.to));

  return (
    <PricingLayout
      title={t('coverage.title')}
      description={t('coverage.description')}
      establishmentId={ctx.establishmentId}
      seasonId={sel.season?.id}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      {!ctx.isLoading && !allowed && (
        <Alert role="note">
          <LockIcon aria-hidden="true" />
          <AlertDescription>{t('configureOnly')}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm font-medium">{t('fields.slots')}</span>
        <SingleToggle
          label={t('fields.slots')}
          value={slots}
          onChange={(v) => {
            setSlots(v as Slots);
            setDay(undefined);
          }}
          options={(['FULL', 'AM', 'PM'] as const).map((s) => ({
            value: s,
            label: t(`slots.${s}`),
          }))}
        />
      </div>
      <QueryState
        isLoading={sel.isLoading || (allowed && !!sel.season && report.isPending)}
        error={sel.error ?? report.error}
        onRetry={() => void report.refetch()}
      />
      {!sel.isLoading && sel.seasons.length === 0 && (
        <EmptyState icon={CalendarXIcon} title={t('noSeason')} />
      )}
      {report.data &&
        (report.data.complete ? (
          <Alert role="status" className="border-success/40 bg-success/10">
            <CircleCheckIcon aria-hidden="true" />
            <AlertDescription className="text-foreground">
              {t('coverage.complete', {
                from: formatIsoDate(report.data.from),
                to: formatIsoDate(report.data.to),
              })}
            </AlertDescription>
          </Alert>
        ) : (
          <>
            <CoverageHeatmap report={report.data} selected={day} onSelect={setDay} />
            <Card>
              <CardHeader>
                <CardTitle className="text-base text-destructive">
                  {t('coverage.gaps', { count: report.data.gaps.length })}
                </CardTitle>
                {day && (
                  <CardDescription className="flex flex-wrap items-center gap-2">
                    {t('coverage.filteredDay', { date: formatIsoDate(day) })}
                    <Button variant="ghost" size="sm" onClick={() => setDay(undefined)}>
                      <XIcon aria-hidden="true" />
                      {t('coverage.showAll')}
                    </Button>
                  </CardDescription>
                )}
              </CardHeader>
              <CardContent>
                <DataTable
                  caption={t('coverage.gaps', { count: report.data.gaps.length })}
                  columns={columns}
                  data={gaps}
                  getRowId={(g) => `${g.pack_id}-${g.price_category}-${g.channel}-${g.from}`}
                  columnToggle={false}
                />
              </CardContent>
            </Card>
          </>
        ))}
      {sel.season && (
        <FormSheet
          size="lg"
          padded={false}
          open={creating.open}
          onOpenChange={creating.setOpen}
          title={t('rules.add')}
        >
          <RuleForm
            key={creating.key}
            establishmentId={ctx.establishmentId}
            seasonId={sel.season.id}
            readOnly={!allowed}
            periods={sel.periods}
            packs={data.packs}
            extras={data.extras}
            zones={data.zones}
            name={data.name}
            locale={ctx.uiLocale}
            initial={prefill}
            onDone={() => {
              creating.close();
              void report.refetch();
            }}
          />
        </FormSheet>
      )}
    </PricingLayout>
  );
}

const HEAT_CLASSES = [
  '[&>button]:bg-success/15',
  '[&>button]:bg-destructive/15',
  '[&>button]:bg-destructive/30',
  '[&>button]:bg-destructive/60 [&>button]:text-white',
  '[&>button]:bg-destructive [&>button]:text-white',
] as const;

/**
 * Season calendar (shadcn Calendar with day modifiers): each day coloured by
 * how many pack/category/channel combinations lack a price. Clicking a day
 * filters the gap list to that day.
 */
export function CoverageHeatmap({
  report,
  selected,
  onSelect,
}: {
  report: CoverageReport;
  selected?: string;
  onSelect: (day: string | undefined) => void;
}) {
  const { t, i18n } = useTranslation('pricing');
  const counts = gapCountsByDay(report);
  const max = Math.max(0, ...counts.values());
  const months = calendarMonths(report.from, report.to);
  const levels: Date[][] = [[], [], [], [], []];
  for (const m of months) {
    for (const d of m.days) {
      const date = isoToDate(d);
      if (date) levels[heatLevel(counts.get(d) ?? 0, max)]?.push(date);
    }
  }
  const from = isoToDate(report.from);
  const to = isoToDate(report.to);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t('coverage.calendar')}</CardTitle>
        <CardDescription className="flex flex-wrap items-center gap-3">
          <Badge variant="outline" className="gap-1.5">
            <span aria-hidden="true" className="size-2.5 rounded-sm bg-success/40" />
            {t('coverage.legendOk')}
          </Badge>
          <Badge variant="outline" className="gap-1.5">
            <span aria-hidden="true" className="size-2.5 rounded-sm bg-destructive" />
            {t('coverage.legendGaps', { max })}
          </Badge>
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Calendar
          mode="single"
          selected={isoToDate(selected)}
          onSelect={(d) => onSelect(d ? dateToIso(d) : undefined)}
          locale={dateFnsLocale(i18n.language)}
          weekStartsOn={1}
          defaultMonth={from}
          startMonth={from}
          endMonth={to}
          numberOfMonths={months.length}
          hideNavigation
          showOutsideDays={false}
          disabled={from && to ? [{ before: from }, { after: to }] : undefined}
          modifiers={{
            heat0: levels[0] ?? [],
            heat1: levels[1] ?? [],
            heat2: levels[2] ?? [],
            heat3: levels[3] ?? [],
            heat4: levels[4] ?? [],
          }}
          modifiersClassNames={{
            heat0: HEAT_CLASSES[0],
            heat1: HEAT_CLASSES[1],
            heat2: HEAT_CLASSES[2],
            heat3: HEAT_CLASSES[3],
            heat4: HEAT_CLASSES[4],
          }}
          labels={{
            labelDayButton: (date) => {
              const iso = dateToIso(date);
              const n = counts.get(iso) ?? 0;
              return n
                ? t('coverage.dayGaps', { count: n, date: formatIsoDate(iso) })
                : t('coverage.dayOk', { date: formatIsoDate(iso) });
            },
          }}
          classNames={{ months: 'relative flex flex-wrap gap-6' }}
          className="p-0"
        />
      </CardContent>
    </Card>
  );
}
