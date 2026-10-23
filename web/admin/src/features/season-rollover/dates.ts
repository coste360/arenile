/**
 * Same calendar day `delta` years later ("YYYY-MM-DD" in and out); 29
 * February becomes 28 February in a non-leap year.
 */
export function shiftYear(iso: string, delta: number): string {
  const [y, m, d] = iso.split('-').map(Number);
  const year = (y ?? 1970) + delta;
  const month = m ?? 1;
  const lastDay = new Date(Date.UTC(year, month, 0)).getUTCDate();
  const day = Math.min(d ?? 1, lastDay);
  return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}
