import type { AvailabilityQuery } from '../../api/public';
import type { FlowState } from '../../flow/selection';

/** Availability query of the current "Quando" choice, or null while incomplete. */
export function availabilityQuery(state: FlowState): AvailabilityQuery | null {
  if (!state.dates?.to || !state.formula) return null;
  return { from: state.dates.from, to: state.dates.to, slots: state.slots, formula: state.formula };
}
