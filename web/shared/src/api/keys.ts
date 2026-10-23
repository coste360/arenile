/**
 * Query keys of the establishment-scoped operational data (M3). Every M3
 * feature builds its TanStack Query keys from these roots, so that the
 * real-time stream (useEstablishmentEvents) can invalidate them by prefix:
 *
 *   useQuery({ queryKey: [...establishmentKeys.area(id, 'bookings'), 'list', filters], ... })
 *
 * Keep the area names stable: they are the contract between the features
 * and the SSE invalidation map below.
 */
export const ESTABLISHMENT_KEY_ROOT = 'est' as const;

export type EstablishmentArea =
  | 'operations'
  | 'planning'
  | 'availability'
  | 'holds'
  | 'blocks'
  | 'bookings'
  | 'customers'
  | 'season-passes'
  | 'payments'
  | 'cash'
  | 'fiscal'
  // M5A: reports (invalidated by booking/cash topics) and integrations.
  | 'reports'
  | 'integrations'
  // M5B: renewal campaigns/proposals, waitlist, customer credit ledgers.
  | 'renewals'
  | 'waitlist'
  | 'credits';

export const establishmentKeys = {
  /** Every operational query of one establishment. */
  all: (establishmentId: string) => [ESTABLISHMENT_KEY_ROOT, establishmentId] as const,
  /** Every query of one area (prefix for invalidation). */
  area: (establishmentId: string, area: EstablishmentArea) =>
    [ESTABLISHMENT_KEY_ROOT, establishmentId, area] as const,
};

/** SSE topics of GET /api/admin/v1/stream (`event:` field, schema StreamTopic). */
export type StreamTopicName =
  | 'booking.changed'
  | 'allocation.changed'
  | 'hold.changed'
  | 'cash.changed'
  | 'customer.changed'
  | 'config.changed'
  | 'event';

/** Areas refreshed by each topic (spec 21.1: the client invalidates the affected queries). */
export const TOPIC_AREAS: Record<StreamTopicName, readonly EstablishmentArea[]> = {
  'booking.changed': [
    'operations',
    'planning',
    'availability',
    'bookings',
    'customers',
    'season-passes',
    'payments',
    'cash',
    'renewals',
    'waitlist',
    'credits',
  ],
  'allocation.changed': [
    'operations',
    'planning',
    'availability',
    'blocks',
    'bookings',
    'season-passes',
    'waitlist',
  ],
  'hold.changed': ['operations', 'planning', 'availability', 'holds', 'waitlist'],
  'cash.changed': ['cash', 'payments', 'fiscal', 'operations', 'bookings'],
  'customer.changed': ['customers', 'bookings', 'operations', 'season-passes', 'credits'],
  'config.changed': ['operations', 'planning', 'availability'],
  event: [],
};
