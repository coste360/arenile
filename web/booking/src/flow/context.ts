import type { PublicEstablishment, Schemas } from '@arenile/shared/api';
import type { ResolvedTheme } from '@arenile/shared/theme';
import { createContext, useContext } from 'react';
import type { ParentChannel } from '../embed/parent';
import type { BookingLocation } from '../location';

export type PublicOnlineConfig = Schemas['PublicOnlineConfig'];

/**
 * Online configuration used when the server sends none (flow not configured
 * yet): no zones, every formula, full day only, manual payment.
 */
export const DEFAULT_ONLINE: PublicOnlineConfig = {
  zones: [],
  maps: [],
  formulas: [],
  slots: ['FULL'],
  hold_minutes: 15,
  payment: { provider: 'manual', online: false, embedded: false, test_mode: false },
  show_prices_on_map: true,
  show_rate_periods: false,
  promo_codes_enabled: false,
  require_phone: false,
};

/** Everything the steps need about the page they run in. */
export interface BookingEnv {
  slug: string;
  establishment: PublicEstablishment;
  online: PublicOnlineConfig;
  theme: ResolvedTheme;
  location: BookingLocation;
  /** Verified host page when embedded (spec 12.2), else null. */
  channel: ParentChannel | null;
  win: Window;
  /** Panel preview: the flow is browsable but never books. */
  preview: boolean;
  /** Today in the establishment time zone ("YYYY-MM-DD"). */
  today: string;
}

const BookingEnvContext = createContext<BookingEnv | null>(null);

export const BookingEnvProvider = BookingEnvContext.Provider;

export function useBookingEnv(): BookingEnv {
  const env = useContext(BookingEnvContext);
  if (!env) throw new Error('useBookingEnv outside BookingEnvProvider');
  return env;
}

/** Today in `timeZone` as "YYYY-MM-DD" (the establishment calendar, not the browser's). */
export function todayIn(timeZone: string, now: Date = new Date()): string {
  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).formatToParts(now);
    const get = (t: string) => parts.find((p) => p.type === t)?.value ?? '';
    return `${get('year')}-${get('month')}-${get('day')}`;
  } catch {
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
  }
}
