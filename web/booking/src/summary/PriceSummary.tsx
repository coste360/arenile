import type { PublicQuote } from '../api/public';

export interface PriceSummaryProps {
  quote: PublicQuote | undefined;
  /** True while a debounced quote is in flight. */
  updating: boolean;
}

/**
 * Always-visible price summary (spec 12.3): items, discounts, total, due
 * now / balance later, payment plan. Sticky aside on desktop, bottom bar on
 * phones (320 px). Owner: booking app builder.
 */
export function PriceSummary(_props: PriceSummaryProps) {
  return null;
}
