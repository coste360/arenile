import type { RevenueBasis, RevenueGroupBy } from '../api';

/**
 * Groupings the revenue report accepts for a basis (the server answers 400
 * otherwise): payment method exists only for collected money, pack and extra
 * only for booked value.
 */
export function revenueGroupAllowed(groupBy: RevenueGroupBy, basis: RevenueBasis): boolean {
  if (groupBy === 'payment_method') return basis === 'collected';
  if (groupBy === 'pack' || groupBy === 'extra') return basis === 'booked';
  return true;
}
