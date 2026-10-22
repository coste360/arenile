import type { TFunction } from 'i18next';
import { formatCents } from '@arenile/shared/lib/money';
import type { PriceAction, PriceConditions, PriceRuleInput } from './api';

/** Human summary of a price action, e.g. "−10%", "+5,00 € al giorno", "7 giorni al prezzo di 6". */
export function describeAction(t: TFunction, a: PriceAction, locale: string): string {
  const money = (c: number) => formatCents(c);
  const pct = (p?: string) => (p ?? '0').replace('.', locale.startsWith('en') ? '.' : ',');
  switch (a.type) {
    case 'set':
      return t('pricing:actionSummary.set', { amount: money(a.amount ?? 0) });
    case 'add':
      return t('pricing:actionSummary.add', {
        amount: `${(a.amount ?? 0) < 0 ? '−' : '+'}${money(Math.abs(a.amount ?? 0))}`,
      });
    case 'multiply':
      return t('pricing:actionSummary.multiply', { factor: pct(a.factor) });
    case 'percent_off':
      return t('pricing:actionSummary.percent_off', { percent: pct(a.percent) });
    case 'amount_off':
      return t(`pricing:actionSummary.amount_off_${a.per ?? 'booking'}`, {
        amount: money(a.amount ?? 0),
      });
    case 'free_days':
      return t('pricing:actionSummary.free_days', {
        every: a.every ?? 0,
        pay: (a.every ?? 0) - (a.free ?? 0),
      });
  }
}

export interface SummaryNames {
  zone: (id: string) => string;
  agreement?: (id: string) => string;
  /** Pack or extra name by id (rule targets). */
  target?: (id: string) => string;
}

/** Lower-cases a label inside a sentence, except in German where nouns keep their capital. */
function inSentence(label: string, locale: string): string {
  return locale.startsWith('de') ? label : label.toLowerCase();
}

function formatIsoDate(d: string, locale: string): string {
  const [y, m, day] = d.split('-').map(Number);
  if (!y || !m || !day) return d;
  return new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(new Date(Date.UTC(y, m - 1, day)));
}

function rangeText(
  t: TFunction,
  key: 'leadTime' | 'occupancy',
  r: { min?: number; max?: number },
): string {
  if (r.min != null && r.max != null) {
    return t(`pricing:conditionSummary.${key}Between`, { min: r.min, max: r.max });
  }
  if (r.min != null) return t(`pricing:conditionSummary.${key}Min`, { count: r.min, min: r.min });
  return t(`pricing:conditionSummary.${key}Max`, { count: r.max ?? 0, max: r.max ?? 0 });
}

