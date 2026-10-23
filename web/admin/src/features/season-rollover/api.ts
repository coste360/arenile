/**
 * Server state of the season-rollover feature (M5B: end-of-season wizard: preview and apply the copy into the next season (spec 8.7, 28)): types from the
 * generated schema. The season rollover frontend builder adds the query key factory
 * (establishmentKeys.area(id, 'seasonRollover')) and the TanStack Query hooks here.
 */
import type { Schemas } from '@arenile/shared/api';

export type SeasonRolloverRequest = Schemas['SeasonRolloverRequest'];
export type SeasonRolloverResult = Schemas['SeasonRolloverResult'];
export type SeasonRolloverCounts = Schemas['SeasonRolloverCounts'];
export type SeasonRollover = Schemas['SeasonRollover'];
export type SeasonRolloverList = Schemas['SeasonRolloverList'];
