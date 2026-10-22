import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { PricingNav, SeasonSelect } from './components';
import { DiscountsTab } from './DiscountsTab';
import { FixedPricesTab } from './FixedPricesTab';
import { RulesTab } from './RulesTab';
import { useSelectedSeason } from './season';
import { SeasonCopyDialog } from './SeasonCopyDialog';
import { usePricingData } from './usePricingData';

const TABS = ['rules', 'fixed', 'discounts'] as const;
type Tab = (typeof TABS)[number];

/** Configurazione → Prezzi (spec 8, 15): rules, fixed prices, promo codes, agreements; season copy action. */
export function PricingPage() {
  const { t } = useTranslation('pricing');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const [tab, setTab] = useState<Tab>('rules');
  const readOnly = !ctx.isLoading && !ctx.canConfigure;
  const season = sel.season;

  const common = season && {
    establishmentId: ctx.establishmentId,
    seasonId: season.id,
    readOnly,
    periods: sel.periods,
    packs: data.packs,
    extras: data.extras,
    zones: data.zones,
    name: data.name,
    locale: ctx.uiLocale,
  };

  return (
    <ConfigPage
      title={t('rules.title')}
      description={t('rules.pageDescription')}
      readOnly={readOnly}
      actions={
        <div className="flex flex-wrap items-center gap-2">
          <SeasonSelect seasons={sel.seasons} season={season} onSelect={sel.select} />
          {season && !readOnly && !ctx.isLoading && (
            <SeasonCopyDialog
              establishmentId={ctx.establishmentId}
              season={season}
              onOpenSeason={(id) => {
                sel.select(id);
                setTab('rules');
              }}
            />
          )}
        </div>
      }
    >
      <PricingNav establishmentId={ctx.establishmentId} seasonId={season?.id} />
      <QueryState
        isLoading={sel.isLoading || data.isLoading}
        error={sel.error ?? data.error}
        onRetry={sel.refetch}
      />
      {!sel.isLoading && sel.seasons.length === 0 && <EmptyState>{t('noSeason')}</EmptyState>}
      {common && (
        <>
          <div
            role="tablist"
            aria-label={t('tabs.label')}
            className="flex flex-wrap gap-1 border-b border-slate-200"
          >
            {TABS.map((id) => (
              <button
                key={id}
                id={`pricing-tab-${id}`}
                type="button"
                role="tab"
                aria-selected={tab === id}
                aria-controls={`pricing-panel-${id}`}
                onClick={() => setTab(id)}
                className={
                  tab === id
                    ? '-mb-px border-b-2 border-[var(--ar-primary)] px-3 py-2 text-sm font-semibold'
                    : 'px-3 py-2 text-sm text-slate-600 hover:text-slate-900'
                }
              >
                {t(`tabs.${id}`)}
              </button>
            ))}
          </div>
          <div role="tabpanel" id={`pricing-panel-${tab}`} aria-labelledby={`pricing-tab-${tab}`}>
            {tab === 'rules' && <RulesTab {...common} />}
            {tab === 'fixed' && <FixedPricesTab {...common} categories={data.categories} />}
            {tab === 'discounts' && <DiscountsTab {...common} />}
          </div>
        </>
      )}
    </ConfigPage>
  );
}