/** One phrase per condition present, in the order of spec 8.2. */
export function conditionParts(
  t: TFunction,
  c: PriceConditions,
  names: SummaryNames,
  locale = 'it',
): string[] {
  const parts: string[] = [];
  const list = (xs: string[]) => xs.join(', ');
  if (c.rate_periods?.length) {
    parts.push(t('pricing:conditionSummary.periods', { list: list(c.rate_periods) }));
  }
  if (c.date_ranges?.length) {
    parts.push(
      list(
        c.date_ranges.map((r) =>
          t('pricing:conditionSummary.dateRange', {
            from: formatIsoDate(r.from, locale),
            to: formatIsoDate(r.to, locale),
          }),
        ),
      ),
    );
  }
  if (c.weekdays?.length) {
    parts.push(
      t('pricing:conditionSummary.weekdays', {
        list: list(c.weekdays.map((d) => t(`pricing:weekdaysShort.${d}`))),
      }),
    );
  }
  if (c.holidays !== undefined) {
    parts.push(
      t(c.holidays ? 'pricing:conditionSummary.holidays' : 'pricing:conditionSummary.noHolidays'),
    );
  }
  if (c.slots?.length) {
    parts.push(
      t('pricing:conditionSummary.slots', {
        list: list(c.slots.map((s) => inSentence(t(`pricing:slots.${s}`), locale))),
      }),
    );
  }
  if (c.formulas?.length) {
    parts.push(
      t('pricing:conditionSummary.formulas', {
        list: list(c.formulas.map((f) => inSentence(t(`pricing:formulas.${f}`), locale))),
      }),
    );
  }
  if (c.price_categories?.length) {
    parts.push(t('pricing:conditionSummary.categories', { list: list(c.price_categories) }));
  }
  if (c.zone_ids?.length) {
    parts.push(t('pricing:conditionSummary.zones', { list: list(c.zone_ids.map(names.zone)) }));
  }
  if (c.spot_attributes?.length) {
    parts.push(t('pricing:conditionSummary.spotAttributes', { list: list(c.spot_attributes) }));
  }
  if (c.channels?.length) {
    parts.push(
      t('pricing:conditionSummary.channels', {
        list: list(c.channels.map((ch) => t(`pricing:channels.${ch}`))),
      }),
    );
  }
  if (c.min_days) parts.push(t('pricing:conditionSummary.minDays', { count: c.min_days }));
  if (c.max_days) parts.push(t('pricing:conditionSummary.maxDays', { count: c.max_days }));
  if (c.lead_time_days) parts.push(rangeText(t, 'leadTime', c.lead_time_days));
  if (c.occupancy_percent) parts.push(rangeText(t, 'occupancy', c.occupancy_percent));
  if (c.customer_tags?.length) {
    parts.push(t('pricing:conditionSummary.customerTags', { list: list(c.customer_tags) }));
  }
  if (c.agreement_ids?.length) {
    const name = names.agreement ?? ((id: string) => id.slice(0, 8));
    parts.push(t('pricing:conditionSummary.agreements', { list: list(c.agreement_ids.map(name)) }));
  }
  return parts;
}

/** Short list of the conditions of a rule ("periodi: high, peak · sab, dom · online"). */
export function describeConditions(
  t: TFunction,
  c: PriceConditions,
  names: SummaryNames,
  locale = 'it',
): string {
  const parts = conditionParts(t, c, names, locale);
  return parts.length ? parts.join(' · ') : t('pricing:conditionSummary.always');
}

type RuleLike = Pick<
  PriceRuleInput,
  'stage' | 'target_type' | 'target_ids' | 'conditions' | 'action' | 'stackable' | 'active'
>;

/**
 * Whole-rule sentence shown in the editor and the list, e.g. (it):
 * "Modificatore per Ombrellone + 2 lettini: +5,00 € al giorno, quando periodi: peak e
 * giorni: sab, dom. Cumulabile."
 */
export function describeRule(
  t: TFunction,
  r: RuleLike,
  names: SummaryNames,
  locale = 'it',
): string {
  const targets = r.target_ids?.length
    ? r.target_ids.map(names.target ?? ((id) => id.slice(0, 8))).join(', ')
    : inSentence(t(`pricing:targets.all_${r.target_type}`), locale);
  const parts = conditionParts(t, r.conditions, names, locale);
  const when =
    parts.length === 0
      ? t('pricing:ruleSummary.always')
      : t('pricing:ruleSummary.when', {
          conditions:
            parts.length === 1
              ? parts[0]
              : `${parts.slice(0, -1).join(', ')} ${t('pricing:ruleSummary.and')} ${parts[parts.length - 1]}`,
        });
  let out = t('pricing:ruleSummary.sentence', {
    stage: t(`pricing:stages.${r.stage}`),
    targets,
    action: describeAction(t, r.action, locale),
    when,
  });
  if (r.stage !== 'base') {
    out += ` ${t(r.stackable ? 'pricing:ruleSummary.stackable' : 'pricing:ruleSummary.exclusive')}`;
  }
  if (!r.active) out += ` ${t('pricing:ruleSummary.inactive')}`;
  return out;
}
