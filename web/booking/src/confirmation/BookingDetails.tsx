import { formatIsoDate } from '@arenile/shared/lib/dates';
import { useTranslation } from 'react-i18next';
import type { CheckoutState } from '../api/public';
import { formatMoney, localize } from '../flow/format';
import { PUBLIC_NS } from '../i18n';

export interface BookingDetailsProps {
  summary: CheckoutState['summary'];
  fallbackLocale: string;
}

/** Dates, lines (spot or "in assegnazione"), extras and amounts of a booking. */
export function BookingDetails({ summary, fallbackLocale }: BookingDetailsProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const lang = i18n.language;
  const money = (c: number) => formatMoney(c, summary.currency, lang);
  return (
    <div className="space-y-3 text-sm">
      <p className="font-medium tabular-nums">
        {t('summary.dates', {
          from: formatIsoDate(summary.starts_on),
          to: formatIsoDate(summary.ends_on),
        })}
        <span className="ml-2 font-normal text-muted-foreground">
          {summary.formula === 'day' || summary.formula === 'half_day'
            ? t(`when.slots.${summary.slots}`)
            : `${t(`when.formulas.${summary.formula}`)} · ${t(`when.slots.${summary.slots}`)}`}
        </span>
      </p>
      <ul className="space-y-2">
        {summary.lines.map((line, i) => (
          <li key={`${line.pack_id}-${i}`} className="rounded-lg border bg-muted/30 p-3">
            <p className="font-medium">
              {localize(line.pack_name, lang, fallbackLocale)}
              {line.quantity > 1 ? ` × ${line.quantity}` : ''}
            </p>
            <p className="text-muted-foreground">
              {[
                line.zone_name ? localize(line.zone_name, lang, fallbackLocale) : null,
                line.spot_label ??
                  (line.assignment === 'pending' ? t('confirmation.pending') : null),
              ]
                .filter(Boolean)
                .join(' · ')}
            </p>
            {line.extras.length > 0 && (
              <p className="text-muted-foreground">
                {line.extras
                  .map((e) => `${localize(e.name, lang, fallbackLocale)} × ${e.quantity}`)
                  .join(', ')}
              </p>
            )}
          </li>
        ))}
      </ul>
      {summary.extras.length > 0 && (
        <p className="text-muted-foreground">
          {summary.extras
            .map((e) => `${localize(e.name, lang, fallbackLocale)} × ${e.quantity}`)
            .join(', ')}
        </p>
      )}
      <dl className="space-y-1">
        <div className="flex justify-between gap-3 text-base font-semibold">
          <dt>{t('summary.total')}</dt>
          <dd className="tabular-nums">{money(summary.total)}</dd>
        </div>
        <div className="flex justify-between gap-3 text-muted-foreground">
          <dt>{t('confirmation.paid')}</dt>
          <dd className="tabular-nums">{money(summary.paid)}</dd>
        </div>
        {summary.balance_due > 0 && (
          <div className="flex justify-between gap-3 text-muted-foreground">
            <dt>{t('confirmation.balance')}</dt>
            <dd className="tabular-nums">{money(summary.balance_due)}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}
