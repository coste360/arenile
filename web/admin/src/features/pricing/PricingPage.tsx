import { Tabs, TabsContent } from '@arenile/shared/ui';
import { EmptyState, QueryState, ResponsiveTabsList } from '@arenile/shared/ui/patterns';
import { CalendarXIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { DiscountsTab } from './DiscountsTab';
import { FixedPricesTab } from './FixedPricesTab';
import { PricingLayout, SeasonSelect } from './kit';
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
    categoryNames: data.categoryNames,
    name: data.name,
    locale: ctx.uiLocale,
  };

  return (
    <PricingLayout
      title={t('rules.title')}
      description={t('rules.pageDescription')}
      readOnly={readOnly}
      establishmentId={ctx.establishmentId}
      seasonId={season?.id}
      actions={
        <>
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
        </>
      }
    >
      <QueryState
        isLoading={sel.isLoading || data.isLoading}
        error={sel.error ?? data.error}
        onRetry={sel.refetch}
      />
      {!sel.isLoading && sel.seasons.length === 0 && (
        <EmptyState icon={CalendarXIcon} title={t('noSeason')} />
      )}
      {common && (
        <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)} className="gap-4">
          <ResponsiveTabsList
            variant="line"
            label={t('tabs.label')}
            value={tab}
            onValueChange={(v) => setTab(v as Tab)}
            options={TABS.map((id) => ({ value: id, label: t(`tabs.${id}`) }))}
          />
          <TabsContent value="rules">
            <RulesTab {...common} />
          </TabsContent>
          <TabsContent value="fixed">
            <FixedPricesTab {...common} categories={data.categories} />
          </TabsContent>
          <TabsContent value="discounts">
            <DiscountsTab {...common} />
          </TabsContent>
        </Tabs>
      )}
    </PricingLayout>
  );
}
