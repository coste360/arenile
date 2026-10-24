import { BeachMap, SpotList, type MapSpot } from '@arenile/shared/map';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { ListIcon, MapIcon } from 'lucide-react';
import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePublicMap, type PublicAvailability } from '../../api/public';
import { useBookingEnv } from '../../flow/context';
import { formatMoney, localize } from '../../flow/format';
import { PUBLIC_NS } from '../../i18n';
import { rowPrices, seatMapModel } from './seatMap';

export interface SeatMapPickerProps {
  mapId: string;
  /** Seat zones of this map shown online. */
  zoneIds: readonly string[];
  /** Localised zone name per zone id (spot accessible names). */
  zoneNames?: Readonly<Record<string, string>>;
  availability: PublicAvailability;
  showPrices: boolean;
  /** Selected spot ids. */
  selected: readonly string[];
  onToggle: (spot: { id: string; label: string; zoneId: string }) => void;
}

const PICKABLE = ['free'] as const;

/**
 * Map of the `seat` zones (spec 12.3 passo 2): the shared BeachMap with the
 * Coste360 kit art in public picker mode (free spots selectable, the others
 * readable but disabled, taps on small spots zoom in to reach 44 px), the
 * price legend per row and a list alternative for screen readers and small
 * screens. Loaded lazily with the step (out of the first-screen budget).
 */
export default function SeatMapPicker({
  mapId,
  zoneIds,
  zoneNames,
  availability,
  showPrices,
  selected,
  onToggle,
}: SeatMapPickerProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { slug, establishment } = useBookingEnv();
  const [asList, setAsList] = useState(false);
  const legendId = useId();
  const map = usePublicMap(slug, mapId);
  const lang = i18n.language;
  const currency = establishment.currency;

  const model = useMemo(
    () =>
      map.data
        ? seatMapModel(map.data, availability, zoneIds, (text) =>
            localize(text, lang, establishment.default_locale),
          )
        : null,
    [map.data, availability, zoneIds, lang, establishment.default_locale],
  );
  const legend = useMemo(
    () => (model && showPrices ? rowPrices(model, availability.price_legend) : []),
    [model, showPrices, availability.price_legend],
  );
  const priceOf = useMemo(() => {
    const byCat = new Map(availability.price_legend.map((e) => [e.price_category, e.from_price]));
    return (spot: MapSpot) => {
      if (!showPrices || !model) return undefined;
      const cents = byCat.get(model.categories[spot.id] ?? '');
      return cents === undefined
        ? undefined
        : t('where.fromPrice', { price: formatMoney(cents, currency, lang) });
    };
  }, [availability.price_legend, showPrices, model, t, currency, lang]);

  if (map.isPending) {
    return (
      <div role="status" className="space-y-2">
        <span className="sr-only">{t('where.mapLoading')}</span>
        <Skeleton aria-hidden="true" className="h-64 w-full" />
      </div>
    );
  }
  if (!model) return null;

  const activate = (spot: MapSpot) =>
    onToggle({ id: spot.id, label: spot.label, zoneId: model.zoneOf[spot.id] ?? '' });

  return (
    <div className="space-y-3">
      {legend.length > 0 && (
        <div className="space-y-1.5">
          <p id={legendId} className="text-xs font-medium text-muted-foreground">
            {t('where.legend')}
          </p>
          <ul className="flex flex-wrap gap-1.5" aria-labelledby={legendId}>
            {legend.map((r) => (
              <li key={`${r.row ?? '-'}-${r.toRow ?? ''}`}>
                <Badge variant="outline" className="gap-1.5 font-normal" data-tone={r.tone}>
                  <span className="font-medium">
                    {r.row && r.toRow
                      ? t('where.legendRows', { from: r.row, to: r.toRow })
                      : r.row
                        ? t('where.legendRow', { row: r.row })
                        : t('where.legendCategory', { category: r.category ?? '' })}
                  </span>
                  {t('where.fromPrice', { price: formatMoney(r.fromPrice, currency, lang) })}
                </Badge>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {selected.length > 0
            ? `${t('where.selected', { count: selected.length })}: ${model.geometry.spots
                .filter((sp) => selected.includes(sp.id))
                .map((sp) => sp.label)
                .join(', ')}`
            : t('where.tapToZoom')}
        </p>
        <Button
          type="button"
          variant="ghost"
          className="h-11"
          aria-pressed={asList}
          onClick={() => setAsList((v) => !v)}
        >
          {asList ? <MapIcon aria-hidden="true" /> : <ListIcon aria-hidden="true" />}
          {asList ? t('where.showMap') : t('where.showList')}
        </Button>
      </div>
      {asList ? (
        <SpotList
          spots={model.geometry.spots}
          statuses={model.statuses}
          selectedIds={selected}
          onSpotActivate={activate}
          spotDescription={priceOf}
        />
      ) : (
        <BeachMap
          geometry={model.geometry}
          kit={establishment.theme.map_kit}
          statuses={model.statuses}
          selectedIds={selected}
          pickableStatuses={PICKABLE}
          minTapTargetPx={44}
          zoomSlider={false}
          legendStatuses={['free', 'unavailable']}
          onSpotActivate={activate}
          spotDescription={priceOf}
          zoneNames={zoneNames}
          height="65vh"
        />
      )}
    </div>
  );
}
