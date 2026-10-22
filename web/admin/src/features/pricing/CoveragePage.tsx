import { Button, Card } from '@arenile/shared/ui';
import { useState } from 'react';
import { useEditor } from '../configuration/api/useCrud';
import { FormDialog } from '../configuration/components/dialogs';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { Select } from '../configuration/components/fields';
import { formatDate } from '../configuration/lib/dates';
import { useCoverage, type CoverageReport, type Slots } from './api';
import { calendarMonths, gapCountsByDay, heatLevel } from './coverage';
import type { RuleValues } from './rules.schema';
import { RuleForm } from './RulesTab';
import { PricingNav, SeasonSelect } from './components';
import { useSelectedSeason } from './season';
import { usePricingData } from './usePricingData';

/** Verifica di copertura (spec 8.7): season days without a price per active pack and category. */
export function CoveragePage() {
  const { t } = useTranslation('pricing');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const [slots, setSlots] = useState<Slots>('FULL');
  const allowed = ctx.canConfigure;
  const report = useCoverage(
    ctx.establishmentId,
    sel.season && allowed ? { season_id: sel.season.id, slots } : null,
  );
  const creating = useEditor<never>();
  const [prefill, setPrefill] = useState<Partial<RuleValues>>();
  type Gap = CoverageReport['gaps'][number];
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
  const packName = (id: string) => {
    const p = data.packs.find((x) => x.id === id);
    return p ? data.name(p.name) || p.code : id.slice(0, 8);
  };
  return (
    <ConfigPage
      title={t('coverage.title')}
      description={t('coverage.description')}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      <PricingNav establishmentId={ctx.establishmentId} seasonId={sel.season?.id} />
      {!ctx.isLoading && !allowed && <p role="note">{t('configureOnly')}</p>}
      <label className="flex items-center gap-2 text-sm">
        {t('fields.slots')}
        <Select className="w-44" value={slots} onChange={(e) => setSlots(e.target.value as Slots)}>
          {(['FULL', 'AM', 'PM'] as const).map((s) => (
            <option key={s} value={s}>
              {t(`slots.${s}`)}
            </option>
          ))}
        </Select>
      </label>
      <QueryState
        isLoading={sel.isLoading || (allowed && !!sel.season && report.isPending)}
        error={sel.error ?? report.error}
        onRetry={() => void report.refetch()}
      />
      {!sel.isLoading && sel.seasons.length === 0 && <EmptyState>{t('noSeason')}</EmptyState>}
      {report.data &&
        (report.data.complete ? (
          <p
            role="status"
            className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-900"
          >
            {t('coverage.complete', {
              from: formatDate(report.data.from, ctx.uiLocale),
              to: formatDate(report.data.to, ctx.uiLocale),
            })}
          </p>
        ) : (
          <>
            <CoverageHeatmap report={report.data} locale={ctx.uiLocale} />
            <Card className="overflow-x-auto">
              <table className="w-full min-w-[36rem] text-sm">
                <caption className="px-4 pt-3 text-left font-medium text-red-800">
                  {t('coverage.gaps', { count: report.data.gaps.length })}
                </caption>
                <thead className="text-left text-slate-600">
                  <tr>
                    <th scope="col" className="px-4 py-2 font-medium">
                      {t('fields.pack')}
                    </th>
                    <th scope="col" className="px-4 py-2 font-medium">
                      {t('fields.price_category')}
                    </th>
                    <th scope="col" className="px-4 py-2 font-medium">
                      {t('fields.channel')}
                    </th>
                    <th scope="col" className="px-4 py-2 font-medium">
                      {t('coverage.days')}
                    </th>
                    <th scope="col" className="px-4 py-2">
                      <span className="sr-only">{t('actionsColumn')}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {report.data.gaps.map((g) => (
                    <tr
                      key={`${g.pack_id}-${g.price_category}-${g.channel}-${g.from}`}
                      className="border-t border-slate-100"
                    >
                      <th scope="row" className="px-4 py-2 text-left font-medium">
                        {packName(g.pack_id)}
                      </th>
                      <td className="px-4 py-2 font-mono text-xs">{g.price_category}</td>
                      <td className="px-4 py-2">{t(`channels.${g.channel}`)}</td>
                      <td className="px-4 py-2 tabular-nums">
                        {g.from === g.to
                          ? formatDate(g.from, ctx.uiLocale)
                          : `${formatDate(g.from, ctx.uiLocale)} → ${formatDate(g.to, ctx.uiLocale)}`}
                      </td>
                      <td className="px-4 py-2 text-right">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => createRule(g)}
                          aria-label={t('coverage.createRuleAria', {
                            pack: packName(g.pack_id),
                            category: g.price_category,
                            from: formatDate(g.from, ctx.uiLocale),
                          })}
                        >
                          {t('coverage.createRule')}
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          </>
        ))}
      {sel.season && (
        <FormDialog
          open={creating.open}
          onOpenChange={creating.setOpen}
          wide
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
        </FormDialog>
      )}
    </ConfigPage>
  );
}

const HEAT = [
  'bg-emerald-100 text-emerald-900',
  'bg-red-100 text-red-900',
  'bg-red-200 text-red-900',
  'bg-red-400 text-white',
  'bg-red-600 text-white',
] as const;

/** Season calendar: each day coloured by how many pack/category/channel combinations lack a price. */
export function CoverageHeatmap({ report, locale }: { report: CoverageReport; locale: string }) {
  const { t } = useTranslation('pricing');
  const counts = gapCountsByDay(report);
  const max = Math.max(0, ...counts.values());
  const monthFmt = new Intl.DateTimeFormat(locale, {
    month: 'long',
    year: 'numeric',
    timeZone: 'UTC',
  });
  return (
    <Card className="space-y-3 p-4">
      <h3 className="text-sm font-semibold">{t('coverage.calendar')}</h3>
      <div className="flex flex-wrap gap-6">
        {calendarMonths(report.from, report.to).map((m) => (
          <div key={m.key}>
            <p className="mb-1 text-xs font-medium capitalize text-slate-700">
              {monthFmt.format(new Date(`${m.key}-01T00:00:00Z`))}
            </p>
            <div role="list" className="grid grid-cols-7 gap-0.5">
              {(['1', '2', '3', '4', '5', '6', '7'] as const).map((d) => (
                <span
                  key={d}
                  aria-hidden="true"
                  className="w-7 text-center text-[10px] text-slate-500"
                >
                  {t(`weekdaysShort.${d}`)}
                </span>
              ))}
              {Array.from({ length: m.offset }, (_, i) => (
                <span key={`pad-${i}`} aria-hidden="true" />
              ))}
              {m.days.map((d) => {
                const n = counts.get(d) ?? 0;
                const label = n
                  ? t('coverage.dayGaps', { count: n, date: formatDate(d, locale) })
                  : t('coverage.dayOk', { date: formatDate(d, locale) });
                return (
                  <span
                    key={d}
                    role="listitem"
                    title={label}
                    aria-label={label}
                    data-gaps={n}
                    className={`flex h-7 w-7 items-center justify-center rounded text-[11px] tabular-nums ${HEAT[heatLevel(n, max)]}`}
                  >
                    {Number(d.slice(8))}
                  </span>
                );
              })}
            </div>
          </div>
        ))}
      </div>
      <p className="flex items-center gap-2 text-xs text-slate-600">
        <span className={`inline-block h-3 w-3 rounded ${HEAT[0]}`} aria-hidden="true" />
        {t('coverage.legendOk')}
        <span className={`ml-3 inline-block h-3 w-3 rounded ${HEAT[4]}`} aria-hidden="true" />
        {t('coverage.legendGaps', { max })}
      </p>
    </Card>
  );
}
