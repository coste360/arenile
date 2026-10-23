import { BeachMap, type SpotStatus } from '@arenile/shared/map';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { OptionSelect, QueryState } from '@arenile/shared/ui/patterns';
import { XIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAvailability } from './api';
import type { Slots } from './model';
import { toGeometry, type EstablishmentSpots } from './spots';

const NO_IDS: readonly string[] = [];

export interface SpotPickerProps {
  establishmentId: string;
  inventory: EstablishmentSpots;
  from: string;
  to: string;
  slots: Slots;
  packId?: string;
  value: string[];
  onChange: (spotIds: string[]) => void;
  /** Allow selecting spots shown as unavailable (they are part of a hold being converted). */
  allowIds?: readonly string[];
}

/**
 * Read-only mini map (shared BeachMap) coloured with the availability of the
 * chosen dates and pack: free spots toggle in and out of the selection,
 * unavailable ones are announced but not selectable.
 */
export function SpotPicker({
  establishmentId,
  inventory,
  from,
  to,
  slots,
  packId,
  value,
  onChange,
  allowIds = NO_IDS,
}: SpotPickerProps) {
  const { t, i18n } = useTranslation('bookings');
  const firstMapWithSelection =
    inventory.spots.find((s) => value.includes(s.id))?.map_id ?? inventory.maps[0]?.id ?? '';
  const [mapChoice, setMapChoice] = useState<string>('');
  const mapId = mapChoice || firstMapWithSelection;
  const doc = inventory.documents.find((d) => d.map.id === mapId);
  const geometry = useMemo(
    () => (doc ? toGeometry(doc, i18n.language) : null),
    [doc, i18n.language],
  );
  const availability = useAvailability(
    establishmentId,
    mapId && from && to ? { from, to, slots, map_id: mapId, pack_id: packId || undefined } : null,
  );

  const free = useMemo(() => {
    const set = new Set<string>(allowIds);
    for (const z of availability.data?.zones ?? []) {
      for (const s of z.spots ?? []) if (s.status === 'available') set.add(s.spot_id);
    }
    return set;
  }, [availability.data, allowIds]);

  const statuses = useMemo(() => {
    const out: Record<string, SpotStatus> = {};
    for (const s of doc?.spots ?? [])
      out[s.id] = free.has(s.id) && s.active ? 'free' : 'unavailable';
    return out;
  }, [doc, free]);

  const toggle = (id: string) => {
    if (value.includes(id)) onChange(value.filter((x) => x !== id));
    else if (free.has(id)) onChange([...value, id]);
  };

  if (inventory.isLoading) return <Skeleton className="h-64 w-full" />;
  if (inventory.maps.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('picker.noMaps')}</p>;
  }
  return (
    <div className="space-y-3">
      {inventory.maps.length > 1 && (
        <OptionSelect
          value={mapId}
          onChange={setMapChoice}
          options={inventory.maps.map((m) => ({ value: m.id, label: m.name }))}
          className="w-full sm:w-64"
          id="spot-picker-map"
        />
      )}
      <QueryState isLoading={false} error={availability.error} />
      {geometry && (
        <BeachMap
          geometry={geometry}
          statuses={statuses}
          selectedIds={value}
          onSpotActivate={(spot) => toggle(spot.id)}
          spotDescription={(spot) =>
            value.includes(spot.id)
              ? t('picker.selected')
              : free.has(spot.id)
                ? t('picker.free')
                : t('picker.unavailable')
          }
          legendStatuses={['free', 'unavailable']}
          title={t('picker.mapTitle')}
          height="45vh"
        />
      )}
      <div className="flex flex-wrap items-center gap-2" aria-live="polite">
        {value.length === 0 ? (
          <span className="text-sm text-muted-foreground">{t('picker.none')}</span>
        ) : (
          value.map((id) => (
            <Badge key={id} variant="secondary" className="gap-1 pr-1">
              {inventory.spotById.get(id)?.label ?? id}
              <Button
                size="icon"
                variant="ghost"
                className="size-5"
                aria-label={t('picker.remove', { label: inventory.spotById.get(id)?.label ?? id })}
                onClick={() => onChange(value.filter((x) => x !== id))}
              >
                <XIcon aria-hidden="true" />
              </Button>
            </Badge>
          ))
        )}
      </div>
    </div>
  );
}
