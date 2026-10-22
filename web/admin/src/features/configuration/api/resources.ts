import type { Schemas } from '@arenile/shared/api';
import { createResource } from './resource';

export type Season = Schemas['Season'];
export type SeasonInput = Schemas['SeasonInput'];
export type RatePeriod = Schemas['RatePeriod'];
export type RatePeriodInput = Schemas['RatePeriodInput'];
export type ClosureDay = Schemas['ClosureDay'];
export type ClosureDayInput = Schemas['ClosureDayInput'];
export type Zone = Schemas['Zone'];
export type ZoneInput = Schemas['ZoneInput'];
export type Pool = Schemas['Pool'];
export type PoolInput = Schemas['PoolInput'];
export type CatalogComponent = Schemas['Component'];
export type ComponentInput = Schemas['ComponentInput'];
export type Pack = Schemas['Pack'];
export type PackInput = Schemas['PackInput'];
export type Extra = Schemas['Extra'];
export type ExtraInput = Schemas['ExtraInput'];
export type BeachMap = Schemas['Map'];
export type MapInput = Schemas['MapInput'];

const base = '/api/admin/v1/establishments/{establishmentId}' as const;

export const seasons = createResource<Season, SeasonInput>({
  key: 'seasons',
  collection: `${base}/seasons`,
  item: `${base}/seasons/{seasonId}`,
  idParam: 'seasonId',
});

export const ratePeriods = createResource<RatePeriod, RatePeriodInput, { season_id?: string }>({
  key: 'rate-periods',
  collection: `${base}/rate-periods`,
  item: `${base}/rate-periods/{ratePeriodId}`,
  idParam: 'ratePeriodId',
});

export const closureDays = createResource<
  ClosureDay,
  ClosureDayInput,
  { from?: string; to?: string }
>({
  key: 'closure-days',
  collection: `${base}/closure-days`,
  item: `${base}/closure-days/{closureDayId}`,
  idParam: 'closureDayId',
});

export const zones = createResource<Zone, ZoneInput>({
  key: 'zones',
  collection: `${base}/zones`,
  item: `${base}/zones/{zoneId}`,
  idParam: 'zoneId',
});

export const pools = createResource<Pool, PoolInput>({
  key: 'pools',
  collection: `${base}/pools`,
  item: `${base}/pools/{poolId}`,
  idParam: 'poolId',
});

export const components = createResource<CatalogComponent, ComponentInput>({
  key: 'components',
  collection: `${base}/components`,
  item: `${base}/components/{componentId}`,
  idParam: 'componentId',
});

export const packs = createResource<Pack, PackInput>({
  key: 'packs',
  collection: `${base}/packs`,
  item: `${base}/packs/{packId}`,
  idParam: 'packId',
});

export const extras = createResource<Extra, ExtraInput>({
  key: 'extras',
  collection: `${base}/extras`,
  item: `${base}/extras/{extraId}`,
  idParam: 'extraId',
});

/** Maps (read here for the zone → map selector; edited by the map-editor feature). */
export const maps = createResource<BeachMap, MapInput>({
  key: 'maps',
  collection: `${base}/maps`,
  item: `${base}/maps/{mapId}`,
  idParam: 'mapId',
});
