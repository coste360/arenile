import { availabilityQuery } from './where/query';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { InfoIcon, RotateCwIcon } from 'lucide-react';
import { Suspense, lazy, useEffect, useMemo, type Dispatch, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePublicAvailability } from '../api/public';
import { useBookingEnv } from '../flow/context';
import { localize } from '../flow/format';
import { problemMessageKey } from '../flow/problems';
import type { FlowAction, FlowState } from '../flow/selection';
import { PUBLIC_NS } from '../i18n';
import { QuantityPicker } from './where/QuantityPicker';
import { ZoneCards } from './where/ZoneCards';

// The map (BeachMap + kit) is its own chunk: zone-only establishments never load it.
const SeatMapPicker = lazy(() => import('./where/SeatMapPicker'));

export interface WhereStepProps {
  state: FlowState;
  dispatch: Dispatch<FlowAction>;
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3">
      <h3 className="text-base font-semibold">{title}</h3>
      {children}
    </section>
  );
}

/**
 * "Dove" (spec 12.3 passo 2), mixed modes rendered together, map first:
 * `seat` zones on the map of each map document, `zone` zones as cards,
 * `quantity` zones with a stepper. Spots that are no longer available when
 * the availability refreshes are removed from the choice, with a notice.
 */
export default function WhereStep({ state, dispatch }: WhereStepProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const { slug, online, establishment, theme } = useBookingEnv();
  const query = availabilityQuery(state);
  const availability = usePublicAvailability(slug, query);
  const showPrices = online.show_prices_on_map && theme.showPricesOnMap;
  const lang = i18n.language;

  const zones = useMemo(
    () => [...online.zones].sort((a, b) => a.sort_order - b.sort_order),
    [online.zones],
  );
  const seatByMap = useMemo(() => {
    const m = new Map<string, string[]>();
    for (const z of zones) {
      if (z.assignment_mode !== 'seat' || !z.map_id) continue;
      m.set(z.map_id, [...(m.get(z.map_id) ?? []), z.id]);
    }
    return m;
  }, [zones]);
  const zoneZones = zones.filter((z) => z.assignment_mode === 'zone');
  const qtyZones = zones.filter((z) => z.assignment_mode === 'quantity');

  // Drop choices that became unavailable (someone else booked them meanwhile).
  const data = availability.data;
  useEffect(() => {
    if (!data || availability.isPlaceholderData) return;
    const byZone = new Map(data.zones.map((z) => [z.zone_id, z]));
    const gone = state.lines
      .filter((l) => {
        const z = byZone.get(l.zoneId);
        if (!z) return true;
        if (l.mode === 'seat') return !z.spots?.some((s) => s.spot_id === l.spotId && s.available);
        return z.available < l.quantity;
      })
      .map((l) => l.key);
    if (gone.length > 0) dispatch({ type: 'dropLines', keys: gone });
  }, [data, availability.isPlaceholderData, state.lines, dispatch]);

  if (zones.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('where.noZones')}</p>;
  }
  if (availability.isPending) {
    return (
      <div role="status" className="space-y-3">
        <span className="sr-only">{tc('common.loading')}</span>
        <Skeleton aria-hidden="true" className="h-8 w-1/2" />
        <Skeleton aria-hidden="true" className="h-56 w-full" />
      </div>
    );
  }
  if (availability.isError || !data) {
    return (
      <Alert variant="destructive">
        <AlertDescription className="space-y-3">
          <p>{tc(problemMessageKey(availability.error))}</p>
          <Button variant="outline" onClick={() => void availability.refetch()}>
            <RotateCwIcon aria-hidden="true" />
            {tc('common.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    );
  }

  const nothing = data.zones.every((z) => z.available <= 0);
  const selectedSpots = state.lines.filter((l) => l.mode === 'seat').map((l) => l.key);
  const mapName = (id: string) =>
    localize(online.maps.find((m) => m.id === id)?.name, lang, establishment.default_locale) ||
    t('where.map');

  return (
    <div className="space-y-8">
      {state.dropped && (
        <Alert role="status">
          <InfoIcon aria-hidden="true" />
          <AlertDescription>{t('where.dropped')}</AlertDescription>
        </Alert>
      )}
      {nothing && (
        <Alert role="status">
          <InfoIcon aria-hidden="true" />
          <AlertDescription>{t('where.nothing')}</AlertDescription>
        </Alert>
      )}
      {[...seatByMap.entries()].map(([mapId, zoneIds]) => (
        <Section key={mapId} title={mapName(mapId)}>
          <Suspense
            fallback={
              <div role="status">
                <span className="sr-only">{t('where.mapLoading')}</span>
                <Skeleton aria-hidden="true" className="h-64 w-full" />
              </div>
            }
          >
            <SeatMapPicker
              mapId={mapId}
              zoneIds={zoneIds}
              availability={data}
              showPrices={showPrices}
              selected={selectedSpots}
              onToggle={(spot) =>
                dispatch({
                  type: 'toggleSpot',
                  zoneId: spot.zoneId,
                  spotId: spot.id,
                  spotLabel: spot.label,
                })
              }
            />
          </Suspense>
        </Section>
      ))}
      {zoneZones.length > 0 && (
        <Section title={t('where.zones')}>
          <ZoneCards
            zones={zoneZones}
            availability={data}
            showPrices={showPrices}
            selectedZoneIds={state.lines.filter((l) => l.mode === 'zone').map((l) => l.key)}
            onToggle={(zoneId) => dispatch({ type: 'toggleZone', zoneId })}
          />
        </Section>
      )}
      {qtyZones.length > 0 && (
        <Section title={t('where.quantity')}>
          <div className="space-y-3">
            {qtyZones.map((z) => {
              const a = data.zones.find((x) => x.zone_id === z.id);
              return (
                <QuantityPicker
                  key={z.id}
                  name={localize(z.name, lang, establishment.default_locale)}
                  max={a?.available ?? 0}
                  value={state.lines.find((l) => l.key === z.id)?.quantity ?? 0}
                  onChange={(quantity) => dispatch({ type: 'quantity', zoneId: z.id, quantity })}
                />
              );
            })}
          </div>
        </Section>
      )}
    </div>
  );
}
