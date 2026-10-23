/**
 * Year-over-year deltas for the KPI cards. The server computes every number
 * (ComparisonMetric: current, previous, delta, delta_bp truncated and absent
 * when previous = 0); this module only decides how to present them.
 */
import type { Schemas } from '@arenile/shared/api';
import { uiIntlLocale } from '@arenile/shared/lib/intl';

export type ComparisonMetric = Schemas['ComparisonMetric'];
export type ComparisonMetricKey = Schemas['ComparisonMetricKey'];

/** Direction of a change for the reader: good, bad or neutral. */
export type DeltaTone = 'positive' | 'negative' | 'neutral';

/** Metrics where an increase is bad news. */
const LOWER_IS_BETTER: ReadonlySet<ComparisonMetricKey> = new Set(['cancellations', 'no_shows']);

export function deltaTone(metric: ComparisonMetric): DeltaTone {
  if (metric.delta === 0) return 'neutral';
  const up = metric.delta > 0;
  return up !== LOWER_IS_BETTER.has(metric.metric) ? 'positive' : 'negative';
}

export function metricsByKey(
  metrics: readonly ComparisonMetric[] | undefined,
): Partial<Record<ComparisonMetricKey, ComparisonMetric>> {
  const out: Partial<Record<ComparisonMetricKey, ComparisonMetric>> = {};
  for (const m of metrics ?? []) out[m.metric] = m;
  return out;
}

/**
 * Integer basis points as a signed percentage with one decimal (truncated, as
 * the server does), in the UI language: 1250 → "+12,5%" (it), "+12.5%" (en),
 * "+12,5 %" (de, fr); -33 → "−0,3%"; 0 → "0,0%".
 */
export function formatSignedBP(bp: number): string {
  const sign = bp > 0 ? '+' : bp < 0 ? '−' : '';
  const tenths = Math.trunc(Math.abs(Math.trunc(bp)) / 10);
  const percent = new Intl.NumberFormat(uiIntlLocale(), {
    style: 'percent',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  });
  // tenths / 1000 is exact enough for display: the value is already truncated.
  return `${sign}${percent.format(tenths / 1000)}`;
}

/** Ratio in basis points as a plain percentage with one decimal: 6234 → "62,3%". */
export function formatRatioBP(bp: number): string {
  return formatSignedBP(bp).replace(/^[+−]/, bp < 0 ? '−' : '');
}

/**
 * Change of a basis-point metric (occupancy) in percentage points:
 * 450 → "+4,5 p.p.".
 */
export function formatPointsDelta(deltaBP: number): string {
  return `${formatSignedBP(deltaBP).replace(/\s?%/, '')} p.p.`;
}

/** Integer count with the grouping of the UI language: 12345 → "12.345" (it), "12,345" (en). */
export function formatCount(n: number): string {
  return new Intl.NumberFormat(uiIntlLocale(), { maximumFractionDigits: 0 }).format(n);
}

/**
 * Change of a metric as shown next to it: percentage points for ratios,
 * relative change otherwise, `newLabel` when the previous value was 0
 * (delta_bp absent) and the current one is not (0 → 0 is no change).
 */
export function deltaLabel(m: ComparisonMetric, newLabel: string): string {
  if (m.unit === 'basis_points') return formatPointsDelta(m.delta);
  if (m.delta_bp !== undefined) return formatSignedBP(m.delta_bp);
  return m.delta === 0 ? formatSignedBP(0) : newLabel;
}
