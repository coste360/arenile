/**
 * Server state of the waitlist feature (M5B: waitlist entries, offers and settings (spec 10.5)): types from the
 * generated schema. The waitlist frontend builder adds the query key factory
 * (establishmentKeys.area(id, 'waitlist')) and the TanStack Query hooks here.
 */
import type { Schemas } from '@arenile/shared/api';

export type WaitlistEntry = Schemas['WaitlistEntry'];
export type WaitlistEntryList = Schemas['WaitlistEntryList'];
export type WaitlistEntryDetail = Schemas['WaitlistEntryDetail'];
export type WaitlistEntryCreate = Schemas['WaitlistEntryCreate'];
export type WaitlistEntryUpdate = Schemas['WaitlistEntryUpdate'];
export type WaitlistEntryStatus = Schemas['WaitlistEntryStatus'];
export type WaitlistOffer = Schemas['WaitlistOffer'];
export type WaitlistOfferStatus = Schemas['WaitlistOfferStatus'];
export type WaitlistOfferCreate = Schemas['WaitlistOfferCreate'];
export type WaitlistReorderRequest = Schemas['WaitlistReorderRequest'];
export type WaitlistSettings = Schemas['WaitlistSettings'];
export type WaitlistSettingsInput = Schemas['WaitlistSettingsInput'];
