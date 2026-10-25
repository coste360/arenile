/**
 * Dates of the `seed demo` season, computed from today in the demo
 * establishment's timezone: the demo season is the current year and always
 * reaches 60 days ahead (internal/domain/setup/demo.go, demoSeason), its demo
 * bookings fill the first 30 days. Tests pick their days from here instead of
 * a fixed year. "Today" is the test process's, so a run started just before
 * midnight may disagree with the server by a day; near New Year (29-31
 * December) bookings starting in January take the next year's code series.
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

/** A demo day with season passes and daily bookings, after the released days (2-4 days ahead). */
export const DEMO_DAY = addDays(demoToday(), 7);

/** A season day after the demo bookings, for creating new ones. */
export const FREE_DAY = addDays(demoToday(), 45);

/** Another free season day, for the map actions. */
export const MAP_ACTIONS_DAY = addDays(demoToday(), 50);

const IT_MONTHS = [
  "gennaio",
  "febbraio",
  "marzo",
  "aprile",
  "maggio",
  "giugno",
  "luglio",
  "agosto",
  "settembre",
  "ottobre",
  "novembre",
  "dicembre",
];

/** A day as the booking calendar names it. */
export interface CalendarDay {
  iso: string;
  /** Day of the month, 1-31. */
  day: number;
  /** "ottobre 2026": matches the calendar's month caption. */
  month: RegExp;
  /** "25 ottobre 2026": matches the day button's name. */
  name: RegExp;
  /** "25/10/2026", as the fields and the summary show it. */
  display: string;
}

export function calendarDay(iso: string): CalendarDay {
  const [y, m, d] = iso.split("-").map(Number) as [number, number, number];
  const month = IT_MONTHS[m - 1];
  return {
    iso,
    day: d,
    month: new RegExp(`${month} ${y}`),
    name: new RegExp(`\\b${d} ${month} ${y}`),
    display: displayDate(iso),
  };
}

/**
 * A free day of the demo season for a public booking: today + 30 + n, after
 * the demo bookings and within the 60 days the season always covers. Tests
 * that book in parallel use distinct n (1-29) so they never race for the
 * same first free spot.
 */
export function bookableDay(n: number): CalendarDay {
  return calendarDay(addDays(demoToday(), 30 + n));
}
