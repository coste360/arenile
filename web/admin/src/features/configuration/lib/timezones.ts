const FALLBACK = [
  'Europe/Rome',
  'Europe/Malta',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Vienna',
  'Europe/Zurich',
  'Europe/Madrid',
  'Europe/Lisbon',
  'Europe/London',
  'Europe/Athens',
  'Europe/Zagreb',
  'Europe/Ljubljana',
  'UTC',
];

/** IANA time zones offered in the selectors; `current` is always included. */
export function timezoneOptions(current?: string): string[] {
  let all: string[];
  try {
    all = Intl.supportedValuesOf('timeZone');
  } catch {
    all = FALLBACK;
  }
  const set = new Set(all);
  if (current) set.add(current);
  return [...set].sort((a, b) => a.localeCompare(b));
}
