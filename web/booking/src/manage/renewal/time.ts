import { useEffect, useState } from 'react';

export interface TimeLeft {
  expired: boolean;
  days: number;
  hours: number;
  minutes: number;
}

/** Time left until `deadline` (RFC 3339) from `now` (ms), rounded down to the minute. */
export function timeLeft(deadline: string, now: number): TimeLeft {
  const ms = Date.parse(deadline) - now;
  if (!(ms > 0)) return { expired: true, days: 0, hours: 0, minutes: 0 };
  const total = Math.floor(ms / 60_000);
  return {
    expired: false,
    days: Math.floor(total / 1440),
    hours: Math.floor((total % 1440) / 60),
    minutes: total % 60,
  };
}

/** `now()` refreshed every `intervalMs` (countdowns on the server clock). */
export function useTicker(now: () => number, intervalMs: number): number {
  const [value, setValue] = useState(now);
  useEffect(() => {
    const id = window.setInterval(() => setValue(now()), intervalMs);
    return () => window.clearInterval(id);
  }, [now, intervalMs]);
  return value;
}
