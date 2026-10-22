import type { PolicyValues } from './policy.schema';

/**
 * Quick-fill examples of spec 9 ("Esempi tipici configurabili senza codice").
 * Each returns only the fields it sets; the form keeps the others. Ids come
 * from the establishment catalog, so a missing pack or period is reported
 * back instead of being invented.
 */
export type PolicyExampleId = 'august_weeks' | 'gazebo_approval' | 'online_full' | 'desk_free';

export const POLICY_EXAMPLES: readonly PolicyExampleId[] = [
  'august_weeks',
  'gazebo_approval',
  'online_full',
  'desk_free',
];

export interface ExampleCatalog {
  packs: { id: string; code: string; composition?: Record<string, number> }[];
  periods: { code: string; ranges?: { from: string; to: string }[] }[];
}

export interface ExampleFill {
  values: Partial<PolicyValues>;
  /** i18n keys (policies namespace) of what could not be resolved from the catalog. */
  warnings: string[];
}

/** True when [from, to] overlaps August of its year (or of the following ones). */
function overlapsAugust(r: { from: string; to: string }): boolean {
  const y0 = Number(r.from.slice(0, 4));
  const y1 = Number(r.to.slice(0, 4));
  for (let y = y0; y <= y1; y++) {
    if (r.from <= `${y}-08-31` && r.to >= `${y}-08-01`) return true;
  }
  return false;
}

/** August periods: the ones named "peak"/"agosto", else every period overlapping August. */
export function augustPeriods(periods: ExampleCatalog['periods']): string[] {
  const named = periods.filter((p) => p.code === 'peak' || p.code === 'agosto');
  if (named.length) return named.map((p) => p.code);
  return periods.filter((p) => (p.ranges ?? []).some(overlapsAugust)).map((p) => p.code);
}

export function gazeboPacks(packs: ExampleCatalog['packs']): string[] {
  return packs
    .filter(
      (p) =>
        p.code.toLowerCase().includes('gazebo') ||
        Object.keys(p.composition ?? {}).some((k) => k.toLowerCase().includes('gazebo')),
    )
    .map((p) => p.id);
}

export function policyExample(id: PolicyExampleId, catalog: ExampleCatalog): ExampleFill {
  const warnings: string[] = [];
  switch (id) {
    case 'august_weeks': {
      const periods = augustPeriods(catalog.periods);
      if (periods.length === 0) warnings.push('examples.warnNoPeak');
      return {
        values: {
          scope_channels: ['online'],
          scope_periods: periods,
          allowed_formulas: ['week'],
          fixed_start_weekday: '6',
        },
        warnings,
      };
    }
    case 'gazebo_approval': {
      const packs = gazeboPacks(catalog.packs);
      if (packs.length === 0) warnings.push('examples.warnNoGazebo');
      return { values: { scope_packs: packs, approval_required: 'yes' }, warnings };
    }
    case 'online_full':
      return { values: { scope_channels: ['online'], payment_mode: 'full' }, warnings };
    case 'desk_free':
      return {
        values: {
          scope_channels: ['desk'],
          payment_mode: 'none',
          balance_due: '',
          cancellation_set: false,
          cancellation: [],
        },
        warnings,
      };
  }
}
