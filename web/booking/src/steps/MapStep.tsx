import { BeachMap, SpotList, type MapGeometry } from '@arenile/shared/map';
import { Button } from '@arenile/shared/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@arenile/shared/ui/collapsible';
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
    return <p className="text-sm text-muted-foreground">{t('booking.comingSoonDetail')}</p>;
  }
  return (
    <div className="space-y-4">
      <BeachMap geometry={geometry} defaultStatus="free" height="min(60vh, 480px)" />
      <Collapsible>
        <CollapsibleTrigger asChild>
          <Button variant="outline" size="sm">
            {t('map.listTitle')}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-3">
          <SpotList spots={geometry.spots} />
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
