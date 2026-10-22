import type { CoverageReport } from './api';

/** Calendar date arithmetic on "YYYY-MM-DD" strings, in UTC so no DST shift. */
function parse(d: string): Date {
  const [y, m, day] = d.split('-').map(Number);
  return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, day ?? 1));
}
function format(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** Every date from `from` to `to` inclusive. */
export function eachDay(from: string, to: string): string[] {
  const out: string[] = [];
  const end = parse(to).getTime();
  for (let d = parse(from); d.getTime() <= end; d.setUTCDate(d.getUTCDate() + 1)) {
    out.push(format(d));
  }
  return out;
}

/** Number of uncovered (pack, category, channel) combinations per season day. */
export function gapCountsByDay(report: CoverageReport): Map<string, number> {
  const counts = new Map<string, number>();
  for (const g of report.gaps) {
    for (const d of eachDay(g.from, g.to)) counts.set(d, (counts.get(d) ?? 0) + 1);
  }
  return counts;
}

export interface CalendarMonth {
  /** "YYYY-MM". */
  key: string;
  /** Leading empty cells so that the first day falls on its ISO weekday (Monday first). */
  offset: number;
  days: string[];
}

/** Season days split by month for the heatmap. */
export function calendarMonths(from: string, to: string): CalendarMonth[] {
  const months: CalendarMonth[] = [];
  for (const d of eachDay(from, to)) {
    const key = d.slice(0, 7);
    let m = months[months.length - 1];
    if (!m || m.key !== key) {
      const weekday = parse(d).getUTCDay(); // 0 = Sunday
      m = { key, offset: (weekday + 6) % 7, days: [] };
      months.push(m);
    }
    m.days.push(d);
  }
  return months;
}

/** Heat level 0–4 relative to the worst day. */
export function heatLevel(count: number, max: number): 0 | 1 | 2 | 3 | 4 {
  if (count <= 0 || max <= 0) return 0;
  return Math.min(4, Math.max(1, Math.ceil((count / max) * 4))) as 1 | 2 | 3 | 4;
}
