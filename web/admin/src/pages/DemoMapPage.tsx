import {
  BeachMap,
  SPOT_STATUSES,
  sampleGeometry,
  sampleStatuses,
  type MapSpot,
} from '@arenile/shared/map';
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
      <div>
        <h1 id="page-title" className="text-2xl font-semibold">
          {t('nav.map')}
        </h1>
        <p className="mt-1 text-sm text-slate-600">
          <strong>{t('admin.demoMap.title')}.</strong> {t('admin.demoMap.note')}
        </p>
      </div>
      <BeachMap
        geometry={geometry}
        statuses={statuses}
        legendStatuses={SPOT_STATUSES.filter((s) => s !== 'unavailable')}
        selectedIds={selected ? [selected.id] : []}
        onSpotActivate={setSelected}
        title={t('admin.demoMap.title')}
        height="min(65vh, 560px)"
        className="rounded-[var(--ar-radius)] border border-slate-200 bg-white p-3"
      />
      <p role="status" aria-live="polite" className="min-h-6 text-sm">
        {selected ? t('admin.demoMap.selected', { label: selected.label }) : ''}
      </p>
    </section>
  );
}
