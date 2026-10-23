import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { localized } from '../../map-editor/model/document';
import { useMapGeometry, useOperationalMaps } from '../api/operations';

/**
 * Maps of the establishment and the one shown on the operational pages
 * (the first by sort order until the user picks another), with its geometry.
 */
export function useSelectedMap(establishmentId: string) {
  const { i18n } = useTranslation();
  const maps = useOperationalMaps(establishmentId);
  const [picked, setPicked] = useState<string | undefined>();
  const items = maps.data ?? [];
  const mapId = items.some((m) => m.id === picked) ? picked : items[0]?.id;
  const geometry = useMapGeometry(establishmentId, mapId);
  return {
    maps,
    options: items.map((m) => ({ id: m.id, name: localized(m.name, i18n.language) })),
    mapId,
    setMapId: setPicked,
    geometry,
    hasNoMaps: maps.isSuccess && items.length === 0,
  };
}
