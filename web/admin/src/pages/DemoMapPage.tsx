import {
  BeachMap,
  SPOT_STATUSES,
  sampleGeometry,
  sampleStatuses,
  type MapSpot,
} from '@arenile/shared/map';
import { PageHeader } from '@arenile/shared/ui/patterns';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

/** Read-only demo of the shared map renderer with sample geometry. */
export function DemoMapPage() {
  const { t } = useTranslation();
  const geometry = useMemo(() => sampleGeometry(6, 14), []);
  const statuses = useMemo(() => sampleStatuses(geometry), [geometry]);
  const [selected, setSelected] = useState<MapSpot | null>(null);
  return (
    <section aria-labelledby="page-title" className="space-y-4">
      <PageHeader
        title={t('nav.map')}
        description={
          <>
            <strong className="font-medium text-foreground">{t('admin.demoMap.title')}.</strong>{' '}
            {t('admin.demoMap.note')}
          </>
        }
      />
      <BeachMap
        geometry={geometry}
        statuses={statuses}
        legendStatuses={SPOT_STATUSES.filter((s) => s !== 'unavailable')}
        selectedIds={selected ? [selected.id] : []}
        onSpotActivate={setSelected}
        title={t('admin.demoMap.title')}
        height="min(65vh, 560px)"
        className="rounded-xl border bg-card p-3 shadow-sm"
      />
      <p role="status" aria-live="polite" className="min-h-6 text-sm text-muted-foreground">
        {selected ? t('admin.demoMap.selected', { label: selected.label }) : ''}
      </p>
    </section>
  );
}
