import { BeachMap, SpotList, type MapGeometry } from '@arenile/shared/map';
import { useTranslation } from 'react-i18next';

export interface MapStepProps {
  /** Public map geometry; not exposed by the M0 public API yet. */
  geometry?: MapGeometry;
}

/**
 * "Dove" step content. Loaded lazily so the map renderer and gesture
 * library stay out of the initial bundle (spec 12.6).
 */
export default function MapStep({ geometry }: MapStepProps) {
  const { t } = useTranslation();
  if (!geometry) {
    return <p className="text-sm text-slate-600">{t('booking.comingSoonDetail')}</p>;
  }
  return (
    <div className="space-y-4">
      <BeachMap geometry={geometry} defaultStatus="free" height="min(60vh, 480px)" />
      <details>
        <summary className="cursor-pointer text-sm font-medium">{t('map.listTitle')}</summary>
        <SpotList spots={geometry.spots} />
      </details>
    </div>
  );
}
