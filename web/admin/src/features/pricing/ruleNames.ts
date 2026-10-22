import type { StatusTone } from '@arenile/shared/ui/patterns';
import { agreements, type PriceRule } from './api';
import type { SummaryNames } from './format';
import type { PricingTabProps } from './RulesTab';
import type { Stage } from './rules.schema';

/** Badge tone of each pricing stage (lists, simulator timeline). */
export const STAGE_TONE: Record<Stage, StatusTone> = {
  base: 'info',
  modifier: 'highlight',
  discount: 'success',
};

/** Rule order inside a stage: what the engine evaluates first (priority, then the latest saved). */
export function sortRules<T extends Pick<PriceRule, 'priority' | 'seq'>>(rules: T[]): T[] {
  return [...rules].sort((a, b) => b.priority - a.priority || b.seq - a.seq);
}

/** Names used by the rule summaries. */
export function useSummaryNames(
  p: Pick<PricingTabProps, 'packs' | 'extras' | 'zones' | 'name'> & {
    establishmentId: string;
  },
) {
  const agreementList = agreements.useList(p.establishmentId);
  const agreementItems = agreementList.data ?? [];
  const short = (id: string) => id.slice(0, 8);
  const catalogName =
    (src: { id: string; name: Record<string, string>; code: string }[]) => (id: string) => {
      const x = src.find((s) => s.id === id);
      return x ? p.name(x.name) || x.code : short(id);
    };
  return {
    agreements: agreementItems.map((a) => ({ id: a.id, name: a.name })),
    names: (targetType: 'pack' | 'extra'): SummaryNames => ({
      zone: (id) => p.zones.find((z) => z.id === id)?.name ?? short(id),
      agreement: (id) => agreementItems.find((a) => a.id === id)?.name ?? short(id),
      target: catalogName(targetType === 'pack' ? p.packs : p.extras),
    }),
  };
}

/**
 * The engine reports each rule's effect in a compact machine form ("set 2500",
 * "+300", "-500", "x1.2", "-10%", amounts in cents): show it as money/percent.
 */
export function formatEffect(
  effect: string,
  money: (cents: number) => string,
  locale: string,
): string {
  const num = (v: string) => Number(v).toLocaleString(locale, { maximumFractionDigits: 4 });
  let m = /^set (-?\d+)$/.exec(effect);
  if (m) return `= ${money(Number(m[1]))}`;
  m = /^([+-])(\d+)$/.exec(effect);
  if (m) return `${m[1] === '-' ? '−' : '+'}${money(Number(m[2]))}`;
  m = /^x(\d+(?:\.\d+)?)$/.exec(effect);
  if (m) return `× ${num(m[1] as string)}`;
  m = /^-(\d+(?:\.\d+)?)%$/.exec(effect);
  if (m) return `−${num(m[1] as string)}%`;
  return effect;
}
