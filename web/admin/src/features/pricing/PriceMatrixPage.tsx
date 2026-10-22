import { Button, Card } from '@arenile/shared/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { Select } from '../configuration/components/fields';
import { formatCents } from '../configuration/lib/money';
import { usePriceMatrix, type Channel, type Formula, type PriceMatrix, type Slots } from './api';
import { PricingNav, SeasonSelect } from './components';
import { downloadCsv, matrixToCsv } from './matrixCsv';
import { CHANNELS } from './rules.schema';
import { useSelectedSeason } from './season';
import { usePricingData } from './usePricingData';

/**
 * Matrice di listino (spec 8.7): one table per pack, price categories ×
 * rate periods, computed by the engine for a representative day.
 */
export function PriceMatrixPage() {
  const { t } = useTranslation('pricing');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const [formula, setFormula] = useState<Formula>('day');
  const [channel, setChannel] = useState<Channel>('desk');
  const [slots, setSlots] = useState<Slots>('FULL');
  const allowed = ctx.canConfigure;
  const matrix = usePriceMatrix(
    ctx.establishmentId,
    sel.season && allowed
      ? {
          season_id: sel.season.id,
          formula,
          channel,
          slots: formula === 'half_day' ? slots : 'FULL',
        }
      : null,
  );
  const packName = (id: string) => {
    const p = data.packs.find((x) => x.id === id);
    return p ? data.name(p.name) || p.code : id.slice(0, 8);
  };
  return (
    <ConfigPage
      title={t('matrix.title')}
      description={t('matrix.description')}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      <PricingNav establishmentId={ctx.establishmentId} seasonId={sel.season?.id} />
      {!ctx.isLoading && !allowed && <p role="note">{t('configureOnly')}</p>}
      <div className="flex flex-wrap gap-4">
        <label className="flex items-center gap-2 text-sm">
          {t('fields.formula')}
          <Select
            className="w-40"
            value={formula}
            onChange={(e) => setFormula(e.target.value as Formula)}
          >
            <option value="day">{t('formulas.day')}</option>
            <option value="half_day">{t('formulas.half_day')}</option>
          </Select>
        </label>
        {formula === 'half_day' && (
          <label className="flex items-center gap-2 text-sm">
            {t('fields.slots')}
            <Select
              className="w-36"
              value={slots}
              onChange={(e) => setSlots(e.target.value as Slots)}
            >
              <option value="AM">{t('slots.AM')}</option>
              <option value="PM">{t('slots.PM')}</option>
            </Select>
          </label>
        )}
        <label className="flex items-center gap-2 text-sm">
          {t('fields.channel')}
          <Select
            className="w-36"
            value={channel}
            onChange={(e) => setChannel(e.target.value as Channel)}
          >
            {CHANNELS.map((c) => (
              <option key={c} value={c}>
                {t(`channels.${c}`)}
              </option>
            ))}
          </Select>
        </label>
      </div>
      <QueryState
        isLoading={sel.isLoading || (allowed && !!sel.season && matrix.isPending)}
        error={sel.error ?? matrix.error}
        onRetry={() => void matrix.refetch()}
      />
      {!sel.isLoading && sel.seasons.length === 0 && <EmptyState>{t('noSeason')}</EmptyState>}
      {matrix.data && (
        <>
          <div className="flex justify-end">
            <Button
              variant="outline"
              size="sm"
              disabled={matrix.data.pack_ids.length === 0}
              onClick={() => {
                const m = matrix.data;
                if (!m || !sel.season) return;
                downloadCsv(
                  `listino-${sel.season.year}-${m.formula}-${m.channel}${m.slots === 'FULL' ? '' : `-${m.slots}`}.csv`,
                  matrixToCsv(m, {
                    packName,
                    locale: ctx.uiLocale,
                    headers: { pack: t('fields.pack'), category: t('fields.price_category') },
                    noPrice: t('matrix.noPrice'),
                  }),
                );
              }}
            >
              {t('matrix.exportCsv')}
            </Button>
          </div>
          <MatrixTables matrix={matrix.data} packName={packName} locale={ctx.uiLocale} />
        </>
      )}
    </ConfigPage>
  );
}

function MatrixTables({
  matrix,
  packName,
  locale,
}: {
  matrix: PriceMatrix;
  packName: (id: string) => string;
  locale: string;
}) {
  const { t } = useTranslation('pricing');
  if (matrix.pack_ids.length === 0 || matrix.categories.length === 0) {
    return <EmptyState>{t('matrix.empty')}</EmptyState>;
  }
  const index = new Map(
    matrix.cells.map((c) => [`${c.pack_id}\u0000${c.price_category}\u0000${c.rate_period}`, c]),
  );
  const cell = (pack: string, cat: string, period: string) =>
    index.get(`${pack}\u0000${cat}\u0000${period}`);
  return (
    <div className="space-y-4">
      {matrix.pack_ids.map((pack) => (
        <Card key={pack} className="overflow-x-auto">
          <table className="w-full min-w-[32rem] text-sm">
            <caption className="px-4 pt-3 text-left font-semibold">{packName(pack)}</caption>
            <thead className="text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('fields.price_category')}
                </th>
                {matrix.rate_periods.map((p) => (
                  <th key={p} scope="col" className="px-4 py-2 text-right font-medium">
                    {p}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {matrix.categories.map((cat) => (
                <tr key={cat} className="border-t border-slate-100">
                  <th scope="row" className="px-4 py-2 text-left font-mono text-xs font-medium">
                    {cat}
                  </th>
                  {matrix.rate_periods.map((period) => {
                    const c = cell(pack, cat, period);
                    return (
                      <td
                        key={period}
                        className={`px-4 py-2 text-right tabular-nums ${c && !c.priced ? 'bg-red-50 text-red-700' : ''}`}
                        title={c ? c.date : undefined}
                      >
                        {!c ? '—' : c.priced ? formatCents(c.price, locale) : t('matrix.noPrice')}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      ))}
      <p className="text-xs text-slate-600">{t('matrix.note')}</p>
    </div>
  );
}
