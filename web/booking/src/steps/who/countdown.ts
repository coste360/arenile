/**
 * Screen-reader announcement bucket: whole minutes above one minute, then
 * 30 and 10 seconds. Announcing every second would flood the live region.
 */
export function announcementBucket(
  ms: number,
): { unit: 'minutes' | 'seconds'; value: number } | null {
  const s = Math.ceil(ms / 1000);
  if (s <= 0) return null;
  if (s > 60) return { unit: 'minutes', value: Math.ceil(s / 60) };
  if (s > 30) return { unit: 'seconds', value: 60 };
  if (s > 10) return { unit: 'seconds', value: 30 };
  return { unit: 'seconds', value: 10 };
}
