import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import type { MapGeometry, SeaSide } from '@arenile/shared/map';
import { useQueries } from '@tanstack/react-query';
import { maps as mapsResource, zones as zonesResource } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';
import { mapDocumentKey } from '../map-editor/api';

type MapDocument = Schemas['MapDocument'];
type Zone = Schemas['Zone'];

/** A spot with what lists and pickers need (zone, map, row). */
export interface SpotInfo {
  id: string;
  label: string;
  row_label?: string;
  number?: number;
  zone_id: string;
  map_id: string;
  active: boolean;
}

export interface EstablishmentSpots {
  maps: { id: string; name: string }[];
  documents: MapDocument[];
  zones: Zone[];
  spots: SpotInfo[];
  spotById: Map<string, SpotInfo>;
  zoneById: Map<string, Zone>;
  zoneName: (zoneId: string | undefined) => string;
  isLoading: boolean;
  error: ApiProblem | null;
}

/**
 * Maps, zones and every spot of the establishment (read from the map
 * documents, cached with the map editor's keys). Used for labels, filters
 * and the spot pickers.
 */
export function useEstablishmentSpots(establishmentId: string, locale = 'it'): EstablishmentSpots {
  const client = useApiClient();
  const mapList = mapsResource.useList(establishmentId);
  const zoneList = zonesResource.useList(establishmentId);
  const mapItems = [...(mapList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const docs = useQueries({
    queries: mapItems.map((m) => ({
      queryKey: mapDocumentKey(establishmentId, m.id),
      queryFn: () =>
        call(() =>
          client.GET('/api/admin/v1/establishments/{establishmentId}/maps/{mapId}', {
            params: { path: { establishmentId, mapId: m.id } },
          }),
        ),
      retry: shouldRetry,
      staleTime: 60_000,
    })),
  });
  const documents = docs.map((d) => d.data).filter((d): d is MapDocument => !!d);
  const zones = [...(zoneList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const zoneById = new Map(zones.map((z) => [z.id, z]));
  const spots: SpotInfo[] = documents.flatMap((doc) =>
    doc.spots.map((s) => ({
      id: s.id,
      label: s.label,
      row_label: s.row_label || undefined,
      number: s.number,
      zone_id: s.zone_id,
      map_id: s.map_id,
      active: s.active,
    })),
  );
  return {
    maps: mapItems.map((m) => ({ id: m.id, name: localize(m.name, locale, 'it') || m.id })),
    documents,
    zones,
    spots,
    spotById: new Map(spots.map((s) => [s.id, s])),
    zoneById,
    zoneName: (id) => {
      const z = id ? zoneById.get(id) : undefined;
      return z ? localize(z.name, locale, 'it') || z.code : '';
    },
    isLoading: mapList.isPending || zoneList.isPending || docs.some((d) => d.isPending),
    error: (mapList.error ??
      zoneList.error ??
      docs.find((d) => d.error)?.error ??
      null) as ApiProblem | null,
  };
}

/** Read-only geometry of a map document for the shared BeachMap. */
export function toGeometry(doc: MapDocument, locale = 'it'): MapGeometry {
  return {
    id: doc.map.id,
    width: doc.map.width,
    height: doc.map.height,
    grid_size: doc.map.grid_size,
    sea_side: doc.map.sea_side as SeaSide,
    spots: doc.spots.map((s) => ({
      id: s.id,
      label: s.label,
      row_label: s.row_label,
      number: s.number,
      kind: s.kind,
      x: s.x,
      y: s.y,
      rotation: s.rotation,
      shape: s.shape,
      zone_id: s.zone_id,
      active: s.active,
    })),
    decorations: doc.decorations.map((d) => ({
      id: d.id,
      kind: d.kind,
      x: d.x,
      y: d.y,
      rotation: d.rotation,
      shape: d.shape,
      label: d.label ? localize(d.label, locale, 'it') : null,
    })),
  };
}

/** Sort key of a spot inside its zone: row, then number, then label. */
export function compareSpots(a: SpotInfo, b: SpotInfo): number {
  return (
    (a.row_label ?? '').localeCompare(b.row_label ?? '', undefined, { numeric: true }) ||
    (a.number ?? 0) - (b.number ?? 0) ||
    a.label.localeCompare(b.label, undefined, { numeric: true })
  );
}
