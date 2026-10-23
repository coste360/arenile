import type { StatusTone } from '@arenile/shared/ui/patterns';
import type { WaitlistEntryStatus, WaitlistOfferStatus } from './api';

/** Badge tone of each entry status (spec 10.5). */
export const ENTRY_TONE: Record<WaitlistEntryStatus, StatusTone> = {
  waiting: 'info',
  offered: 'warning',
  converted: 'success',
  expired: 'neutral',
  removed: 'neutral',
};

export const OFFER_TONE: Record<WaitlistOfferStatus, StatusTone> = {
  pending: 'warning',
  accepted: 'success',
  declined: 'danger',
  expired: 'neutral',
  revoked: 'neutral',
};
