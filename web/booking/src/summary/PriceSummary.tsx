import { cn } from '@arenile/shared/lib/utils';
import { Card, CardContent, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Separator } from '@arenile/shared/ui/separator';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ReceiptTextIcon, TriangleAlertIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { PublicQuote } from '../api/public';
import { formatMoney, formatStay, localize } from '../flow/format';
import { PUBLIC_NS } from '../i18n';

export interface PriceSummaryProps {
  quote: PublicQuote | undefined;
  /** True while a debounced quote is in flight. */
  updating: boolean;
  currency: string;
  /** Establishment default locale (fallback of item labels). */
  fallbackLocale: string;
  dates?: { from: string; to?: string };
  className?: string;
}

/**
 * Always-visible price summary (spec 12.3): items, discounts, total, due
 * now / balance later and the payment plan, from the binding `quote`. The
 * total is announced politely when it changes.
 */
export function PriceSummary({
  quote,
  updating,
  currency,
  fallbackLocale,
  dates,
  className,
}: PriceSummaryProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const lang = i18n.language;
  const money = (c: number) => formatMoney(c, quote?.currency ?? currency, lang);
  return (
    <Card className={cn('gap-3 py-4', className)} aria-labelledby="price-summary-title">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center gap-2">
          <ReceiptTextIcon aria-hidden="true" className="size-4 text-muted-foreground" />
          <h2 id="price-summary-title" className="text-base">
            {t('summary.title')}
          </h2>
          {updating && (
            <span className="ml-auto flex items-center gap-1 text-xs font-normal text-muted-foreground">
              <Spinner className="size-3" />
              <span className="sr-only">{t('summary.updating')}</span>
            </span>
          )}
        </CardTitle>
        {dates?.to && (
          <p className="text-sm text-muted-foreground tabular-nums">
            {formatStay(dates.from, dates.to)}
          </p>
        )}
      </CardHeader>
      <CardContent className="space-y-3 px-4 text-sm">
        {!quote ? (
          <p className="text-muted-foreground">{t('summary.empty')}</p>
        ) : (
          <>
            {!quote.available && (
              <p role="alert" className="flex items-start gap-2 text-destructive">
                <TriangleAlertIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
                {t('summary.unavailable')}
              </p>
            )}
            {quote.violations.map((v) => (
              <p key={v.code} className="text-destructive">
                {t(`summary.violations.${v.code}`, {
                  defaultValue: t('summary.violation', { code: v.code }),
                })}
              </p>
            ))}
            <ul className="space-y-1.5">
              {quote.items.map((item, i) => (
                <li
                  key={`${item.kind}-${item.ref_id ?? i}`}
                  className={cn(
                    'flex items-start justify-between gap-3',
                    item.kind === 'discount' && 'text-primary-on-surface',
                  )}
                >
                  <span className="min-w-0">
                    {localize(item.label, lang, fallbackLocale)}
                    {item.quantity && item.quantity > 1 ? ` × ${item.quantity}` : ''}
                  </span>
                  <span className="shrink-0 tabular-nums">{money(item.amount)}</span>
                </li>
              ))}
            </ul>
            <Separator />
            <div className="flex items-baseline justify-between gap-3 text-base font-semibold">
              <span>{t('summary.total')}</span>
              <span className="tabular-nums" aria-live="polite" aria-atomic="true">
                {money(quote.total)}
              </span>
            </div>
            {quote.due_now !== quote.total && (
              <dl className="space-y-1 text-muted-foreground">
                <div className="flex justify-between gap-3">
                  <dt>{t('summary.dueNow')}</dt>
                  <dd className="font-medium text-foreground tabular-nums">
                    {money(quote.due_now)}
                  </dd>
                </div>
                {quote.balance_later > 0 && (
                  <div className="flex justify-between gap-3">
                    <dt>{t('summary.balanceLater')}</dt>
                    <dd className="tabular-nums">{money(quote.balance_later)}</dd>
                  </div>
                )}
              </dl>
            )}
            <p className="text-xs text-muted-foreground">
              {t(`summary.plan.${quote.payment_plan}`)}
            </p>
          </>
        )}
      </CardContent>
    </Card>
  );
}
