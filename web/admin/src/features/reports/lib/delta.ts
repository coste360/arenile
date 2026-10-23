/**
 * Year-over-year deltas for the KPI cards. The server computes every number
 * (ComparisonMetric: current, previous, delta, delta_bp truncated and absent
 * when previous = 0); this module only decides how to present them.
 */
import type { Schemas } from '@arenile/shared/api';

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
 * Integer basis points as a signed Italian percentage with one decimal,
 * without floats: 1250 → "+12,5%", -33 → "−0,3%", 0 → "0,0%".
 */
export function formatSignedBP(bp: number): string {
  const sign = bp > 0 ? '+' : bp < 0 ? '−' : '';
  const abs = Math.abs(Math.trunc(bp));
  const whole = Math.trunc(abs / 100);
  const tenth = Math.trunc((abs % 100) / 10);
  return `${sign}${whole.toLocaleString('it-IT')},${tenth}%`;
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
  return `${formatSignedBP(deltaBP).replace('%', '')} p.p.`;
}

const COUNT = new Intl.NumberFormat('it-IT', { maximumFractionDigits: 0 });

/** Integer count with Italian grouping: 12345 → "12.345". */
export function formatCount(n: number): string {
  return COUNT.format(n);
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
