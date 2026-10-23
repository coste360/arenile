import { lazy } from 'react';

/**
 * Views of the public SPA loaded on demand (no router library, spec 12.6
 * budget): main.tsx picks the view from `parseBookingLocation().view`.
 * The manage area is a separate chunk and never weighs on the booking
 * first screen; the booking flow steps after "Quando" (map, catalog,
 * customer form) should follow the same pattern inside the Stepper.
 */
export const ManagePage = lazy(() => import('./manage/ManagePage'));
/** Personal waitlist link `/b/{slug}/m/w/{token}` (M5B, spec 10.5): its own chunk. */
export const WaitlistOfferPage = lazy(() => import('./manage/waitlist/WaitlistOfferPage'));
