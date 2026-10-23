import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Separator } from '@arenile/shared/ui/separator';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { CircleAlertIcon, ShieldCheckIcon, TriangleAlertIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { BookingQuote, PolicyRules } from './api';
import { describeProblem, formatConflicts, type ConflictGroup } from './problem';

export interface QuotePreviewProps {
  quote: BookingQuote | undefined;
  loading: boolean;
  error: unknown;
  /** Readable names of packs and zones (breakdown carries codes). */
  resourceLabel?: (group: ConflictGroup) => string | undefined;
  /** True while a newer selection is being quoted (keeps the old figures dimmed). */
  stale?: boolean;
}

/**
 * Live quote of the new-booking sheet: total, lines, extras, discounts, VAT,
 * deposit and the policy that will be snapshotted (payment and cancellation
 * terms), plus availability conflicts and policy violations.
 */
export function QuotePreview({ quote, loading, error, resourceLabel, stale }: QuotePreviewProps) {
  const { t } = useTranslation('bookings');
  if (error) {
    const p = describeProblem(error, t, { resourceLabel });
    return (
      <Alert variant="destructive" aria-live="polite">
        <CircleAlertIcon aria-hidden="true" />
        <AlertTitle>{p.title}</AlertTitle>
        {p.details.length > 0 && (
          <AlertDescription>
            <ul className="list-inside list-disc">
              {p.details.map((d) => (
                <li key={d}>{d}</li>
              ))}
            </ul>
          </AlertDescription>
        )}
      </Alert>
    );
  }
  if (!quote) {
    return loading ? (
      <div role="status" className="space-y-2">
        <span className="sr-only">{t('quote.loading')}</span>
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-16 w-full" />
      </div>
    ) : (
      <p className="text-sm text-muted-foreground">{t('quote.empty')}</p>
    );
  }
  const b = quote.breakdown;
  const money = (c: number) => formatCents(c, b.currency);
  const rules = quote.policy.rules;
  return (
    <section
      aria-label={t('quote.title')}
      aria-busy={stale || loading}
      className={stale ? 'space-y-4 opacity-60 transition-opacity' : 'space-y-4 transition-opacity'}
    >
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-sm text-muted-foreground">{t('quote.total')}</p>
          <p className="text-2xl font-semibold tabular-nums" data-testid="quote-total">
            {money(b.total)}
          </p>
        </div>
        <div className="text-right">
          <p className="text-sm text-muted-foreground">{t('quote.deposit')}</p>
          <p className="font-medium tabular-nums" data-testid="quote-deposit">
            {money(quote.deposit_due)}
          </p>
        </div>
      </div>

      {!quote.available && (
        <Alert variant="destructive">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('quote.unavailable')}</AlertTitle>
          {quote.conflicts.length > 0 && (
            <AlertDescription>
              <ul className="list-inside list-disc">
                {formatConflicts(quote.conflicts, t, resourceLabel).map((line) => (
                  <li key={line}>{line}</li>
                ))}
              </ul>
            </AlertDescription>
          )}
        </Alert>
      )}

      {quote.policy.violations.length > 0 && (
        <Alert variant="warning">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('quote.violations')}</AlertTitle>
          <AlertDescription>
            <ul className="list-inside list-disc">
              {quote.policy.violations.map((v) => (
                <li key={v.code}>
                  {t(`pricing:violations.${v.code}`, { defaultValue: v.code, ...(v.params ?? {}) })}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <ul className="space-y-1 text-sm" aria-label={t('quote.lines')}>
        {b.lines.map((line, i) => (
          <li
            key={`${line.pack}-${line.spot ?? line.zone}-${i}`}
            className="flex justify-between gap-2"
          >
            <span>
              {line.spot ? `${line.spot} · ` : ''}
              {line.pack}
              <span className="text-muted-foreground">
                {' '}
                · {t('quote.days', { count: line.days.length })}
              </span>
            </span>
            <span className="tabular-nums">{money(line.total)}</span>
          </li>
        ))}
        {b.lines
          .flatMap((line) => line.extras)
          .map((x, i) => (
            <li
              key={`lx-${x.code}-${i}`}
              className="flex justify-between gap-2 text-muted-foreground"
            >
              <span>{t('quote.extra', { code: x.code, qty: x.qty })}</span>
              <span className="tabular-nums">{money(x.total)}</span>
            </li>
          ))}
        {(b.extras ?? []).map((x) => (
          <li key={`x-${x.code}`} className="flex justify-between gap-2 text-muted-foreground">
            <span>{t('quote.extra', { code: x.code, qty: x.qty })}</span>
            <span className="tabular-nums">{money(x.total)}</span>
          </li>
        ))}
        {b.discounts.map((d) => (
          <li key={`d-${d.source}`} className="flex justify-between gap-2 text-success">
            <span>{t('quote.discount', { source: d.source })}</span>
            <span className="tabular-nums">−{money(d.amount)}</span>
          </li>
        ))}
        {b.rounding !== 0 && (
          <li className="flex justify-between gap-2 text-muted-foreground">
            <span>{t('quote.rounding')}</span>
            <span className="tabular-nums">{money(b.rounding)}</span>
          </li>
        )}
      </ul>
      {b.vat.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {b.vat.map((v) => t('quote.vat', { rate: v.rate, tax: money(v.tax) })).join(' · ')}
        </p>
      )}
      <Separator />
      <PolicySummary rules={rules} />
    </section>
  );
}

/** Payment and cancellation terms of a policy (spec 9), in plain words. */
export function PolicySummary({ rules }: { rules: PolicyRules }) {
  const { t } = useTranslation('bookings');
  const payment = rules.payment;
  const tiers = [...(rules.cancellation ?? [])].sort(
    (a, b) => b.until_days_before - a.until_days_before,
  );
  const balance = payment?.balance_due;
  const balanceDays = balance?.startsWith('days_before:') ? balance.split(':')[1] : undefined;
  return (
    <div className="space-y-2 text-sm" aria-label={t('policy.title')} role="group">
      <p className="flex items-center gap-2 font-medium">
        <ShieldCheckIcon aria-hidden="true" className="size-4 text-muted-foreground" />
        {t('policy.title')}
        {rules.approval_required && <Badge variant="outline">{t('policy.approval')}</Badge>}
      </p>
      <p className="text-muted-foreground">
        {payment?.mode === 'deposit' && payment.deposit
          ? payment.deposit.type === 'percent'
            ? t('policy.depositPercent', { value: payment.deposit.value ?? 0 })
            : t('policy.depositAmount', { value: formatCents(payment.deposit.value ?? 0) })
          : payment?.mode === 'none'
            ? t('policy.payNone')
            : t('policy.payFull')}
        {balance === 'on_arrival' && ` ${t('policy.balanceOnArrival')}`}
        {balanceDays && ` ${t('policy.balanceDaysBefore', { count: Number(balanceDays) })}`}
      </p>
      {tiers.length > 0 ? (
        <ul
          className="list-inside list-disc text-muted-foreground"
          aria-label={t('policy.cancellation')}
        >
          {tiers.map((tier) => (
            <li key={tier.until_days_before}>
              {t('policy.tier', {
                count: tier.until_days_before,
                percent: tier.refund_percent,
                as: t(`policy.as.${tier.as ?? 'refund'}`),
              })}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground">{t('policy.noCancellation')}</p>
      )}
    </div>
  );
}
