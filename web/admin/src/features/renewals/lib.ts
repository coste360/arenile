import type { StatusTone } from '@arenile/shared/ui/patterns';
import { useEffect, useState } from 'react';
import type { RenewalCampaignStatus, RenewalProposalStatus } from './api';

/** Badge tone of each proposal status (spec 10.4). */
export const PROPOSAL_TONE: Record<RenewalProposalStatus, StatusTone> = {
  proposed: 'warning',
  accepted: 'info',
  converted: 'success',
  declined: 'danger',
  expired: 'neutral',
};

export const CAMPAIGN_TONE: Record<RenewalCampaignStatus, StatusTone> = {
  open: 'success',
  closed: 'neutral',
};

export const PROPOSAL_STATUSES: readonly RenewalProposalStatus[] = [
  'proposed',
  'accepted',
  'converted',
  'declined',
  'expired',
];

/** Offset (ms) of `timeZone` from UTC at instant `utcMs`. */
function zoneOffsetMs(utcMs: number, timeZone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(new Date(utcMs));
  const get = (type: string) => Number(parts.find((p) => p.type === type)?.value ?? 0);
  const asUtc = Date.UTC(
    get('year'),
    get('month') - 1,
    get('day'),
    get('hour'),
    get('minute'),
    get('second'),
  );
  return asUtc - Math.floor(utcMs / 1000) * 1000;
}

/**
 * UTC instant (RFC 3339) of `HH:MM` on the calendar date `isoDate` in the
 * establishment time zone (deadlines are chosen as dates, sent as instants).
 */
export function zonedInstant(isoDate: string, timeZone: string, time = '23:59'): string {
  const [y, m, d] = isoDate.split('-').map(Number);
  const [hh, mm] = time.split(':').map(Number);
  const wall = Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1, hh ?? 0, mm ?? 0, 0);
  // Two passes settle the offset across a DST change.
  let utc = wall - zoneOffsetMs(wall, timeZone);
  utc = wall - zoneOffsetMs(utc, timeZone);
  return new Date(utc).toISOString().replace('.000Z', 'Z');
}

/** Calendar date (YYYY-MM-DD) of an instant in `timeZone`. */
export function zonedDate(instant: string, timeZone: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date(instant));
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? '';
  return `${get('year')}-${get('month')}-${get('day')}`;
}

export interface Remaining {
  expired: boolean;
  days: number;
  hours: number;
  minutes: number;
}

/** Time left until `deadline` (RFC 3339) from `now` (ms), rounded down to the minute. */
export function remaining(deadline: string, now: number): Remaining {
  const ms = Date.parse(deadline) - now;
  if (!(ms > 0)) return { expired: true, days: 0, hours: 0, minutes: 0 };
  const totalMinutes = Math.floor(ms / 60_000);
  return {
    expired: false,
    days: Math.floor(totalMinutes / 1440),
    hours: Math.floor((totalMinutes % 1440) / 60),
    minutes: totalMinutes % 60,
  };
}

/** Current time, refreshed every `intervalMs` (deadline countdowns). */
export function useNow(intervalMs = 60_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);
  return now;
}
