/**
 * Dates of the `seed demo` season, computed from today in the demo
 * establishment's timezone: the demo season is the current year and always
 * reaches 60 days ahead (internal/domain/setup/demo.go, demoSeason), its demo
 * bookings fill the first 30 days. Tests pick their days from here instead of
 * a fixed year.
 */

/** Today in Europe/Rome, "YYYY-MM-DD". */
export function demoToday(): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Rome" }).format(new Date());
}

/** iso + n calendar days, "YYYY-MM-DD". */
export function addDays(iso: string, n: number): string {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}

/** "2026-06-16" → "16/06/2026", as the panel shows it. */
export function displayDate(iso: string): string {
  const [y, m, d] = iso.split("-");
  return `${d}/${m}/${y}`;
}

/** Year of the demo season (it always opens in the current year). */
export const DEMO_YEAR = Number(demoToday().slice(0, 4));

/** A demo day with season passes and daily bookings. */
export const DEMO_DAY = addDays(demoToday(), 3);

/** A season day after the demo bookings, for creating new ones. */
export const FREE_DAY = addDays(demoToday(), 45);

/** Another free season day, for the map actions. */
export const MAP_ACTIONS_DAY = addDays(demoToday(), 50);
